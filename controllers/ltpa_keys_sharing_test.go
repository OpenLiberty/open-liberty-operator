package controllers

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	openlibertyv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	tree "github.com/OpenLiberty/open-liberty-operator/tree"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	oputils "github.com/application-stacks/runtime-component-operator/utils"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	name       = "app"
	namespace  = "openliberty"
	trueValue  = true
	falseValue = false
)

type Test struct {
	test     string
	expected interface{}
	actual   interface{}
}

func TestIsLTPAKeySharingEnabled(t *testing.T) {
	logger := zap.New()
	logf.SetLogger(logger)
	os.Setenv("WATCH_NAMESPACE", namespace)

	// Test default values no config
	spec := openlibertyv1.OpenLibertyApplicationSpec{}

	// Create Liberty app
	instance := createOpenLibertyApp(name, namespace, spec)
	r := createReconcilerFromOpenLibertyApp(instance)

	// test disabled by default
	tests := []Test{
		{"LTPA disabled when .spec.manageLTPA is nil", false, r.isLTPAKeySharingEnabled(instance)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// enable LTPA
	spec.ManageLTPA = &trueValue
	instance = createOpenLibertyApp(name, namespace, spec)

	// test enabled
	tests = []Test{
		{"LTPA enabled when .spec.manageLTPA is set to true", true, r.isLTPAKeySharingEnabled(instance)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// disable LTPA
	spec.ManageLTPA = &falseValue
	instance = createOpenLibertyApp(name, namespace, spec)

	// test disabled
	tests = []Test{
		{"LTPA disabled when .spec.manageLTPA is set to false", false, r.isLTPAKeySharingEnabled(instance)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func getControllersFolder() string {
	cwd, err := os.Getwd()
	if err != nil || !strings.HasSuffix(cwd, "/controllers") {
		return "controllers"
	}
	return cwd
}

func ignoreSubleases(leaderTracker map[string]string) map[string]string {
	delete(leaderTracker, lutils.ResourceSubleasesKey)
	return leaderTracker
}

func TestLTPALeaderTracker(t *testing.T) {
	logger := zap.New()
	logf.SetLogger(logger)
	os.Setenv("WATCH_NAMESPACE", namespace)

	// Test default values no config
	spec := openlibertyv1.OpenLibertyApplicationSpec{}

	// Create Liberty app
	instance := createOpenLibertyApp(name, namespace, spec)
	r := createReconcilerFromOpenLibertyApp(instance)

	// First, get the LTPA leader tracker which is not initialized
	configMap, _, err := r.getLTPALeaderTracker(instance)

	emptyConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "olo-managed-leader-tracking-ltpa",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/instance":   "olo-managed-leader-tracking-ltpa",
				"app.kubernetes.io/managed-by": "open-liberty-operator",
				"app.kubernetes.io/name":       "olo-managed-leader-tracking-ltpa",
			},
		},
	}
	tests := []Test{
		{"get LTPA leader tracker is nil", emptyConfigMap, configMap},
		{"get LTPA leader tracker is not found", true, kerrors.IsNotFound(err)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Second, initialize the LTPA leader tracker
	latestOperandVersion := "v10_4_1"
	fileName := getControllersFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseLTPADecisionTree(&fileName)
	tests = []Test{
		{"parse decision tree complex", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	err = r.initializeLTPALeaderTracker(instance, treeMap, replaceMap, "v10_4_1")
	tests = []Test{
		{"initialize LTPA leader tracker", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	configMap, _, err = r.getLTPALeaderTracker(instance)
	expectedConfigMapData := map[string]string{}
	expectedConfigMapData[lutils.ResourcesKey] = ""
	expectedConfigMapData[lutils.ResourceOwnersKey] = ""
	expectedConfigMapData[lutils.ResourcePathsKey] = ""
	expectedConfigMapData[lutils.ResourcePathIndicesKey] = ""
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", configMap.Name},
		{"get LTPA leader tracker namespace", namespace, configMap.Namespace},
		{"get LTPA leader tracker data", expectedConfigMapData, ignoreSubleases(configMap.Data)},
		{"get LTPA leader tracker label", latestOperandVersion, configMap.Labels[lutils.LTPAVersionLabel]},
		{"get LTPA leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Thirdly, create an LTPA Secret based upon a path in ltpa-decision-tree-complex.yaml
	complexSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "olo-managed-ltpa-ab215",
			Namespace: namespace,
			Labels: map[string]string{
				lutils.LTPAPathIndexLabel: latestOperandVersion + ".2", // choosing path index 2 under tree v10_4_1 (i.e. v10_4_1.a.b.e.true)
			},
		},
		Data: map[string][]byte{}, // create empty data
	}
	// LTPA Secrets can't be created without a ServiceAccount (this mock ServiceAccount allows function CreateOrUpdateWithLeaderTrackingLabels to mock the LTPA Secret)
	// In a live environment, the LTPA Secrets depend on the ServiceAccount to issue a Job that creates the LTPA Secret
	complexServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "olo-ltpa",
			Namespace: namespace,
		},
	}
	err1 := r.CreateOrUpdate(complexSecret, nil, func() error { return nil })
	err2 := r.CreateOrUpdate(complexServiceAccount, instance, func() error { return nil })
	tests = []Test{
		{"create LTPA Secret from based on path index 2 of complex decision tree", nil, err1},
		{"create LTPA ServiceAccount", nil, err2},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Mock the process where the operator saves the LTPA Secret, storing it into the leader tracker
	leaderName, isLeader, pathIndex, err := r.CreateOrUpdateWithLeaderTrackingLabels(complexServiceAccount, instance, &lutils.LTPAMetadata{
		Path:       latestOperandVersion + ".a.b.e.true",
		PathIndex:  latestOperandVersion + ".2",
		NameSuffix: "-ab215",
	}, true)
	tests = []Test{
		{"update leader tracker based on path index 2 of complex decision tree - error", nil, err},
		{"update leader tracker based on path index 2 of complex decision tree - path index", pathIndex, latestOperandVersion + ".2"},
		{"update leader tracker based on path index 2 of complex decision tree - isLeader", true, isLeader},
		{"update leader tracker based on path index 2 of complex decision tree - leader name", instance.Name, leaderName},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Fourth, check that the leader tracker received the new LTPA state
	configMap, _, err = r.getLTPALeaderTracker(instance)
	expectedConfigMapData = map[string]string{
		lutils.ResourcesKey:           "-ab215",
		lutils.ResourceOwnersKey:      name,
		lutils.ResourcePathsKey:       latestOperandVersion + ".a.b.e.true",
		lutils.ResourcePathIndicesKey: latestOperandVersion + ".2",
	}
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", configMap.Name},
		{"get LTPA leader tracker namespace", namespace, configMap.Namespace},
		{"get LTPA leader tracker data", expectedConfigMapData, ignoreSubleases(configMap.Data)},
		{"get LTPA leader tracker label", latestOperandVersion, configMap.Labels[lutils.LTPAVersionLabel]},
		{"get LTPA leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Fourthly, remove the LTPA leader
	err1 = r.deleteLTPAKeysResources(instance)
	hasNoOwners, err2 := r.DeleteResourceWithLeaderTrackingLabels(complexServiceAccount, instance)
	tests = []Test{
		{"remove LTPA - deleteLTPAKeysResource errors", nil, err1},
		{"remove LTPA - DeleteResourceWithLeaderTrackingLabels errors", nil, err2},
		{"remove LTPA - DeleteResourceWithLeaderTrackingLabels has no owners", true, hasNoOwners},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Lastly, check that the LTPA leader tracker was updated
	configMap, _, err = r.getLTPALeaderTracker(instance)
	expectedConfigMapData = map[string]string{
		lutils.ResourcesKey:           "-ab215",
		lutils.ResourceOwnersKey:      "", // The owner reference was removed
		lutils.ResourcePathsKey:       latestOperandVersion + ".a.b.e.true",
		lutils.ResourcePathIndicesKey: latestOperandVersion + ".2",
	}
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", configMap.Name},
		{"get LTPA leader tracker namespace", namespace, configMap.Namespace},
		{"get LTPA leader tracker data", expectedConfigMapData, ignoreSubleases(configMap.Data)},
		{"get LTPA leader tracker label", latestOperandVersion, configMap.Labels[lutils.LTPAVersionLabel]},
		{"get LTPA leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// This tests that the LTPA leader tracker can have cluster awareness of LTPA Secrets before operator reconciliation
func TestInitializeLTPALeaderTrackerWhenLTPASecretsExist(t *testing.T) {
	logger := zap.New()
	logf.SetLogger(logger)
	os.Setenv("WATCH_NAMESPACE", namespace)

	// Test default values no config
	spec := openlibertyv1.OpenLibertyApplicationSpec{}

	// Create Liberty app
	instance := createOpenLibertyApp(name, namespace, spec)
	r := createReconcilerFromOpenLibertyApp(instance)

	// Using the LTPA Decision Tree (complex) at version v10_4_1
	latestOperandVersion := "v10_4_1"
	fileName := getControllersFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseLTPADecisionTree(&fileName)
	tests := []Test{
		{"parse decision tree complex", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Firstly, Before initializing the leader tracker, create two LTPA Secrets based upon paths in ltpa-decision-tree-complex.yaml
	ltpaRootName := "olo-managed-ltpa"
	complexSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ltpaRootName + "-b12g1", // random lower alphanumeric suffix of length 5
			Namespace: namespace,
			Labels: map[string]string{
				lutils.LTPAPathIndexLabel: latestOperandVersion + ".2", // choosing path index 2 under tree v10_4_1 (i.e. v10_4_1.a.b.e.true)
				"app.kubernetes.io/name":  ltpaRootName,
			},
		},
		Data: map[string][]byte{}, // create empty data
	}
	complexSecret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ltpaRootName + "-bazc1", // random lower alphanumeric suffix of length 5
			Namespace: namespace,
			Labels: map[string]string{
				lutils.LTPAPathIndexLabel: latestOperandVersion + ".3", // choosing path index 3 under tree v10_4_1 (i.e. v10_4_1.a.b.e.false)
				"app.kubernetes.io/name":  ltpaRootName,
			},
		},
		Data: map[string][]byte{}, // create empty data
	}
	tests = []Test{
		{"create LTPA Secret from based on path index 2 of complex decision tree", nil, r.CreateOrUpdate(complexSecret, nil, func() error { return nil })},
		{"create LTPA Secret from based on path index 3 of complex decision tree", nil, r.CreateOrUpdate(complexSecret2, nil, func() error { return nil })},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Second, initialize the LTPA leader tracker
	tests = []Test{
		{"initialize LTPA leader tracker error", nil, r.initializeLTPALeaderTracker(instance, treeMap, replaceMap, latestOperandVersion)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Lastly, check that the LTPA leader tracker processes the two LTPA Secrets created
	configMap, _, err := r.getLTPALeaderTracker(instance)
	expectedConfigMapData := map[string]string{
		lutils.ResourcesKey:           "-b12g1,-bazc1",
		lutils.ResourceOwnersKey:      ",", // no owners associated with the LTPA Secrets because this decision tree (only for test) is not registered to use with the operator
		lutils.ResourcePathsKey:       "v10_4_1.a.b.e.true,v10_4_1.a.b.e.false",
		lutils.ResourcePathIndicesKey: "v10_4_1.2,v10_4_1.3",
	}
	tests = []Test{
		{"get LTPA leader tracker error", nil, err},
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", configMap.Name},
		{"get LTPA leader tracker namespace", namespace, configMap.Namespace},
		{"get LTPA leader tracker data", expectedConfigMapData, ignoreSubleases(configMap.Data)},
		{"get LTPA leader tracker label", latestOperandVersion, configMap.Labels[lutils.LTPAVersionLabel]},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// This tests that the LTPA leader tracker can have cluster awareness of LTPA Secrets before operator reconciliation and upgrade the LTPA Secrets to the latest decision tree version
func TestInitializeLTPALeaderTrackerWhenLTPASecretsExistWithUpgrade(t *testing.T) {
	spec := openlibertyv1.OpenLibertyApplicationSpec{}
	instance := createOpenLibertyApp(name, namespace, spec)
	r := createReconcilerFromOpenLibertyApp(instance)

	fileName := getControllersFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseLTPADecisionTree(&fileName)
	tests := []Test{
		{"parse decision tree complex", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Firstly, Before initializing the leader tracker, create two LTPA Secrets based upon paths in ltpa-decision-tree-complex.yaml
	latestOperandVersion := "v10_4_1"
	ltpaRootName := "olo-managed-ltpa"
	complexSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ltpaRootName + "-b12g1", // random lower alphanumeric suffix of length 5
			Namespace: namespace,
			Labels: map[string]string{
				lutils.LTPAPathIndexLabel: latestOperandVersion + ".2", // choosing path index 2 under tree v10_4_1 (i.e. v10_4_1.a.b.e.true)
				"app.kubernetes.io/name":  ltpaRootName,
			},
		},
		Data: map[string][]byte{}, // create empty data
	}
	complexSecret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ltpaRootName + "-bazc1", // random lower alphanumeric suffix of length 5
			Namespace: namespace,
			Labels: map[string]string{
				lutils.LTPAPathIndexLabel: latestOperandVersion + ".3", // choosing path index 3 under tree v10_4_1 (i.e. v10_4_1.a.b.e.false)
				"app.kubernetes.io/name":  ltpaRootName,
			},
		},
		Data: map[string][]byte{}, // create empty data
	}
	tests = []Test{
		{"create LTPA Secret from based on path index 2 of complex decision tree", nil, r.CreateOrUpdate(complexSecret, nil, func() error { return nil })},
		{"create LTPA Secret from based on path index 3 of complex decision tree", nil, r.CreateOrUpdate(complexSecret2, nil, func() error { return nil })},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Second, initialize the leader tracker but on a higher version of the LTPA decision tree
	latestOperandVersion = "v10_4_20" // upgrade the version
	tests = []Test{
		{"initializeLTPALeaderTracker at version v10_4_20", nil, r.initializeLTPALeaderTracker(instance, treeMap, replaceMap, latestOperandVersion)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Lastly, check that the LTPA leader tracker upgraded the two LTPA Secrets created
	configMap, _, err := r.getLTPALeaderTracker(instance)
	expectedConfigMapData := map[string]string{
		lutils.ResourcesKey:           "-b12g1,-bazc1",
		lutils.ResourceOwnersKey:      ",",                                       // no owners associated with the LTPA Secrets because this decision tree (only for test) is not registered to use with the operator
		lutils.ResourcePathsKey:       "v10_4_20.a.b.e.foo,v10_4_20.a.f.g.i.bar", // These paths have been upgraded to v10_4_20 based on replaceMap
		lutils.ResourcePathIndicesKey: "v10_4_20.2,v10_4_20.3",                   // These path indices have been upgraded to v10_4_20 based on replaceMap
	}
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", configMap.Name},
		{"get LTPA leader tracker namespace", namespace, configMap.Namespace},
		{"get LTPA leader tracker data", expectedConfigMapData, ignoreSubleases(configMap.Data)},
		{"get LTPA leader tracker label", latestOperandVersion, configMap.Labels[lutils.LTPAVersionLabel]},
		{"get LTPA leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// This tests that the LTPA leader tracker can have cluster awareness of LTPA Secrets before operator reconciliation and upgrade the LTPA Secrets to the latest decision tree version
func TestInitializeLTPALeaderTrackerWhenLTPASecretsExistWithMultipleUpgradesAndDowngrades(t *testing.T) {
	spec := openlibertyv1.OpenLibertyApplicationSpec{}
	instance := createOpenLibertyApp(name, namespace, spec)
	r := createReconcilerFromOpenLibertyApp(instance)

	fileName := getControllersFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseLTPADecisionTree(&fileName)
	tests := []Test{
		{"parse decision tree complex", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Firstly, Before initializing the leader tracker, create two LTPA Secrets based upon paths in ltpa-decision-tree-complex.yaml
	latestOperandVersion := "v10_4_1"
	ltpaRootName := "olo-managed-ltpa"
	complexSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ltpaRootName + "-b12g1", // random lower alphanumeric suffix of length 5
			Namespace: namespace,
			Labels: map[string]string{
				lutils.LTPAPathIndexLabel: "v10_4_1.2", // choosing path index 2 under tree v10_4_1 (i.e. v10_4_1.a.b.e.true)
				"app.kubernetes.io/name":  ltpaRootName,
			},
		},
		Data: map[string][]byte{}, // create empty data
	}
	complexSecret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ltpaRootName + "-bazc1", // random lower alphanumeric suffix of length 5
			Namespace: namespace,
			Labels: map[string]string{
				lutils.LTPAPathIndexLabel: "v10_4_1.3", // choosing path index 3 under tree v10_4_1 (i.e. v10_4_1.a.b.e.false)
				"app.kubernetes.io/name":  ltpaRootName,
			},
		},
		Data: map[string][]byte{}, // create empty data
	}
	complexSecret3 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ltpaRootName + "-ccccc", // random lower alphanumeric suffix of length 5
			Namespace: namespace,
			Labels: map[string]string{
				lutils.LTPAPathIndexLabel: "v10_4_1.4", // choosing path index 4 under tree v10_4_1 (i.e. v10_4_1.j.fizz)
				"app.kubernetes.io/name":  ltpaRootName,
			},
		},
		Data: map[string][]byte{}, // create empty data
	}
	tests = []Test{
		{"create LTPA Secret from based on path index 2 of complex decision tree", nil, r.CreateOrUpdate(complexSecret, nil, func() error { return nil })},
		{"create LTPA Secret from based on path index 3 of complex decision tree", nil, r.CreateOrUpdate(complexSecret2, nil, func() error { return nil })},
		{"create LTPA Secret from based on path index 4 of complex decision tree", nil, r.CreateOrUpdate(complexSecret3, nil, func() error { return nil })},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Second, initialize the leader tracker but on a higher version of the LTPA decision tree
	latestOperandVersion = "v10_4_500" // upgrade the version
	tests = []Test{
		{"initializeLTPALeaderTracker at version v10_4_500", nil, r.initializeLTPALeaderTracker(instance, treeMap, replaceMap, latestOperandVersion)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Thirdly, check that the LTPA leader tracker upgraded the two LTPA Secrets created
	configMap, _, err := r.getLTPALeaderTracker(instance)
	expectedConfigMapData := map[string]string{
		lutils.ResourcesKey:           "-b12g1,-bazc1,-ccccc",
		lutils.ResourceOwnersKey:      ",,",                                                        // no owners associated with the LTPA Secrets because this decision tree (only for test) is not registered to use with the operator
		lutils.ResourcePathsKey:       "v10_4_500.a.b.b.true,v10_4_500.a.f.g.i.bar,v10_4_1.j.fizz", // These paths have been upgraded to v10_4_500 based on replaceMap
		lutils.ResourcePathIndicesKey: "v10_4_500.0,v10_4_500.4,v10_4_1.4",                         // These path indices have been upgraded to v10_4_500 based on replaceMap
	}
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", configMap.Name},
		{"get LTPA leader tracker namespace", namespace, configMap.Namespace},
		{"get LTPA leader tracker data", expectedConfigMapData, ignoreSubleases(configMap.Data)},
		{"get LTPA leader tracker label", latestOperandVersion, configMap.Labels[lutils.LTPAVersionLabel]},
		{"get LTPA leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Fourthly, downgrade the decision tree version and initialize the leader tracker (run initialize once to delete the old configMap)
	latestOperandVersion = "v10_3_3"
	tests = []Test{
		{"Downgrade LTPA Leader Tracker from v10_4_500 to v10_3_3", nil, r.initializeLTPALeaderTracker(instance, treeMap, replaceMap, latestOperandVersion)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	r.initializeLTPALeaderTracker(instance, treeMap, replaceMap, latestOperandVersion)

	configMap, _, err = r.getLTPALeaderTracker(instance)
	expectedConfigMapData = map[string]string{
		lutils.ResourcesKey:           "-b12g1,-bazc1,-ccccc",
		lutils.ResourceOwnersKey:      ",,",                                             // no owners associated with the LTPA Secrets because this decision tree (only for test) is not registered to use with the operator
		lutils.ResourcePathsKey:       "v10_3_3.a.b,v10_4_1.a.b.e.false,v10_4_1.j.fizz", // v10_4_1 has no path to v10_3_3 so it is kept to be reference for a future upgrade
		lutils.ResourcePathIndicesKey: "v10_3_3.0,v10_4_1.3,v10_4_1.4",                  // These path indices have been upgraded to v10_4_500 based on replaceMap
	}
	tests = []Test{
		{"get LTPA leader tracker error", nil, err},
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", configMap.Name},
		{"get LTPA leader tracker namespace", namespace, configMap.Namespace},
		{"get LTPA leader tracker data", expectedConfigMapData, ignoreSubleases(configMap.Data)},
		{"get LTPA leader tracker label", latestOperandVersion, configMap.Labels[lutils.LTPAVersionLabel]},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func createReconcilerFromOpenLibertyApp(olapp *openlibertyv1.OpenLibertyApplication) *ReconcileOpenLiberty {
	objs, s := []runtime.Object{olapp}, scheme.Scheme
	s.AddKnownTypes(openlibertyv1.GroupVersion, olapp)
	cl := fakeclient.NewFakeClient(objs...)
	rcl := fakeclient.NewFakeClient(objs...)
	rb := oputils.NewReconcilerBase(rcl, cl, s, &rest.Config{}, record.NewFakeRecorder(10))
	rol := &ReconcileOpenLiberty{
		ReconcilerBase: rb,
	}
	return rol
}

func createOpenLibertyApp(n, ns string, spec openlibertyv1.OpenLibertyApplicationSpec) *openlibertyv1.OpenLibertyApplication {
	app := &openlibertyv1.OpenLibertyApplication{
		ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: ns},
		Spec:       spec,
	}
	return app
}

func verifyTests(tests []Test) error {
	for _, tt := range tests {
		if !reflect.DeepEqual(tt.actual, tt.expected) {
			return fmt.Errorf("%s test expected: (%v) actual: (%v)", tt.test, tt.expected, tt.actual)
		}
	}
	return nil
}
