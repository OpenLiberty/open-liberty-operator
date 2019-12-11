package utils

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	autils "github.com/appsody/appsody-operator/pkg/utils"
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	coretesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	name                = "app"
	namespace           = "openliberty"
	appImage            = "my-image"
	consoleFormat       = "json"
	replicas      int32 = 3
)

type Test struct {
	test     string
	expected interface{}
	actual   interface{}
}

func TestCustomizeLibertyEnv(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	os.Setenv("WATCH_NAMESPACE", namespace)

	// Test with only consoleFormat defined
	spec := openlibertyv1beta1.LibertyApplicationSpec{
		Logs: &openlibertyv1beta1.LibertyApplicationLogs{
			ConsoleFormat: &consoleFormat,
		},
	}
	envList := []corev1.EnvVar{{Name: "WLP_LOGGING_CONSOLE_FORMAT", Value: consoleFormat}}
	pts := &corev1.PodTemplateSpec{}

	// Always call CustomizePodSpec to populate Containers & simulate real behaviour
	openliberty := createOpenLibertyApp(name, namespace, spec)
	autils.CustomizePodSpec(pts, openliberty)
	CustomizeLibertyEnv(pts, openliberty)

	testEnv := []Test{
		{"Test consoleFormat variable", envList, pts.Spec.Containers[0].Env},
	}

	if err := verifyTests(testEnv); err != nil {
		t.Fatalf("%v", err)
	}

	// Test with both env var set and consoleFormat defined (consoleFormat takes priority)
	pts = &corev1.PodTemplateSpec{}
	spec = openlibertyv1beta1.LibertyApplicationSpec{
		Env: []corev1.EnvVar{{Name: "WLP_LOGGING_CONSOLE_FORMAT", Value: "yaml"}},
		Logs: &openlibertyv1beta1.LibertyApplicationLogs{
			ConsoleFormat: &consoleFormat,
		},
	}

	openliberty = createOpenLibertyApp(name, namespace, spec)
	autils.CustomizePodSpec(pts, openliberty)
	CustomizeLibertyEnv(pts, openliberty)

	testEnv = []Test{
		{"Test w/ consoleFormat & EnvVar", envList, pts.Spec.Containers[0].Env},
	}

	if err := verifyTests(testEnv); err != nil {
		t.Fatalf("%v", err)
	}

	// Test with only direct EnvVar set
	pts = &corev1.PodTemplateSpec{}
	envList = []corev1.EnvVar{{Name: "WLP_LOGGING_CONSOLE_FORMAT", Value: "yaml"}}
	spec = openlibertyv1beta1.LibertyApplicationSpec{
		Env: envList,
	}

	openliberty = createOpenLibertyApp(name, namespace, spec)
	autils.CustomizePodSpec(pts, openliberty)
	CustomizeLibertyEnv(pts, openliberty)

	testEnv = []Test{
		{"Test w/ EnvVar", envList, pts.Spec.Containers[0].Env},
	}

	if err := verifyTests(testEnv); err != nil {
		t.Fatalf("%v", err)
	}

	// Remove all consoleFormat declarations
	pts = &corev1.PodTemplateSpec{}
	envList = []corev1.EnvVar{{Name: "WLP_LOGGING_CONSOLE_FORMAT", Value: consoleFormat}}
	spec = openlibertyv1beta1.LibertyApplicationSpec{}

	openliberty = createOpenLibertyApp(name, namespace, spec)
	autils.CustomizePodSpec(pts, openliberty)
	CustomizeLibertyEnv(pts, openliberty)

	testEnv = []Test{
		{"Test default consoleFormat value", envList, pts.Spec.Containers[0].Env},
	}

	if err := verifyTests(testEnv); err != nil {
		t.Fatalf("%v", err)
	}
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

func createReconcileRequest(n, ns string) reconcile.Request {
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: n, Namespace: ns},
	}
	return req
}

// verifyReconcile checks that there was no error and that the reconcile is valid
func verifyReconcile(res reconcile.Result, err error) error {
	if err != nil {
		return fmt.Errorf("reconcile: (%v)", err)
	}

	if res != (reconcile.Result{}) {
		return fmt.Errorf("reconcile did not return an empty result (%v)", res)
	}

	return nil
}

func verifyTests(tests []Test) error {
	for _, tt := range tests {
		if !reflect.DeepEqual(tt.actual, tt.expected) {
			return fmt.Errorf("%s test expected: (%v) actual: (%v)", tt.test, tt.expected, tt.actual)
		}
	}
	return nil
}
