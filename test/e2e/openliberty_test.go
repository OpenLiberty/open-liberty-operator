package e2e

import (
	"os"
	"testing"

	"github.com/OpenLiberty/open-liberty-operator/pkg/apis"
	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// basic tests that are runnable locally in minishift/kube
	// t.Run("OpenLibertyPullPolicyTest", OpenLibertyPullPolicyTest)
	t.Run("OpenLibertyBasicTest", OpenLibertyBasicTest)
	// t.Run("OpenLibertyProbeTest", OpenLibertyProbeTest)
	// t.Run("OpenLibertyAutoScalingTest", OpenLibertyAutoScalingTest)
	// t.Run("OpenLibertyStorageTest", OpenLibertyBasicStorageTest)
	// t.Run("OpenLibertyPersistenceTest", OpenLibertyPersistenceTest)
	// t.Run("OpenLibertyTraceTest", OpenLibertyTraceTest)

	if cluster != "local" {
		// only test non-OCP features on minikube
		if cluster == "minikube" {
			testIndependantFeatures(t)
			return
		}

		// test all features that require some configuration
		testAdvancedFeatures(t)

		// test features that require OCP
		if cluster == "ocp" {
			testOCPFeatures(t)
		}
	}
}

func testAdvancedFeatures(t *testing.T) {
	// These features require a bit of configuration
	// which makes them less ideal for quick minikube tests
	// t.Run("OpenLibertyServiceMonitorTest", OpenLibertyServiceMonitorTest)
	// t.Run("OpenLibertyKnativeTest", OpenLibertyKnativeTest)
	// t.Run("OpenLibertyServiceBindingTest", OpenLibertyServiceBindingTest)
	// t.Run("OpenLibertyCertManagerTest", OpenLibertyCertManagerTest)
	// t.Run("OpenLibertyDumpsTest", OpenLibertyDumpsTest)
	// t.Run("OpenLibertyKappNavTest", OpenLibertyKappNavTest)
	// t.Run("OpenLibertySSOTest", OpenLibertySSOTest)
}

// Verify functionality that is tied to OCP
func testOCPFeatures(t *testing.T) {
	// t.Run("OpenLibertyImageStreamTest", OpenLibertyImageStreamTest)
}

// Verify functionality that is not expected to run on OCP
func testIndependantFeatures(t *testing.T) {
	// TODO: implement test for ingress
}
