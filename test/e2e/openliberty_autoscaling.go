package e2e

import (
	goctx "context"
	"errors"
	"testing"
	"time"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	"github.com/OpenLiberty/open-liberty-operator/test/util"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	e2eutil "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	k "sigs.k8s.io/controller-runtime/pkg/client"
)

// OpenLibertyAutoScalingTest : More indepth testing of autoscaling
func OpenLibertyAutoScalingTest(t *testing.T) {

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
	const name string = "example-liberty-autoscaling"

	// Wait for the operator as the following configmaps won't exist until it has deployed
	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "open-liberty-operator", 1, retryInterval, operatorTimeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	timestamp := time.Now().UTC()
	t.Logf("%s - Starting open liberty autoscaling test...", timestamp)

	// create one replica of the operator deployment in current namespace with provided name
	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "open-liberty-operator", 1, retryInterval, operatorTimeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// Make basic open liberty application with 1 replica
	replicas := int32(1)
	openLibertyApplication := util.MakeBasicOpenLibertyApplication(t, f, name, namespace, replicas)

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(goctx.TODO(), openLibertyApplication, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// wait for example-liberty-autoscaling to reach 1 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, name, 1, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	target := types.NamespacedName{Name: name, Namespace: namespace}
	err = util.UpdateApplication(f, target, func(o *openlibertyv1beta1.OpenLibertyApplication) {
		o.Spec.ResourceConstraints = setResources("0.2")
		o.Spec.Autoscaling = setAutoScale(5, 50)
	})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// Check the name field that matches
	m := map[string]string{"metadata.name": name}
	l := fields.Set(m)
	selec := l.AsSelector()

	hpa := &autoscalingv1.HorizontalPodAutoscalerList{}
	options := k.ListOptions{FieldSelector: selec, Namespace: namespace}
	hpa = getHPA(hpa, t, f, options)

	timestamp = time.Now().UTC()
	t.Logf("%s - Deployment created, verifying autoscaling...", timestamp)

	err = waitForHPA(hpa, t, 1, 5, 50, f, options)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	updateTest(t, f, openLibertyApplication, options, namespace, hpa)
	minMaxTest(t, f, openLibertyApplication, options, namespace, hpa)
	minBoundaryTest(t, f, openLibertyApplication, options, namespace, hpa)
	incorrectFieldsTest(t, f, ctx)
	replicasTest(t, f, ctx)
}

func getHPA(hpa *autoscalingv1.HorizontalPodAutoscalerList, t *testing.T, f *framework.Framework, options k.ListOptions) *autoscalingv1.HorizontalPodAutoscalerList {
	if err := f.Client.List(goctx.TODO(), hpa, &options); err != nil {
		t.Logf("Get HPA: (%v)", err)
	}
	return hpa
}

func waitForHPA(hpa *autoscalingv1.HorizontalPodAutoscalerList, t *testing.T, minReplicas int32, maxReplicas int32, utiliz int32, f *framework.Framework, options k.ListOptions) error {
	for counter := 0; counter < 6; counter++ {
		time.Sleep(4000 * time.Millisecond)
		hpa = getHPA(hpa, t, f, options)
		if checkValues(hpa, t, minReplicas, maxReplicas, utiliz) == nil {
			return nil
		}
	}
	return checkValues(hpa, t, minReplicas, maxReplicas, utiliz)
}

func setResources(cpu string) *corev1.ResourceRequirements {
	cpuRequest := resource.MustParse(cpu)

	return &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: cpuRequest,
		},
	}
}

func setAutoScale(values ...int32) *openlibertyv1beta1.OpenLibertyApplicationAutoScaling {
	if len(values) == 3 {
		return &openlibertyv1beta1.OpenLibertyApplicationAutoScaling{
			TargetCPUUtilizationPercentage: &values[2],
			MaxReplicas:                    values[0],
			MinReplicas:                    &values[1],
		}
	} else if len(values) == 2 {
		return &openlibertyv1beta1.OpenLibertyApplicationAutoScaling{
			TargetCPUUtilizationPercentage: &values[1],
			MaxReplicas:                    values[0],
		}
	}

	return &openlibertyv1beta1.OpenLibertyApplicationAutoScaling{}

}

func checkValues(hpa *autoscalingv1.HorizontalPodAutoscalerList, t *testing.T, minReplicas int32, maxReplicas int32, utiliz int32) error {

	if hpa.Items[0].Spec.MaxReplicas != maxReplicas {
		t.Logf("Max replicas is set to: %d", hpa.Items[0].Spec.MaxReplicas)
		return errors.New("Error: Max replicas is not correctly set")
	}

	if *hpa.Items[0].Spec.MinReplicas != minReplicas {
		t.Logf("Min replicas is set to: %d", *hpa.Items[0].Spec.MinReplicas)
		return errors.New("Error: Min replicas is not correctly set")
	}

	if *hpa.Items[0].Spec.TargetCPUUtilizationPercentage != utiliz {
		t.Logf("TargetCPUUtilization is set to: %d", *hpa.Items[0].Spec.TargetCPUUtilizationPercentage)
		return errors.New("Error: TargetCPUUtilizationis is not correctly set")
	}

	return nil
}

// Updates the values and checks they are changed
func updateTest(t *testing.T, f *framework.Framework, openLibertyApplication *openlibertyv1beta1.OpenLibertyApplication, options k.ListOptions, namespace string, hpa *autoscalingv1.HorizontalPodAutoscalerList) {

	err := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "example-liberty-autoscaling", Namespace: namespace}, openLibertyApplication)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	openLibertyApplication.Spec.ResourceConstraints = setResources("0.2")
	openLibertyApplication.Spec.Autoscaling = setAutoScale(3, 2, 30)

	err = f.Client.Update(goctx.TODO(), openLibertyApplication)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	timestamp := time.Now().UTC()
	t.Logf("%s - Deployment created, verifying autoscaling...", timestamp)

	hpa = getHPA(hpa, t, f, options)

	err = waitForHPA(hpa, t, 2, 3, 30, f, options)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
}

// Checks when max is less than min, there should be no update
func minMaxTest(t *testing.T, f *framework.Framework, openLibertyApplication *openlibertyv1beta1.OpenLibertyApplication, options k.ListOptions, namespace string, hpa *autoscalingv1.HorizontalPodAutoscalerList) {

	err := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "example-liberty-autoscaling", Namespace: namespace}, openLibertyApplication)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	openLibertyApplication.Spec.ResourceConstraints = setResources("0.2")
	openLibertyApplication.Spec.Autoscaling = setAutoScale(1, 6, 10)

	err = f.Client.Update(goctx.TODO(), openLibertyApplication)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	timestamp := time.Now().UTC()
	t.Logf("%s - Deployment created, verifying autoscaling...", timestamp)

	hpa = getHPA(hpa, t, f, options)

	err = waitForHPA(hpa, t, 2, 3, 30, f, options)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
}

// When min is set to less than 1, there should be no update since the minReplicas are updated to a value less than 1
func minBoundaryTest(t *testing.T, f *framework.Framework, openLibertyApplication *openlibertyv1beta1.OpenLibertyApplication, options k.ListOptions, namespace string, hpa *autoscalingv1.HorizontalPodAutoscalerList) {

	err := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "example-liberty-autoscaling", Namespace: namespace}, openLibertyApplication)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	openLibertyApplication.Spec.ResourceConstraints = setResources("0.5")
	openLibertyApplication.Spec.Autoscaling = setAutoScale(4, 0, 20)

	err = f.Client.Update(goctx.TODO(), openLibertyApplication)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	timestamp := time.Now().UTC()
	t.Logf("%s - Deployment created, verifying autoscaling...", timestamp)

	hpa = getHPA(hpa, t, f, options)

	err = waitForHPA(hpa, t, 2, 3, 30, f, options)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
}

// When the mandatory fields for autoscaling are not set
func incorrectFieldsTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) {

	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatalf("could not get namespace: %v", err)
	}
	const name string = "example-liberty-autoscaling2"

	timestamp := time.Now().UTC()
	t.Logf("%s - Starting liberty autoscaling test...", timestamp)

	// Make basic liberty application with 1 replica
	replicas := int32(1)
	openLibertyApplication := util.MakeBasicOpenLibertyApplication(t, f, name, namespace, replicas)

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(goctx.TODO(), openLibertyApplication, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// wait for example-liberty-autoscaling to reach 1 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, name, 1, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// Check the name field that matches
	m := map[string]string{"metadata.name": name}
	l := fields.Set(m)
	selec := l.AsSelector()

	options := k.ListOptions{FieldSelector: selec, Namespace: namespace}

	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, openLibertyApplication)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	openLibertyApplication.Spec.ResourceConstraints = setResources("0.3")
	openLibertyApplication.Spec.Autoscaling = setAutoScale(4)

	err = f.Client.Update(goctx.TODO(), openLibertyApplication)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	timestamp = time.Now().UTC()
	t.Logf("%s - Deployment created, verifying autoscaling...", timestamp)

	hpa := &autoscalingv1.HorizontalPodAutoscalerList{}
	hpa = getHPA(hpa, t, f, options)

	if len(hpa.Items) == 0 {
		t.Log("The mandatory fields were not set so autoscaling is not enabled")
	} else {
		t.Fatal("Error: The mandatory fields were not set so autoscaling should not be enabled")
	}
}

func replicasTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) {
	const name = "liberty-autoscaling-replicas"
	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatalf("could not get namespace: %v", err)
	}

	timestamp := time.Now().UTC()
	t.Logf("%s - Starting runtime autoscaling test...", timestamp)

	// Make basic runtime omponent with 1 replica
	replicas := int32(2)
	runtime := util.MakeBasicOpenLibertyApplication(t, f, name, namespace, replicas)

	err = f.Client.Create(goctx.TODO(), runtime, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, name, int(replicas), retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// check that it prioritizes the HPA's minimum number of replicas over spec replicas
	target := types.NamespacedName{Namespace: namespace, Name: name}
	err = util.UpdateApplication(f, target, func(o *openlibertyv1beta1.OpenLibertyApplication) {
		o.Spec.ResourceConstraints = setResources("0.5")
		var cpu int32 = 50
		var min int32 = 3
		o.Spec.Autoscaling = &openlibertyv1beta1.OpenLibertyApplicationAutoScaling{
			TargetCPUUtilizationPercentage: &cpu,
			MaxReplicas:                    5,
			MinReplicas:                    &min,
		}
	})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, name, 3, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// check that it correctly returns to defined replica count after deleting HPA
	err = util.UpdateApplication(f, target, func(o *openlibertyv1beta1.OpenLibertyApplication) {
		o.Spec.ResourceConstraints = nil
		o.Spec.Autoscaling = nil
	})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, name, int(replicas), retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
}
