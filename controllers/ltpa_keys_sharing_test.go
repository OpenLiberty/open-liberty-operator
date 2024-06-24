package controllers

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	openlibertyv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
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
	name      = "app"
	namespace = "openliberty"
	// appImage            = "my-image"
	// consoleFormat       = "json"
	// replicas      int32 = 3
	// clusterType         = corev1.ServiceTypeClusterIP
	trueValue  = true
	falseValue = false
)

type Test struct {
	test     string
	expected interface{}
	actual   interface{}
}

func TestCompareOperandVersion(t *testing.T) {
	tests := []Test{
		{"same version", compareOperandVersion("v0_0_0", "v0_0_0"), 0},
		{"same version, multiple digits", compareOperandVersion("v10_10_10", "v10_10_10"), 0},
		{"same version, build tags", compareOperandVersion("v2_0_0alpha", "v2_0_0alpha"), 0},
		{"different patch version, build tags", compareOperandVersion("v2_0_10alpha", "v2_0_2alpha"), 8},
		{"different patch version, build tags, reversed", compareOperandVersion("v2_0_2alpha", "v2_0_10alpha"), -8},
		{"different patch version", compareOperandVersion("v1_0_0", "v1_0_1"), -1},
		{"different minor version", compareOperandVersion("v1_0_0", "v1_1_0"), -1},
		{"different major version", compareOperandVersion("v2_0_0", "v1_0_0"), 1},
		{"minor less than patch", compareOperandVersion("v1_10_0", "v1_0_5"), 10},
		{"major less than patch", compareOperandVersion("v2_0_0", "v1_0_10"), 1},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
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

func TestGetCommaSeparatedString(t *testing.T) {
	emptyString, emptyStringErr := getCommaSeparatedString("", 0)
	oneElement, oneElementErr := getCommaSeparatedString("one", 0)
	oneElementAOOB, oneElementAOOBErr := getCommaSeparatedString("one", 1)
	multiElement, multiElementErr := getCommaSeparatedString("one,two,three,four,five", 3)
	multiElementAOOB, multiElementAOOBErr := getCommaSeparatedString("one,two,three,four,five", 5)
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
	match := commaSeparatedStringContains("one,two,three,four", "three")
	substringNonMatch := commaSeparatedStringContains("one,two,three,four", "thre")
	substringNonMatch2 := commaSeparatedStringContains("one,two,three,four", "threee")
	oneElementMatch := commaSeparatedStringContains("one", "one")
	noElementNonMatch := commaSeparatedStringContains("", "one")
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

func getControllersFolder() string {
	cwd, err := os.Getwd()
	if err != nil || !strings.HasSuffix(cwd, "/controllers") {
		return "controllers"
	}
	return cwd
}

func TestParseDecisionTree(t *testing.T) {
	// Test 2 generations
	fileName := getControllersFolder() + "/assets/tests/ltpa-decision-tree-2-generations.yaml"
	treeMap, replaceMap, err := parseDecisionTree(&fileName)

	// Expect tree map
	expectedTreeMap := make(map[string]interface{})
	expectedTreeMap["v10_4_0"] = make(map[string]interface{})
	expectedTreeMap["v10_4_0"].(map[string]interface{})["managePasswordEncryption"] = make([]interface{}, 2)
	expectedTreeMap["v10_4_0"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[0] = true
	expectedTreeMap["v10_4_0"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[1] = false
	expectedTreeMap["v10_4_1"] = make(map[string]interface{})
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"] = make(map[string]interface{})
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"] = make(map[string]interface{})
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"].(map[string]interface{})["managePasswordEncryption"] = make([]interface{}, 2)
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[0] = true
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[1] = false
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["xor"] = "type"

	// Expect replace map
	expectedReplaceMap := make(map[string]map[string]string)
	expectedReplaceMap["v10_4_1"] = make(map[string]string)
	expectedReplaceMap["v10_4_1"]["v10_4_0.managePasswordEncryption.true"] = "v10_4_1.type.aes.managePasswordEncryption.true"
	expectedReplaceMap["v10_4_1"]["v10_4_0.managePasswordEncryption.false"] = "v10_4_1.type.aes.managePasswordEncryption.false"

	tests := []Test{
		{"parse decision tree (2 generations) error", nil, err},
		{"parse decision tree (2 generations) map", expectedTreeMap, treeMap},
		{"parse decision tree (2 generations) replace map", expectedReplaceMap, replaceMap},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Test 3 generations
	fileName = getControllersFolder() + "/assets/tests/ltpa-decision-tree-3-generations.yaml"
	treeMap, replaceMap, err = parseDecisionTree(&fileName)

	// Expect tree map
	expectedTreeMap = make(map[string]interface{})
	expectedTreeMap["v10_3_3"] = make(map[string]interface{})
	expectedTreeMap["v10_3_3"].(map[string]interface{})["manageLTPA"] = true
	expectedTreeMap["v10_4_0"] = make(map[string]interface{})
	expectedTreeMap["v10_4_0"].(map[string]interface{})["managePasswordEncryption"] = make([]interface{}, 2)
	expectedTreeMap["v10_4_0"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[0] = true
	expectedTreeMap["v10_4_0"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[1] = false
	expectedTreeMap["v10_4_1"] = make(map[string]interface{})
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"] = make(map[string]interface{})
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"] = make(map[string]interface{})
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"].(map[string]interface{})["managePasswordEncryption"] = make([]interface{}, 2)
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[0] = true
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[1] = false
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["xor"] = "type"

	// Expect replace map
	expectedReplaceMap = make(map[string]map[string]string)
	expectedReplaceMap["v10_4_0"] = make(map[string]string)
	expectedReplaceMap["v10_4_0"]["v10_3_3.manageLTPA.true"] = "v10_4_0.managePasswordEncryption.false"
	expectedReplaceMap["v10_4_1"] = make(map[string]string)
	expectedReplaceMap["v10_4_1"]["v10_4_0.managePasswordEncryption.true"] = "v10_4_1.type.aes.managePasswordEncryption.true"
	expectedReplaceMap["v10_4_1"]["v10_4_0.managePasswordEncryption.false"] = "v10_4_1.type.aes.managePasswordEncryption.false"

	tests = []Test{
		{"parse decision tree (3 generations) error", nil, err},
		{"parse decision tree (3 generations) map", expectedTreeMap, treeMap},
		{"parse decision tree (3 generations) replace map", expectedReplaceMap, replaceMap},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Test 1 generation
	fileName = getControllersFolder() + "/assets/tests/ltpa-decision-tree-1-generation.yaml"
	treeMap, replaceMap, err = parseDecisionTree(&fileName)

	expectedTreeMap = make(map[string]interface{})
	expectedTreeMap["v10_3_3"] = make(map[string]interface{})

	expectedReplaceMap = make(map[string]map[string]string)

	tests = []Test{
		{"parse decision tree (1 generation) error", nil, err},
		{"parse decision tree (1 generation) map", expectedTreeMap, treeMap},
		{"parse decision tree (1 generation) replace map", expectedReplaceMap, replaceMap},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
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
	configMap, err := r.getLTPALeaderTracker(instance)

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
	fileName := getControllersFolder() + "/assets/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, err := parseDecisionTree(&fileName)
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

	configMap, err = r.getLTPALeaderTracker(instance)
	expectedConfigMapData := map[string]string{}
	expectedConfigMapData[lutils.ResourcesKey] = ""
	expectedConfigMapData[lutils.ResourceOwnersKey] = ""
	expectedConfigMapData[lutils.ResourcePathsKey] = ""
	expectedConfigMapData[lutils.ResourcePathIndicesKey] = ""
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", configMap.Name},
		{"get LTPA leader tracker namespace", namespace, configMap.Namespace},
		{"get LTPA leader tracker data", expectedConfigMapData, configMap.Data},
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
	// LTPA Secrets can't be created without a ServiceAccount (this is used as a mock for CreateOrUpdateWithLeaderTrackingLabels in order to mock the LTPA Secret)
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
	configMap, err = r.getLTPALeaderTracker(instance)
	expectedConfigMapData = map[string]string{
		lutils.ResourcesKey:           "-ab215",
		lutils.ResourceOwnersKey:      name,
		lutils.ResourcePathsKey:       latestOperandVersion + ".a.b.e.true",
		lutils.ResourcePathIndicesKey: latestOperandVersion + ".2",
	}
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", configMap.Name},
		{"get LTPA leader tracker namespace", namespace, configMap.Namespace},
		{"get LTPA leader tracker data", expectedConfigMapData, configMap.Data},
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
	configMap, err = r.getLTPALeaderTracker(instance)
	expectedConfigMapData = map[string]string{
		lutils.ResourcesKey:           "-ab215",
		lutils.ResourceOwnersKey:      "", // The owner reference was removed
		lutils.ResourcePathsKey:       latestOperandVersion + ".a.b.e.true",
		lutils.ResourcePathIndicesKey: latestOperandVersion + ".2",
	}
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", configMap.Name},
		{"get LTPA leader tracker namespace", namespace, configMap.Namespace},
		{"get LTPA leader tracker data", expectedConfigMapData, configMap.Data},
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
	fileName := getControllersFolder() + "/assets/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, err := parseDecisionTree(&fileName)
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
	configMap, err := r.getLTPALeaderTracker(instance)
	expectedConfigMapData := map[string]string{
		lutils.ResourcesKey:           "-b12g1,-bazc1",
		lutils.ResourceOwnersKey:      ",", // no owners associated with the LTPA Secrets because this decision tree (only for test) is not registered to use with the operator
		lutils.ResourcePathsKey:       latestOperandVersion + ".a.b.e.true," + latestOperandVersion + ".a.b.e.false",
		lutils.ResourcePathIndicesKey: latestOperandVersion + ".2," + latestOperandVersion + ".3",
	}
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", configMap.Name},
		{"get LTPA leader tracker namespace", namespace, configMap.Namespace},
		{"get LTPA leader tracker data", expectedConfigMapData, configMap.Data},
		{"get LTPA leader tracker label", latestOperandVersion, configMap.Labels[lutils.LTPAVersionLabel]},
		{"get LTPA leader tracker error", nil, err},
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
