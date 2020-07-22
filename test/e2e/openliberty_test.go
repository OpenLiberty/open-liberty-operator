package e2e

import (
	"os"
	"testing"

	"github.com/OpenLiberty/open-liberty-operator/pkg/apis"
	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Test struct {
	Name string
	Test func(*testing.T)
}

var (
	basicTests = []Test{
		{"OpenLibertyPullPolicyTest", OpenLibertyPullPolicyTest},
		{"OpenLibertyBasicTest", OpenLibertyBasicTest},
		{"OpenLibertyProbeTest", OpenLibertyProbeTest},
		{"OpenLibertyAutoScalingTest", OpenLibertyAutoScalingTest},
		{"OpenLibertyStorageTest", OpenLibertyBasicStorageTest},
		{"OpenLibertyPersistenceTest", OpenLibertyPersistenceTest},
	}
	advancedTests = []Test{
		{"OpenLibertyServiceMonitorTest", OpenLibertyServiceMonitorTest},
		{"OpenLibertyKnativeTest", OpenLibertyKnativeTest},
		{"OpenLibertyServiceBindingTest", OpenLibertyServiceBindingTest},
		{"OpenLibertyCertManagerTest", OpenLibertyCertManagerTest},
		{"OpenLibertyDumpsTest", OpenLibertyDumpsTest},
		{"OpenLibertyTraceTest", OpenLibertyTraceTest},
		{"OpenLibertyKappNavTest", OpenLibertyKappNavTest},
	}
	ocpTests = []Test{
		{"OpenLibertyImageStreamTest", OpenLibertyImageStreamTest},
	}
	independantTests = []Test{}
)

func TestOpenLibertyApplication(t *testing.T) {
	cluster := os.Getenv("CLUSTER_ENV")
	t.Logf("running e2e tests as '%s'", cluster)

	openLibertyApplicationList := &openlibertyv1beta1.OpenLibertyApplicationList{
		TypeMeta: metav1.TypeMeta{
			Kind: "OpenLibertyApplication",
		},
	}
	openLibertyTraceList := &openlibertyv1beta1.OpenLibertyTraceList{
		TypeMeta: metav1.TypeMeta{
			Kind: "OpenLibertyTrace",
		},
	}

	err := framework.AddToFrameworkScheme(apis.AddToScheme, openLibertyApplicationList)
	if err != nil {
		t.Fatalf("Failed to add CR scheme to framework: %v", err)
	}

	err = framework.AddToFrameworkScheme(apis.AddToScheme, openLibertyTraceList)
	if err != nil {
		t.Fatalf("Failed to add Trace scheme to framework: %v", err)
	}

	if cluster != "minikube" {
		t.Run("OpenLibertySSOTest", OpenLibertySSOTest)
	}

	// basic tests that are runnable locally in minishift/kube
	for _, test := range basicTests {
		RuntimeTestRunner(t, test)
	}

	// tests for features that will require cluster configuration
	// i.e. knative requires installations
	if cluster != "minikube" {
		for _, test := range advancedTests {
			RuntimeTestRunner(t, test)
		}
	}

	// tests for features NOT expected to run in OpenShift
	// i.e. Ingress
	if cluster == "minikube" || cluster == "kubernetes" {
		for _, test := range independantTests {
			RuntimeTestRunner(t, test)
		}
	}

	// tests for features that ONLY exist in OpenShift
	// i.e. image streams are only in OpenShift
	if cluster == "ocp" {
		for _, test := range ocpTests {
			RuntimeTestRunner(t, test)
		}
	}
}

func testAdvancedFeatures(t *testing.T) {
	// These features require a bit of configuration
	// which makes them less ideal for quick minikube tests
	t.Run("OpenLibertyServiceMonitorTest", OpenLibertyServiceMonitorTest)
	t.Run("OpenLibertyKnativeTest", OpenLibertyKnativeTest)
	t.Run("OpenLibertyServiceBindingTest", OpenLibertyServiceBindingTest)
	t.Run("OpenLibertyCertManagerTest", OpenLibertyCertManagerTest)
	t.Run("OpenLibertyDumpsTest", OpenLibertyDumpsTest)
	t.Run("OpenLibertyKappNavTest", OpenLibertyKappNavTest)
	t.Run("OpenLibertySSOTest", OpenLibertySSOTest)
}

// Verify functionality that is tied to OCP
func testOCPFeatures(t *testing.T) {
	t.Run("OpenLibertyImageStreamTest", OpenLibertyImageStreamTest)
}

// Verify functionality that is not expected to run on OCP
func testIndependantFeatures(t *testing.T) {
	// TODO: implement test for ingress
}

func RuntimeTestRunner(t *testing.T, test Test) {
	t.Run(test.Name, test.Test)
}
