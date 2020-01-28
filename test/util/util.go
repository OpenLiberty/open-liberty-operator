package util

import (
	goctx "context"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
	"os/exec"
)

// MakeBasicOpenLibertyApplication : Create a simple Liberty App with provided number of replicas.
func MakeBasicOpenLibertyApplication(t *testing.T, f *framework.Framework, n string, ns string, replicas int32) *openlibertyv1beta1.OpenLibertyApplication {
	probe := corev1.Handler{
		HTTPGet: &corev1.HTTPGetAction{
			Path: "/",
			Port: intstr.FromInt(9080),
		},
	}
	expose := false
	return &openlibertyv1beta1.OpenLibertyApplication{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OpenLibertyApplication",
			APIVersion: "openliberty.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      n,
			Namespace: ns,
		},
		Spec: openlibertyv1beta1.OpenLibertyApplicationSpec{
			ApplicationImage: "openliberty/open-liberty:full-java8-openj9-ubi",
			Replicas:         &replicas,
			Expose:           &expose,
			ReadinessProbe: &corev1.Probe{
				Handler:             probe,
				InitialDelaySeconds: 1,
				TimeoutSeconds:      1,
				PeriodSeconds:       5,
				SuccessThreshold:    1,
				FailureThreshold:    24,
			},
			LivenessProbe: &corev1.Probe{
				Handler:             probe,
				InitialDelaySeconds: 8,
				TimeoutSeconds:      1,
				PeriodSeconds:       5,
				SuccessThreshold:    1,
				FailureThreshold:    12,
			},
		},
	}
}

func MakeBasicOpenLibertyTrace(n, ns, pod string) *openlibertyv1beta1.OpenLibertyTrace {
	maxFiles := int32(5)
	maxFileSize := int32(20)
	return &openlibertyv1beta1.OpenLibertyTrace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OpenLibertyTrace",
			APIVersion: "openliberty.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      n,
			Namespace: ns,
		},
		Spec: openlibertyv1beta1.OpenLibertyTraceSpec{
			PodName:            pod,
			TraceSpecification: "*=info:com.ibm.was.webcontainer*=all",
			MaxFiles:           &maxFiles,
			MaxFileSize:        &maxFileSize,
		},
	}
}

// WaitForStatefulSet : Identical to WaitForDeployment but for StatefulSets.
func WaitForStatefulSet(t *testing.T, kc kubernetes.Interface, ns, n string, replicas int, retryInterval, timeout time.Duration) error {
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		statefulset, err := kc.AppsV1().StatefulSets(ns).Get(n, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for availability of %s statefulset\n", n)
				return false, nil
			}
			return false, err
		}

		if int(statefulset.Status.ReadyReplicas) == replicas {
			return true, nil
		}
		t.Logf("Waiting for full availability of %s statefulset (%d/%d)\n", n, statefulset.Status.CurrentReplicas, replicas)
		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("StatefulSet available (%d/%d)\n", replicas, replicas)
	return nil
}

// InitializeContext : Sets up initial context
func InitializeContext(t *testing.T, clean, retryInterval time.Duration) (*framework.TestCtx, error) {
	ctx := framework.NewTestCtx(t)
	err := ctx.InitializeClusterResources(&framework.CleanupOptions{
		TestContext:   ctx,
		Timeout:       clean,
		RetryInterval: retryInterval,
	})
	if err != nil {
		if ctx != nil {
			ctx.Cleanup()
		}
		return nil, err
	}

	t.Log("Cluster context initialized.")
	return ctx, nil
}

// ResetConfigMap : Resets the configmaps to original empty values, this is required to allow tests to be run after the configmaps test
func ResetConfigMap(t *testing.T, f *framework.Framework, configMap *corev1.ConfigMap, cmName string, fileName string, namespace string) {
	err := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: cmName, Namespace: namespace}, configMap)
	if err != nil {
		t.Fatal(err)
	}
	fData, err := ioutil.ReadFile(fileName)
	if err != nil {
		t.Fatal(err)
	}

	configMap = &corev1.ConfigMap{}
	err = yaml.Unmarshal(fData, configMap)
	if err != nil {
		t.Fatal(err)
	}
	configMap.Namespace = namespace
	err = f.Client.Update(goctx.TODO(), configMap)
	if err != nil {
		t.Fatal(err)
	}
}

// FailureCleanup : Log current state of the namespace and exit with fatal
func FailureCleanup(t *testing.T, f *framework.Framework, ns string, failure error) {
	t.Log("***** FAILURE")
	t.Logf("*** ERROR: %s", failure.Error())
	options := &dynclient.ListOptions{
		Namespace: ns,
	}
	podlist := &corev1.PodList{}
	err := f.Client.List(goctx.TODO(), podlist, options)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("***** Logging pods in namespace: %s", ns)
	for _, p := range podlist.Items {
		t.Logf("Pod: %s", p.GetName())
		t.Log("--------------------------------------------------------------")
		t.Log(p)
	}

	crlist := &openlibertyv1beta1.OpenLibertyApplicationList{}
	err = f.Client.List(goctx.TODO(), crlist, options)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("***** Logging Open Liberty Applications in namespace: %s", ns)
	for _, application := range crlist.Items {
		t.Log("-------------------------------------------------------------")
		t.Log(application)
	}

	t.Log("****** See above for namespace logs")
	t.Fatal("---------------------------------------------------------------")
}

// WaitForKnativeDeployment : Poll for ksvc creation when createKnativeService is set to true
func WaitForKnativeDeployment(t *testing.T, f *framework.Framework, ns, n string, retryInterval, timeout time.Duration) error {
	// add to scheme to framework can find the resource
	err := servingv1alpha1.AddToScheme(f.Scheme)
	if err != nil {
		return err
	}

	err = wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		ksvc := &servingv1alpha1.ServiceList{}
		lerr := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: n, Namespace: ns}, ksvc)
		if lerr != nil {
			if apierrors.IsNotFound(lerr) {
				t.Logf("Waiting for knative service %s...", n)
				return false, nil
			}
			// issue retrieving ksvc
			return false, lerr
		}

		t.Logf("Found knative service %s", n)
		return true, nil
	})
	return err
}

// WaitForStatusConditions waits for dump/trace to be created and for non error conditions to appear
func WaitForStatusConditions(t *testing.T, f *framework.Framework, n, ns string, retryInterval, timeout time.Duration) error {
	oltrace := &openlibertyv1beta1.OpenLibertyTrace{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {

		err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: n, Namespace: ns}, oltrace)
		if err != nil {
			// Not found, keep polling
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for trace %s...", n)
				return false, nil
			}
			// Unexpected Error, exit
			return true, err
		}

		ok, err := checkTraceStatus(f, ns, oltrace)
		if err != nil {
			// Bad Conditions found, exit
			t.Log("****** Status Conditions:")
			t.Log(oltrace.Status.Conditions)
			return true, err
		} else if !ok {
			// No Conditions found, keep polling
			return false, nil
		}


		t.Log("****** Status Conditions:")
		t.Log(oltrace.Status.Conditions)
		// Good State, exit
		return true, nil
	})
	return err
}

// TraceIsEnabled check for add_trace.xml in the targetted pod of a OL trace
func TraceIsEnabled(t *testing.T, f *framework.Framework, podName, ns string) (bool, error) {
	const traceConfigFile = "/config/configDropins/overrides/add_trace.xml"

	out, err := exec.Command("kubectl", "exec", "-n", ns, "-it", podName, "--", "ls", traceConfigFile).Output()
	if err != err {
		if exiterr, ok := err.(*exec.ExitError); ok {
			t.Log("failed to execute ls command, see below")
			t.Log(exiterr.Error())
			return false, nil
		}
		t.Log("unknown error occurred, see below")
		t.Log(err.Error())
		return false, err
	}

	if len(out) == 0 {
		t.Log("no output returned")
		return false, nil
	}

	t.Log("add_trace.xml found!")
	return true, nil
}

// checkTraceStatus verifies there are no bad conditions in the trace post run
func checkTraceStatus(f *framework.Framework, ns string, trace *openlibertyv1beta1.OpenLibertyTrace) (bool, error) {
	tmp := &openlibertyv1beta1.OpenLibertyTrace{}
	err := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: trace.GetName(), Namespace: ns}, tmp)
	if err != nil {
		return false, err
	}

	// no conditions reported yet, not done
	if len(tmp.Status.Conditions) == 0 {
		return false, nil
	}

	// check for error conditions
	for _, c := range trace.Status.Conditions {
		if c.Status == corev1.ConditionFalse {
			return true, fmt.Errorf("Bad Condition: %s", c.Message)
		}
	}

	return true, nil
}
