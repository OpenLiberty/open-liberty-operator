package utils

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	openlibertyv1beta2 "github.com/OpenLiberty/open-liberty-operator/api/v1beta2"
	oputils "github.com/application-stacks/runtime-component-operator/utils"
	routev1 "github.com/openshift/api/route/v1"
	v1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	coretesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	name                = "app"
	namespace           = "openliberty"
	appImage            = "my-image"
	consoleFormat       = "json"
	replicas      int32 = 3
	clusterType         = corev1.ServiceTypeClusterIP
)

type Test struct {
	test     string
	expected interface{}
	actual   interface{}
}

func TestCustomizeLibertyEnv(t *testing.T) {
	logger := zap.New()
	logf.SetLogger(logger)
	os.Setenv("WATCH_NAMESPACE", namespace)

	// Test default values no config
	svc := &openlibertyv1beta2.OpenLibertyApplicationService{Port: 8080, Type: &clusterType}
	spec := openlibertyv1beta2.OpenLibertyApplicationSpec{Service: svc}
	pts := &corev1.PodTemplateSpec{}

	targetEnv := []corev1.EnvVar{
		{Name: "WLP_LOGGING_CONSOLE_LOGLEVEL", Value: "info"},
		{Name: "WLP_LOGGING_CONSOLE_SOURCE", Value: "message,accessLog,ffdc,audit"},
		{Name: "WLP_LOGGING_CONSOLE_FORMAT", Value: "json"},
	}
	// Always call CustomizePodSpec to populate Containers & simulate real behaviour
	openliberty := createOpenLibertyApp(name, namespace, spec)
	oputils.CustomizePodSpec(pts, openliberty)
	CustomizeLibertyEnv(pts, openliberty)

	testEnv := []Test{
		{"Test environment defaults", pts.Spec.Containers[0].Env, targetEnv},
	}

	if err := verifyTests(testEnv); err != nil {
		t.Fatalf("%v", err)
	}

	// test with env variables set by user
	targetEnv = []corev1.EnvVar{
		{Name: "WLP_LOGGING_CONSOLE_LOGLEVEL", Value: "error"},
		{Name: "WLP_LOGGING_CONSOLE_SOURCE", Value: "trace,accessLog,ffdc"},
		{Name: "WLP_LOGGING_CONSOLE_FORMAT", Value: "basic"},
	}

	spec = openlibertyv1beta2.OpenLibertyApplicationSpec{
		Env:     targetEnv,
		Service: svc,
	}
	pts = &corev1.PodTemplateSpec{}

	openliberty = createOpenLibertyApp(name, namespace, spec)
	oputils.CustomizePodSpec(pts, openliberty)
	CustomizeLibertyEnv(pts, openliberty)

	testEnv = []Test{
		{"Test environment config", pts.Spec.Containers[0].Env, targetEnv},
	}
	if err := verifyTests(testEnv); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCustomizeEnvSSO(t *testing.T) {
	logger := zap.New()
	logf.SetLogger(logger)
	os.Setenv("WATCH_NAMESPACE", namespace)
	svc := &openlibertyv1beta2.OpenLibertyApplicationService{Port: 8080, Type: &clusterType}
	spec := openlibertyv1beta2.OpenLibertyApplicationSpec{Service: svc}
	liberty := createOpenLibertyApp(name, namespace, spec)
	objs, s := []runtime.Object{liberty}, scheme.Scheme
	s.AddKnownTypes(openlibertyv1beta2.GroupVersion, liberty)

	cl := fakeclient.NewFakeClient(objs...)
	rcl := fakeclient.NewFakeClient(objs...)

	rb := oputils.NewReconcilerBase(rcl, cl, s, &rest.Config{}, record.NewFakeRecorder(10))

	terminationPolicy := v1.TLSTerminationReencrypt
	expose := true
	spec.Env = []corev1.EnvVar{
		{Name: "SEC_TLS_TRUSTDEFAULTCERTS", Value: "true"},
		{Name: "SEC_IMPORT_K8S_CERTS", Value: "true"},
	}
	spec.Expose = &expose
	spec.Route = &openlibertyv1beta2.OpenLibertyApplicationRoute{
		Host:        "myapp.mycompany.com",
		Termination: &terminationPolicy,
	}
	spec.SSO = &openlibertyv1beta2.OpenLibertyApplicationSSO{
		RedirectToRPHostAndPort: "redirectvalue",
		MapToUserRegistry:       &expose,
		Github:                  &openlibertyv1beta2.GithubLogin{Hostname: "github.com"},
		OIDC: []openlibertyv1beta2.OidcClient{
			{
				DiscoveryEndpoint:           "myapp.mycompany.com",
				ID:                          "custom3",
				GroupNameAttribute:          "specify-required-value1",
				UserNameAttribute:           "specify-required-value2",
				DisplayName:                 "specify-required-value3",
				UserInfoEndpointEnabled:     &expose,
				RealmNameAttribute:          "specify-required-value4",
				Scope:                       "specify-required-value5",
				TokenEndpointAuthMethod:     "specify-required-value6",
				HostNameVerificationEnabled: &expose,
			},
		},
		Oauth2: []openlibertyv1beta2.OAuth2Client{
			{
				ID:                    "custom1",
				AuthorizationEndpoint: "specify-required-value",
				TokenEndpoint:         "specify-required-value",
			},
			{
				ID:                      "custom2",
				AuthorizationEndpoint:   "specify-required-value1",
				TokenEndpoint:           "specify-required-value2",
				GroupNameAttribute:      "specify-required-value3",
				UserNameAttribute:       "specify-required-value4",
				DisplayName:             "specify-required-value5",
				RealmNameAttribute:      "specify-required-value6",
				RealmName:               "specify-required-value7",
				Scope:                   "specify-required-value8",
				TokenEndpointAuthMethod: "specify-required-value9",
				AccessTokenHeaderName:   "specify-required-value10",
				AccessTokenRequired:     &expose,
				AccessTokenSupported:    &expose,
				UserApiType:             "specify-required-value11",
				UserApi:                 "specify-required-value12",
			},
		},
	}
	data := map[string][]byte{
		"github-clientId":     []byte("bW9vb29vb28="),
		"github-clientSecret": []byte("dGhlbGF1Z2hpbmdjb3c="),
		"oidc-clientId":       []byte("bW9vb29vb28="),
		"oidc-clientSecret":   []byte("dGhlbGF1Z2hpbmdjb3c="),
	}
	pts := &corev1.PodTemplateSpec{}
	ssoSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-olapp-sso",
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	err := rb.GetClient().Create(context.TODO(), ssoSecret)
	if err != nil {
		t.Fatalf(err.Error())
	}
	openliberty := createOpenLibertyApp(name, namespace, spec)
	oputils.CustomizePodSpec(pts, openliberty)
	CustomizeEnvSSO(pts, openliberty, rb.GetClient(), false)

	podEnv := envSliceToMap(pts.Spec.Containers[0].Env, data, t)
	tests := []Test{
		{"Github clientid set", string(data["github-clientId"]), podEnv["SEC_SSO_GITHUB_CLIENTID"]},
		{"Github clientSecret set", string(data["github-clientSecret"]), podEnv["SEC_SSO_GITHUB_CLIENTSECRET"]},
		{"OIDC clientId set", string(data["oidc-clientId"]), podEnv["SEC_SSO_OIDC_CLIENTID"]},
		{"OIDC clientSecret set", string(data["oidc-clientSecret"]), podEnv["SEC_SSO_OIDC_CLIENTSECRET"]},
		{"redirect to rp host and port", "redirectvalue", podEnv["SEC_SSO_REDIRECTTORPHOSTANDPORT"]},
		{"map to user registry", "true", podEnv["SEC_SSO_MAPTOUSERREGISTRY"]},
		{"Github hostname set", "github.com", podEnv["SEC_SSO_GITHUB_HOSTNAME"]},
		{"oidc-custom3 discovery endpoint", "myapp.mycompany.com", podEnv["SEC_SSO_CUSTOM3_DISCOVERYENDPOINT"]},
		{"oidc-custom3 group name attribute", "specify-required-value1", podEnv["SEC_SSO_CUSTOM3_GROUPNAMEATTRIBUTE"]},
		{"oidc-custom3 user name attribute", "specify-required-value2", podEnv["SEC_SSO_CUSTOM3_USERNAMEATTRIBUTE"]},
		{"oidc-custom3 display name", "specify-required-value3", podEnv["SEC_SSO_CUSTOM3_DISPLAYNAME"]},
		{"oidc-custom3 user info endpoint enabled", "true", podEnv["SEC_SSO_CUSTOM3_USERINFOENDPOINTENABLED"]},
		{"oidc-custom3 realm name attribute", "specify-required-value4", podEnv["SEC_SSO_CUSTOM3_REALMNAMEATTRIBUTE"]},
		{"oidc-custom3 scope", "specify-required-value5", podEnv["SEC_SSO_CUSTOM3_SCOPE"]},
		{"oidc-custom3 token endpoint auth method", "specify-required-value6", podEnv["SEC_SSO_CUSTOM3_TOKENENDPOINTAUTHMETHOD"]},
		{"oidc-custom3 host name verification enabled", "true", podEnv["SEC_SSO_CUSTOM3_HOSTNAMEVERIFICATIONENABLED"]},
		{"oauth2-custom1 authorization endpoint", "specify-required-value", podEnv["SEC_SSO_CUSTOM1_AUTHORIZATIONENDPOINT"]},
		{"oauth2-custom1 token endpoint", "specify-required-value", podEnv["SEC_SSO_CUSTOM1_TOKENENDPOINT"]},
		{"oauth2-custom2 authorization endpoint", "specify-required-value1", podEnv["SEC_SSO_CUSTOM2_AUTHORIZATIONENDPOINT"]},
		{"oauth2-custom2 token endpoint", "specify-required-value2", podEnv["SEC_SSO_CUSTOM2_TOKENENDPOINT"]},
		{"oauth2-custom2 group name attribute", "specify-required-value3", podEnv["SEC_SSO_CUSTOM2_GROUPNAMEATTRIBUTE"]},
		{"oauth2-custom2 user name attribute", "specify-required-value4", podEnv["SEC_SSO_CUSTOM2_USERNAMEATTRIBUTE"]},
		{"oauth2-custom2 display name", "specify-required-value5", podEnv["SEC_SSO_CUSTOM2_DISPLAYNAME"]},
		{"oauth2-custom2 realm name attribute", "specify-required-value6", podEnv["SEC_SSO_CUSTOM2_REALMNAMEATTRIBUTE"]},
		{"oauth2-custom2 realm name", "specify-required-value7", podEnv["SEC_SSO_CUSTOM2_REALMNAME"]},
		{"oauth2-custom2 scope", "specify-required-value8", podEnv["SEC_SSO_CUSTOM2_SCOPE"]},
		{"oauth2-custom2 token endpoint auth method", "specify-required-value9", podEnv["SEC_SSO_CUSTOM2_TOKENENDPOINTAUTHMETHOD"]},
		{"oauth2-custom2 access token header name", "specify-required-value10", podEnv["SEC_SSO_CUSTOM2_ACCESSTOKENHEADERNAME"]},
		{"oauth2-custom2 access token required", "true", podEnv["SEC_SSO_CUSTOM2_ACCESSTOKENREQUIRED"]},
		{"oauth2-custom2 access token supported", "true", podEnv["SEC_SSO_CUSTOM2_ACCESSTOKENSUPPORTED"]},
		{"oauth2-custom2 user api type", "specify-required-value11", podEnv["SEC_SSO_CUSTOM2_USERAPITYPE"]},
		{"oauth2-custom2 user api", "specify-required-value12", podEnv["SEC_SSO_CUSTOM2_USERAPI"]},
	}

	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// Helper Functions
func envSliceToMap(env []corev1.EnvVar, data map[string][]byte, t *testing.T) map[string]string {
	out := map[string]string{}
	for _, el := range env {
		if el.ValueFrom != nil {
			val := data[el.ValueFrom.SecretKeyRef.Key]
			out[el.Name] = "" + string(val)
		} else {
			out[el.Name] = string(el.Value)
		}
	}
	return out
}
func createOpenLibertyApp(n, ns string, spec openlibertyv1beta2.OpenLibertyApplicationSpec) *openlibertyv1beta2.OpenLibertyApplication {
	app := &openlibertyv1beta2.OpenLibertyApplication{
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
			GroupVersion: servingv1.SchemeGroupVersion.String(),
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
