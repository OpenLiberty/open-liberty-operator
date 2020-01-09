package e2e

import (
	goctx "context"
	"sync"
	"fmt"
	"testing"
	"time"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	"github.com/OpenLiberty/open-liberty-operator/test/util"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	e2eutil "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	targetApp = "open-liberty-target"
)

func OpenLibertyTraceTest(t *testing.T) {
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

	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "open-liberty-operator", 1, retryInterval, operatorTimeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = openLibertyBasicTraceTest(t, f, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
}

func openLibertyBasicTraceTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace: %v", err)
	}

	// Set up OL app to get trace from
	if err = createTargetApp(t, f, ctx); err != nil {
		return err
	}

	var wg sync.WaitGroup

	pods, err := getTargetPodList(f, ctx, targetApp)
	if err != nil {
		return err
	}

	// spawn len(pods) trace CRs
	for i, p := range pods.Items {
		traceName := fmt.Sprintf("open-liberty-trace-%d", i)

		// register goroutine with waitgroup and spawn trace
		wg.Add(1)
		go spawnTraceTest(&wg, f, ctx, traceName, p.GetName(), namespace)
	}

	t.Log("****** Waiting for traces...")
	wg.Wait()
	t.Log("****** Traces complete")
	return nil
}

func createTargetApp(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	ns, err := ctx.GetNamespace()
	// this determines scope of test
	// higher values REQUIRES higher probe retries
	// significantly slower at each increase
	replicas := 3

	if err != nil {
		return err
	}
	// client resource creation options
	options := &framework.CleanupOptions{
		TestContext:   ctx,
		Timeout:       time.Second,
		RetryInterval: time.Second,
	}

	ol := util.MakeBasicOpenLibertyApplication(t, f, targetApp, ns, int32(replicas))
	// set up serviceability, prereq to tracing
	ol.Spec.Serviceability = &openlibertyv1beta1.OpenLibertyApplicationServiceability{
		Size: "1Gi",
	}

	err = f.Client.Create(goctx.TODO(), ol, options)
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, targetApp, replicas, retryInterval, timeout)
	if err != nil {
		return err
	}

	return nil
}

func getTargetPodList(f *framework.Framework, ctx *framework.TestCtx, target string) (*corev1.PodList, error) {
	key := map[string]string{"app.kubernetes.io/name": target}

	options := &dynclient.ListOptions{
		LabelSelector: labels.Set(key).AsSelector(),
	}

	podList := &corev1.PodList{}

	err := f.Client.List(goctx.TODO(), podList, options)
	if err != nil {
		return nil, err
	}

	return podList, nil
}

func checkTraceStatus(f *framework.Framework, ns string, trace *openlibertyv1beta1.OpenLibertyTrace) error {
	tmp := &openlibertyv1beta1.OpenLibertyTrace{}
	err := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: trace.GetName(), Namespace: ns}, tmp)
	if err != nil {
		return err
	}

	for _, c := range trace.Status.Conditions {
		if c.Status == corev1.ConditionFalse {
			return fmt.Errorf("Bad Condition: %s", c.Message)
		}
	}

	return nil
}

func spawnTraceTest(wg *sync.WaitGroup, f *framework.Framework, ctx *framework.TestCtx, traceName, targetPodName, ns string) error {
	defer wg.Done()

	options := &framework.CleanupOptions{
		TestContext:   ctx,
		Timeout:       time.Second,
		RetryInterval: time.Second,
	}

	olTrace := util.MakeBasicOpenLibertyTrace(traceName, ns, targetPodName)
	err := f.Client.Create(goctx.TODO(), olTrace, options)
	if err != nil {
		return err
	}

	if err = checkTraceStatus(f, ns, olTrace); err != nil {
		return err
	}

	return nil
}
