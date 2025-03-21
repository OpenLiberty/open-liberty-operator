package utils

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	openlibertyv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
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
	svc := &openlibertyv1.OpenLibertyApplicationService{Port: 8080, Type: &clusterType}
	spec := openlibertyv1.OpenLibertyApplicationSpec{Service: svc}
	pts := &corev1.PodTemplateSpec{}

	// Always call CustomizePodSpec to populate Containers & simulate real behaviour
	openliberty := createOpenLibertyApp(name, namespace, spec)
	objs, s := []runtime.Object{openliberty}, scheme.Scheme
	s.AddKnownTypes(openlibertyv1.GroupVersion, openliberty)

	cl := fakeclient.NewFakeClient(objs...)
	rcl := fakeclient.NewFakeClient(objs...)

	rb := oputils.NewReconcilerBase(rcl, cl, s, &rest.Config{}, record.NewFakeRecorder(10))

	oputils.CustomizePodSpec(pts, openliberty)
	CustomizeLibertyEnv(pts, openliberty, rb.GetClient())

	targetEnv := []corev1.EnvVar{
		{Name: "TLS_DIR", Value: "/etc/x509/certs"},
		{Name: "WLP_LOGGING_CONSOLE_LOGLEVEL", Value: "info"},
		{Name: "WLP_LOGGING_CONSOLE_SOURCE", Value: "message,accessLog,ffdc,audit"},
		{Name: "WLP_LOGGING_CONSOLE_FORMAT", Value: "json"},
		{Name: "SEC_IMPORT_K8S_CERTS", Value: "true"},
	}

	testEnv := []Test{
		{"Test environment defaults", targetEnv, pts.Spec.Containers[0].Env},
	}

	if err := verifyTests(testEnv); err != nil {
		t.Fatalf("%v", err)
	}

	// test with env variables set by user
	setupEnv := []corev1.EnvVar{
		{Name: "WLP_LOGGING_CONSOLE_LOGLEVEL", Value: "error"},
		{Name: "WLP_LOGGING_CONSOLE_SOURCE", Value: "trace,accessLog,ffdc"},
		{Name: "WLP_LOGGING_CONSOLE_FORMAT", Value: "basic"},
		{Name: "SEC_IMPORT_K8S_CERTS", Value: "true"},
	}

	spec = openlibertyv1.OpenLibertyApplicationSpec{
		Env:     setupEnv,
		Service: svc,
	}
	pts = &corev1.PodTemplateSpec{}

	openliberty = createOpenLibertyApp(name, namespace, spec)
	oputils.CustomizePodSpec(pts, openliberty)

	CustomizeLibertyEnv(pts, openliberty, rb.GetClient())

	targetEnv = append(
		setupEnv, corev1.EnvVar{Name: "TLS_DIR", Value: "/etc/x509/certs"},
	)

	testEnv = []Test{
		{"Test environment config", targetEnv, pts.Spec.Containers[0].Env},
	}
	if err := verifyTests(testEnv); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCustomizeEnvSSO(t *testing.T) {
	logger := zap.New()
	logf.SetLogger(logger)
	os.Setenv("WATCH_NAMESPACE", namespace)
	svc := &openlibertyv1.OpenLibertyApplicationService{Port: 8080, Type: &clusterType}
	spec := openlibertyv1.OpenLibertyApplicationSpec{Service: svc}
	liberty := createOpenLibertyApp(name, namespace, spec)
	objs, s := []runtime.Object{liberty}, scheme.Scheme
	s.AddKnownTypes(openlibertyv1.GroupVersion, liberty)

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
	spec.Route = &openlibertyv1.OpenLibertyApplicationRoute{
		Host:        "myapp.mycompany.com",
		Termination: &terminationPolicy,
	}
	spec.SSO = &openlibertyv1.OpenLibertyApplicationSSO{
		RedirectToRPHostAndPort: "redirectvalue",
		MapToUserRegistry:       &expose,
		Github:                  &openlibertyv1.GithubLogin{Hostname: "github.com"},
		OIDC: []openlibertyv1.OidcClient{
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
		Oauth2: []openlibertyv1.OAuth2Client{
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
		t.Fatalf("%s", err.Error())
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

func createOpenLibertyApp(n, ns string, spec openlibertyv1.OpenLibertyApplicationSpec) *openlibertyv1.OpenLibertyApplication {
	app := &openlibertyv1.OpenLibertyApplication{
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

func TestCompareOperandVersion(t *testing.T) {
	tests := []Test{
		{"same version", CompareOperandVersion("v0_0_0", "v0_0_0"), 0},
		{"same version, multiple digits", CompareOperandVersion("v10_10_10", "v10_10_10"), 0},
		{"same version, build tags", CompareOperandVersion("v2_0_0alpha", "v2_0_0alpha"), 0},
		{"different patch version, build tags", CompareOperandVersion("v2_0_10alpha", "v2_0_2alpha"), 8},
		{"different patch version, build tags, reversed", CompareOperandVersion("v2_0_2alpha", "v2_0_10alpha"), -8},
		{"different patch version", CompareOperandVersion("v1_0_0", "v1_0_1"), -1},
		{"different minor version", CompareOperandVersion("v1_0_0", "v1_1_0"), -1},
		{"different major version", CompareOperandVersion("v2_0_0", "v1_0_0"), 1},
		{"minor less than patch", CompareOperandVersion("v1_10_0", "v1_0_5"), 10},
		{"major less than patch", CompareOperandVersion("v2_0_0", "v1_0_10"), 1},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGetCommaSeparatedString(t *testing.T) {
	emptyString, emptyStringErr := GetCommaSeparatedString("", 0)
	oneElement, oneElementErr := GetCommaSeparatedString("one", 0)
	oneElementAOOB, oneElementAOOBErr := GetCommaSeparatedString("one", 1)
	multiElement, multiElementErr := GetCommaSeparatedString("one,two,three,four,five", 3)
	multiElementAOOB, multiElementAOOBErr := GetCommaSeparatedString("one,two,three,four,five", 5)
	tests := []Test{
		{"empty string", "", emptyString},
		{"empty string errors", fmt.Errorf("there is no element"), emptyStringErr},
		{"one element", "one", oneElement},
		{"one element errors", nil, oneElementErr},
		{"one element array out of bounds", "", oneElementAOOB},
		{"one element array out of bounds error", fmt.Errorf("cannot index string list with only one element"), oneElementAOOBErr},
		{"multi element", "four", multiElement},
		{"multi element error", nil, multiElementErr},
		{"multi element array out of bounds", "", multiElementAOOB},
		{"multi element array out of bounds error", fmt.Errorf("element not found"), multiElementAOOBErr},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCommaSeparatedStringContains(t *testing.T) {
	match := CommaSeparatedStringContains("one,two,three,four", "three")
	substringNonMatch := CommaSeparatedStringContains("one,two,three,four", "thre")
	substringNonMatch2 := CommaSeparatedStringContains("one,two,three,four", "threee")
	oneElementMatch := CommaSeparatedStringContains("one", "one")
	noElementNonMatch := CommaSeparatedStringContains("", "one")
	tests := []Test{
		{"single match", 2, match},
		{"substring should not match", -1, substringNonMatch},
		{"substring 2 should not match", -1, substringNonMatch2},
		{"one element match", 0, oneElementMatch},
		{"no element non match", -1, noElementNonMatch},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func Test_kebabToCamelCase(t *testing.T) {
	tests := []Test{
		{"single replace 1", "passwordEncryption", kebabToCamelCase("password-encryption")},
		{"single replace 1, corner case 1", "passwordEncryption", kebabToCamelCase("-password-encryption")},
		{"single replace 1, corner case 2", "passwordEncryption", kebabToCamelCase("-password-encryption-")},
		{"single replace 1, corner case 3", "passWordEncryption", kebabToCamelCase("---pass-word-encryption")},
		{"double replace 1", "passwordEncryptionKey", kebabToCamelCase("password-encryption-key")},
		{"double replace 1", "passwordEncryptionKey", kebabToCamelCase("-password-encryption-key")},
		{"single replace 2", "ltpa", kebabToCamelCase("ltpa")},
		{"single replace 2, corner case 1", "ltpa1", kebabToCamelCase("ltpa-1")},
		{"single replace 2, corner case 2", "ltpa.", kebabToCamelCase("ltpa-.")},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}
