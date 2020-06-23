package e2e

import (
	goctx "context"
	"fmt"
	"testing"
	"time"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	"github.com/OpenLiberty/open-liberty-operator/test/util"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	e2eutil "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"

	"k8s.io/apimachinery/pkg/types"
)

var (
	retryInterval        = time.Second * 5
	operatorTimeout      = time.Minute * 4
	timeout              = time.Minute * 4
	cleanupRetryInterval = time.Second * 1
	cleanupTimeout       = time.Second * 5
)

// OpenLibertyBasicTest barebones deployment test that makes sure applications will deploy and scale.
func OpenLibertyBasicTest(t *testing.T) {
	ctx, err := util.InitializeContext(t, cleanupTimeout, retryInterval)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Cleanup()

	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatalf("Couldn't get namespace: %v", err)
	}

	t.Logf("Namespace: %s", namespace)

	f := framework.Global

	// create one replica of the operator deployment in current namespace with provided name
	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "open-liberty-operator", 1, retryInterval, operatorTimeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = openLibertyBasicScaleTest(t, f, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
}

func openLibertyBasicScaleTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	const name = "example-open-liberty"
	ns, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace: %v", err)
	}

	helper := int32(1)

	exampleOpenLiberty := util.MakeBasicOpenLibertyApplication(t, f, name, ns, helper)

	timestamp := time.Now().UTC()
	t.Logf("%s - Creating basic liberty application for scaling test...", timestamp)
	// create application deployment and wait
	err = f.Client.Create(goctx.TODO(), exampleOpenLiberty, &framework.CleanupOptions{TestContext: ctx, Timeout: time.Second, RetryInterval: time.Second})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, name, 1, retryInterval, timeout)
	if err != nil {
		return err
	}
	// -- Run all scaling tests below based on the above example deployment of 1 pods ---
	// update the number of replicas and return if failure occurs
	exampleOpenLiberty = &openlibertyv1beta1.OpenLibertyApplication{}
	target := types.NamespacedName{Name: name, Namespace: ns}
	err = f.Client.Get(goctx.TODO(), target, exampleOpenLiberty)
	if err != nil {
		return err
	}

	helper2 := int32(2)
	exampleOpenLiberty.Spec.Replicas = &helper2
	err = f.Client.Update(goctx.TODO(), exampleOpenLiberty)
	if err != nil {
		return err
	}

	// wait for example-memcached to reach 2 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, target.Namespace, target.Name, 2, retryInterval, timeout)
	timestamp = time.Now().UTC()
	t.Logf("%s - Completed basic openLiberty scale test", timestamp)
	return err
}
