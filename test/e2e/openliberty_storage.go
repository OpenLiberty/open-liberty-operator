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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// OpenLibertyBasicStorageTest check that when persistence is configured that a statefulset is deployed
func OpenLibertyBasicStorageTest(t *testing.T) {
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
	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "open-liberty-operator", 1, retryInterval, operatorTimeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	exampleOpenLiberty := util.MakeBasicOpenLibertyApplication(t, f, "example-liberty-storage", namespace, 1)
	exampleOpenLiberty.Spec.Storage = &openlibertyv1beta1.OpenLibertyApplicationStorage{
		Size:      "10Mi",
		MountPath: "/mnt/data",
	}

	err = f.Client.Create(goctx.TODO(), exampleOpenLiberty, &framework.CleanupOptions{
		TestContext:   ctx,
		Timeout:       time.Second * 5,
		RetryInterval: time.Second * 1,
	})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
	err = util.WaitForStatefulSet(t, f.KubeClient, namespace, "example-liberty-storage", 1, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
	// verify that removing the storage config returns it to a deployment not a stateful set
	if err = updateStorageConfig(t, f, ctx, exampleOpenLiberty); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
}

func updateStorageConfig(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, app *openlibertyv1beta1.OpenLibertyApplication) error {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return err
	}

	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: app.Name, Namespace: namespace}, app)
	if err != nil {
		return err
	}
	// remove storage definition to return it to a deployment
	app.Spec.Storage = nil
	app.Spec.VolumeMounts = nil

	err = f.Client.Update(goctx.TODO(), app)
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, app.Name, 1, retryInterval, timeout)
	return err // implicitly return nil if no error
}

// OpenLibertyPersistenceTest Verify the volume persistence claims.
func OpenLibertyPersistenceTest(t *testing.T) {
	ctx, err := util.InitializeContext(t, cleanupTimeout, retryInterval)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Cleanup()

	f := framework.Global

	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatal(err)
	}

	RequestLimits := map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceStorage: resource.MustParse("1Gi"),
	}

	// create PVC and mount for our statefulset.
	exampleOpenLiberty := util.MakeBasicOpenLibertyApplication(t, f, "example-liberty-persistence", namespace, 1)
	exampleOpenLiberty.Spec.Storage = &openlibertyv1beta1.OpenLibertyApplicationStorage{
		VolumeClaimTemplate: &corev1.PersistentVolumeClaim{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: "pvc",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: RequestLimits,
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{},
		},
	}
	exampleOpenLiberty.Spec.VolumeMounts = []corev1.VolumeMount{{
		Name:      "pvc",
		MountPath: "/data",
	}}

	err = f.Client.Create(goctx.TODO(), exampleOpenLiberty, &framework.CleanupOptions{
		TestContext:   ctx,
		Timeout:       cleanupTimeout,
		RetryInterval: cleanupRetryInterval,
	})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	err = util.WaitForStatefulSet(t, f.KubeClient, namespace, "example-liberty-persistence", 1, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// again remove the storage configuration and see that it deploys correctly.
	if err = updateStorageConfig(t, f, ctx, exampleOpenLiberty); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
}
