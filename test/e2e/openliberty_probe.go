package e2e

import (
	goctx "context"
	"testing"
	"time"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	"github.com/OpenLiberty/open-liberty-operator/test/util"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	e2eutil "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	corev1 "k8s.io/api/core/v1"
)

// OpenLibertyProbeTest make sure user defined liveness/readiness probes reach ready state.
func OpenLibertyProbeTest(t *testing.T) {
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

	libertyProbe := corev1.Handler{
		HTTPGet: &corev1.HTTPGetAction{
			Path: "/",
			Port: intstr.FromInt(3000),
		},
	}

	// run test for readiness probe and then liveness
	if err = probeTest(t, f, ctx, libertyProbe); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = editProbeTest(t, f, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = deleteProbeTest(t, f, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
}

func probeTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, probe corev1.Handler) error {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return err
	}
	// default liberty test now has to define probes manually, so we will use those and change in the edit test.
	exampleOpenLiberty := util.MakeBasicOpenLibertyApplication(t, f, "example-liberty-readiness", namespace, 1)

	err = f.Client.Create(goctx.TODO(), exampleOpenLiberty, &framework.CleanupOptions{
		TestContext:   ctx,
		Timeout:       time.Second * 5,
		RetryInterval: time.Second,
	})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "example-liberty-readiness", 1, retryInterval, timeout)
	if err != nil {
		return err
	}
	return nil
}

func editProbeTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	applicationName := "example-liberty-readiness"
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return err
	}

	target := types.NamespacedName{Name: applicationName, Namespace: namespace}
	util.UpdateApplication(f, target, func(r *openlibertyv1beta1.OpenLibertyApplication) {
		// Adjust tests for update SMALL amounts to keep the test fast.
		r.Spec.LivenessProbe.InitialDelaySeconds = int32(6)
		r.Spec.ReadinessProbe.InitialDelaySeconds = int32(3)
	})

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, applicationName, 1, retryInterval, timeout)
	if err != nil {
		return err
	}
	return nil
}

func deleteProbeTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	applicationName := "example-liberty-readiness"
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return err
	}

	target := types.NamespacedName{Namespace: namespace, Name: applicationName}
	util.UpdateApplication(f, target, func(r *openlibertyv1beta1.OpenLibertyApplication) {
		r.Spec.LivenessProbe = nil
		r.Spec.ReadinessProbe = nil
	})

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, applicationName, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	return nil
}