package e2e

import (
	goctx "context"
	"errors"
	"fmt"
	"sync"
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

	targetApp := "open-liberty-target"
	// set up OL app to get trace from
	if err := createTargetApp(t, f, ctx, targetApp, 3); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// get the pods that were created from above app
	pods, err := getTargetPodList(f, ctx, targetApp)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = deletionTraceTest(t, f, ctx, pods); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = editTraceTest(t, f, ctx, pods); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = concurrentTraceTest(t, f, ctx, pods); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
}

// Verify OLTrace on set of replicas with basic config
func concurrentTraceTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, pods *corev1.PodList) error {
	if len(pods.Items) < 3 {
		return errors.New("pod list not long enough for concurrentTraceTest")
	}

	namespace, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace: %v", err)
	}

	var wg sync.WaitGroup
	// spawn a trace for each replica
	for i, p := range pods.Items {
		traceName := fmt.Sprintf("open-liberty-trace-%d", i)

		// register goroutine with waitgroup and spawn trace
		wg.Add(1)
		go spawnBasicTraceTest(&wg, t, f, ctx, traceName, p.GetName(), namespace)
	}

	t.Log("****** Waiting for traces...")
	wg.Wait()
	t.Log("****** Traces complete")
	return nil
}

func deletionTraceTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, pods *corev1.PodList) error {
	if len(pods.Items) < 1 {
		return errors.New("pod list not long enough for deletionTraceTest")
	}

	ns, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace: %v", err)
	}

	targetPodName := pods.Items[0].GetName()
	traceName := fmt.Sprintf("open-liberty-trace-delete")

	options := &framework.CleanupOptions{
		TestContext:   ctx,
		Timeout:       time.Second,
		RetryInterval: time.Second,
	}

	olTrace := util.MakeBasicOpenLibertyTrace(traceName, ns, targetPodName)
	err = f.Client.Create(goctx.TODO(), olTrace, options)
	if err != nil {
		return err
	}

	// wait for trace
	if err = util.WaitForStatusConditions(t, f, traceName, ns, retryInterval, timeout); err != nil {
		return err
	}

	// add for debugging
	time.Sleep(2000)

	// check for trace file in pod
	ok, err := util.TraceIsEnabled(t, f, targetPodName, ns)
	if err != nil {
		return err
	} else if !ok {
		return errors.New("could not find trace file despite good status")
	}

	err = f.Client.Delete(goctx.TODO(), olTrace)
	if err != nil {
		t.Log("failed to delete trace during deletion test")
		return err
	}

	ok, err = util.TraceIsEnabled(t, f, targetPodName, ns)
	if err != nil {
		return err
	} else if ok {
		return errors.New("trace not removed from pod during trace deletion")
	}

	return nil
}

func editTraceTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, pods *corev1.PodList) error {
	if len(pods.Items) < 2 {
		return errors.New("pod list not long enough for editTraceTest")
	}

	ns, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace: %v", err)
	}

	targetPodName := pods.Items[0].GetName()
	traceName := fmt.Sprintf("open-liberty-trace-edit")

	options := &framework.CleanupOptions{
		TestContext:   ctx,
		Timeout:       time.Second,
		RetryInterval: time.Second,
	}

	// Create trace and verify successful creation
	olTrace := util.MakeBasicOpenLibertyTrace(traceName, ns, targetPodName)
	err = f.Client.Create(goctx.TODO(), olTrace, options)
	if err != nil {
		return err
	}

	if err = util.WaitForStatusConditions(t, f, traceName, ns, retryInterval, timeout); err != nil {
		return err
	}

	ok, err := util.TraceIsEnabled(t, f, targetPodName, ns)
	if err != nil {
		return err
	} else if !ok {
		return errors.New("could not find trace file despite good status")
	}

	// Update trace to target new pod and verify transition
	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: olTrace.GetName(), Namespace: ns}, olTrace)
	if err != nil {
		return err
	}

	secondTargetPodName := pods.Items[1].GetName()
	olTrace.Spec.PodName = secondTargetPodName

	err = f.Client.Update(goctx.TODO(), olTrace)
	if err != nil {
		return err
	}

	if err = util.WaitForStatusConditions(t, f, traceName, ns, retryInterval, timeout); err != nil {
		return err
	}

	// Trace should now be enabled in new pod and disabled in old
	ok, err = util.TraceIsEnabled(t, f, targetPodName, ns)
	if err != nil {
		return err
	} else if ok {
		return errors.New("trace config not removed from previous pod target after edit")
	}

	ok, err = util.TraceIsEnabled(t, f, secondTargetPodName, ns)
	if err != nil {
		return err
	} else if !ok {
		return errors.New("trace config not found on new pod target after edit")
	}

	return nil
}

// createTargetApp generates the OLApp with indicated # of replicas & serviceability on
func createTargetApp(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, target string, replicas int) error {
	ns, err := ctx.GetNamespace()
	// higher values REQUIRE higher probe retries
	// significantly slower at each increase

	if err != nil {
		return err
	}

	options := &framework.CleanupOptions{
		TestContext:   ctx,
		Timeout:       time.Second,
		RetryInterval: time.Second,
	}

	ol := util.MakeBasicOpenLibertyApplication(t, f, target, ns, int32(replicas))
	ol.Spec.Serviceability = &openlibertyv1beta1.OpenLibertyApplicationServiceability{
		Size: "1Gi",
	}

	err = f.Client.Create(goctx.TODO(), ol, options)
	if err != nil {
		return err
	}

	err = e2eutil.WaitForDeployment(t, f.KubeClient, ns, target, replicas, retryInterval, timeout)
	if err != nil {
		return err
	}

	return nil
}

// getTargetPodList returns the pods created for targetApplication as a podList
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

// spawnBasicTraceTest creates a OLTrace CR belonging to wg, waits for good conditions
func spawnBasicTraceTest(wg *sync.WaitGroup, t *testing.T, f *framework.Framework, ctx *framework.TestCtx, traceName, targetPodName, ns string) error {
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

	if err = util.WaitForStatusConditions(t, f, traceName, ns, retryInterval, timeout); err != nil {
		return err
	}

	// check for trace file in pod
	ok, err := util.TraceIsEnabled(t, f, targetPodName, ns)
	if err != nil {
		return err
	} else if !ok {
		return errors.New("could not find trace file despite good status")
	}

	return nil
}
