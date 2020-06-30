package e2e

import (
	goctx "context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/OpenLiberty/open-liberty-operator/test/util"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	e2eutil "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"

	corev1 "k8s.io/api/core/v1"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)


var (
	messages = map[string]string{
		"IfNotPresent": "Container image \"%s\" already present on machine",
		"Always": "Pulling image \"%s\"",
		"Never": "Container image \"%s-fake\" is not present with pull policy of Never",
	}
	olAppName = "example-liberty-pullpolicy"
)



// OpenLibertyPullPolicyTest checks that the configured pull policy is applied to deployment
func OpenLibertyPullPolicyTest(t *testing.T) {
	// standard initialization
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
	applicationName := olAppName + "-always"
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

	return searchEventMessages(t, f, policy, namespace)
}

func testPullPolicyIfNotPresent(t *testing.T, f *framework.Framework, namespace string, ctx *framework.TestCtx) error {
	applicationName := olAppName + "-ifnotpresent"
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

	return searchEventMessages(t, f, corev1.PullIfNotPresent, namespace)
}

func testPullPolicyNever(t *testing.T, f *framework.Framework, namespace string, ctx *framework.TestCtx) error {
	applicationName := olAppName + "-never"
	replicas := int32(1)
	policy := corev1.PullNever

	openLibertyApplication := util.MakeBasicOpenLibertyApplication(t, f, applicationName, namespace, replicas)
	openLibertyApplication.Spec.PullPolicy = &policy
	openLibertyApplication.Spec.ApplicationImage = getImageTarget() + "-fake"

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err := f.Client.Create(goctx.TODO(), openLibertyApplication,
		&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		util.FailureCleanup(t, f, namespace, err)
	}

	time.Sleep(5 * time.Second)

	timestamp := time.Now().UTC()
	t.Logf("%s - Deployment created, verifying pull policy...", timestamp)

	return searchEventMessages(t, f, policy, namespace)
}

func searchEventMessages(t *testing.T, f *framework.Framework, policy corev1.PullPolicy, namespace string) error {
	options := &dynclient.ListOptions{
		Namespace: namespace,
	}

	eventlist := &corev1.EventList{}
	err := f.Client.List(goctx.TODO(), eventlist, options)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("***** Logging events in namespace: %s", namespace)
	message := fmt.Sprintf(messages[string(policy)], getImageTarget())
	t.Log(message)
	t.Log("****")
	lowerKey := strings.ToLower(message)
	for i := len(eventlist.Items) - 1; i >= 0; i-- {
		lowerMessage := strings.ToLower(eventlist.Items[i].Message)
		if lowerMessage == lowerKey {
			return nil
		}
		t.Log("------------------------------------------------------------")
		t.Log(eventlist.Items[i].Message)
	}

	return errors.New(fmt.Sprintf("The pull policy of %s was not correctly set", string(policy)))
}

func getImageTarget() string {
	image, exists := os.LookupEnv("LIBERTY_IMAGE")
	if !exists {
		image = "openliberty/open-liberty:kernel-java8-openj9-ubi"
	}
	return image
}
