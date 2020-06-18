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
	"k8s.io/apimachinery/pkg/util/wait"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)

//OpenLibertyImageStreamTest consists of tests that verify the behaviour of OpenShift's Image Streams feature.
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

	// adds the imagestream resources to the scheme
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
	target := types.NamespacedName{Name: name, Namespace: ns}

	// create the imagestream
	out, err := exec.Command("oc", "import-image", imgstreamName, "--from=openliberty/open-liberty:kernel-java8-openj9-ubi", "-n", ns, "--confirm").Output()
	if err != nil {
		t.Fatalf("Creating the imagestream failed: %s", out)
	}

	err = waitForImageStream(f, ctx, imgstreamName, ns)
	if err != nil {
		return err
	}

	// make an appplication that points to the imagestream
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

	previousImage, err := getCurrImageRef(f, ctx, target)
	if err != nil {
		return err
	}
	// update the imagestreamtag
	tag := `{"tag":{"from":{"name": "openliberty/open-liberty:kernel-java11-openj9-ubi"}}}`
	out, err = exec.Command("oc", "patch", "imagestreamtag", imgstreamName+":latest", "-n", ns, "-p", tag).Output()
	if err != nil {
		t.Fatalf("Updating the imagestreamtag failed: %s", out)
	}
	// return err if the image reference is not updated successfully
	err = waitImageRefUpdated(t, f, ctx, target, previousImage)
	if err != nil {
		return err
	}

	previousImage, err = getCurrImageRef(f, ctx, target)
	if err != nil {
		return err
	}
	// update the imagestreamtag again
	tag = `{"tag":{"from":{"name": "openliberty/open-liberty:kernel-java8-openj9-ubi"}}}`
	out, err = exec.Command("oc", "patch", "imagestreamtag", imgstreamName+":latest", "-n", ns, "-p", tag).Output()
	if err != nil {
		t.Fatalf("Updating the imagestreamtag failed: %s", out)
	}

	// return err if the image reference is not updated successfully
	err = waitImageRefUpdated(t, f, ctx, target, previousImage)
	if err != nil {
		return err
	}

	// delete the image stream
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

	imageRef, err := getCurrImageRef(f, ctx, target)
	if err != nil {
		return err
	}

	if imageRef != "openliberty/open-liberty" {
		return errors.New("image reference not updated to docker hub ref")
	}

	return nil
}

/* Helper Functions Below */
// Wait for the ImageStreamList contains at least one item.
func waitForImageStream(f *framework.Framework, ctx *framework.TestCtx, imgstreamName string, ns string) error {
	// check the name field that matches
	key := map[string]string{"metadata.name": imgstreamName}

	options := &dynclient.ListOptions{
		FieldSelector: fields.Set(key).AsSelector(),
		Namespace:     ns,
	}

	imageStreamList := &imagev1.ImageStreamList{}

	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		err = f.Client.List(goctx.TODO(), imageStreamList, options)
		if err != nil {
			return true, err
		}

		if len(imageStreamList.Items) == 0 {
			return false, nil
		}

		return true, nil
	})

	if errors.Is(err, wait.ErrWaitTimeout) {
		return errors.New("imagestream not found")
	}

	return err
}

// Get the target's current image reference.
func getCurrImageRef(f *framework.Framework, ctx *framework.TestCtx,
	target types.NamespacedName) (string, error) {
	exampleOpenLiberty := openlibertyv1beta1.OpenLibertyApplication{}
	err := f.Client.Get(goctx.TODO(), target, &exampleOpenLiberty)
	if err != nil {
		return "", err
	}
	return exampleOpenLiberty.Status.ImageReference, nil
}

// Polling wait for the target's image reference to be updated to a new one.
func waitImageRefUpdated(t *testing.T, f *framework.Framework, ctx *framework.TestCtx,
	target types.NamespacedName, oldImageRef string) error {
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		currImage, err := getCurrImageRef(f, ctx, target)
		if err != nil {
			return true, err // if error, stop polling and return err
		}

		// check if the image the application is pointing to has been changed
		if currImage == oldImageRef {
			// keep polling if the image ref is not updated
			t.Log("Waiting for the image reference to be updated ...")
			return false, nil
		}
		return true, nil
	})

	if errors.Is(err, wait.ErrWaitTimeout) {
		return errors.New("image reference not updated")
	}

	return err // implicitly return nil if no errors
}
