package e2e

import (
	goctx "context"
	"errors"
	"fmt"
	"os/exec"
	"testing"
	"time"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	"github.com/OpenLiberty/open-liberty-operator/test/util"
	imagev1 "github.com/openshift/api/image/v1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)

//OpenLibertyImageStreamTest ...
func OpenLibertyImageStreamTest(t *testing.T) {
	ctx, err := util.InitializeContext(t, cleanupTimeout, retryInterval)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Cleanup()

	f := framework.Global

	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatalf("Couldn't get namespace: %v", err)
	}

	// Adds the imagestream resources to the scheme
	if err = imagev1.AddToScheme(f.Scheme); err != nil {
		t.Logf("Unable to add image scheme: (%v)", err)
		util.FailureCleanup(t, f, namespace, err)
	}

	t.Logf("Namespace: %s", namespace)

	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "open-liberty-operator", 1, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = libertyImageStreamTest(t, f, ctx); err != nil {
		out, err := exec.Command("oc", "delete", "imagestream", "imagestream-example").Output()
		if err != nil {
			t.Fatalf("Failed to delete imagestream: %s", out)
		}
		util.FailureCleanup(t, f, namespace, err)
	}
}

func libertyImageStreamTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	const name = "liberty-app"
	const imgstreamName = "imagestream-example"

	ns, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace: %v", err)
	}
	t.Logf("Namespace: %s", ns)

	// Create the imagestream
	out, err := exec.Command("oc", "import-image", imgstreamName, "--from=openliberty/open-liberty:kernel-java8-openj9-ubi", "-n", ns, "--confirm").Output()
	if err != nil {
		t.Fatalf("Creating the imagestream failed: %s", out)
	}

	// Check the name field that matches
	key := map[string]string{"metadata.name": imgstreamName}

	options := &dynclient.ListOptions{
		FieldSelector: fields.Set(key).AsSelector(),
		Namespace:     ns,
	}

	imageStreamList := &imagev1.ImageStreamList{}
	if err = f.Client.List(goctx.TODO(), imageStreamList, options); err != nil {
		t.Logf("Imagestreams not found: %s", err)
	}

	if len(imageStreamList.Items) == 0 {
		for i := 0; i < 10; i++ {
			time.Sleep(4000 * time.Millisecond)
			if err = f.Client.List(goctx.TODO(), imageStreamList, options); err != nil {
				t.Logf("Imagestreams not found: %s", err)
			}
		}
	}

	// Make an appplication that points to the imagestream
	exampleOpenLiberty := util.MakeBasicOpenLibertyApplication(t, f, name, ns, 1)
	exampleOpenLiberty.Spec.ApplicationImage = imgstreamName

	timestamp := time.Now().UTC()
	t.Logf("%s - Creating Open Liberty application...", timestamp)
	err = f.Client.Create(goctx.TODO(), exampleOpenLiberty, 
		&framework.CleanupOptions{TestContext: ctx, Timeout: time.Second, RetryInterval: time.Second})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, name, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	// Get the application
	f.Client.Get(goctx.TODO(), types.NamespacedName{Name: name, Namespace: ns}, exampleOpenLiberty)
	if err != nil {
		t.Fatal(err)
	}

	firstImage := exampleOpenLiberty.Status.ImageReference
	// Update the imagestreamtag
	tag := `{"tag":{"from":{"name": "openliberty/open-liberty:kernel-java11-openj9-ubi"}}}`
	out, err = exec.Command("oc", "patch", "imagestreamtag", imgstreamName+":latest", "-n", ns, "-p", tag).Output()
	if err != nil {
		t.Fatalf("Updating the imagestreamtag failed: %s", out)
	}

	time.Sleep(4000 * time.Millisecond)

	// Get the application
	f.Client.Get(goctx.TODO(), types.NamespacedName{Name: name, Namespace: ns}, exampleOpenLiberty)
	if err != nil {
		t.Fatal(err)
	}

	secondImage := exampleOpenLiberty.Status.ImageReference
	// Check if the image the application is pointing to has been changed
	if firstImage == secondImage {
		t.Fatalf("The docker image has not been updated. It is still %s", firstImage)
	}

	// Update the imagestreamtag again
	tag = `{"tag":{"from":{"name": "openliberty/open-liberty:kernel-java8-openj9-ubi"}}}`
	out, err = exec.Command("oc", "patch", "imagestreamtag", imgstreamName+":latest", "-n", ns, "-p", tag).Output()
	if err != nil {
		t.Fatalf("Updating the imagestreamtag failed: %s", out)
	}

	time.Sleep(4000 * time.Millisecond)

	// Get the application
	f.Client.Get(goctx.TODO(), types.NamespacedName{Name: name, Namespace: ns}, exampleOpenLiberty)
	if err != nil {
		t.Fatal(err)
	}

	firstImage = exampleOpenLiberty.Status.ImageReference
	// Check if the image the application is pointing to has been changed
	if firstImage == secondImage {
		t.Fatalf("The docker image has not been updated. It is still %s", secondImage)
	}

	out, err = exec.Command("oc", "delete", "imagestream", "imagestream-example", "-n", ns).Output()
	if err != nil {
		t.Fatalf("Failed to delete imagestream: %s", out)
	}

	if err = testRemoveImageStream(t, f, ctx); err != nil {
		t.Fatal(err)
	}

	return nil
}

func testRemoveImageStream(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	const name = "liberty-app"
	ns, err := ctx.GetNamespace()
	if err != nil {
		return err
	}
	target := types.NamespacedName{Namespace: ns, Name: name}
	err = util.UpdateApplication(f, target, func(r *openlibertyv1beta1.OpenLibertyApplication) {
		r.Spec.ApplicationImage = "openliberty/open-liberty"
	})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, name, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	application := openlibertyv1beta1.OpenLibertyApplication{}

	// Get the application
	f.Client.Get(goctx.TODO(), target, &application)
	if err != nil {
		t.Fatal(err)
	}

	if application.Status.ImageReference != "openliberty/open-liberty" {
		return errors.New("image reference not updated to docker hub ref")
	}

	return nil
}