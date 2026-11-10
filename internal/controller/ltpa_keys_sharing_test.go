package controller

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	openlibertyv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	tree "github.com/OpenLiberty/open-liberty-operator/utils/tree"
	oputils "github.com/application-stacks/runtime-component-operator/utils"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

func getControllerFolder() string {
	cwd, err := os.Getwd()
	if err != nil || !strings.HasSuffix(cwd, "internal/controller") {
		return "internal/controller"
	}
	return cwd
}

func getAssetsFolder() string {
	return getControllerFolder() + "/assets"
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
	leaderTracker, _, err := lutils.GetLeaderTracker(instance, OperatorShortName, LTPA_RESOURCE_SHARING_FILE_NAME, r.GetClient())

	emptyLeaderTracker := &corev1.Secret{
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
		{"get LTPA leader tracker is nil", emptyLeaderTracker, leaderTracker},
		{"get LTPA leader tracker is not found", true, kerrors.IsNotFound(err)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Second, initialize the LTPA leader tracker
	latestOperandVersion := "v10_4_1"
	fileName := getControllerFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseDecisionTree(LTPA_RESOURCE_SHARING_FILE_NAME, &fileName)
	tests = []Test{
		{"parse decision tree complex", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	assetsFolder := getAssetsFolder()
	err = r.reconcileLeaderTracker(instance, treeMap, replaceMap, "v10_4_1", LTPA_RESOURCE_SHARING_FILE_NAME, &assetsFolder)
	tests = []Test{
		{"initialize LTPA leader tracker", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	leaderTracker, _, err = lutils.GetLeaderTracker(instance, OperatorShortName, LTPA_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	expectedLeaderTrackerData := map[string][]byte{}
	expectedLeaderTrackerData[lutils.ResourcesKey] = []byte("")
	expectedLeaderTrackerData[lutils.ResourceOwnersKey] = []byte("")
	expectedLeaderTrackerData[lutils.ResourcePathsKey] = []byte("")
	expectedLeaderTrackerData[lutils.ResourcePathIndicesKey] = []byte("")
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", leaderTracker.Name},
		{"get LTPA leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get LTPA leader tracker data", expectedLeaderTrackerData, leaderTracker.Data},
		{"get LTPA leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
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
				lutils.ResourcePathIndexLabel: latestOperandVersion + ".2", // choosing path index 2 under tree v10_4_1 (i.e. v10_4_1.a.b.e.true)
			},
		},
		Data: map[string][]byte{}, // create empty data
	}

	err1 := r.CreateOrUpdate(complexSecret, nil, func() error { return nil })
	tests = []Test{
		{"create LTPA Secret from based on path index 2 of complex decision tree", nil, err1},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Mock the process where the operator saves the LTPA Secret, storing it into the leader tracker
	leaderName, isLeader, pathIndex, err := r.reconcileLeader(instance, &lutils.LTPAMetadata{
		Path:      latestOperandVersion + ".a.b.e.true",
		PathIndex: latestOperandVersion + ".2",
		Name:      "-ab215",
	}, LTPA_RESOURCE_SHARING_FILE_NAME, true)
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
	leaderTracker, leaderTrackers, err := lutils.GetLeaderTracker(instance, OperatorShortName, LTPA_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	expectedLeaderTrackerData = map[string][]byte{
		lutils.ResourcesKey:           []byte("-ab215"),
		lutils.ResourceOwnersKey:      []byte(name),
		lutils.ResourcePathsKey:       []byte(latestOperandVersion + ".a.b.e.true"),
		lutils.ResourcePathIndicesKey: []byte(latestOperandVersion + ".2"),
	}
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", leaderTracker.Name},
		{"get LTPA leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get LTPA leader tracker data", expectedLeaderTrackerData, leaderTracker.Data},
		{"get LTPA leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
		{"get LTPA leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Fifth, add another Secret
	complexSecretTwo := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "olo-managed-ltpa-cd123",
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: latestOperandVersion + ".1",
			},
		},
		Data: map[string][]byte{}, // create empty data
	}

	err2 := r.CreateOrUpdate(complexSecretTwo, nil, func() error { return nil })
	tests = []Test{
		{"create LTPA Secret from based on path index 1 of complex decision tree", nil, err2},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Mock the process where the operator saves the LTPA Secret, storing it into the leader tracker
	r.reconcileLeader(instance, &lutils.LTPAMetadata{
		Path:      latestOperandVersion + ".a.b.d.true",
		PathIndex: latestOperandVersion + ".1",
		Name:      "-cd123",
	}, LTPA_RESOURCE_SHARING_FILE_NAME, true)

	// Sixth, check that the LTPA leader tracker was updated
	leaderTracker, _, err = lutils.GetLeaderTracker(instance, OperatorShortName, LTPA_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	expectedLeaderTrackerData = map[string][]byte{
		lutils.ResourcesKey:           []byte("-ab215,-cd123"),
		lutils.ResourceOwnersKey:      []byte(fmt.Sprintf("%s,%s", instance.Name, instance.Name)),
		lutils.ResourcePathsKey:       []byte(fmt.Sprintf("%s.a.b.e.true,%s.a.b.d.true", latestOperandVersion, latestOperandVersion)),
		lutils.ResourcePathIndicesKey: []byte(fmt.Sprintf("%s.2,%s.1", latestOperandVersion, latestOperandVersion)),
	}
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", leaderTracker.Name},
		{"get LTPA leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get LTPA leader tracker data", expectedLeaderTrackerData, leaderTracker.Data},
		{"get LTPA leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
		{"get LTPA leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Lastly, remove the LTPA leader
	err1 = r.RemoveLeaderTrackerReference(instance, LTPA_RESOURCE_SHARING_FILE_NAME)
	err2 = r.RemoveLeader(instance, leaderTracker, leaderTrackers, LTPA_RESOURCE_SHARING_FILE_NAME)
	_, leaderTrackers, leaderTrackerErr := lutils.GetLeaderTracker(instance, OperatorShortName, LTPA_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	var nilLeaderTrackers *[]lutils.LeaderTracker
	tests = []Test{
		{"remove LTPA - deleteLTPAKeysResource errors", nil, err1},
		{"remove LTPA - RemoveLeader errors", nil, err2},
		{"remove LTPA - GetLeaderTracker is not found", true, kerrors.IsNotFound(leaderTrackerErr)},
		{"remove LTPA - leader trackers list is nil", nilLeaderTrackers, leaderTrackers},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// This tests that the LTPA leader tracker can have cluster awareness of LTPA Secrets before operator reconciliation
func TestReconcileLeaderTrackerWhenLTPASecretsExist(t *testing.T) {
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
	fileName := getControllerFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseDecisionTree(LTPA_RESOURCE_SHARING_FILE_NAME, &fileName)
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
				lutils.ResourcePathIndexLabel: latestOperandVersion + ".2", // choosing path index 2 under tree v10_4_1 (i.e. v10_4_1.a.b.e.true)
				"app.kubernetes.io/name":      ltpaRootName,
			},
		},
		Data: map[string][]byte{}, // create empty data
	}
	complexSecret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ltpaRootName + "-bazc1", // random lower alphanumeric suffix of length 5
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: latestOperandVersion + ".3", // choosing path index 3 under tree v10_4_1 (i.e. v10_4_1.a.b.e.false)
				"app.kubernetes.io/name":      ltpaRootName,
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
	assetsFolder := getAssetsFolder()
	tests = []Test{
		{"initialize LTPA leader tracker error", nil, r.reconcileLeaderTracker(instance, treeMap, replaceMap, latestOperandVersion, LTPA_RESOURCE_SHARING_FILE_NAME, &assetsFolder)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Lastly, check that the LTPA leader tracker processes the two LTPA Secrets created
	leaderTracker, _, err := lutils.GetLeaderTracker(instance, OperatorShortName, LTPA_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	expectedLeaderTrackerData := map[string][]byte{
		lutils.ResourcesKey:           []byte("-b12g1,-bazc1"),
		lutils.ResourceOwnersKey:      []byte(","), // no owners associated with the LTPA Secrets because this decision tree (only for test) is not registered to use with the operator
		lutils.ResourcePathsKey:       []byte("v10_4_1.a.b.e.true,v10_4_1.a.b.e.false"),
		lutils.ResourcePathIndicesKey: []byte("v10_4_1.2,v10_4_1.3"),
	}
	tests = []Test{
		{"get LTPA leader tracker error", nil, err},
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", leaderTracker.Name},
		{"get LTPA leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get LTPA leader tracker data", expectedLeaderTrackerData, leaderTracker.Data},
		{"get LTPA leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// This tests that the LTPA leader tracker can have cluster awareness of LTPA Secrets before operator reconciliation and upgrade the LTPA Secrets to the latest decision tree version
func TestReconcileLeaderTrackerWhenLTPASecretsExistWithUpgrade(t *testing.T) {
	spec := openlibertyv1.OpenLibertyApplicationSpec{}
	instance := createOpenLibertyApp(name, namespace, spec)
	r := createReconcilerFromOpenLibertyApp(instance)

	fileName := getControllerFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseDecisionTree(LTPA_RESOURCE_SHARING_FILE_NAME, &fileName)
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
				lutils.ResourcePathIndexLabel: latestOperandVersion + ".2", // choosing path index 2 under tree v10_4_1 (i.e. v10_4_1.a.b.e.true)
				"app.kubernetes.io/name":      ltpaRootName,
			},
		},
		Data: map[string][]byte{}, // create empty data
	}
	complexSecret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ltpaRootName + "-bazc1", // random lower alphanumeric suffix of length 5
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: latestOperandVersion + ".3", // choosing path index 3 under tree v10_4_1 (i.e. v10_4_1.a.b.e.false)
				"app.kubernetes.io/name":      ltpaRootName,
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
	assetsFolder := getAssetsFolder()
	tests = []Test{
		{"reconcileLeaderTracker at version v10_4_20", nil, r.reconcileLeaderTracker(instance, treeMap, replaceMap, latestOperandVersion, LTPA_RESOURCE_SHARING_FILE_NAME, &assetsFolder)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Lastly, check that the LTPA leader tracker upgraded the two LTPA Secrets created
	leaderTracker, _, err := lutils.GetLeaderTracker(instance, OperatorShortName, LTPA_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	expectedLeaderTrackerData := map[string][]byte{
		lutils.ResourcesKey:           []byte("-b12g1,-bazc1"),
		lutils.ResourceOwnersKey:      []byte(","),                                       // no owners associated with the LTPA Secrets because this decision tree (only for test) is not registered to use with the operator
		lutils.ResourcePathsKey:       []byte("v10_4_20.a.b.e.foo,v10_4_20.a.f.g.i.bar"), // These paths have been upgraded to v10_4_20 based on replaceMap
		lutils.ResourcePathIndicesKey: []byte("v10_4_20.2,v10_4_20.3"),                   // These path indices have been upgraded to v10_4_20 based on replaceMap
	}
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", leaderTracker.Name},
		{"get LTPA leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get LTPA leader tracker data", expectedLeaderTrackerData, leaderTracker.Data},
		{"get LTPA leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
		{"get LTPA leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// This tests that the LTPA leader tracker can have cluster awareness of LTPA Secrets before operator reconciliation and upgrade the LTPA Secrets to the latest decision tree version
func TestReconcileLeaderTrackerWhenLTPASecretsExistWithMultipleUpgradesAndDowngrades(t *testing.T) {
	spec := openlibertyv1.OpenLibertyApplicationSpec{}
	instance := createOpenLibertyApp(name, namespace, spec)
	r := createReconcilerFromOpenLibertyApp(instance)

	fileName := getControllerFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseDecisionTree(LTPA_RESOURCE_SHARING_FILE_NAME, &fileName)
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
				lutils.ResourcePathIndexLabel: "v10_4_1.2", // choosing path index 2 under tree v10_4_1 (i.e. v10_4_1.a.b.e.true)
				"app.kubernetes.io/name":      ltpaRootName,
			},
		},
		Data: map[string][]byte{}, // create empty data
	}
	complexSecret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ltpaRootName + "-bazc1", // random lower alphanumeric suffix of length 5
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: "v10_4_1.3", // choosing path index 3 under tree v10_4_1 (i.e. v10_4_1.a.b.e.false)
				"app.kubernetes.io/name":      ltpaRootName,
			},
		},
		Data: map[string][]byte{}, // create empty data
	}
	complexSecret3 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ltpaRootName + "-ccccc", // random lower alphanumeric suffix of length 5
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: "v10_4_1.4", // choosing path index 4 under tree v10_4_1 (i.e. v10_4_1.j.fizz)
				"app.kubernetes.io/name":      ltpaRootName,
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
	assetsFolder := getAssetsFolder()
	tests = []Test{
		{"reconcileLeaderTracker at version v10_4_500", nil, r.reconcileLeaderTracker(instance, treeMap, replaceMap, latestOperandVersion, LTPA_RESOURCE_SHARING_FILE_NAME, &assetsFolder)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Thirdly, check that the LTPA leader tracker upgraded the two LTPA Secrets created
	leaderTracker, _, err := lutils.GetLeaderTracker(instance, OperatorShortName, LTPA_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	expectedLeaderTrackerData := map[string][]byte{
		lutils.ResourcesKey:           []byte("-b12g1,-bazc1,-ccccc"),
		lutils.ResourceOwnersKey:      []byte(",,"),                                                        // no owners associated with the LTPA Secrets because this decision tree (only for test) is not registered to use with the operator
		lutils.ResourcePathsKey:       []byte("v10_4_500.a.b.b.true,v10_4_500.a.f.g.i.bar,v10_4_1.j.fizz"), // These paths have been upgraded to v10_4_500 based on replaceMap
		lutils.ResourcePathIndicesKey: []byte("v10_4_500.0,v10_4_500.4,v10_4_1.4"),                         // These path indices have been upgraded to v10_4_500 based on replaceMap
	}
	tests = []Test{
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", leaderTracker.Name},
		{"get LTPA leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get LTPA leader tracker data", expectedLeaderTrackerData, leaderTracker.Data},
		{"get LTPA leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
		{"get LTPA leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Fourthly, downgrade the decision tree version and initialize the leader tracker (run initialize once to delete the old configMap)
	latestOperandVersion = "v10_3_3"
	tests = []Test{
		{"Downgrade LTPA Leader Tracker from v10_4_500 to v10_3_3", nil, r.reconcileLeaderTracker(instance, treeMap, replaceMap, latestOperandVersion, LTPA_RESOURCE_SHARING_FILE_NAME, &assetsFolder)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	r.reconcileLeaderTracker(instance, treeMap, replaceMap, latestOperandVersion, LTPA_RESOURCE_SHARING_FILE_NAME, &assetsFolder)

	leaderTracker, _, err = lutils.GetLeaderTracker(instance, OperatorShortName, LTPA_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	expectedLeaderTrackerData = map[string][]byte{
		lutils.ResourcesKey:           []byte("-b12g1,-bazc1,-ccccc"),
		lutils.ResourceOwnersKey:      []byte(",,"),                                             // no owners associated with the LTPA Secrets because this decision tree (only for test) is not registered to use with the operator
		lutils.ResourcePathsKey:       []byte("v10_3_3.a.b,v10_4_1.a.b.e.false,v10_4_1.j.fizz"), // v10_4_1 has no path to v10_3_3 so it is kept to be reference for a future upgrade
		lutils.ResourcePathIndicesKey: []byte("v10_3_3.0,v10_4_1.3,v10_4_1.4"),                  // These path indices have been upgraded to v10_4_500 based on replaceMap
	}
	tests = []Test{
		{"get LTPA leader tracker error", nil, err},
		{"get LTPA leader tracker name", "olo-managed-leader-tracking-ltpa", leaderTracker.Name},
		{"get LTPA leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get LTPA leader tracker data", expectedLeaderTrackerData, leaderTracker.Data},
		{"get LTPA leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGenerateLTPAKeysWithUserSecret(t *testing.T) {
	spec := openlibertyv1.OpenLibertyApplicationSpec{
		ManageLTPA: &trueValue,
	}
	instance := createOpenLibertyApp(name, namespace, spec)
	r := createReconcilerFromOpenLibertyApp(instance)

	latestOperandVersion := "v10_4_1"
	fileName := getControllerFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseDecisionTree(LTPA_RESOURCE_SHARING_FILE_NAME, &fileName)
	if err != nil {
		t.Fatalf("parse decision tree complex: %v", err)
	}
	assetsFolder := getAssetsFolder()
	if err := r.reconcileLeaderTracker(instance, treeMap, replaceMap, latestOperandVersion, LTPA_RESOURCE_SHARING_FILE_NAME, &assetsFolder); err != nil {
		t.Fatalf("initialize leader tracker: %v", err)
	}

	ltpaMetadata := &lutils.LTPAMetadata{
		Name:      "",
		Path:      "v1_4_0.key.true",
		PathIndex: "v1_4_0.0",
	}

	// 1. Create a user LTPA Secret (e.g. mirrored from OpenBao)
	userLTPASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lutils.LTPAKeysRootName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			lutils.LTPAKeysFileName: []byte("my-custom-binary-ltpa-keys-data"),
			"rawPassword":           []byte("my-openbao-ltpa-password"),
		},
	}
	if err := r.CreateOrUpdate(userLTPASecret, nil, func() error { return nil }); err != nil {
		t.Fatalf("failed to create user LTPA secret: %v", err)
	}

	secretName, lastRotation, leaderName, err := r.generateLTPAKeys(instance, ltpaMetadata)
	if err != nil {
		t.Fatalf("generateLTPAKeys returned error: %v", err)
	}

	managedSecret := &corev1.Secret{}
	if err := r.GetClient().Get(nil, types.NamespacedName{Name: secretName, Namespace: namespace}, managedSecret); err != nil {
		t.Fatalf("failed to get managed LTPA secret: %v", err)
	}

	tests := []Test{
		{"managed secret name", "olo-managed-ltpa", secretName},
		{"managed secret leaderName", "app", leaderName},
		{"managed secret keys match user keys", "my-custom-binary-ltpa-keys-data", string(managedSecret.Data[lutils.LTPAKeysFileName])},
		{"managed secret rawPassword matches user rawPassword", "my-openbao-ltpa-password", string(managedSecret.Data["rawPassword"])},
		{"managed secret has lastRotation", lastRotation, string(managedSecret.Data["lastRotation"])},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// 2. Rotate the user LTPA Secret (e.g. OpenBao update)
	if err := r.CreateOrUpdate(userLTPASecret, nil, func() error {
		userLTPASecret.Data["rawPassword"] = []byte("new-rotated-password")
		userLTPASecret.Data[lutils.LTPAKeysFileName] = []byte("new-rotated-keys-data")
		return nil
	}); err != nil {
		t.Fatalf("failed to update user LTPA secret: %v", err)
	}

	secretName2, lastRotation2, _, err := r.generateLTPAKeys(instance, ltpaMetadata)
	if err != nil {
		t.Fatalf("generateLTPAKeys rotation returned error: %v", err)
	}

	if err := r.GetClient().Get(nil, types.NamespacedName{Name: secretName2, Namespace: namespace}, managedSecret); err != nil {
		t.Fatalf("failed to get managed LTPA secret after rotation: %v", err)
	}

	tests = []Test{
		{"rotated managed secret keys match new user keys", "new-rotated-keys-data", string(managedSecret.Data[lutils.LTPAKeysFileName])},
		{"rotated managed secret rawPassword matches new user rawPassword", "new-rotated-password", string(managedSecret.Data["rawPassword"])},
		{"rotated managed secret has new lastRotation", lastRotation2, string(managedSecret.Data["lastRotation"])},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// 3. Test base64 encoded user LTPA keys decoding
	encodedKeys := []byte("c29tZS1iYXNlNjQtZW5jb2RlZC1rZXlz") // base64 of "some-base64-encoded-keys"
	if err := r.CreateOrUpdate(userLTPASecret, nil, func() error {
		userLTPASecret.Data[lutils.LTPAKeysFileName] = encodedKeys
		userLTPASecret.Data["password"] = []byte("password-field-test")
		delete(userLTPASecret.Data, "rawPassword")
		return nil
	}); err != nil {
		t.Fatalf("failed to update user LTPA secret with base64: %v", err)
	}

	_, _, _, err = r.generateLTPAKeys(instance, ltpaMetadata)
	if err != nil {
		t.Fatalf("generateLTPAKeys with base64 returned error: %v", err)
	}

	if err := r.GetClient().Get(nil, types.NamespacedName{Name: secretName, Namespace: namespace}, managedSecret); err != nil {
		t.Fatalf("failed to get managed LTPA secret: %v", err)
	}

	tests = []Test{
		{"base64 user keys decoded to binary", "some-base64-encoded-keys", string(managedSecret.Data[lutils.LTPAKeysFileName])},
		{"password field alias recognized", "password-field-test", string(managedSecret.Data["rawPassword"])},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// 4. Test real LTPA key file format content (starts with #comment / properties)
	realLTPAContent := []byte("#Tue Nov 10 17:36:18 GMT 2026\ncom.ibm.websphere.CreationDate=Tue Nov 10 17\\:36\\:18 GMT 2026\ncom.ibm.websphere.CreationHost=localhost\ncom.ibm.websphere.ltpa.3DESKey=IDzNmMSY9nJi6B0UB49peEjMcsI0d4YTYZvg4o0qFwQ\\=\ncom.ibm.websphere.ltpa.Realm=defaultRealm\ncom.ibm.websphere.ltpa.version=1.0\n")
	if err := r.CreateOrUpdate(userLTPASecret, nil, func() error {
		userLTPASecret.Data[lutils.LTPAKeysFileName] = realLTPAContent
		userLTPASecret.Data["rawPassword"] = []byte("IfAKtBtYOMPDzYF")
		return nil
	}); err != nil {
		t.Fatalf("failed to update user LTPA secret with real LTPA content: %v", err)
	}

	_, _, _, err = r.generateLTPAKeys(instance, ltpaMetadata)
	if err != nil {
		t.Fatalf("generateLTPAKeys with real LTPA content returned error: %v", err)
	}

	if err := r.GetClient().Get(nil, types.NamespacedName{Name: secretName, Namespace: namespace}, managedSecret); err != nil {
		t.Fatalf("failed to get managed LTPA secret: %v", err)
	}

	tests = []Test{
		{"real LTPA key file content preserved intact", string(realLTPAContent), string(managedSecret.Data[lutils.LTPAKeysFileName])},
		{"real password preserved", "IfAKtBtYOMPDzYF", string(managedSecret.Data["rawPassword"])},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// The user LTPA Secret (wlp-ltpa-keys) has a fixed name; the leader tracker suffix (e.g. "-cq06s") only applies to the managed Secret.
func TestGenerateLTPAKeysWithUserSecretAndSuffix(t *testing.T) {
	spec := openlibertyv1.OpenLibertyApplicationSpec{
		ManageLTPA: &trueValue,
	}
	instance := createOpenLibertyApp(name, namespace, spec)
	r := createReconcilerFromOpenLibertyApp(instance)

	latestOperandVersion := "v10_4_1"
	fileName := getControllerFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseDecisionTree(LTPA_RESOURCE_SHARING_FILE_NAME, &fileName)
	if err != nil {
		t.Fatalf("parse decision tree complex: %v", err)
	}
	assetsFolder := getAssetsFolder()
	if err := r.reconcileLeaderTracker(instance, treeMap, replaceMap, latestOperandVersion, LTPA_RESOURCE_SHARING_FILE_NAME, &assetsFolder); err != nil {
		t.Fatalf("initialize leader tracker: %v", err)
	}

	ltpaMetadata := &lutils.LTPAMetadata{
		Name:      "-cq06s",
		Path:      "v1_4_0.key.true",
		PathIndex: "v1_4_0.0",
	}

	userLTPASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lutils.LTPAKeysRootName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			lutils.LTPAKeysFileName: []byte("initial-keys"),
			"rawPassword":           []byte("initial-password"),
		},
	}
	if err := r.CreateOrUpdate(userLTPASecret, nil, func() error { return nil }); err != nil {
		t.Fatalf("failed to create user LTPA secret: %v", err)
	}

	secretName, _, _, err := r.generateLTPAKeys(instance, ltpaMetadata)
	if err != nil {
		t.Fatalf("generateLTPAKeys returned error: %v", err)
	}
	managedSecret := &corev1.Secret{}
	if err := r.GetClient().Get(nil, types.NamespacedName{Name: secretName, Namespace: namespace}, managedSecret); err != nil {
		t.Fatalf("failed to get managed LTPA secret: %v", err)
	}
	tests := []Test{
		{"suffixed managed secret name", "olo-managed-ltpa-cq06s", secretName},
		{"suffixed managed secret keys match user keys", "initial-keys", string(managedSecret.Data[lutils.LTPAKeysFileName])},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Modify the user Secret, the suffixed managed Secret must be updated with a new lastRotation
	if err := r.CreateOrUpdate(userLTPASecret, nil, func() error {
		userLTPASecret.Data[lutils.LTPAKeysFileName] = []byte("rotated-keys")
		userLTPASecret.Data["rawPassword"] = []byte("rotated-password")
		return nil
	}); err != nil {
		t.Fatalf("failed to update user LTPA secret: %v", err)
	}
	_, lastRotation2, _, err := r.generateLTPAKeys(instance, ltpaMetadata)
	if err != nil {
		t.Fatalf("generateLTPAKeys rotation returned error: %v", err)
	}
	if err := r.GetClient().Get(nil, types.NamespacedName{Name: secretName, Namespace: namespace}, managedSecret); err != nil {
		t.Fatalf("failed to get managed LTPA secret after rotation: %v", err)
	}
	tests = []Test{
		{"suffixed managed secret rotated keys", "rotated-keys", string(managedSecret.Data[lutils.LTPAKeysFileName])},
		{"suffixed managed secret rotated password", "rotated-password", string(managedSecret.Data["rawPassword"])},
		{"suffixed managed secret lastRotation", lastRotation2, string(managedSecret.Data["lastRotation"])},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestUserLTPASecretWithPasswordEncryption(t *testing.T) {
	cwd, _ := os.Getwd()
	if strings.HasSuffix(cwd, "internal/controller") {
		os.Chdir("../..")
		defer os.Chdir(cwd)
	}

	spec := openlibertyv1.OpenLibertyApplicationSpec{
		ManageLTPA:               &trueValue,
		ManagePasswordEncryption: &trueValue,
	}
	instance := createOpenLibertyApp(name, namespace, spec)
	r := createReconcilerFromOpenLibertyApp(instance)

	// Initialize password encryption leader tracker
	latestOperandVersion := "v10_4_1"
	encFileName := getControllerFolder() + "/assets/password-encryption-decision-tree.yaml"
	encTreeMap, encReplaceMap, err := tree.ParseDecisionTree(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, &encFileName)
	if err != nil {
		t.Fatalf("parse enc tree: %v", err)
	}
	assetsFolder := getAssetsFolder()
	if err := r.reconcileLeaderTracker(instance, encTreeMap, encReplaceMap, latestOperandVersion, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, &assetsFolder); err != nil {
		t.Fatalf("init enc leader tracker: %v", err)
	}

	// Initialize LTPA leader tracker
	ltpaFileName := getControllerFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	ltpaTreeMap, ltpaReplaceMap, err := tree.ParseDecisionTree(LTPA_RESOURCE_SHARING_FILE_NAME, &ltpaFileName)
	if err != nil {
		t.Fatalf("parse ltpa tree: %v", err)
	}
	if err := r.reconcileLeaderTracker(instance, ltpaTreeMap, ltpaReplaceMap, latestOperandVersion, LTPA_RESOURCE_SHARING_FILE_NAME, &assetsFolder); err != nil {
		t.Fatalf("init ltpa leader tracker: %v", err)
	}

	// 1. Create user password encryption key (mirrored from OpenBao)
	userEncSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lutils.PasswordEncryptionKeyRootName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			PasswordEncryptionKey: []byte("my-encryption-key-secret"),
		},
	}
	if err := r.CreateOrUpdate(userEncSecret, nil, func() error { return nil }); err != nil {
		t.Fatalf("failed to create user encryption secret: %v", err)
	}

	// Reconcile encryption key
	pwdEncMeta := &lutils.PasswordEncryptionMetadata{Name: ""}
	_, encSecretName, encLastRotation, err := r.reconcileEncryptionKey(instance, pwdEncMeta)
	if err != nil {
		t.Fatalf("reconcileEncryptionKey failed: %v", err)
	}
	if encSecretName == "" || encLastRotation == "" {
		t.Fatalf("expected encSecretName and encLastRotation to be set, got %s, %s", encSecretName, encLastRotation)
	}

	// 2. Create user LTPA key secret (mirrored from OpenBao)
	userLTPASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lutils.LTPAKeysRootName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			lutils.LTPAKeysFileName: []byte("#sample-ltpa-keys-data"),
			"rawPassword":           []byte("IfAKtBtYOMPDzYF"),
		},
	}
	if err := r.CreateOrUpdate(userLTPASecret, nil, func() error { return nil }); err != nil {
		t.Fatalf("failed to create user LTPA secret: %v", err)
	}

	ltpaKeysMeta := &lutils.LTPAMetadata{Name: "", Path: "v1_4_0.key.true", PathIndex: "v1_4_0.0"}
	ltpaConfigMeta := &lutils.LTPAMetadata{Name: "", Path: "v1_4_0.config.passwordencryption", PathIndex: "v1_4_0.1"}

	_, ltpaSecretName, ltpaLastRotation, err := r.reconcileLTPAKeys(instance, ltpaKeysMeta)
	if err != nil {
		t.Fatalf("reconcileLTPAKeys failed: %v", err)
	}
	if ltpaSecretName == "" || ltpaLastRotation == "" {
		t.Fatalf("expected ltpaSecretName and ltpaLastRotation to be set")
	}

	lastKeyRelatedRotation, err := lutils.GetMaxTime(encLastRotation, ltpaLastRotation)
	if err != nil {
		t.Fatalf("GetMaxTime failed: %v", err)
	}

	// Reconcile LTPA config
	_, ltpaXMLSecretName, err := r.reconcileLTPAConfig(instance, ltpaKeysMeta, ltpaConfigMeta, pwdEncMeta, ltpaLastRotation, lastKeyRelatedRotation)
	if err != nil {
		t.Fatalf("reconcileLTPAConfig failed: %v", err)
	}
	if ltpaXMLSecretName == "" {
		t.Fatalf("expected ltpaXMLSecretName to be set")
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
