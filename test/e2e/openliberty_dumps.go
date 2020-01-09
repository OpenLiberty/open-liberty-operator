package e2e

import (
	goctx "context"
	"testing"
	"time"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	"github.com/OpenLiberty/open-liberty-operator/test/util"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	e2eutil "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// OpenLibertyDumpsTest ... Check dumps
func OpenLibertyDumpsTest(t *testing.T) {

	ctx, err := util.InitializeContext(t, cleanupTimeout, retryInterval)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Cleanup()

	f := framework.Global
	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatalf("could not get namespace: %v", err)
	}

	// Wait for the operator
	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "open-liberty-operator", 1, retryInterval, operatorTimeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	timestamp := time.Now().UTC()
	t.Logf("%s - Starting open liberty dumps test...", timestamp)

	// create one replica of the operator deployment in current namespace with provided name
	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "open-liberty-operator", 1, retryInterval, operatorTimeout)
	if err != nil {
		t.Fatal(err)
	}

	// Make basic open liberty application with 1 replica
	replicas := int32(1)
	openLibertyApplication := util.MakeBasicOpenLibertyApplication(t, f, "example-liberty-dumps", namespace, replicas)
	// set up serviceability, prereq to dumps
	openLibertyApplication.Spec.Serviceability = &openlibertyv1beta1.OpenLibertyApplicationServiceability{
		Size: "1Gi",
	}

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(goctx.TODO(), openLibertyApplication, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// wait for example-liberty-dumps to reach 1 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "example-liberty-dumps", 1, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "example-liberty-dumps", Namespace: namespace}, openLibertyApplication)
	if err != nil {
		t.Fatal(err)
	}

	// get the pod name that is running the open-liberty application
	// check the name label that matches
	m := map[string]string{"app.kubernetes.io/name": "example-liberty-dumps"}
	l := labels.Set(m)
	selec := l.AsSelector()
	options := &dynclient.ListOptions{
		LabelSelector: selec,
	}
	podList := &corev1.PodList{}
	err = f.Client.List(goctx.TODO(), podList, options)
	if err != nil {
		t.Fatal(err)
	}
	podName := podList.Items[0].GetName()
	dump := util.MakeOpenLibertyDump(t, f, "test-dump", namespace, podName)

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(goctx.TODO(), dump, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	counter := 0
	for {
		err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "test-dump", Namespace: namespace}, dump)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second * 2)
		counter++
		if len(dump.Status.Conditions) > 1 || counter == 300 {
			break
		}
	}

	for i := 0; i < len(dump.Status.Conditions); i++ {
		if dump.Status.Conditions[i].Type == "Started" {
			if dump.Status.Conditions[i].Status != "True" {
				t.Fatal("The Started State's Status is not True")
			}
		} else if dump.Status.Conditions[i].Type == "Completed" {
			if dump.Status.Conditions[i].Status != "True" {
				t.Fatal("The Completed State's Status is not True")
			}
		}
		if dump.Status.DumpFile == "" {
			t.Fatal("Dump file not created")
		}
	}

}
