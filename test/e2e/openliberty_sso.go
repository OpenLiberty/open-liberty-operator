package e2e

import (
	goctx "context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	"github.com/OpenLiberty/open-liberty-operator/test/util"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	e2eutil "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// OpenLibertySSOTest - e2e test for SSO feature
func OpenLibertySSOTest(t *testing.T) {
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

	// create one replica of the operator deployment in current namespace with provided name
	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "open-liberty-operator", 1, retryInterval, operatorTimeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = testSocialLogin(t, f, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = testProviderLogins(t, f, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

}

// Test simple social login and various runtime behaviour
func testSocialLogin(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	ns, err := ctx.GetNamespace()
	if err != nil {
		return err
	}

	const name string = "openliberty-sso"

	// create Secret for Github Login
	secretTarget := types.NamespacedName{Name: name + "-olapp-sso", Namespace: ns}
	a := "mysecretvalue"
	data := map[string]string{
		"github-clientId":     a,
		"github-clientSecret": a,
		"oidc-clientId":       a,
		"oidc-clientSecret":   a,
	}

	err = util.CreateSecretForSSO(f, ctx, secretTarget, data)
	if err != nil {
		return err
	}
	// set up certificates for route, see: cert-manager test in RC
	expose := true
	clusterIP := corev1.ServiceTypeClusterIP
	githubLogin := v1beta1.GithubLogin{Hostname: "github.com"}
	openliberty := util.MakeBasicOpenLibertyApplication(t, f, name, ns, 1)
	openliberty.Spec.ApplicationImage = "openliberty/open-liberty:full-java8-openj9-ubi"
	// TODO debug why these environment variables are causing it to crash loop
	openliberty.Spec.Env = []corev1.EnvVar{
		{Name: "SEC_TLS_TRUSTDEFAULTCERTS", Value: "true"},
		{Name: "SEC_IMPORTS_K8S_CERTS", Value: "true"},
	}
	openliberty.Spec.Service = &v1beta1.OpenLibertyApplicationService{
		Type:        &clusterIP,
		Port:        9080,
		Certificate: &v1beta1.Certificate{},
	}
	openliberty.Spec.Expose = &expose
	openliberty.Spec.SSO = &v1beta1.OpenLibertyApplicationSSO{
		Github: &githubLogin,
	}
	err = f.Client.Create(goctx.TODO(), openliberty, &framework.CleanupOptions{TestContext: ctx, RetryInterval: time.Second, Timeout: time.Second})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, name, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	t.Log("verifying environment variables of containers")

	target := types.NamespacedName{Name: name, Namespace: ns}
	if err = verifyEnvVariables(t, ctx, f, target, secretTarget); err != nil {
		return err
	}
	util.LogTestUpdates(t, "secret data initially applied successfully")

	/* update existing values and verify */
	secret := corev1.Secret{}
	err = f.Client.Get(goctx.TODO(), secretTarget, &secret)
	if err != nil {
		return err
	}

	secret.StringData = map[string]string{
		"github-clientId":     `mygithubclientid`,
		"github-clientSecret": `mygithubclientsecret`,
		"oidc-clientId":       a,
		"oidc-clientSecret":   a,
	}

	err = f.Client.Update(goctx.TODO(), &secret)
	if err != nil {
		return err
	}

	err = util.WaitForPodUpdates(t, f, ctx, target, 1)
	if err != nil {
		return err
	}

	if err = verifyEnvVariables(t, ctx, f, target, secretTarget); err != nil {
		return err
	}
	util.LogTestUpdates(t, "secret data updated in-place successfully")

	/* update with new values and verify */
	secret = corev1.Secret{}
	err = f.Client.Get(goctx.TODO(), secretTarget, &secret)
	if err != nil {
		return err
	}

	secret.StringData = map[string]string{
		"github-clientId":        `mygithubclientid`,
		"github-clientSecret":    `mygithubclientsecret`,
		"oidc-clientId":          a,
		"oidc-clientSecret":      a,
		"twitter-consumerKey":    `twitterconsumerkey`,
		"twitter-consumerSecret": `twitterconsumersecret`,
	}

	err = f.Client.Update(goctx.TODO(), &secret)
	if err != nil {
		return err
	}

	err = util.WaitForPodUpdates(t, f, ctx, target, 1)
	if err != nil {
		return err
	}

	if err = verifyEnvVariables(t, ctx, f, target, secretTarget); err != nil {
		return err
	}
	util.LogTestUpdates(t, "secret data updated with new values successfully")

	// turn off SSO and verify cleanup
	err = util.UpdateApplication(f, target, func(r *openlibertyv1beta1.OpenLibertyApplication) {
		ol := util.MakeBasicOpenLibertyApplication(t, f, target.Name, target.Namespace, 1)
		ol.Spec.ApplicationImage = "openliberty/open-liberty:full-java8-openj9-ubi"
		r.Spec = ol.Spec
	})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, name, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	// wait for old pod to finish deleting
	err = util.WaitForPodUpdates(t, f, ctx, target, 1)
	if err != nil {
		return err
	}

	removed, err := verifySSOEnvVarRemoved(ctx, f, target)
	if err != nil {
		return err
	} else if !removed {
		return errors.New("not all SSO env var were removed")
	}
	util.LogTestUpdates(t, "secret data removed on disabling SSO successfully")

	return nil
}

func testProviderLogins(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	// same as above but use oidc or oauth2 instead
	ns, err := ctx.GetNamespace()
	if err != nil {
		return err
	}

	const name string = "openliberty-sso-1"

	// create Secret for Github Login
	secretTarget := types.NamespacedName{Name: name + "-olapp-sso", Namespace: ns}
	data := map[string]string{
		"github-clientId":        `gclientid`,
		"github-clientSecret":    `gclientsecret`,
		"provider1-clientId":     `provider1clientid`,
		"provider1-clientSecret": `provider1clientsecret`,
		"provider2-clientId":     `provider2clientid`,
		"provider2-clientSecret": `provider2clientsecret`,
		"custom1-clientId":       `custom1clientid`,
		"custom1-clientSecret":   `custom1clientsecret`,
	}

	err = util.CreateSecretForSSO(f, ctx, secretTarget, data)
	if err != nil {
		return err
	}
	// set up certificates for route, see: cert-manager test in RC
	// terminationPolicy := v1.TLSTerminationReencrypt
	expose := true
	clusterIP := corev1.ServiceTypeClusterIP
	githubLogin := v1beta1.GithubLogin{Hostname: "github.com"}
	openliberty := util.MakeBasicOpenLibertyApplication(t, f, name, ns, 1)
	openliberty.Spec.ApplicationImage = "openliberty/open-liberty:full-java8-openj9-ubi"
	// TODO debug why these environment variables are causing it to crash loop
	openliberty.Spec.Env = []corev1.EnvVar{
		{Name: "SEC_TLS_TRUSTDEFAULTCERTS", Value: "true"},
		{Name: "SEC_IMPORTS_K8S_CERTS", Value: "true"},
	}
	openliberty.Spec.Service = &v1beta1.OpenLibertyApplicationService{
		Type:        &clusterIP,
		Port:        9080,
		Certificate: &v1beta1.Certificate{},
	}
	openliberty.Spec.Expose = &expose
	openliberty.Spec.SSO = &v1beta1.OpenLibertyApplicationSSO{
		Github: &githubLogin,
		OIDC: []v1beta1.OidcClient{
			{ID: "provider1", DiscoveryEndpoint: "specify-required-value"},
		},
		Oauth2: []v1beta1.OAuth2Client{
			{ID: "custom1", AuthorizationEndpoint: "specify-required-value", TokenEndpoint: "specify-value"},
		},
	}
	err = f.Client.Create(goctx.TODO(), openliberty, &framework.CleanupOptions{TestContext: ctx, RetryInterval: time.Second, Timeout: time.Second})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, name, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	t.Log("verifying environment variables of containers")
	target := types.NamespacedName{Name: name, Namespace: ns}
	if err = verifyEnvVariables(t, ctx, f, target, secretTarget); err != nil {
		return err
	}
	t.Log("env variables correctly set for individual providers")

	err = util.UpdateApplication(f, target, func(r *openlibertyv1beta1.OpenLibertyApplication) {
		r.Spec.SSO = &v1beta1.OpenLibertyApplicationSSO{
			Github: &githubLogin,
			OIDC: []v1beta1.OidcClient{
				{ID: "provider1", DiscoveryEndpoint: "specify-required-value"},
				{ID: "provider2", DiscoveryEndpoint: "specify-required-value"},
			},
			Oauth2: []v1beta1.OAuth2Client{
				{ID: "custom1", AuthorizationEndpoint: "specify-required-value", TokenEndpoint: "specify-value"},
				{ID: "custom2", AuthorizationEndpoint: "specify-required-value", TokenEndpoint: "specify-value"},
				{ID: "custom3", AuthorizationEndpoint: "specify-required-value", TokenEndpoint: "specify-value"},
			},
		}
	})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, name, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	// wait for old pod to finish deleting
	err = util.WaitForPodUpdates(t, f, ctx, target, 1)
	if err != nil {
		return err
	}

	err = verifyEnvVariables(t, ctx, f, target, secretTarget)
	if err != nil {
		return err
	}
	t.Log("env variables correctly updated for multiple providers")

	return nil
}

/* Helper functions */
// Verify that the secret data is applied as env var and that CR SSO fields are also applied
func verifyEnvVariables(t *testing.T, ctx *framework.TestCtx, f *framework.Framework, target types.NamespacedName, secretTarget types.NamespacedName) error {
	podList, err := util.GetPods(f, ctx, target.Name, target.Namespace)
	if err != nil {
		return err
	} else if len(podList.Items) != 1 {
		return errors.New("pod list length not consistent with replicas")
	}

	pod := podList.Items[0]

	secret := corev1.Secret{}
	err = f.Client.Get(goctx.TODO(), secretTarget, &secret)
	if err != nil {
		return err
	}

	env := pod.Spec.Containers[0].Env
	for key, val := range secret.Data {
		e := findEnvFromWithKey(key, env)
		if e == nil {
			return fmt.Errorf("could not find key %s in pod envfrom", key)
		}

		ok, err := util.CheckEnvVarValue(t, val, e, &pod)
		if err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("env var %s not set correctly", e.Name)
		}
	}

	ol := v1beta1.OpenLibertyApplication{}
	err = f.Client.Get(goctx.TODO(), target, &ol)
	if err != nil {
		return err
	}

	// additionally verify the SSO fields from CR definition itself
	if err = verifyConfiguredSSOFields(env, ol.Spec); err != nil {
		return err
	}

	return nil
}

// NOTE this is not a comprehensive check, is only to verify that the config was read
// the unit tests verify values more comprehensively
func verifyConfiguredSSOFields(env []corev1.EnvVar, spec v1beta1.OpenLibertyApplicationSpec) error {
	// verify Oauth2 and OIDC values are present
	for _, e := range spec.SSO.OIDC {
		envName := fmt.Sprintf("SEC_SSO_%s_DISCOVERYENDPOINT", strings.ToUpper(e.ID))
		if !findEnvFromWithName(envName, env) {
			return fmt.Errorf("failed to find oidc provider %s's env var", e.ID)
		}
	}
	for _, e := range spec.SSO.Oauth2 {
		envName := fmt.Sprintf("SEC_SSO_%s_AUTHORIZATIONENDPOINT", strings.ToUpper(e.ID))
		if !findEnvFromWithName(envName, env) {
			return fmt.Errorf("failed to find oauth2 provider %s's env var", e.ID)
		}
	}

	// verify Github/Twitter for e2e tests
	if spec.SSO.Github != nil && !findEnvFromWithName("SEC_SSO_GITHUB_HOSTNAME", env) {
		return errors.New("failed to find github hostname in env var")
	}
	return nil
}

// check that all SSO related env var in general are not present (i.e. SEC_SSO_*)
func verifySSOEnvVarRemoved(ctx *framework.TestCtx, f *framework.Framework, target types.NamespacedName) (bool, error) {
	podList, err := util.GetPods(f, ctx, target.Name, target.Namespace)
	if err != nil {
		return false, err
	} else if len(podList.Items) != 1 {
		return false, errors.New("pod list length not consistent with replicas")
	}

	pod := podList.Items[0]
	env := pod.Spec.Containers[0].Env
	for _, e := range env {
		if strings.Contains(e.Name, "SEC_SSO_") {
			return false, nil
		}
	}

	return true, nil
}

func findEnvFromWithKey(key string, env []corev1.EnvVar) *corev1.EnvVar {
	for _, e := range env {
		if e.ValueFrom == nil {
			continue
		} else if e.ValueFrom.SecretKeyRef.Key == key {
			return &e
		}
	}
	return nil
}

func findEnvFromWithName(name string, env []corev1.EnvVar) bool {
	for _, e := range env {
		if e.ValueFrom == nil && e.Name == name {
			return true
		}
	}
	return false
}

func secretKeyToEnvName(key string) string {
	provider := strings.ToUpper(key[:strings.IndexByte(key, '-')])
	field := strings.ToUpper(key[strings.IndexByte(key, '-')+1:])
	return fmt.Sprintf("SEC_SSO_%s_%s", provider, field)
}
