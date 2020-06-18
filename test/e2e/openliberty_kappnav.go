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

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	applicationsv1beta1 "sigs.k8s.io/application/pkg/apis/app/v1beta1"
)

var appName string = "test-app"

// OpenLibertyKappNavTest : Test kappnav feature set
func OpenLibertyKappNavTest(t *testing.T) {
	// standard initialization
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

	// add to scheme to framework can find the resource
	err = applicationsv1beta1.AddToScheme(f.Scheme)
	if err != nil {
		t.Fatal(err)
	}

	// wait for the operator as the following configmaps won't exist until it has deployed
	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "open-liberty-operator", 1, retryInterval, operatorTimeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = createKappNavApplication(t, f, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = updateKappNavApplications(t, f, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = useExistingApplications(t, f, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
}

func createKappNavApplication(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	ns, err := ctx.GetNamespace()
	if err != nil {
		return err
	}

	const name string = "example-openliberty-kappnav"

	openliberty := util.MakeBasicOpenLibertyApplication(t, f, name, ns, 1)
	openliberty.Spec.ApplicationName = appName

	err = f.Client.Create(goctx.TODO(), openliberty, &framework.CleanupOptions{TestContext: ctx, Timeout: timeout, RetryInterval: retryInterval})
	if err != nil {
		return err
	}

	// verify readiness of created resource
	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, name, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	target := types.NamespacedName{Namespace: ns, Name: name}

	ok, err := verifyKappNavLabels(t, f, target)
	if err != nil {
		return err
	} else if !ok {
		return errors.New("could not find kappnav labels")
	}
	t.Log("kappnav labels found")

	err = util.WaitForApplicationCreated(t, f, types.NamespacedName{Name: appName, Namespace: ns})
	if err != nil {
		return err
	}
	t.Log("related application definition found")

	return nil
}

func updateKappNavApplications(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	ns, err := ctx.GetNamespace()
	if err != nil {
		return err
	}

	const name string = "example-openliberty-kappnav"

	target := types.NamespacedName{Namespace: ns, Name: name}

	err = util.UpdateApplication(f, target, func(r *openlibertyv1beta1.OpenLibertyApplication) {
		appDef := false
		r.Spec.CreateAppDefinition = &appDef
	})
	if err != nil {
		return err
	}

	ok, err := verifyKappNavLabels(t, f, target)
	if err != nil {
		return err
	} else if !ok {
		return errors.New("kappnav labels present after disabling")
	}
	t.Log("kappnav labels successfully removed")

	err = util.WaitForApplicationDelete(t, f, types.NamespacedName{Name: appName, Namespace: ns})
	if err != nil {
		return err
	}
	t.Log("created application definition removed")

	return nil
}

func useExistingApplications(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	ns, err := ctx.GetNamespace()
	if err != nil {
		return err
	}

	const name string = "example-openliberty-kappnav"
	var existingAppName string = "existing-app"
	// add selector labels to verify that app was actually found
	selectMatchLabels := map[string]string{
		"test-key": "test-value",
	}

	// create existing application
	err = util.CreateApplicationTarget(f, ctx, types.NamespacedName{Name: existingAppName, Namespace: ns}, selectMatchLabels)
	if err != nil {
		return err
	}

	// connect to existing application IN namespace
	target := types.NamespacedName{Namespace: ns, Name: name}

	err = util.UpdateApplication(f, target, func(r *openlibertyv1beta1.OpenLibertyApplication) {
		r.Spec.ApplicationName = existingAppName
	})
	if err != nil {
		return err
	}

	t.Log("waiting 5 seconds")
	time.Sleep(5 * time.Second)

	openliberty := &openlibertyv1beta1.OpenLibertyApplication{}
	err = f.Client.Get(goctx.TODO(), target, openliberty)
	if err != nil {
		return err
	}

	openlibertyLabels := openliberty.Labels

	if _, ok := openlibertyLabels["test-key"]; !ok {
		return errors.New("selector labels from target application not present")
	}
	t.Log("target application correctly applied to the component")

	if openlibertyLabels["app.kubernetes.io/part-of"] != existingAppName {
		return errors.New("part-of label not correctly set")
	}
	t.Log("part-of label correctly set")

	return nil
}

func verifyKappNavLabels(t *testing.T, f *framework.Framework, target types.NamespacedName) (bool, error) {
	dep := &appsv1.Deployment{}
	err := f.Client.Get(goctx.TODO(), target, dep)
	if err != nil {
		return false, err
	}

	labels := dep.GetLabels()

	// verify that label present, full set of annos checked by unit tests
	if labels["kappnav.app.auto-create"] != "true" {
		return false, nil
	}

	return true, nil
}
