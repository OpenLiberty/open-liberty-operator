package controller

import (
	"context"
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

// Leaf indexes are persisted in Secret labels, so they must never shift
func TestLTPADecisionTreeLeafIndexes(t *testing.T) {
	fileName := getAssetsFolder() + "/ltpa-decision-tree.yaml"
	treeMap, _, err := tree.ParseDecisionTree(LTPA_RESOURCE_SHARING_FILE_NAME, &fileName)
	if err != nil {
		t.Fatalf("%v", err)
	}
	tests := []Test{
		{"v1_4_0.config.passwordencryption index", 0, tree.GetLeafIndex(treeMap, "v1_4_0.config.passwordencryption")},
		{"v1_4_0.config.default index", 1, tree.GetLeafIndex(treeMap, "v1_4_0.config.default")},
		{"v1_4_0.key.true index (legacy, must not change)", 2, tree.GetLeafIndex(treeMap, "v1_4_0.key.true")},
		{"v1_4_0.key.generateaes index", 3, tree.GetLeafIndex(treeMap, "v1_4_0.key.generateaes")},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// Assets and securityUtility are read relative to the repo root
func chdirToRepoRoot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if strings.HasSuffix(cwd, "internal/controller") {
		if err := os.Chdir("../.."); err != nil {
			t.Fatalf("%v", err)
		}
		t.Cleanup(func() { os.Chdir(cwd) })
	}
}

func TestIsGeneratedAESKeyPathIndex(t *testing.T) {
	chdirToRepoRoot(t)
	tests := []Test{
		{"v1_3_3 key", false, isGeneratedAESKeyPathIndex("v1_3_3.0")},
		{"v1_4_0.key.true (legacy)", false, isGeneratedAESKeyPathIndex("v1_4_0.2")},
		{"v1_4_0.key.generateaes", true, isGeneratedAESKeyPathIndex("v1_4_0.3")},
		{"config path is denied", false, isGeneratedAESKeyPathIndex("v1_4_0.0")},
		{"unresolvable path is denied", false, isGeneratedAESKeyPathIndex("v1_4_0.99")},
		{"malformed path index is denied", false, isGeneratedAESKeyPathIndex("v1_4_0")},
		{"unlabelled secret is denied", false, isGeneratedAESKeyPathIndex("")},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func getLTPAKeyMetadataForStoredSecrets(t *testing.T, storedPathIndices map[string]string) *lutils.LTPAMetadata {
	_, _, keyMetadata := setupLTPAKeyTest(t, storedPathIndices)
	return keyMetadata
}

// Creates LTPA key Secrets with the given path indices (Secret suffix to path index) and returns the reconciler, app and chosen LTPA key metadata
func setupLTPAKeyTest(t *testing.T, storedPathIndices map[string]string) (*ReconcileOpenLiberty, *openlibertyv1.OpenLibertyApplication, *lutils.LTPAMetadata) {
	logf.SetLogger(zap.New())
	os.Setenv("WATCH_NAMESPACE", namespace)
	chdirToRepoRoot(t)
	spec := openlibertyv1.OpenLibertyApplicationSpec{ManageLTPA: &trueValue}
	instance := createOpenLibertyApp(name, namespace, spec)
	r := createReconcilerFromOpenLibertyApp(instance)

	fileName := getAssetsFolder() + "/ltpa-decision-tree.yaml"
	treeMap, replaceMap, err := tree.ParseDecisionTree(LTPA_RESOURCE_SHARING_FILE_NAME, &fileName)
	if err != nil {
		t.Fatalf("%v", err)
	}
	ltpaRootName := "olo-managed-ltpa"
	for suffix, pathIndex := range storedPathIndices {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ltpaRootName + suffix,
				Namespace: namespace,
				Labels: map[string]string{
					lutils.ResourcePathIndexLabel: pathIndex,
					"app.kubernetes.io/name":      ltpaRootName,
				},
			},
			Data: map[string][]byte{"lastRotation": []byte("1700000000")},
		}
		if err := r.CreateOrUpdate(secret, nil, func() error { return nil }); err != nil {
			t.Fatalf("%v", err)
		}
	}
	assetsFolder := getAssetsFolder()
	if err := r.reconcileLeaderTracker(instance, treeMap, replaceMap, "v1_4_0", LTPA_RESOURCE_SHARING_FILE_NAME, &assetsFolder); err != nil {
		t.Fatalf("%v", err)
	}
	metadataList, err := r.reconcileLTPAMetadata(instance, treeMap, "v1_4_0", &assetsFolder)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(metadataList.GetItems()) != 2 {
		t.Fatalf("expected 2 metadata items (key and config), got %d", len(metadataList.GetItems()))
	}
	return r, instance, metadataList.GetItems()[LTPAKey].(*lutils.LTPAMetadata)
}

func generatedAESSecretExists(r *ReconcileOpenLiberty) bool {
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: lutils.GeneratedAESEncryptionKeyRootName, Namespace: namespace}, &corev1.Secret{})
	return err == nil
}

// securityUtility is unavailable in unit tests, so this error proves AES key generation was attempted
func isAESKeyGenerationAttempted(r *ReconcileOpenLiberty, err error) bool {
	return generatedAESSecretExists(r) || (err != nil && strings.Contains(err.Error(), "failed to generate AES key"))
}

func TestReconcileLTPAMetadataFreshInstallUsesDefaultKeyPath(t *testing.T) {
	keyMetadata := getLTPAKeyMetadataForStoredSecrets(t, map[string]string{})
	tests := []Test{
		{"fresh install key path", "v1_4_0.key.generateaes", keyMetadata.Path},
		{"fresh install key path index", "v1_4_0.3", keyMetadata.PathIndex},
		{"fresh install generates AES key", true, isGeneratedAESKeyPathIndex(keyMetadata.PathIndex)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestReconcileLTPAMetadataUpgradeKeepsLegacyKeyPath(t *testing.T) {
	// created by a 1.4.0 - 1.6.2 operator
	keyMetadata := getLTPAKeyMetadataForStoredSecrets(t, map[string]string{"-aaaaa": "v1_4_0.2"})
	tests := []Test{
		{"upgrade key path", "v1_4_0.key.true", keyMetadata.Path},
		{"upgrade key path index", "v1_4_0.2", keyMetadata.PathIndex},
		{"upgrade key name re-used", "-aaaaa", keyMetadata.Name},
		{"upgrade does not generate AES key", false, isGeneratedAESKeyPathIndex(keyMetadata.PathIndex)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// A 1.3.3 LTPA Secret is upgraded to the legacy key path and never generates an AES key
func TestReconcileLTPAMetadataV133KeyUpgradesToLegacyKeyPath(t *testing.T) {
	keyMetadata := getLTPAKeyMetadataForStoredSecrets(t, map[string]string{"": "v1_3_3.0"})
	tests := []Test{
		{"1.3.3 key path", "v1_4_0.key.true", keyMetadata.Path},
		{"1.3.3 key path index", "v1_4_0.2", keyMetadata.PathIndex},
		{"1.3.3 key does not generate AES key", false, isGeneratedAESKeyPathIndex(keyMetadata.PathIndex)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestReconcileLTPAMetadataBothKeyPathsStoredPrefersLegacyKeyPath(t *testing.T) {
	// a fresh install rolled back to 1.6.2 and forward again
	keyMetadata := getLTPAKeyMetadataForStoredSecrets(t, map[string]string{"-aaaaa": "v1_4_0.3", "-bbbbb": "v1_4_0.2"})
	tests := []Test{
		{"both stored key path", "v1_4_0.key.true", keyMetadata.Path},
		{"both stored key name", "-bbbbb", keyMetadata.Name},
		{"both stored does not generate AES key", false, isGeneratedAESKeyPathIndex(keyMetadata.PathIndex)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestIsEncodedWithStaleGeneratedAESKey(t *testing.T) {
	recorded := func(v string) *corev1.Secret {
		return &corev1.Secret{Data: map[string][]byte{encryptionKeyLastRotationField: []byte(v)}}
	}
	tests := []Test{
		{"same rotation", false, isEncodedWithStaleGeneratedAESKey(recorded("100"), &generatedAESKey{LastRotation: "100"})},
		{"different rotation", true, isEncodedWithStaleGeneratedAESKey(recorded("100"), &generatedAESKey{LastRotation: "200"})},
		{"no recorded rotation (legacy secret)", false, isEncodedWithStaleGeneratedAESKey(&corev1.Secret{}, &generatedAESKey{LastRotation: "200"})},
		{"no generated AES key", false, isEncodedWithStaleGeneratedAESKey(recorded("100"), nil)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func reconcileGeneratedAESKeyForTest(r *ReconcileOpenLiberty, instance *openlibertyv1.OpenLibertyApplication, keyMetadata *lutils.LTPAMetadata) (*generatedAESKey, error) {
	return r.reconcileGeneratedAESKey(instance, keyMetadata, &lutils.PasswordEncryptionMetadata{Name: ""})
}

func createGeneratedAESSecretForTest(t *testing.T, r *ReconcileOpenLiberty, lastRotation string) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: lutils.GeneratedAESEncryptionKeyRootName, Namespace: namespace},
		Data: map[string][]byte{
			AESEncryptionKey: []byte("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="), // base64 of a 32 byte key
			"lastRotation":   []byte(lastRotation),
		},
	}
	if err := r.CreateOrUpdate(secret, nil, func() error { return nil }); err != nil {
		t.Fatalf("%v", err)
	}
}

// Creates the internal mirror of a user provided AES key
func createInternalUserAESKeySecretForTest(t *testing.T, r *ReconcileOpenLiberty, lastRotation string) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: lutils.LocalAESEncryptionKeyRootName + "-internal", Namespace: namespace},
		Data: map[string][]byte{
			AESEncryptionKey: []byte("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="), // base64 of a 32 byte key
			"lastRotation":   []byte(lastRotation),
		},
	}
	if err := r.CreateOrUpdate(secret, nil, func() error { return nil }); err != nil {
		t.Fatalf("%v", err)
	}
}

func setLTPAKeySecretEncryptionKeyLastRotationForTest(t *testing.T, r *ReconcileOpenLiberty, suffix string, lastRotation string) {
	secret := &corev1.Secret{}
	if err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: "olo-managed-ltpa" + suffix, Namespace: namespace}, secret); err != nil {
		t.Fatalf("%v", err)
	}
	secret.Data[encryptionKeyLastRotationField] = []byte(lastRotation)
	if err := r.GetClient().Update(context.TODO(), secret); err != nil {
		t.Fatalf("%v", err)
	}
}

func ltpaKeySecretExistsForTest(r *ReconcileOpenLiberty, suffix string) bool {
	return r.GetClient().Get(context.TODO(), types.NamespacedName{Name: "olo-managed-ltpa" + suffix, Namespace: namespace}, &corev1.Secret{}) == nil
}

// Upgrade: an existing key.true Secret never generates an AES key
func TestReconcileGeneratedAESKeyUpgradeDoesNotGenerateAESKey(t *testing.T) {
	r, instance, keyMetadata := setupLTPAKeyTest(t, map[string]string{"-aaaaa": "v1_4_0.2"})
	generatedAES, err := reconcileGeneratedAESKeyForTest(r, instance, keyMetadata)
	_, _, _, ltpaErr := r.generateLTPAKeys(instance, keyMetadata, generatedAES)
	tests := []Test{
		{"upgrade reconcileGeneratedAESKey error", nil, err},
		{"upgrade generated AES key", true, generatedAES == nil},
		{"upgrade generated AES secret exists", false, generatedAESSecretExists(r)},
		{"upgrade generateLTPAKeys error", nil, ltpaErr},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// Fresh install: an existing key.generateaes Secret generates the AES key
func TestReconcileGeneratedAESKeyFreshInstallGeneratesAESKey(t *testing.T) {
	r, instance, keyMetadata := setupLTPAKeyTest(t, map[string]string{"-aaaaa": "v1_4_0.3"})
	keyMetadata.Name = "-aaaaa"
	_, err := reconcileGeneratedAESKeyForTest(r, instance, keyMetadata)
	tests := []Test{
		{"fresh install key path", "v1_4_0.key.generateaes", keyMetadata.Path},
		{"fresh install generated AES key generation attempted", true, isAESKeyGenerationAttempted(r, err)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// Fresh install: a new key.generateaes Secret generates the AES key
func TestReconcileGeneratedAESKeyNewSecretOnGenerateAESPathGeneratesAESKey(t *testing.T) {
	r, instance, keyMetadata := setupLTPAKeyTest(t, map[string]string{})
	_, err := reconcileGeneratedAESKeyForTest(r, instance, keyMetadata)
	tests := []Test{
		{"new install key path", "v1_4_0.key.generateaes", keyMetadata.Path},
		{"new install generated AES key generation attempted", true, isAESKeyGenerationAttempted(r, err)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// A new key.true Secret never generates an AES key
func TestReconcileGeneratedAESKeyNewSecretOnLegacyPathDoesNotGenerateAESKey(t *testing.T) {
	r, instance, keyMetadata := setupLTPAKeyTest(t, map[string]string{})
	keyMetadata.Path = "v1_4_0.key.true"
	keyMetadata.PathIndex = "v1_4_0.2"
	generatedAES, err := reconcileGeneratedAESKeyForTest(r, instance, keyMetadata)
	tests := []Test{
		{"legacy new secret reconcileGeneratedAESKey error", nil, err},
		{"legacy new secret generated AES key", true, generatedAES == nil},
		{"legacy new secret generated AES key generation attempted", false, isAESKeyGenerationAttempted(r, err)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// Upgrade: an unlabelled Secret adopted by suffix never generates an AES key
func TestReconcileGeneratedAESKeyUnlabelledSecretDoesNotGenerateAESKey(t *testing.T) {
	r, instance, keyMetadata := setupLTPAKeyTest(t, map[string]string{"-aaaaa": ""})
	keyMetadata.Name = "-aaaaa" // adopt the existing Secret by suffix, as LTPA_KEY_RESOURCE_SUFFIXES does
	generatedAES, err := reconcileGeneratedAESKeyForTest(r, instance, keyMetadata)
	tests := []Test{
		{"unlabelled reconcileGeneratedAESKey error", nil, err},
		{"unlabelled generated AES key", true, generatedAES == nil},
		{"unlabelled generated AES key generation attempted", false, isAESKeyGenerationAttempted(r, err)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// An existing generated AES key is reused and reports its lastRotation
func TestReconcileGeneratedAESKeyReusesExistingKey(t *testing.T) {
	r, instance, keyMetadata := setupLTPAKeyTest(t, map[string]string{"-aaaaa": "v1_4_0.3"})
	keyMetadata.Name = "-aaaaa"
	createGeneratedAESSecretForTest(t, r, "200")
	generatedAES, err := reconcileGeneratedAESKeyForTest(r, instance, keyMetadata)
	if generatedAES == nil {
		t.Fatalf("expected a generated AES key, got error: %v", err)
	}
	tests := []Test{
		{"reuse error", nil, err},
		{"reuse secret name", lutils.GeneratedAESEncryptionKeyRootName, generatedAES.SecretName},
		{"reuse lastRotation", "200", generatedAES.LastRotation},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// A user provided key takes precedence: no generated AES key is created
func TestReconcileGeneratedAESKeyUserProvidedKeyTakesPrecedence(t *testing.T) {
	r, instance, keyMetadata := setupLTPAKeyTest(t, map[string]string{})
	instance.Spec.ManagePasswordEncryption = &trueValue
	createInternalUserAESKeySecretForTest(t, r, "100")
	generatedAES, err := reconcileGeneratedAESKeyForTest(r, instance, keyMetadata)
	_, _, _, ltpaErr := r.generateLTPAKeys(instance, keyMetadata, generatedAES)
	secret := &corev1.Secret{}
	if getErr := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: "olo-managed-ltpa" + keyMetadata.Name, Namespace: namespace}, secret); getErr != nil {
		t.Fatalf("%v", getErr)
	}
	tests := []Test{
		{"user key new secret path", "v1_4_0.key.generateaes", keyMetadata.Path},
		{"user key reconcileGeneratedAESKey error", nil, err},
		{"user key generated AES key", true, generatedAES == nil},
		{"user key generated AES key not created", false, isAESKeyGenerationAttempted(r, err)},
		{"user key generateLTPAKeys error", nil, ltpaErr},
		{"LTPA keys Secret records the user key rotation", "100", string(secret.Data[encryptionKeyLastRotationField])},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// managePasswordEncryption without a key Secret returns an error instead of generating a key
func TestGenerateLTPAKeysManagePasswordEncryptionWithoutKeyDoesNotGenerateAESKey(t *testing.T) {
	r, instance, keyMetadata := setupLTPAKeyTest(t, map[string]string{})
	instance.Spec.ManagePasswordEncryption = &trueValue
	generatedAES, err := reconcileGeneratedAESKeyForTest(r, instance, keyMetadata)
	_, _, _, ltpaErr := r.generateLTPAKeys(instance, keyMetadata, generatedAES)
	tests := []Test{
		{"no user key reconcileGeneratedAESKey error", nil, err},
		{"no user key generated AES key", true, generatedAES == nil},
		{"no user key generateLTPAKeys returns an error", true, ltpaErr != nil},
		{"no user key generated AES key not created", false, generatedAESSecretExists(r)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// A rotated generated AES key deletes the LTPA keys Secret
func TestGenerateLTPAKeysRotatedGeneratedAESKeyDeletesLTPAKeys(t *testing.T) {
	r, instance, keyMetadata := setupLTPAKeyTest(t, map[string]string{"-aaaaa": "v1_4_0.3"})
	keyMetadata.Name = "-aaaaa"
	setLTPAKeySecretEncryptionKeyLastRotationForTest(t, r, "-aaaaa", "100")
	createGeneratedAESSecretForTest(t, r, "200")
	generatedAES, err := reconcileGeneratedAESKeyForTest(r, instance, keyMetadata)
	_, _, _, ltpaErr := r.generateLTPAKeys(instance, keyMetadata, generatedAES)
	tests := []Test{
		{"rotated reconcileGeneratedAESKey error", nil, err},
		{"rotated generateLTPAKeys returns an error", true, ltpaErr != nil},
		{"rotated generated AES key deletes the LTPA keys Secret", false, ltpaKeySecretExistsForTest(r, "-aaaaa")},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// A matching rotation keeps the LTPA keys Secret
func TestGenerateLTPAKeysMatchingGeneratedAESKeyKeepsLTPAKeys(t *testing.T) {
	r, instance, keyMetadata := setupLTPAKeyTest(t, map[string]string{"-aaaaa": "v1_4_0.3"})
	keyMetadata.Name = "-aaaaa"
	setLTPAKeySecretEncryptionKeyLastRotationForTest(t, r, "-aaaaa", "200")
	createGeneratedAESSecretForTest(t, r, "200")
	generatedAES, err := reconcileGeneratedAESKeyForTest(r, instance, keyMetadata)
	_, _, _, ltpaErr := r.generateLTPAKeys(instance, keyMetadata, generatedAES)
	tests := []Test{
		{"matching rotation reconcileGeneratedAESKey error", nil, err},
		{"matching rotation generateLTPAKeys error", nil, ltpaErr},
		{"matching rotation keeps the LTPA keys Secret", true, ltpaKeySecretExistsForTest(r, "-aaaaa")},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// A key.true Secret ignores a leftover generated AES key
func TestGenerateLTPAKeysLegacyPathIgnoresGeneratedAESKeyRotation(t *testing.T) {
	r, instance, keyMetadata := setupLTPAKeyTest(t, map[string]string{"-aaaaa": "v1_4_0.2"})
	setLTPAKeySecretEncryptionKeyLastRotationForTest(t, r, "-aaaaa", "100")
	createGeneratedAESSecretForTest(t, r, "200")
	generatedAES, err := reconcileGeneratedAESKeyForTest(r, instance, keyMetadata)
	_, _, _, ltpaErr := r.generateLTPAKeys(instance, keyMetadata, generatedAES)
	tests := []Test{
		{"legacy path reconcileGeneratedAESKey error", nil, err},
		{"legacy path generated AES key", true, generatedAES == nil},
		{"legacy path generateLTPAKeys error", nil, ltpaErr},
		{"legacy path keeps the LTPA keys Secret", true, ltpaKeySecretExistsForTest(r, "-aaaaa")},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// With a user provided key a leftover generated AES key rotation is ignored
func TestGenerateLTPAKeysUserProvidedKeyIgnoresGeneratedAESKeyRotation(t *testing.T) {
	r, instance, keyMetadata := setupLTPAKeyTest(t, map[string]string{"-aaaaa": "v1_4_0.3"})
	keyMetadata.Name = "-aaaaa"
	instance.Spec.ManagePasswordEncryption = &trueValue
	setLTPAKeySecretEncryptionKeyLastRotationForTest(t, r, "-aaaaa", "100")
	createInternalUserAESKeySecretForTest(t, r, "100")
	createGeneratedAESSecretForTest(t, r, "200")
	generatedAES, err := reconcileGeneratedAESKeyForTest(r, instance, keyMetadata)
	_, _, _, ltpaErr := r.generateLTPAKeys(instance, keyMetadata, generatedAES)
	tests := []Test{
		{"user key reconcileGeneratedAESKey error", nil, err},
		{"user key generated AES key", true, generatedAES == nil},
		{"user key generateLTPAKeys error", nil, ltpaErr},
		{"user key keeps the LTPA keys Secret", true, ltpaKeySecretExistsForTest(r, "-aaaaa")},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// Switching managePasswordEncryption to false regenerates secrets encoded with the user's key
func TestGenerateLTPAKeysSwitchFromUserKeyToGeneratedAESKeyDeletesLTPAKeys(t *testing.T) {
	r, instance, keyMetadata := setupLTPAKeyTest(t, map[string]string{"-aaaaa": "v1_4_0.3"})
	keyMetadata.Name = "-aaaaa"
	instance.Spec.ManagePasswordEncryption = &falseValue
	setLTPAKeySecretEncryptionKeyLastRotationForTest(t, r, "-aaaaa", "100") // recorded from the user's key
	createInternalUserAESKeySecretForTest(t, r, "100")
	createGeneratedAESSecretForTest(t, r, "200")
	generatedAES, err := reconcileGeneratedAESKeyForTest(r, instance, keyMetadata)
	_, _, _, ltpaErr := r.generateLTPAKeys(instance, keyMetadata, generatedAES)
	tests := []Test{
		{"switch reconcileGeneratedAESKey error", nil, err},
		{"switch to generated key returns an error", true, ltpaErr != nil},
		{"switch to generated key deletes the LTPA keys Secret", false, ltpaKeySecretExistsForTest(r, "-aaaaa")},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}
