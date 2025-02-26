package controller

import (
	"fmt"
	"os"
	"testing"

	openlibertyv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	tree "github.com/OpenLiberty/open-liberty-operator/utils/tree"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestTraceLeaderTrackerComplex(t *testing.T) {
	logger := zap.New()
	logf.SetLogger(logger)
	os.Setenv("WATCH_NAMESPACE", namespace)

	spec := openlibertyv1.OpenLibertyTraceSpec{}

	// Create Liberty trace
	instance := createOpenLibertyTrace(name, namespace, spec)
	r := createReconcilerFromOpenLibertyTrace(instance)

	// First, get the Trace leader tracker which is not initialized
	leaderTracker, _, err := lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME, r.GetClient())

	leaderTrackerName := "olo-managed-leader-tracking-" + TRACE_RESOURCE_SHARING_FILE_NAME
	emptyLeaderTracker := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaderTrackerName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/instance":   leaderTrackerName,
				"app.kubernetes.io/managed-by": "open-liberty-operator",
				"app.kubernetes.io/name":       leaderTrackerName,
			},
		},
	}
	tests := []Test{
		{"get Trace leader tracker is nil", emptyLeaderTracker, leaderTracker},
		{"get Trace leader tracker is not found", true, kerrors.IsNotFound(err)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Second, initialize the Trace leader tracker
	latestOperandVersion := "v10_4_1"
	fileName := getControllerFolder() + "/tests/decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseDecisionTree(TRACE_RESOURCE_SHARING_FILE_NAME, &fileName)
	tests = []Test{
		{"parse decision tree complex", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	assetsFolder := getAssetsFolder()
	rsf := r.createResourceSharingFactory(instance, treeMap, replaceMap, "v10_4_1", TRACE_RESOURCE_SHARING_FILE_NAME)
	err = tree.ReconcileLeaderTracker(instance.GetNamespace(), OperatorShortName, r.GetClient(), rsf, treeMap, replaceMap, latestOperandVersion, TRACE_RESOURCE_SHARING_FILE_NAME, &assetsFolder)
	tests = []Test{
		{"initialize Trace leader tracker", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	leaderTracker, _, err = lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	expectedLeaderTrackerData := map[string][]byte{}
	expectedLeaderTrackerData[lutils.ResourcesKey] = []byte("")
	expectedLeaderTrackerData[lutils.ResourceOwnersKey] = []byte("")
	expectedLeaderTrackerData[lutils.ResourcePathsKey] = []byte("")
	expectedLeaderTrackerData[lutils.ResourcePathIndicesKey] = []byte("")
	tests = []Test{
		{"get Trace leader tracker name", "olo-managed-leader-tracking-" + TRACE_RESOURCE_SHARING_FILE_NAME, leaderTracker.Name},
		{"get Trace leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get Trace leader tracker data", expectedLeaderTrackerData, leaderTracker.Data},
		{"get Trace leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
		{"get Trace leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Thirdly, create an Trace CR based upon a path in decision-tree-complex.yaml
	complexTrace := &openlibertyv1.OpenLibertyTrace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "complex-trace-one",
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: latestOperandVersion + ".2", // choosing path index 2 under tree v10_4_1 (i.e. v10_4_1.a.b.e.true)
			},
		},
		Spec: openlibertyv1.OpenLibertyTraceSpec{
			PodName:            "test-pod",
			TraceSpecification: "*:info",
		},
	}

	err1 := r.CreateOrUpdate(complexTrace, nil, func() error { return nil })
	tests = []Test{
		{"create Trace CR from based on path index 2 of complex decision tree", nil, err1},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Mock the process where the reconciler checks the OpenLibertyTrace (itself), storing itself into the leader tracker
	leaderName, isLeader, pathIndex, err := tree.ReconcileLeader(r.GetClient(), func(obj client.Object, owner metav1.Object, cb func() error) error {
		return r.CreateOrUpdate(obj, owner, cb)
	}, OperatorShortName, complexTrace.GetName(), complexTrace.GetNamespace(), &lutils.TraceMetadata{
		Path:      latestOperandVersion + ".a.b.e.true",
		PathIndex: latestOperandVersion + ".2",
		Name:      "test-pod",
	}, TRACE_RESOURCE_SHARING_FILE_NAME, true, false)
	tests = []Test{
		{"update leader tracker based on path index 2 of complex decision tree - error", nil, err},
		{"update leader tracker based on path index 2 of complex decision tree - path index", pathIndex, latestOperandVersion + ".2"},
		{"update leader tracker based on path index 2 of complex decision tree - isLeader", true, isLeader},
		{"update leader tracker based on path index 2 of complex decision tree - leader name", complexTrace.GetName(), leaderName},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Fourth, check that the leader tracker received the new Trace CR state
	leaderTracker, leaderTrackers, err := lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	expectedLeaderTrackerData = map[string][]byte{
		lutils.ResourcesKey:           []byte("test-pod"),
		lutils.ResourceOwnersKey:      []byte(complexTrace.GetName()),
		lutils.ResourcePathsKey:       []byte(latestOperandVersion + ".a.b.e.true"),
		lutils.ResourcePathIndicesKey: []byte(latestOperandVersion + ".2"),
	}
	tests = []Test{
		{"get Trace leader tracker name", "olo-managed-leader-tracking-" + TRACE_RESOURCE_SHARING_FILE_NAME, leaderTracker.Name},
		{"get Trace leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get Trace leader tracker data", expectedLeaderTrackerData, leaderTracker.Data},
		{"get Trace leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
		{"get Trace leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Fifth, add another Secret
	complexTraceTwo := &openlibertyv1.OpenLibertyTrace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "complex-trace-two",
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: latestOperandVersion + ".1",
			},
		},
		Spec: openlibertyv1.OpenLibertyTraceSpec{
			PodName:            "test-pod-2",
			TraceSpecification: "*:info",
		},
	}

	err2 := r.CreateOrUpdate(complexTraceTwo, nil, func() error { return nil })
	tests = []Test{
		{"create Trace Secret from based on path index 1 of complex decision tree", nil, err2},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Mock the process where the operator saves the Trace CR, storing it into the leader tracker
	tree.ReconcileLeader(r.GetClient(), func(obj client.Object, owner metav1.Object, cb func() error) error {
		return r.CreateOrUpdate(obj, owner, cb)
	}, OperatorShortName, complexTraceTwo.GetName(), complexTraceTwo.GetNamespace(), &lutils.TraceMetadata{
		Path:      latestOperandVersion + ".a.b.d.true",
		PathIndex: latestOperandVersion + ".1",
		Name:      "test-pod-2",
	}, TRACE_RESOURCE_SHARING_FILE_NAME, true, false)

	// Sixth, check that the Trace leader tracker was updated
	leaderTracker, _, err = lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	expectedLeaderTrackerData = map[string][]byte{
		lutils.ResourcesKey:           []byte("test-pod,test-pod-2"),
		lutils.ResourceOwnersKey:      []byte(fmt.Sprintf("%s,%s", complexTrace.GetName(), complexTraceTwo.GetName())),
		lutils.ResourcePathsKey:       []byte(fmt.Sprintf("%s.a.b.e.true,%s.a.b.d.true", latestOperandVersion, latestOperandVersion)),
		lutils.ResourcePathIndicesKey: []byte(fmt.Sprintf("%s.2,%s.1", latestOperandVersion, latestOperandVersion)),
	}
	tests = []Test{
		{"get Trace leader tracker name", "olo-managed-leader-tracking-trace", leaderTracker.Name},
		{"get Trace leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get Trace leader tracker data", expectedLeaderTrackerData, leaderTracker.Data},
		{"get Trace leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
		{"get Trace leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Lastly, remove the Trace leader
	removeErr1 := tree.RemoveLeaderTrackerReference(r.GetClient(),
		func(obj client.Object, owner metav1.Object, cb func() error) error {
			return r.CreateOrUpdate(obj, owner, cb)
		},
		func(obj client.Object) error {
			return r.DeleteResource(obj)
		}, complexTrace.GetName(), complexTrace.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME)
	removeErr2 := tree.RemoveLeaderTrackerReference(r.GetClient(),
		func(obj client.Object, owner metav1.Object, cb func() error) error {
			return r.CreateOrUpdate(obj, owner, cb)
		},
		func(obj client.Object) error {
			return r.DeleteResource(obj)
		}, complexTraceTwo.GetName(), complexTraceTwo.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME)
	removeLeaderErr1 := tree.RemoveLeader(func(obj client.Object, owner metav1.Object, cb func() error) error {
		return r.CreateOrUpdate(obj, owner, cb)
	}, func(obj client.Object) error {
		return r.DeleteResource(obj)
	}, complexTrace.GetName(), leaderTracker, leaderTrackers)
	removeLeaderErr2 := tree.RemoveLeader(func(obj client.Object, owner metav1.Object, cb func() error) error {
		return r.CreateOrUpdate(obj, owner, cb)
	}, func(obj client.Object) error {
		return r.DeleteResource(obj)
	}, complexTraceTwo.GetName(), leaderTracker, leaderTrackers)
	_, leaderTrackers, leaderTrackerErr := lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	var nilLeaderTrackers *[]lutils.LeaderTracker
	tests = []Test{
		{"remove Trace - deleteTraceKeysResource errors 1", nil, removeErr1},
		{"remove Trace - deleteTraceKeysResource errors 2", nil, removeErr2},
		{"remove Trace - RemoveLeader errors 1", nil, removeLeaderErr1},
		{"remove Trace - RemoveLeader errors 2", nil, removeLeaderErr2},
		{"remove Trace - GetLeaderTracker is not found", true, kerrors.IsNotFound(leaderTrackerErr)},
		{"remove Trace - leader trackers list is nil", nilLeaderTrackers, leaderTrackers},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// This tests that the Trace leader tracker can have cluster awareness of Trace CRs before operator reconciliation
func TestReconcileLeaderTrackerComplexWhenTraceExists(t *testing.T) {
	logger := zap.New()
	logf.SetLogger(logger)
	os.Setenv("WATCH_NAMESPACE", namespace)

	// Test default values no config
	spec := openlibertyv1.OpenLibertyTraceSpec{} // dummy spec

	// Create Liberty trace
	traceName := "example-trace"
	instance := createOpenLibertyTrace(traceName+"-zero", namespace, spec) // dummy olt - only used to mock the reconciler
	r := createReconcilerFromOpenLibertyTrace(instance)

	// First, check that the Trace leader tracker is not initialized
	leaderTracker, _, err := lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME, r.GetClient())

	leaderTrackerName := "olo-managed-leader-tracking-" + TRACE_RESOURCE_SHARING_FILE_NAME
	emptyLeaderTracker := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaderTrackerName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/instance":   leaderTrackerName,
				"app.kubernetes.io/managed-by": "open-liberty-operator",
				"app.kubernetes.io/name":       leaderTrackerName,
			},
		},
	}
	tests := []Test{
		{"get Trace leader tracker is nil", emptyLeaderTracker, leaderTracker},
		{"get Trace leader tracker is not found", true, kerrors.IsNotFound(err)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Using the Decision Tree (complex) at version v10_4_1
	latestOperandVersion := "v10_4_1"
	fileName := getControllerFolder() + "/tests/decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseDecisionTree(TRACE_RESOURCE_SHARING_FILE_NAME, &fileName)
	tests = []Test{
		{"parse decision tree complex", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Firstly, Before initializing the leader tracker, create two Trace CRs based upon paths in decision-tree-complex.yaml
	complexTraceName := traceName + "-one"
	complexTracePodName := traceName + "-pod-one"
	complexTrace := &openlibertyv1.OpenLibertyTrace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      complexTraceName,
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: latestOperandVersion + ".2", // choosing path index 2 under tree v10_4_1 (i.e. v10_4_1.a.b.e.true)
				"app.kubernetes.io/name":      complexTraceName,
			},
		},
		Spec: openlibertyv1.OpenLibertyTraceSpec{
			PodName:            complexTracePodName,
			TraceSpecification: "*:info",
		},
	}
	complexTrace2Name := traceName + "-two"
	complexTrace2PodName := traceName + "-pod-two"
	complexTrace2 := &openlibertyv1.OpenLibertyTrace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      complexTrace2Name,
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: latestOperandVersion + ".3", // choosing path index 3 under tree v10_4_1 (i.e. v10_4_1.a.b.e.false)
				"app.kubernetes.io/name":      complexTrace2Name,
			},
		},
		Spec: openlibertyv1.OpenLibertyTraceSpec{
			PodName:            complexTrace2PodName,
			TraceSpecification: "*:info",
		}, // create empty data
	}
	tests = []Test{
		{"create Trace CR from based on path index 2 of complex decision tree", nil, r.CreateOrUpdate(complexTrace, nil, func() error { return nil })},
		{"create Trace CR from based on path index 3 of complex decision tree", nil, r.CreateOrUpdate(complexTrace2, nil, func() error { return nil })},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Second, initialize the Trace leader tracker
	assetsFolder := getAssetsFolder()
	rsf := r.createResourceSharingFactory(instance, treeMap, replaceMap, latestOperandVersion, TRACE_RESOURCE_SHARING_FILE_NAME)
	tests = []Test{
		{"initialize Trace leader tracker error", nil, tree.ReconcileLeaderTracker(instance.GetNamespace(), OperatorShortName, r.GetClient(), rsf, treeMap, replaceMap, latestOperandVersion, TRACE_RESOURCE_SHARING_FILE_NAME, &assetsFolder)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Lastly, check that the Trace leader tracker processes the two Trace CRs created
	leaderTracker, _, err = lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	expectedLeaderTrackerData := map[string][]byte{
		lutils.ResourcesKey:           []byte(fmt.Sprintf("%s,%s", complexTracePodName, complexTrace2PodName)),
		lutils.ResourceOwnersKey:      []byte(","), // no owners associated with the Trace CRs because this decision tree (only for test) is not registered to use with the operator
		lutils.ResourcePathsKey:       []byte("v10_4_1.a.b.e.true,v10_4_1.a.b.e.false"),
		lutils.ResourcePathIndicesKey: []byte("v10_4_1.2,v10_4_1.3"),
	}
	tests = []Test{
		{"get Trace leader tracker error", nil, err},
		{"get Trace leader tracker name", "olo-managed-leader-tracking-" + TRACE_RESOURCE_SHARING_FILE_NAME, leaderTracker.Name},
		{"get Trace leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get Trace leader tracker data", expectedLeaderTrackerData, leaderTracker.Data},
		{"get Trace leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// This tests that the Trace leader tracker can have cluster awareness of Trace CRs before operator reconciliation and upgrade the Trace CRs to the latest decision tree version
func TestReconcileLeaderTrackerWhenTraceExistWithUpgrade(t *testing.T) {
	spec := openlibertyv1.OpenLibertyTraceSpec{}
	instance := createOpenLibertyTrace(name, namespace, spec) // dummy trace CR for instantiating the reconciler, not used in test
	r := createReconcilerFromOpenLibertyTrace(instance)

	fileName := getControllerFolder() + "/tests/decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseDecisionTree(TRACE_RESOURCE_SHARING_FILE_NAME, &fileName)
	tests := []Test{
		{"parse decision tree complex", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Firstly, Before initializing the leader tracker, create two Trace CR based upon paths in decision-tree-complex.yaml
	latestOperandVersion := "v10_4_1"
	traceName := "example-trace"
	complexTraceName := traceName + "-one"
	complexTracePodName := traceName + "-pod-one"
	complexTrace := &openlibertyv1.OpenLibertyTrace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      complexTraceName, // random lower alphanumeric suffix of length 5
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: latestOperandVersion + ".2", // choosing path index 2 under tree v10_4_1 (i.e. v10_4_1.a.b.e.true)
				"app.kubernetes.io/name":      complexTraceName,
			},
		},
		Spec: openlibertyv1.OpenLibertyTraceSpec{
			PodName:            complexTracePodName,
			TraceSpecification: "*:info",
		},
	}
	complexTrace2Name := traceName + "-two"
	complexTrace2PodName := traceName + "-pod-two"
	complexTrace2 := &openlibertyv1.OpenLibertyTrace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      complexTrace2Name, // random lower alphanumeric suffix of length 5
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: latestOperandVersion + ".3", // choosing path index 3 under tree v10_4_1 (i.e. v10_4_1.a.b.e.false)
				"app.kubernetes.io/name":      complexTrace2Name,
			},
		},
		Spec: openlibertyv1.OpenLibertyTraceSpec{
			PodName:            complexTrace2PodName,
			TraceSpecification: "*:info",
		},
	}
	tests = []Test{
		{"create Trace CR based on path index 2 of complex decision tree", nil, r.CreateOrUpdate(complexTrace, nil, func() error { return nil })},
		{"create Trace CR based on path index 3 of complex decision tree", nil, r.CreateOrUpdate(complexTrace2, nil, func() error { return nil })},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Second, initialize the leader tracker but on a higher version of the decision tree
	latestOperandVersion = "v10_4_20" // upgrade the version
	assetsFolder := getAssetsFolder()
	rsf := r.createResourceSharingFactory(instance, treeMap, replaceMap, latestOperandVersion, TRACE_RESOURCE_SHARING_FILE_NAME)
	tests = []Test{
		{"reconcileLeaderTracker at version v10_4_20", nil, tree.ReconcileLeaderTracker(instance.GetNamespace(), OperatorShortName, r.GetClient(), rsf, treeMap, replaceMap, latestOperandVersion, TRACE_RESOURCE_SHARING_FILE_NAME, &assetsFolder)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Lastly, check that the Trace leader tracker upgraded the two Trace CRs created
	leaderTracker, _, err := lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	expectedLeaderTrackerData := map[string][]byte{
		lutils.ResourcesKey:           []byte(fmt.Sprintf("%s,%s", complexTracePodName, complexTrace2PodName)),
		lutils.ResourceOwnersKey:      []byte(","),                                       // no owners associated with the Trace CR because this decision tree (only for test) is not registered to use with the operator
		lutils.ResourcePathsKey:       []byte("v10_4_20.a.b.e.foo,v10_4_20.a.f.g.i.bar"), // These paths have been upgraded to v10_4_20 based on replaceMap
		lutils.ResourcePathIndicesKey: []byte("v10_4_20.2,v10_4_20.3"),                   // These path indices have been upgraded to v10_4_20 based on replaceMap
	}
	tests = []Test{
		{"get Trace leader tracker name", "olo-managed-leader-tracking-" + TRACE_RESOURCE_SHARING_FILE_NAME, leaderTracker.Name},
		{"get Trace leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get Trace leader tracker key name", fmt.Sprintf("%s,%s", complexTracePodName, complexTrace2PodName), string(leaderTracker.Data[lutils.ResourcesKey])},
		{"get Trace leader tracker data", expectedLeaderTrackerData, leaderTracker.Data},
		{"get Trace leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
		{"get Trace leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

// This tests that the Trace leader tracker can have cluster awareness of Trace CRs before operator reconciliation and upgrade the Trace CRs to the latest decision tree version
func TestReconcileLeaderTrackerWhenTraceExistsWithMultipleUpgradesAndDowngrades(t *testing.T) {
	spec := openlibertyv1.OpenLibertyTraceSpec{}
	instance := createOpenLibertyTrace(name, namespace, spec)
	r := createReconcilerFromOpenLibertyTrace(instance)

	fileName := getControllerFolder() + "/tests/decision-tree-complex.yaml"
	treeMap, replaceMap, err := tree.ParseDecisionTree(TRACE_RESOURCE_SHARING_FILE_NAME, &fileName)
	tests := []Test{
		{"parse decision tree complex", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Firstly, Before initializing the leader tracker, create two Trace CR based upon paths in decision-tree-complex.yaml
	latestOperandVersion := "v10_4_1"
	traceName := "example-trace"
	complexTraceName := traceName + "-one"
	complexTrace2Name := traceName + "-two"
	complexTrace3Name := traceName + "-three"
	complexTracePodName := traceName + "-pod-one"
	complexTrace2PodName := traceName + "-pod-two"
	complexTrace3PodName := traceName + "-pod-three"
	complexTrace := &openlibertyv1.OpenLibertyTrace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      complexTraceName,
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: "v10_4_1.2", // choosing path index 2 under tree v10_4_1 (i.e. v10_4_1.a.b.e.true)
				"app.kubernetes.io/name":      complexTraceName,
			},
		},
		Spec: openlibertyv1.OpenLibertyTraceSpec{
			PodName:            complexTracePodName,
			TraceSpecification: "*:info",
		},
	}
	complexTrace2 := &openlibertyv1.OpenLibertyTrace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      complexTrace2Name,
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: "v10_4_1.3", // choosing path index 3 under tree v10_4_1 (i.e. v10_4_1.a.b.e.false)
				"app.kubernetes.io/name":      complexTrace2Name,
			},
		},
		Spec: openlibertyv1.OpenLibertyTraceSpec{
			PodName:            complexTrace2PodName,
			TraceSpecification: "*:info",
		},
	}
	complexTrace3 := &openlibertyv1.OpenLibertyTrace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      complexTrace3Name,
			Namespace: namespace,
			Labels: map[string]string{
				lutils.ResourcePathIndexLabel: "v10_4_1.4", // choosing path index 4 under tree v10_4_1 (i.e. v10_4_1.j.fizz)
				"app.kubernetes.io/name":      complexTrace3Name,
			},
		},
		Spec: openlibertyv1.OpenLibertyTraceSpec{
			PodName:            complexTrace3PodName,
			TraceSpecification: "*:info",
		},
	}
	tests = []Test{
		{"create Trace CR based on path index 2 of complex decision tree", nil, r.CreateOrUpdate(complexTrace, nil, func() error { return nil })},
		{"create Trace CR based on path index 3 of complex decision tree", nil, r.CreateOrUpdate(complexTrace2, nil, func() error { return nil })},
		{"create Trace CR based on path index 4 of complex decision tree", nil, r.CreateOrUpdate(complexTrace3, nil, func() error { return nil })},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Second, initialize the leader tracker but on a higher version of the decision tree
	latestOperandVersion = "v10_4_500" // upgrade the version
	assetsFolder := getAssetsFolder()
	rsf := r.createResourceSharingFactory(instance, treeMap, replaceMap, latestOperandVersion, TRACE_RESOURCE_SHARING_FILE_NAME)
	tests = []Test{
		{"reconcileLeaderTracker at version v10_4_500", nil, tree.ReconcileLeaderTracker(instance.GetNamespace(), OperatorShortName, r.GetClient(), rsf, treeMap, replaceMap, latestOperandVersion, TRACE_RESOURCE_SHARING_FILE_NAME, &assetsFolder)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Thirdly, check that the Trace leader tracker upgraded the two Trace CRs created
	leaderTracker, leaderTrackers, err := lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	leader1 := lutils.LeaderTracker{
		Name:      complexTracePodName,
		Owner:     "",                     // no owners associated with the Trace CRs because this decision tree (only for test) is not registered to use with the operator
		Path:      "v10_4_500.a.b.b.true", // These paths have been upgraded to v10_4_500 based on replaceMap
		PathIndex: "v10_4_500.0",          // These path indices have been upgraded to v10_4_500 based on replaceMap
	}
	leader2 := lutils.LeaderTracker{
		Name:      complexTrace2PodName,
		Owner:     "",
		Path:      "v10_4_500.a.f.g.i.bar",
		PathIndex: "v10_4_500.4",
	}
	leader3 := lutils.LeaderTracker{
		Name:      complexTrace3PodName,
		Owner:     "",
		Path:      "v10_4_1.j.fizz",
		PathIndex: "v10_4_1.4",
	}

	tests = []Test{
		{"get Trace leader tracker name", "olo-managed-leader-tracking-" + TRACE_RESOURCE_SHARING_FILE_NAME, leaderTracker.Name},
		{"get Trace leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get Trace leader trackers is not nil", leaderTrackers != nil, true},
		{"get Trace leader trackers matches length", len(*leaderTrackers), 3},
		{"get Trace leader trackers contains leader1", lutils.LeaderTrackersContains(leaderTrackers, leader1), true},
		{"get Trace leader trackers contains leader2", lutils.LeaderTrackersContains(leaderTrackers, leader2), true},
		{"get Trace leader trackers contains leader3", lutils.LeaderTrackersContains(leaderTrackers, leader3), true},
		{"get Trace leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
		{"get Trace leader tracker error", nil, err},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Fourthly, downgrade the decision tree version and initialize the leader tracker (run initialize once to delete the old configMap)
	latestOperandVersion = "v10_3_3"
	rsf = r.createResourceSharingFactory(instance, treeMap, replaceMap, latestOperandVersion, TRACE_RESOURCE_SHARING_FILE_NAME)
	tests = []Test{
		{"Downgrade Trace Leader Tracker from v10_4_500 to v10_3_3", nil, tree.ReconcileLeaderTracker(instance.GetNamespace(), OperatorShortName, r.GetClient(), rsf, treeMap, replaceMap, latestOperandVersion, TRACE_RESOURCE_SHARING_FILE_NAME, &assetsFolder)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	rsf = r.createResourceSharingFactory(instance, treeMap, replaceMap, latestOperandVersion, TRACE_RESOURCE_SHARING_FILE_NAME)
	tree.ReconcileLeaderTracker(instance.GetNamespace(), OperatorShortName, r.GetClient(), rsf, treeMap, replaceMap, latestOperandVersion, TRACE_RESOURCE_SHARING_FILE_NAME, &assetsFolder)

	leaderTracker, leaderTrackers, err = lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	leader1 = lutils.LeaderTracker{
		Name:      complexTracePodName,
		Owner:     "",            // no owners associated with the Trace CRs because this decision tree (only for test) is not registered to use with the operator
		Path:      "v10_3_3.a.b", // This path has been upgraded to v10_3_3 based on replaceMap
		PathIndex: "v10_3_3.0",   // This path index has been upgraded to v10_3_3.0 based on replaceMap
	}
	leader2 = lutils.LeaderTracker{
		Name:      complexTrace2PodName,
		Owner:     "",
		Path:      "v10_4_1.a.b.e.false",
		PathIndex: "v10_4_1.3",
	}
	leader3 = lutils.LeaderTracker{
		Name:      complexTrace3PodName,
		Owner:     "",
		Path:      "v10_4_1.j.fizz",
		PathIndex: "v10_4_1.4",
	}

	tests = []Test{
		{"get Trace leader tracker error", nil, err},
		{"get Trace leader tracker name", "olo-managed-leader-tracking-" + TRACE_RESOURCE_SHARING_FILE_NAME, leaderTracker.Name},
		{"get Trace leader tracker namespace", namespace, leaderTracker.Namespace},
		{"get Trace leader trackers matches length", len(*leaderTrackers), 3},
		{"get Trace leader trackers contains leader1", lutils.LeaderTrackersContains(leaderTrackers, leader1), true},
		{"get Trace leader trackers contains leader2", lutils.LeaderTrackersContains(leaderTrackers, leader2), true},
		{"get Trace leader trackers contains leader3", lutils.LeaderTrackersContains(leaderTrackers, leader3), true},
		{"get Trace leader tracker label", latestOperandVersion, leaderTracker.Labels[lutils.LeaderVersionLabel]},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func createReconcilerFromOpenLibertyTrace(olt *openlibertyv1.OpenLibertyTrace) *ReconcileOpenLibertyTrace {
	objs, s := []runtime.Object{olt}, scheme.Scheme
	oltl := &openlibertyv1.OpenLibertyTraceList{}
	s.AddKnownTypes(openlibertyv1.GroupVersion, olt)
	s.AddKnownTypes(openlibertyv1.GroupVersion, oltl)
	cl := fakeclient.NewFakeClient(objs...)
	rol := &ReconcileOpenLibertyTrace{
		Client:     cl,
		Scheme:     s,
		Recorder:   record.NewFakeRecorder(10),
		RestConfig: &rest.Config{},
	}
	return rol
}

func createOpenLibertyTrace(n, ns string, spec openlibertyv1.OpenLibertyTraceSpec) *openlibertyv1.OpenLibertyTrace {
	app := &openlibertyv1.OpenLibertyTrace{
		ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: ns},
		Spec:       spec,
	}
	return app
}
