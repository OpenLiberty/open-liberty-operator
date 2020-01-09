package e2e

import (
	"testing"

	"github.com/OpenLiberty/open-liberty-operator/pkg/apis"
	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOpenLibertyApplication(t *testing.T) {
	openLibertyApplicationList := &openlibertyv1beta1.OpenLibertyApplicationList{
		TypeMeta: metav1.TypeMeta{
			Kind: "OpenLibertyApplication",
		},
	}
	err := framework.AddToFrameworkScheme(apis.AddToScheme, openLibertyApplicationList)
	if err != nil {
		t.Fatalf("Failed to add CR scheme to framework: %v", err)
	}
	t.Run("OpenLibertyPullPolicyTest", OpenLibertyPullPolicyTest)
	t.Run("OpenLibertyBasicTest", OpenLibertyBasicTest)
	t.Run("OpenLibertyStorageTest", OpenLibertyBasicStorageTest)
	t.Run("OpenLibertyPersistenceTest", OpenLibertyPersistenceTest)
	t.Run("OpenLibertyProbeTest", OpenLibertyProbeTest)
	t.Run("OpenLibertyAutoScalingTest", OpenLibertyAutoScalingTest)
	t.Run("OpenLibertyServiceMonitorTest", OpenLibertyServiceMonitorTest)
	t.Run("OpenLibertyKnativeTest", OpenLibertyKnativeTest)
	t.Run("OpenLibertyDumpsTest", OpenLibertyDumpsTest)
}
