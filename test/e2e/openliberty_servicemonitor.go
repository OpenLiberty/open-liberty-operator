package e2e

import (
	goctx "context"
	"errors"
	"testing"
	"time"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	"github.com/OpenLiberty/open-liberty-operator/test/util"
	prometheusv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	e2eutil "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	k "sigs.k8s.io/controller-runtime/pkg/client"
)

// OpenLibertyServiceMonitorTest ...
func OpenLibertyServiceMonitorTest(t *testing.T) {
	ctx, err := util.InitializeContext(t, cleanupTimeout, retryInterval)
	defer ctx.Cleanup()
	if err != nil {
		t.Fatal(err)
	}

	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatalf("Couldn't get namespace: %v", err)
	}

	t.Logf("Namespace: %s", namespace)
	f := framework.Global

	// Adds the prometheus resources to the scheme
	if err = prometheusv1.AddToScheme(f.Scheme); err != nil {
		t.Logf("Unable to add prometheus scheme: (%v)", err)
		util.FailureCleanup(t, f, namespace, err)
	}

	// create one replica of the operator deployment in current namespace with provided name
	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "open-liberty-operator", 1, retryInterval, operatorTimeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	helper := int32(1)
	openLiberty := util.MakeBasicOpenLibertyApplication(t, f, "example-liberty-sm", namespace, helper)

	// Create application deployment and wait
	err = f.Client.Create(goctx.TODO(), openLiberty, &framework.CleanupOptions{TestContext: ctx, Timeout: time.Second, RetryInterval: time.Second})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "example-liberty-sm", 1, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// Returns a list of the service monitor with the specified label
	m := map[string]string{"apps-prometheus": ""}
	l := labels.Set(m)
	selec := l.AsSelector()

	smList := &prometheusv1.ServiceMonitorList{}
	options := k.ListOptions{LabelSelector: selec, Namespace: namespace}

	// If there are no service monitors deployed an error will be thrown below
	err = f.Client.List(goctx.TODO(), smList, &options)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if len(smList.Items) != 0 {
		util.FailureCleanup(t, f, namespace, errors.New("There is another service monitor running"))
	}

	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "example-liberty-sm", Namespace: namespace}, openLiberty)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// Adds the mandatory label to the application so it will be picked up by the prometheus operator
	label := map[string]string{"apps-prometheus": ""}
	monitor := &openlibertyv1beta1.OpenLibertyApplicationMonitoring{Labels: label}
	openLiberty.Spec.Monitoring = monitor

	// Updates the application so the operator is reconciled
	helper = int32(2)
	openLiberty.Spec.Replicas = &helper

	err = f.Client.Update(goctx.TODO(), openLiberty)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "example-liberty-sm", 2, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// If there are no service monitors deployed an error will be thrown below
	err = f.Client.List(goctx.TODO(), smList, &options)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// Gets the service monitor
	sm := smList.Items[0]

	smPath := sm.Spec.Endpoints[0].Path
	smPort := sm.Spec.Endpoints[0].Port
	smParams := sm.Spec.Endpoints[0].Params
	smScheme := sm.Spec.Endpoints[0].Scheme
	smScrapeTimeout := sm.Spec.Endpoints[0].ScrapeTimeout
	smInterval := sm.Spec.Endpoints[0].Interval
	smBTF := sm.Spec.Endpoints[0].BearerTokenFile

	if sm.Spec.Selector.MatchLabels["app.kubernetes.io/instance"] != "example-liberty-sm" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor is not connected to the liberty application?"))
	}

	if smPath != "" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor path default is incorrect"))
	}

	if smPort != "9080-tcp" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor port default is incorrect"))
	}

	if smParams != nil {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor params default is incorrect"))
	}

	if smScheme != "" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor scheme default is incorrect"))
	}

	if smScrapeTimeout != "" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor scrape timeout default is incorrect"))
	}

	if smInterval != "" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor interval default is incorrect"))
	}

	if smBTF != "" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor bearer token file default is incorrect"))
	}

	testSettingOpenLibertyServiceMonitor(t, f, namespace, openLiberty)
}

func testSettingOpenLibertyServiceMonitor(t *testing.T, f *framework.Framework, namespace string, openLiberty *openlibertyv1beta1.OpenLibertyApplication) {
	err := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "example-liberty-sm", Namespace: namespace}, openLiberty)
	if err != nil {
		t.Fatal(err)
	}

	params := map[string][]string{
		"params": []string{"param1", "param2"},
	}
	username := v1.SecretKeySelector{Key: "username"}
	password := v1.SecretKeySelector{Key: "password"}

	// Creates the endpoint fields the user can customize
	endpoint := prometheusv1.Endpoint{
		Path:            "/path",
		Scheme:          "myScheme",
		Params:          params,
		Interval:        "30s",
		ScrapeTimeout:   "10s",
		TLSConfig:       &prometheusv1.TLSConfig{InsecureSkipVerify: true},
		BearerTokenFile: "myBTF",
		BasicAuth:       &prometheusv1.BasicAuth{Username: username, Password: password},
	}

	endpoints := []prometheusv1.Endpoint{endpoint}

	// Adds the mandatory label to the application so it will be picked up by the prometheus operator
	label := map[string]string{"apps-prometheus": ""}
	monitor := &openlibertyv1beta1.OpenLibertyApplicationMonitoring{Labels: label, Endpoints: endpoints}
	openLiberty.Spec.Monitoring = monitor

	// Updates the application so the operator is reconciled
	helper := int32(3)
	openLiberty.Spec.Replicas = &helper

	err = f.Client.Update(goctx.TODO(), openLiberty)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "example-liberty-sm", 3, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// Returns a list of the service monitor with the specified label
	m := map[string]string{"apps-prometheus": ""}
	l := labels.Set(m)
	selec := l.AsSelector()

	smList := &prometheusv1.ServiceMonitorList{}
	options := k.ListOptions{LabelSelector: selec, Namespace: namespace}

	// If there are no service monitors deployed an error will be thrown below
	err = f.Client.List(goctx.TODO(), smList, &options)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// Gets the service monitor
	sm := smList.Items[0]

	smPath := sm.Spec.Endpoints[0].Path
	smPort := sm.Spec.Endpoints[0].Port
	smParams := sm.Spec.Endpoints[0].Params
	smScheme := sm.Spec.Endpoints[0].Scheme
	smScrapeTimeout := sm.Spec.Endpoints[0].ScrapeTimeout
	smInterval := sm.Spec.Endpoints[0].Interval
	smBTF := sm.Spec.Endpoints[0].BearerTokenFile
	smTLSConfig := sm.Spec.Endpoints[0].TLSConfig
	smBasicAuth := sm.Spec.Endpoints[0].BasicAuth

	if sm.Spec.Selector.MatchLabels["app.kubernetes.io/instance"] != "example-liberty-sm" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor is not connected to the liberty application?"))
	}

	if smPath != "/path" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor path is incorrect"))
	}

	if smPort != "9080-tcp" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor port is incorrect"))
	}

	if smParams == nil {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor params is incorrect"))
	}

	if smScheme != "myScheme" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor scheme is incorrect"))
	}

	if smScrapeTimeout != "10s" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor scrape timeout is incorrect"))
	}

	if smInterval != "30s" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor interval is incorrect"))
	}

	if smBTF != "myBTF" {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor bearer token file is incorrect"))
	}

	if smTLSConfig == nil {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor TLSConfig is not set"))
	}

	if smBasicAuth == nil {
		util.FailureCleanup(t, f, namespace, errors.New("The service monitor basic auth is not set"))
	}

}
