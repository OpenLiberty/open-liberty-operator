package e2e

import (
	goctx "context"
	"fmt"
	"testing"
	"time"

	"github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	"github.com/OpenLiberty/open-liberty-operator/test/util"
	cmmeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	v1 "github.com/openshift/api/route/v1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpenLibertyCertManagerTest ...
func OpenLibertyCertManagerTest(t *testing.T) {
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

	if !util.IsCertManagerInstalled(t, f, ctx) {
		t.Log("cert manager not installed, skipping...")
		return
	}

	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "open-liberty-operator", 1, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = libertyPodCertificateTest(t, f, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = libertyRouteCertificateTest(t, f, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
}

func libertyPodCertificateTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	const name = "example-liberty-pod-cert"

	ns, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace: %v", err)
	}

	openLibertyApplication := util.MakeBasicOpenLibertyApplication(t, f, name, ns, 1)
	serviceType := corev1.ServiceTypeClusterIP
	openLibertyApplication.Spec.Service = &v1beta1.OpenLibertyApplicationService{
			Port: 3000,
			Type: &serviceType,
			Certificate: &v1beta1.Certificate{},
		}

	timestamp := time.Now().UTC()
	t.Logf("%s - Creating cert-manager pod test...", timestamp)
	err = f.Client.Create(goctx.TODO(), openLibertyApplication,
		&framework.CleanupOptions{TestContext: ctx, Timeout: time.Second, RetryInterval: time.Second})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, name, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	err = util.WaitForCertificate(t, f, ns, fmt.Sprintf("%s-svc-crt", name), retryInterval, timeout)
	if err != nil {
		return err
	}

	return nil
}

func libertyRouteCertificateTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	const name = "example-liberty-route-cert"

	ns, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace %v", err)
	}

	openLibertyApplication := util.MakeBasicOpenLibertyApplication(t, f, name, ns, 1)
	terminationPolicy := v1.TLSTerminationReencrypt
	expose := true
	openLibertyApplication.Spec.Expose = &expose
	openLibertyApplication.Spec.Route = &v1beta1.OpenLibertyApplicationRoute{
		Host:        "myapp.mycompany.com",
		Termination: &terminationPolicy,
		Certificate: &v1beta1.Certificate{},
	}

	timestamp := time.Now().UTC()
	t.Logf("%s - Creating cert-manager route test...", timestamp)

	err = f.Client.Create(goctx.TODO(), openLibertyApplication,
		&framework.CleanupOptions{TestContext: ctx, Timeout: time.Second, RetryInterval: time.Second})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, name, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	err = util.WaitForCertificate(t, f, ns, fmt.Sprintf("%s-route-crt", name), retryInterval, timeout)
	if err != nil {
		return err
	}

	return nil
}

func libertyAdvancedCertificateTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	const name = "example-liberty-advanced-cert"
	ns, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace %v", err)
	}

	openLibertyApplication := util.MakeBasicOpenLibertyApplication(t, f, name, ns, 1)
	terminationPolicy := v1.TLSTerminationReencrypt
	expose := true
	var durationTime time.Duration = 8000
	duration := metav1.Duration{
		Duration: durationTime,
	}
	openLibertyApplication.Spec.Expose = &expose
	openLibertyApplication.Spec.Route = &v1beta1.OpenLibertyApplicationRoute{
		Host:        "myapp.mycompany.com",
		Termination: &terminationPolicy,
		Certificate: &v1beta1.Certificate{
			Duration:     &duration,
			Organization: []string{"My Company"},
			IssuerRef: cmmeta.ObjectReference{
				Name: "self-signed",
				Kind: "ClusterIssuer",
			},
		},
	}

	timestamp := time.Now().UTC()
	t.Logf("%s - Creating cert-manager route test...", timestamp)

	err = f.Client.Create(goctx.TODO(), openLibertyApplication,
		&framework.CleanupOptions{TestContext: ctx, Timeout: time.Second, RetryInterval: time.Second})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, name, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	err = util.WaitForCertificate(t, f, ns, fmt.Sprintf("%s-route-crt", name), retryInterval, timeout)
	if err != nil {
		return err
	}

	return nil
}
