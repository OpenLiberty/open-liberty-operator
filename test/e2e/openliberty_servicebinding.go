package e2e

import (
	goctx "context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	"github.com/OpenLiberty/open-liberty-operator/test/util"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	e2eutil "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	openlibertyProvider    = "openliberty-provider"
	openlibertyConsumer    = "openliberty-consumer"
	openlibertyConsumer2   = "openliberty-consumer2"
	openlibertyConsumerEnv = "openliberty-consumer-env"
	openlibertySecret      = "my-secret"
	openlibertySecret2     = "my-secret2"
	context                = "my-context"
	port                   = "9443"
	mount                  = "sample"
	usernameValue          = "admin"
	passwordValue          = "adminpass"
	usernameValue2         = "admin2"
	passwordValue2         = "adminpass2"
	context2               = "my-context2"
)

// OpenLibertyServiceBindingTest verify behaviour of service binding feature
func OpenLibertyServiceBindingTest(t *testing.T) {
	ctx, err := util.InitializeContext(t, cleanupTimeout, retryInterval)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Cleanup()

	ns, err := ctx.GetNamespace()
	if err != nil {
		t.Fatalf("Couldn't get namespace: %v", err)
	}

	t.Logf("Namespace: %s", ns)

	f := framework.Global

	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, ns, "open-liberty-operator", 1, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, ns, err)
	}

	// run basic tests for same namespace
	// checks the files are mounted in the correct directories
	// whether namespace is set under the consumes field
	setUpMounting(t, f, ctx, ns)
	err = mountingTest(t, f, ctx, ns, usernameValue, passwordValue, context)
	if err != nil {
		t.Fatal(err)
	}

	// run tests when the mountpath is not set
	// checks the correvt env vars are set
	err = envTest(t, f, ctx, ns)
	if err != nil {
		t.Fatal(err)
	}

	// run tests for changing provides
	err = updateProviderTest(t, f, ctx, ns)
	if err != nil {
		t.Fatal(err)
	}

}

func createSecret(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, ns string, n string, userValue string, passValue string) error {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      n,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"username": []byte(userValue),
			"password": []byte(passValue),
		},
		Type: corev1.SecretTypeOpaque,
	}

	err := f.Client.Create(goctx.TODO(), &secret, &framework.CleanupOptions{TestContext: ctx, Timeout: timeout, RetryInterval: retryInterval})
	if err != nil {
		return err
	}

	return nil
}

func createProviderService(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, ns string, con string) error {
	openliberty := util.MakeBasicOpenLibertyApplication(t, f, openlibertyProvider, ns, 1)
	svctype := v1.ServiceTypeClusterIP
	openliberty.Spec.Service = &v1beta1.OpenLibertyApplicationService{Type: &svctype, Port: 9443}
	openliberty.Spec.Service.Provides = &v1beta1.ServiceBindingProvides{
		Category: "openapi",
		Context:  "/" + con,
		Auth: &v1beta1.ServiceBindingAuth{
			Username: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: openlibertySecret},
				Key:                  "username",
			},
			Password: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: openlibertySecret},
				Key:                  "password",
			},
		},
	}

	err := f.Client.Create(goctx.TODO(), openliberty, &framework.CleanupOptions{TestContext: ctx, Timeout: timeout, RetryInterval: retryInterval})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, openlibertyProvider, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	return nil
}

func createConsumeServiceMount(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, ns string, n string, appName string, set bool) error {
	openliberty := util.MakeBasicOpenLibertyApplication(t, f, appName, ns, 1)
	svctype := v1.ServiceTypeClusterIP
	openliberty.Spec.Service = &v1beta1.OpenLibertyApplicationService{Type: &svctype, Port: 9443}
	if set == true {
		openliberty.Spec.Service.Consumes = []v1beta1.ServiceBindingConsumes{
			v1beta1.ServiceBindingConsumes{
				Name:      n,
				Namespace: ns,
				Category:  "openapi",
				MountPath: "/" + mount,
			},
		}
	} else if set == false {
		openliberty.Spec.Service.Consumes = []v1beta1.ServiceBindingConsumes{
			v1beta1.ServiceBindingConsumes{
				Name:      n,
				Category:  "openapi",
				MountPath: "/" + mount,
			},
		}
	}

	err := f.Client.Create(goctx.TODO(), openliberty, &framework.CleanupOptions{TestContext: ctx, Timeout: timeout, RetryInterval: retryInterval})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, appName, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	return nil
}

func setUpMounting(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, ns string) error {
	err := createSecret(t, f, ctx, ns, openlibertySecret, usernameValue, passwordValue)
	if err != nil {
		util.FailureCleanup(t, f, ns, err)
	}

	err = createProviderService(t, f, ctx, ns, context)
	if err != nil {
		util.FailureCleanup(t, f, ns, err)
	}

	// Create service with namespace under consumes
	err = createConsumeServiceMount(t, f, ctx, ns, openlibertyProvider, openlibertyConsumer, true)
	if err != nil {
		util.FailureCleanup(t, f, ns, err)
	}

	// Create service without namespace under consumes
	err = createConsumeServiceMount(t, f, ctx, ns, openlibertyProvider, openlibertyConsumer2, false)
	if err != nil {
		util.FailureCleanup(t, f, ns, err)
	}

	return nil
}

func mountingTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, ns string, userValue string, passValue string, con string) error {

	// Get consumer pod
	pods, err := util.GetPods(f, ctx, openlibertyConsumer, ns)
	if err != nil {
		util.FailureCleanup(t, f, ns, err)
	}
	podName := pods.Items[0].GetName()

	// Go inside the pod the pod for Consume service and check values are set
	out, err := exec.Command("kubectl", "exec", "-n", ns, "-it", podName, "--", "ls", "../"+mount+"/"+ns+"/"+openlibertyProvider).Output()
	err = util.CommandError(t, err, out)
	if err != nil {
		t.Fatal("Directory not made")
	}
	directories := strings.Split(string(out), "\n")
	t.Log(directories)

	// Set values to check
	valuePairs := map[string]string{
		"context":  con,
		"hostname": openlibertyProvider + "." + ns + ".svc.cluster.local",
		"password": passValue,
		"port":     port,
		"protocol": "http",
		"url":      "http://" + openlibertyProvider + "." + ns + ".svc.cluster.local:" + port + "/" + con,
		"username": userValue,
	}

	for i := 0; i < len(directories)-1; i++ {
		checkSecret(t, f, ns, podName, directories[i], valuePairs, true)
	}

	// Get consumer pod
	pods, err = util.GetPods(f, ctx, openlibertyConsumer2, ns)
	if err != nil {
		util.FailureCleanup(t, f, ns, err)
	}
	podName = pods.Items[0].GetName()

	// Go inside the pod the pod for Consume service and check values are set
	out, err = exec.Command("kubectl", "exec", "-n", ns, "-it", podName, "--", "ls", "../"+mount+"/"+openlibertyProvider).Output()
	err = util.CommandError(t, err, out)
	if err != nil {
		t.Fatal("Directory not made")

	}
	directories = strings.Split(string(out), "\n")

	for i := 0; i < len(directories)-1; i++ {
		checkSecret(t, f, ns, podName, directories[i], valuePairs, false)
	}

	return nil
}

func checkSecret(t *testing.T, f *framework.Framework, ns string, podName string, directory string, valuePairs map[string]string, setNamespace bool) {
	out, err := []byte(""), errors.New("")

	for i := 0; i < 20; i++ {
		if setNamespace == true {
			out, err = exec.Command("kubectl", "exec", "-n", ns, "-it", podName, "--", "cat", "../"+mount+"/"+ns+"/"+openlibertyProvider+"/"+directory).Output()
		} else if setNamespace == false {
			out, err = exec.Command("kubectl", "exec", "-n", ns, "-it", podName, "--", "cat", "../"+mount+"/"+openlibertyProvider+"/"+directory).Output()
		}
		err = util.CommandError(t, err, out)
		if err != nil {
			t.Log(directory + " is not set")
		}

		if valuePairs[directory] != string(out) {
			t.Logf("The value is not set correctly. Expected: %s. Actual: %s", valuePairs[directory], string(out))
		} else {
			t.Logf("The value is set correctly. %s", string(out))
			return
		}
		// Wait for updates
		time.Sleep(5000 * time.Millisecond)
	}
	t.Fatal("The values were not set correctly.")
}

func createConsumeServiceEnv(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, ns string, n string, appName string) error {
	openliberty := util.MakeBasicOpenLibertyApplication(t, f, appName, ns, 1)
	svctype := v1.ServiceTypeClusterIP
	openliberty.Spec.Service = &v1beta1.OpenLibertyApplicationService{Type: &svctype, Port: 9443}
	openliberty.Spec.Service.Consumes = []v1beta1.ServiceBindingConsumes{
		v1beta1.ServiceBindingConsumes{
			Name:      n,
			Namespace: ns,
			Category:  "openapi",
		},
	}

	err := f.Client.Create(goctx.TODO(), openliberty, &framework.CleanupOptions{TestContext: ctx, Timeout: timeout, RetryInterval: retryInterval})
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, appName, 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	return nil
}

func envTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, ns string) error {

	// Create service with namespace under consumes
	err := createConsumeServiceEnv(t, f, ctx, ns, openlibertyProvider, openlibertyConsumerEnv)
	if err != nil {
		util.FailureCleanup(t, f, ns, err)
	}

	// Get consumer pod
	pods, err := util.GetPods(f, ctx, openlibertyConsumerEnv, ns)
	if err != nil {
		util.FailureCleanup(t, f, ns, err)
	}
	podEnv := pods.Items[0].Spec.Containers[0].Env

	// Check the values are set correctly
	err = searchValues(t, ns, podEnv)
	if err != nil {
		util.FailureCleanup(t, f, ns, err)
	}

	return nil
}

func searchValues(t *testing.T, ns string, podEnv []corev1.EnvVar) error {
	nsUpper := strings.ToUpper(ns)
	providerUpper := strings.ToUpper(strings.ReplaceAll(openlibertyProvider, "-", "_"))
	values := [7]string{"username", "password", "context", "hostname", "port", "protocol", "url"}

	for i := 0; i < len(podEnv); i++ {
		for j := 0; j < len(values); j++ {
			if podEnv[i].Name == nsUpper+"_"+providerUpper+"_"+strings.ToUpper(values[j]) {
				if podEnv[i].ValueFrom.SecretKeyRef.Key == values[j] {
					t.Log(podEnv[i].Name, podEnv[i].ValueFrom.SecretKeyRef.Key)
				} else {
					t.Fatalf("Expected: %s. Actual: %s", values[j], podEnv[i].ValueFrom.SecretKeyRef.Key)
					return errors.New("wrong key set in the env var")
				}
			}
		}
	}
	return nil
}

func updateProviderTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, ns string) error {
	err := createSecret(t, f, ctx, ns, openlibertySecret2, usernameValue2, passwordValue2)
	if err != nil {
		util.FailureCleanup(t, f, ns, err)
	}

	// Update provider application
	target := types.NamespacedName{Name: openlibertyProvider, Namespace: ns}
	err = util.UpdateApplication(f, target, func(r *openlibertyv1beta1.OpenLibertyApplication) {
		r.Spec.Service.Provides = &v1beta1.ServiceBindingProvides{
			Category: "openapi",
			Context:  "/" + context2,
			Auth: &v1beta1.ServiceBindingAuth{
				Username: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: openlibertySecret2},
					Key:                  "username",
				},
				Password: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: openlibertySecret2},
					Key:                  "password",
				},
			},
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	err = mountingTest(t, f, ctx, ns, usernameValue2, passwordValue2, context2)
	if err != nil {
		t.Fatal(err)
	}

	return nil
}
