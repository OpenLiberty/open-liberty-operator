package openliberty

import (
	"testing"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	coretesting "k8s.io/client-go/testing"
)

var (
	name                       = "app"
	namespace                  = "openliberty"
	appImage                   = "my-image"
	ksvcAppImage               = "ksvc-image"
	replicas             int32 = 3
	autoscaling                = &openlibertyv1beta1.LibertyApplicationAutoScaling{MaxReplicas: 3}
	pullPolicy                 = corev1.PullAlways
	serviceType                = corev1.ServiceTypeClusterIP
	service                    = &openlibertyv1beta1.LibertyApplicationService{Type: serviceType, Port: 8443}
	genService                 = &openlibertyv1beta1.LibertyApplicationService{Type: serviceType, Port: 9080}
	expose                     = true
	serviceAccountName         = "service-account"
	volumeCT                   = &corev1.PersistentVolumeClaim{TypeMeta: metav1.TypeMeta{Kind: "StatefulSet"}}
	storage                    = openlibertyv1beta1.LibertyApplicationStorage{Size: "10Mi", MountPath: "/mnt/data", VolumeClaimTemplate: volumeCT}
	createKnativeService       = true
	statefulSetSN              = name + "-headless"
)

type Test struct {
	test     string
	expected interface{}
	actual   interface{}
}

func TestOpenLibertyController(t *testing.T) {

}

// Helper Functions
func createOpenLibertyApp(n, ns string, spec openlibertyv1beta1.LibertyApplicationSpec) *openlibertyv1beta1.LibertyApplication {
	app := &openlibertyv1beta1.LibertyApplication{
		ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: ns},
		Spec:       spec,
	}
	return app
}

func createFakeDiscoveryClient() discovery.DiscoveryInterface {
	fakeDiscoveryClient := &fakediscovery.FakeDiscovery{Fake: &coretesting.Fake{}}
	fakeDiscoveryClient.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: routev1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "routes", Namespaced: true, Kind: "Route"},
			},
		},
		{
			GroupVersion: servingv1alpha1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "services", Namespaced: true, Kind: "Service", SingularName: "service"},
			},
		},
	}

	return fakeDiscoveryClient
}
