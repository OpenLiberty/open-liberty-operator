package e2e

import (
	goctx "context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/OpenLiberty/open-liberty-operator/test/util"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	e2eutil "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	corev1 "k8s.io/api/core/v1"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ifNotPresentMessage = "Container image \"openliberty/open-liberty:kernel-java8-openj9-ubi\" already present on machine"
	alwaysMessage       = "Pulling image \"openliberty/open-liberty:kernel-java8-openj9-ubi\""
	neverMessage        = "Container image \"openliberty/open-liberty-fake\" is not present with pull policy of Never"
)

// OpenLibertyPullPolicyTest checks that the configured pull policy is applied to deployment
func OpenLibertyPullPolicyTest(t *testing.T) {

	ctx, err := util.InitializeContext(t, cleanupTimeout, retryInterval)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Cleanup()

	f := framework.Global
	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatalf("could not get namespace: %v", err)
	}
	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "open-liberty-operator", 1, retryInterval, operatorTimeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
	timestamp := time.Now().UTC()
	t.Logf("%s - Starting liberty pull policy test...", timestamp)

	if err = testPullPolicyAlways(t, f, namespace, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = testPullPolicyIfNotPresent(t, f, namespace, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	if err = testPullPolicyNever(t, f, namespace, ctx); err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}
}
func testPullPolicyAlways(t *testing.T, f *framework.Framework, namespace string, ctx *framework.TestCtx) error {
	applicationName := "example-liberty-pullpolicy"
	replicas := int32(1)
	policy := corev1.PullAlways

	openLibertyApplication := util.MakeBasicOpenLibertyApplication(t, f, applicationName, namespace, replicas)
	openLibertyApplication.Spec.PullPolicy = &policy

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err := f.Client.Create(goctx.TODO(), openLibertyApplication, 
		&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// wait for example-liberty-pullpolicy to reach 2 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, applicationName, 1, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	timestamp := time.Now().UTC()
	t.Logf("%s - Deployment created, verifying pull policy...", timestamp)

	return searchEventMessages(t, f, alwaysMessage, namespace)
}

func testPullPolicyIfNotPresent(t *testing.T, f *framework.Framework, namespace string, ctx *framework.TestCtx) error {
	applicationName := "example-liberty-pullpolicy-ifnotpresent"
	replicas := int32(1)

	openLibertyApplication := util.MakeBasicOpenLibertyApplication(t, f, applicationName, namespace, replicas)

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err := f.Client.Create(goctx.TODO(), openLibertyApplication,
		&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	// wait for example-runtime-pullpolicy-ifnotpresent to reach 1 replica
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, applicationName, 1, retryInterval, timeout)
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	timestamp := time.Now().UTC()
	t.Logf("%s - Deployment created, verifying pull policy...", timestamp)

	return searchEventMessages(t, f, ifNotPresentMessage, namespace)
}

func searchEventMessages(t *testing.T, f *framework.Framework, key string, namespace string) error {
	options := &dynclient.ListOptions{
		Namespace: namespace,
	}

	eventlist := &corev1.EventList{}
	err := f.Client.List(goctx.TODO(), eventlist, options)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("***** Logging events in namespace: %s", namespace)
	lowerKey := strings.ToLower(key)
	for i := len(eventlist.Items) - 1; i >= 0; i-- {
		lowerMessage := strings.ToLower(eventlist.Items[i].Message)
		if lowerMessage == lowerKey {
			return nil
		}
		t.Log("------------------------------------------------------------")
		t.Log(eventlist.Items[i].Message)
	}

	return errors.New("The pull policy was not correctly set")
}

func testPullPolicyNever(t *testing.T, f *framework.Framework, namespace string, ctx *framework.TestCtx) error {
	replicas := int32(1)
	policy := corev1.PullNever

	openLibertyApplication := util.MakeBasicOpenLibertyApplication(t, f, "example-runtime-pullpolicy-never", namespace, replicas)
	openLibertyApplication.Spec.PullPolicy = &policy
	openLibertyApplication.Spec.ApplicationImage = "openliberty/open-liberty-fake"

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err := f.Client.Create(goctx.TODO(), openLibertyApplication,
		&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	for i := 0; i < 5; i++ {
		time.Sleep(time.Millisecond * 1000)
	}

	timestamp := time.Now().UTC()
	t.Logf("%s - Deployment created, verifying pull policy...", timestamp)

	return searchEventMessages(t, f, neverMessage, namespace)
}