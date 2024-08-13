package utils

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	openlibertyv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCustomizeLeaderTrackerNil(t *testing.T) {
	leaderTracker := &corev1.Secret{}
	leaderTracker.Name = "leader-tracker-test"

	CustomizeLeaderTracker(leaderTracker, nil)

	expectedLeaderTrackerData := make(map[string][]byte)
	expectedLeaderTrackerData[ResourceOwnersKey] = []byte("")
	expectedLeaderTrackerData[ResourcesKey] = []byte("")
	expectedLeaderTrackerData[ResourcePathIndicesKey] = []byte("")
	expectedLeaderTrackerData[ResourcePathsKey] = []byte("")

	tests := []Test{
		{"nil leader tracker list", expectedLeaderTrackerData, ignoreSubleases(leaderTracker.Data)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCustomizeLeaderTrackerEmpty(t *testing.T) {
	leaderTracker := &corev1.Secret{}
	leaderTracker.Name = "leader-tracker-test"

	trackerList := make([]LeaderTracker, 0)
	CustomizeLeaderTracker(leaderTracker, &trackerList)

	expectedLeaderTrackerData := make(map[string][]byte)
	expectedLeaderTrackerData[ResourcesKey] = []byte("")
	expectedLeaderTrackerData[ResourceOwnersKey] = []byte("")
	expectedLeaderTrackerData[ResourcePathIndicesKey] = []byte("")
	expectedLeaderTrackerData[ResourcePathsKey] = []byte("")

	tests := []Test{
		{"empty leader tracker list", expectedLeaderTrackerData, ignoreSubleases(leaderTracker.Data)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCustomizeLeaderTrackerSingle(t *testing.T) {
	leaderTracker := &corev1.Secret{}
	leaderTracker.Name = "leader-tracker-test"

	ref1LeaderTracker := createMock1LeaderTracker()
	trackerList := make([]LeaderTracker, 0)
	trackerList = append(trackerList, createMock1LeaderTracker())

	CustomizeLeaderTracker(leaderTracker, &trackerList)

	expectedLeaderTrackerData := make(map[string][]byte)
	expectedLeaderTrackerData[ResourcesKey] = []byte(ref1LeaderTracker.Name)
	expectedLeaderTrackerData[ResourceOwnersKey] = []byte(ref1LeaderTracker.Owner)
	expectedLeaderTrackerData[ResourcePathIndicesKey] = []byte(ref1LeaderTracker.PathIndex)
	expectedLeaderTrackerData[ResourcePathsKey] = []byte(ref1LeaderTracker.Path)

	tests := []Test{
		{"single entry leader tracker", expectedLeaderTrackerData, ignoreSubleases(leaderTracker.Data)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCustomizeLeaderTrackerMultiple(t *testing.T) {
	leaderTracker := &corev1.Secret{}
	leaderTracker.Name = "leader-tracker-test"

	ref1LeaderTracker, ref2LeaderTracker := createMock1LeaderTracker(), createMock2LeaderTracker()
	trackerList := make([]LeaderTracker, 0)
	trackerList = append(trackerList, createMock1LeaderTracker())
	trackerList = append(trackerList, createMock2LeaderTracker())

	CustomizeLeaderTracker(leaderTracker, &trackerList)

	expectedLeaderTrackerData := make(map[string][]byte)
	expectedLeaderTrackerData[ResourcesKey] = []byte(fmt.Sprintf("%s,%s", ref1LeaderTracker.Name, ref2LeaderTracker.Name))
	expectedLeaderTrackerData[ResourceOwnersKey] = []byte(fmt.Sprintf("%s,%s", ref1LeaderTracker.Owner, ref2LeaderTracker.Owner))
	expectedLeaderTrackerData[ResourcePathIndicesKey] = []byte(fmt.Sprintf("%s,%s", ref1LeaderTracker.PathIndex, ref2LeaderTracker.PathIndex))
	expectedLeaderTrackerData[ResourcePathsKey] = []byte(fmt.Sprintf("%s,%s", ref1LeaderTracker.Path, ref2LeaderTracker.Path))

	tests := []Test{
		{"multiple entry leader tracker", expectedLeaderTrackerData, ignoreSubleases(leaderTracker.Data)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	trackerList = trackerList[1:]
	CustomizeLeaderTracker(leaderTracker, &trackerList)
	expectedLeaderTrackerData[ResourcesKey] = []byte(ref2LeaderTracker.Name)
	expectedLeaderTrackerData[ResourceOwnersKey] = []byte(ref2LeaderTracker.Owner)
	expectedLeaderTrackerData[ResourcePathIndicesKey] = []byte(ref2LeaderTracker.PathIndex)
	expectedLeaderTrackerData[ResourcePathsKey] = []byte(ref2LeaderTracker.Path)

	tests = []Test{
		{"remove entry leader tracker", expectedLeaderTrackerData, ignoreSubleases(leaderTracker.Data)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestEvictOwnerIfSubleaseHasExpired(t *testing.T) {
	leaderTracker, referenceLeaderTracker := createMock1LeaderTracker(), createMock1LeaderTracker()
	changeDetected := leaderTracker.EvictOwnerIfSubleaseHasExpired()

	tests := []Test{
		{"evict owner if sublease has expired - change detected", true, changeDetected},
		{"evict owner if sublease has expired - name unchanged", referenceLeaderTracker.Name, leaderTracker.Name},
		{"evict owner if sublease has expired - owner evicted", "", leaderTracker.Owner},
		{"evict owner if sublease has expired - path unchanged", referenceLeaderTracker.Path, leaderTracker.Path},
		{"evict owner if sublease has expired - path index unchanged", referenceLeaderTracker.PathIndex, leaderTracker.PathIndex},
		{"evict owner if sublease has expired - sublease removed", "", leaderTracker.Sublease},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestEvictOwnerIfSubleaseHasExpiredWithValidSublease(t *testing.T) {
	leaderTracker, referenceLeaderTracker := createMock1LeaderTracker(), createMock1LeaderTracker()
	leaderTracker.SetOwner("sublease-test")
	sublease, subleaseErr := strconv.ParseInt(leaderTracker.Sublease, 10, 64)
	now := time.Now().Unix()
	timeDiff := sublease - now
	changeDetected := leaderTracker.EvictOwnerIfSubleaseHasExpired()
	nextSublease, nextSubleaseErr := strconv.ParseInt(leaderTracker.Sublease, 10, 64)

	tests := []Test{
		{"evict owner if sublease has expired with valid sublease - sublease can convert to int64", nil, subleaseErr},
		{"evict owner if sublease has expired with valid sublease - sublease set in less than 20 seconds", true, timeDiff < 20},
		{"evict owner if sublease has expired with valid sublease - next sublease can convert to int64", nil, nextSubleaseErr},
		{"evict owner if sublease has expired with valid sublease - next sublease matches original sublease", sublease, nextSublease},
		{"evict owner if sublease has expired with valid sublease - no change detected", false, changeDetected},
		{"evict owner if sublease has expired with valid sublease - name unchanged", referenceLeaderTracker.Name, leaderTracker.Name},
		{"evict owner if sublease has expired with valid sublease - owner set", "sublease-test", leaderTracker.Owner},
		{"evict owner if sublease has expired with valid sublease - path unchanged", referenceLeaderTracker.Path, leaderTracker.Path},
		{"evict owner if sublease has expired with valid sublease - path index unchanged", referenceLeaderTracker.PathIndex, leaderTracker.PathIndex},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestEvictOwnerIfSubleaseHasExpiredWithInvalidSublease(t *testing.T) {
	leaderTracker, referenceLeaderTracker := createMock1LeaderTracker(), createMock1LeaderTracker()
	leaderTracker.Sublease = "abc123" // when specifying an invalid sublease duration that can not be parsed into type int, the operator evicts the owner
	changeDetected := leaderTracker.EvictOwnerIfSubleaseHasExpired()

	tests := []Test{
		{"evict owner if sublease has expired with invalid sublease - change detected", true, changeDetected},
		{"evict owner if sublease has expired with invalid sublease - name unchanged", referenceLeaderTracker.Name, leaderTracker.Name},
		{"evict owner if sublease has expired with invalid sublease - owner evicted", "", leaderTracker.Owner},
		{"evict owner if sublease has expired with invalid sublease - path unchanged", referenceLeaderTracker.Path, leaderTracker.Path},
		{"evict owner if sublease has expired with invalid sublease - path index unchanged", referenceLeaderTracker.PathIndex, leaderTracker.PathIndex},
		{"evict owner if sublease has expired with invalid sublease - sublease removed", "", leaderTracker.Sublease},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestClearOwnerIfMatching(t *testing.T) {
	leaderTracker, referenceLeaderTracker := createMock1LeaderTracker(), createMock1LeaderTracker()
	leaderTracker.Owner = "test-3"
	changeDetected := leaderTracker.ClearOwnerIfMatching(referenceLeaderTracker.Owner)

	tests := []Test{
		{"clear owner if matching - no change detected", false, changeDetected},
		{"clear owner if matching - name unchanged", referenceLeaderTracker.Name, leaderTracker.Name},
		{"clear owner if matching - owner unchanged", "test-3", leaderTracker.Owner},
		{"clear owner if matching - path unchanged", referenceLeaderTracker.Path, leaderTracker.Path},
		{"clear owner if matching - path index unchanged", referenceLeaderTracker.PathIndex, leaderTracker.PathIndex},
		{"clear owner if matching - sublease unchanged", referenceLeaderTracker.Sublease, leaderTracker.Sublease},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	referenceLeaderTracker.Owner = leaderTracker.Owner
	changeDetected = leaderTracker.ClearOwnerIfMatching(referenceLeaderTracker.Owner)

	tests = []Test{
		{"clear owner if matching - change detected", true, changeDetected},
		{"clear owner if matching - name unchanged", referenceLeaderTracker.Name, leaderTracker.Name},
		{"clear owner if matching - owner removed", "", leaderTracker.Owner},
		{"clear owner if matching - path unchanged", referenceLeaderTracker.Path, leaderTracker.Path},
		{"clear owner if matching - path index unchanged", referenceLeaderTracker.PathIndex, leaderTracker.PathIndex},
		{"clear owner if matching - sublease unchanged", referenceLeaderTracker.Sublease, leaderTracker.Sublease},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestSetOwner(t *testing.T) {
	leaderTracker, referenceLeaderTracker := createMock1LeaderTracker(), createMock1LeaderTracker()
	changeDetected := leaderTracker.SetOwner("new-owner-name")
	currTime, currErr := strconv.ParseInt(leaderTracker.Sublease, 10, 64)
	tests := []Test{
		{"set owner - change detected", true, changeDetected},
		{"set owner - name unchanged", referenceLeaderTracker.Name, leaderTracker.Name},
		{"set owner - owner changed", "new-owner-name", leaderTracker.Owner},
		{"set owner - path unchanged", referenceLeaderTracker.Path, leaderTracker.Path},
		{"set owner - path index unchanged", referenceLeaderTracker.PathIndex, leaderTracker.PathIndex},
		{"set owner - sublease converts to int64 without error", nil, currErr},
		{"set owner - new sublease time greater or equal to current time", true, currTime >= time.Now().Unix()},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestRemoveLeaderTracker(t *testing.T) {
	leaderTracker1 := createMock1LeaderTracker()
	leaderTracker2, ref2LeaderTracker := createMock2LeaderTracker(), createMock2LeaderTracker()
	leaderTrackerList := []LeaderTracker{
		leaderTracker1,
		leaderTracker2,
		leaderTracker1,
	}
	changeDetected := RemoveLeaderTracker(&leaderTrackerList, 3)

	tests := []Test{
		{"remove leader tracker, out of bounds - no change detected", false, changeDetected},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	changeDetected = RemoveLeaderTracker(&leaderTrackerList, 2)

	tests = []Test{
		{"remove leader tracker, remove last element - change detected", true, changeDetected},
		{"remove leader tracker, remove last element - length check", 2, len(leaderTrackerList)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	changeDetected = RemoveLeaderTracker(&leaderTrackerList, 0)

	tests = []Test{
		{"remove leader tracker, remove first element - change detected", true, changeDetected},
		{"remove leader tracker, remove first element - length check", 1, len(leaderTrackerList)},
		{"remove leader tracker, remove first element", ref2LeaderTracker.Name, leaderTrackerList[0].Name},
		{"remove leader tracker, remove first element", ref2LeaderTracker.Owner, leaderTrackerList[0].Owner},
		{"remove leader tracker, remove first element", ref2LeaderTracker.Path, leaderTrackerList[0].Path},
		{"remove leader tracker, remove first element", ref2LeaderTracker.PathIndex, leaderTrackerList[0].PathIndex},
		{"remove leader tracker, remove first element", ref2LeaderTracker.Sublease, leaderTrackerList[0].Sublease},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	changeDetected = RemoveLeaderTracker(&leaderTrackerList, 0)

	tests = []Test{
		{"remove leader tracker, remove first element - change detected", true, changeDetected},
		{"remove leader tracker, remove first element - length check", 0, len(leaderTrackerList)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	changeDetected = RemoveLeaderTracker(&leaderTrackerList, 0)

	tests = []Test{
		{"remove leader tracker, remove first element - no change detected", false, changeDetected},
		{"remove leader tracker, remove first element - length check", 0, len(leaderTrackerList)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	changeDetected = RemoveLeaderTracker(nil, 0)

	tests = []Test{
		{"remove leader tracker, nil array - no change detected", false, changeDetected},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGetLeaderTrackerWithoutSecret(t *testing.T) {
	spec := openlibertyv1.OpenLibertyApplicationSpec{}

	// Create Liberty app
	instance := createOpenLibertyApp(name, namespace, spec)

	// Create client
	objs, s := []runtime.Object{instance}, scheme.Scheme
	s.AddKnownTypes(openlibertyv1.GroupVersion, instance)
	cl := fakeclient.NewFakeClient(objs...)

	var referenceLeaderTrackers *[]LeaderTracker
	leaderTrackerSecret, leaderTrackers, err := GetLeaderTracker(instance, "olo", "ltpa", cl)
	tests := []Test{
		{"get leader tracker without secret - secret name matches", "olo-managed-leader-tracking-ltpa", leaderTrackerSecret.Name},
		{"get leader tracker without secret - leaderTrackers is nil", referenceLeaderTrackers, leaderTrackers},
		{"get leader tracker without secret - resource not found", true, kerrors.IsNotFound(err)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGetLeaderTrackerWithEmptySecret(t *testing.T) {
	spec := openlibertyv1.OpenLibertyApplicationSpec{}

	// Create Liberty app
	instance := createOpenLibertyApp(name, namespace, spec)

	// Create client
	objs, s := []runtime.Object{instance}, scheme.Scheme
	s.AddKnownTypes(openlibertyv1.GroupVersion, instance)
	cl := fakeclient.NewFakeClient(objs...)

	// Create empty secret
	emptySecret := &corev1.Secret{}
	emptySecret.Name = "olo-managed-leader-tracking-ltpa"
	emptySecret.Namespace = namespace
	createEmptySecretErr := cl.Create(context.TODO(), emptySecret)

	_, leaderTrackers, err := GetLeaderTracker(instance, "olo", "ltpa", cl)
	tests := []Test{
		{"get leader tracker with empty secret - create empty secret without error", nil, createEmptySecretErr},
		{"get leader tracker with empty secret - no error", nil, err},
		{"get leader tracker with empty secret - leaderTrackers empty", 0, len(*leaderTrackers)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGetLeaderTrackerWithOneSecretEntry(t *testing.T) {
	spec := openlibertyv1.OpenLibertyApplicationSpec{}

	// Create Liberty app
	instance := createOpenLibertyApp(name, namespace, spec)

	// Create client
	objs, s := []runtime.Object{instance}, scheme.Scheme
	s.AddKnownTypes(openlibertyv1.GroupVersion, instance)
	cl := fakeclient.NewFakeClient(objs...)

	// Create one secret entry
	oneSecret := &corev1.Secret{}
	oneSecret.Name = "olo-managed-leader-tracking-ltpa"
	oneSecret.Namespace = namespace
	mockLeaderTracker := createMock1LeaderTracker()
	oneSecret.Data = make(map[string][]byte)
	oneSecret.Data[ResourcesKey] = []byte(mockLeaderTracker.Name)
	oneSecret.Data[ResourceOwnersKey] = []byte(mockLeaderTracker.Owner)
	oneSecret.Data[ResourcePathIndicesKey] = []byte(mockLeaderTracker.PathIndex)
	oneSecret.Data[ResourcePathsKey] = []byte(mockLeaderTracker.Path)
	oneSecret.Data[ResourceSubleasesKey] = []byte(mockLeaderTracker.Sublease)
	createOneSecretErr := cl.Create(context.TODO(), oneSecret)

	_, leaderTrackers, err := GetLeaderTracker(instance, "olo", "ltpa", cl)
	tests := []Test{
		{"get leader tracker with one secret entry - create secret without error", nil, createOneSecretErr},
		{"get leader tracker with one secret entry - no error", nil, err},
		{"get leader tracker with one secret entry - leaderTrackers empty", 1, len(*leaderTrackers)},
		{"get leader tracker with one secret entry - name unchanged", mockLeaderTracker.Name, (*leaderTrackers)[0].Name},
		{"get leader tracker with one secret entry - owner unchanged", mockLeaderTracker.Owner, (*leaderTrackers)[0].Owner},
		{"get leader tracker with one secret entry - path index unchanged", mockLeaderTracker.PathIndex, (*leaderTrackers)[0].PathIndex},
		{"get leader tracker with one secret entry - path unchanged", mockLeaderTracker.Path, (*leaderTrackers)[0].Path},
		{"get leader tracker with one secret entry - sublease unchanged", mockLeaderTracker.Sublease, (*leaderTrackers)[0].Sublease},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGetLeaderTrackerWithOneSecretEntryWithMissingKey(t *testing.T) {
	spec := openlibertyv1.OpenLibertyApplicationSpec{}

	// Create Liberty app
	instance := createOpenLibertyApp(name, namespace, spec)

	// Create client
	objs, s := []runtime.Object{instance}, scheme.Scheme
	s.AddKnownTypes(openlibertyv1.GroupVersion, instance)
	cl := fakeclient.NewFakeClient(objs...)

	// Create one secret entry
	oneSecret := &corev1.Secret{}
	oneSecret.Name = "olo-managed-leader-tracking-ltpa"
	oneSecret.Namespace = namespace
	mockLeaderTracker := createMock1LeaderTracker()
	oneSecret.Data = make(map[string][]byte)
	oneSecret.Data[ResourcesKey] = []byte(mockLeaderTracker.Name)
	oneSecret.Data[ResourceOwnersKey] = []byte(mockLeaderTracker.Owner)
	oneSecret.Data[ResourcePathIndicesKey] = []byte(mockLeaderTracker.PathIndex)
	oneSecret.Data[ResourcePathsKey] = []byte(mockLeaderTracker.Path)
	// oneSecret.Data[ResourceSubleasesKey] = []byte(mockLeaderTracker.Sublease) // remove sublease key
	createOneSecretErr := cl.Create(context.TODO(), oneSecret)
	_, leaderTrackers, err := GetLeaderTracker(instance, "olo", "ltpa", cl)

	// GetLeaderTracker should delete the secret
	checkOneSecret := &corev1.Secret{}
	checkOneSecret.Name = "olo-managed-leader-tracking-ltpa"
	checkOneSecret.Namespace = namespace

	var nilLeaderTrackers *[]LeaderTracker
	checkOneSecretError := cl.Get(context.TODO(), types.NamespacedName{Name: checkOneSecret.Name, Namespace: checkOneSecret.Namespace}, checkOneSecret)
	tests := []Test{
		{"get leader tracker with one secret entry and missing key - create secret without error", nil, createOneSecretErr},
		{"get leader tracker with one secret entry and missing key - errors", false, err == nil},
		{"get leader tracker with one secret entry and missing key - leader tracker invalid", nilLeaderTrackers, leaderTrackers},
		{"get leader tracker with one secret entry and missing key - secret deleted", true, kerrors.IsNotFound(checkOneSecretError)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGetLeaderTrackerWithTwoSecretEntries(t *testing.T) {
	spec := openlibertyv1.OpenLibertyApplicationSpec{}

	// Create Liberty app
	instance := createOpenLibertyApp(name, namespace, spec)

	// Create client
	objs, s := []runtime.Object{instance}, scheme.Scheme
	s.AddKnownTypes(openlibertyv1.GroupVersion, instance)
	cl := fakeclient.NewFakeClient(objs...)

	// Create two secret entries
	twoSecret := &corev1.Secret{}
	twoSecret.Name = "olo-managed-leader-tracking-ltpa"
	twoSecret.Namespace = namespace
	mock1LeaderTracker, mock2LeaderTracker := createMock1LeaderTracker(), createMock2LeaderTracker()
	twoSecret.Data = make(map[string][]byte)
	twoSecret.Data[ResourcesKey] = []byte(fmt.Sprintf("%s,%s", mock1LeaderTracker.Name, mock2LeaderTracker.Name))
	twoSecret.Data[ResourceOwnersKey] = []byte(fmt.Sprintf("%s,%s", mock1LeaderTracker.Owner, mock2LeaderTracker.Owner))
	twoSecret.Data[ResourcePathIndicesKey] = []byte(fmt.Sprintf("%s,%s", mock1LeaderTracker.PathIndex, mock2LeaderTracker.PathIndex))
	twoSecret.Data[ResourcePathsKey] = []byte(fmt.Sprintf("%s,%s", mock1LeaderTracker.Path, mock2LeaderTracker.Path))
	twoSecret.Data[ResourceSubleasesKey] = []byte(fmt.Sprintf("%s,%s", mock1LeaderTracker.Sublease, mock2LeaderTracker.Sublease))
	createTwoSecretErr := cl.Create(context.TODO(), twoSecret)
	_, leaderTrackers, err := GetLeaderTracker(instance, "olo", "ltpa", cl)

	tests := []Test{
		{"get leader tracker with two secret entries - create secret without error", nil, createTwoSecretErr},
		{"get leader tracker with two secret entries - errors", nil, err},
		{"get leader tracker with two secret entries - leader tracker length", 2, len(*leaderTrackers)},
		{"get leader tracker with two secret entries - (1) name unchanged", mock1LeaderTracker.Name, (*leaderTrackers)[0].Name},
		{"get leader tracker with two secret entries - (1) owner unchanged", mock1LeaderTracker.Owner, (*leaderTrackers)[0].Owner},
		{"get leader tracker with two secret entries - (1) path index unchanged", mock1LeaderTracker.PathIndex, (*leaderTrackers)[0].PathIndex},
		{"get leader tracker with two secret entries - (1) path unchanged", mock1LeaderTracker.Path, (*leaderTrackers)[0].Path},
		{"get leader tracker with two secret entries - (1) sublease unchanged", mock1LeaderTracker.Sublease, (*leaderTrackers)[0].Sublease},
		{"get leader tracker with two secret entries - (2) name unchanged", mock2LeaderTracker.Name, (*leaderTrackers)[1].Name},
		{"get leader tracker with two secret entries - (2) owner unchanged", mock2LeaderTracker.Owner, (*leaderTrackers)[1].Owner},
		{"get leader tracker with two secret entries - (2) path index unchanged", mock2LeaderTracker.PathIndex, (*leaderTrackers)[1].PathIndex},
		{"get leader tracker with two secret entries - (2) path unchanged", mock2LeaderTracker.Path, (*leaderTrackers)[1].Path},
		{"get leader tracker with two secret entries - (2) sublease unchanged", mock2LeaderTracker.Sublease, (*leaderTrackers)[1].Sublease},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGetLeaderTrackerWithTwoSecretEntriesAndMissingEntry(t *testing.T) {
	spec := openlibertyv1.OpenLibertyApplicationSpec{}

	// Create Liberty app
	instance := createOpenLibertyApp(name, namespace, spec)

	// Create client
	objs, s := []runtime.Object{instance}, scheme.Scheme
	s.AddKnownTypes(openlibertyv1.GroupVersion, instance)
	cl := fakeclient.NewFakeClient(objs...)

	// Create two secret entries
	twoSecret := &corev1.Secret{}
	twoSecret.Name = "olo-managed-leader-tracking-ltpa"
	twoSecret.Namespace = namespace
	mock1LeaderTracker, mock2LeaderTracker := createMock1LeaderTracker(), createMock2LeaderTracker()
	twoSecret.Data = make(map[string][]byte)
	twoSecret.Data[ResourcesKey] = []byte(fmt.Sprintf("%s,%s", mock1LeaderTracker.Name, mock2LeaderTracker.Name))
	twoSecret.Data[ResourceOwnersKey] = []byte(fmt.Sprintf("%s,%s", mock1LeaderTracker.Owner, mock2LeaderTracker.Owner))
	twoSecret.Data[ResourcePathIndicesKey] = []byte(fmt.Sprintf("%s,%s", mock1LeaderTracker.PathIndex, mock2LeaderTracker.PathIndex))
	twoSecret.Data[ResourcePathsKey] = []byte(fmt.Sprintf("%s,%s", mock1LeaderTracker.Path, mock2LeaderTracker.Path))
	twoSecret.Data[ResourceSubleasesKey] = []byte(fmt.Sprint(mock1LeaderTracker.Sublease)) // missing mock2LeaderTracker.Sublease
	createTwoSecretErr := cl.Create(context.TODO(), twoSecret)
	_, leaderTrackers, err := GetLeaderTracker(instance, "olo", "ltpa", cl)

	// GetLeaderTracker should delete the secret
	checkTwoSecret := &corev1.Secret{}
	checkTwoSecret.Name = "olo-managed-leader-tracking-ltpa"
	checkTwoSecret.Namespace = namespace

	var nilLeaderTrackers *[]LeaderTracker
	checkTwoSecretError := cl.Get(context.TODO(), types.NamespacedName{Name: checkTwoSecret.Name, Namespace: checkTwoSecret.Namespace}, checkTwoSecret)
	tests := []Test{
		{"get leader tracker with two secret entries and missing entry - create secret without error", nil, createTwoSecretErr},
		{"get leader tracker with two secret entries and missing entry - error is not nil", false, err == nil},
		{"get leader tracker with two secret entries and missing entry - leader tracker invalid", nilLeaderTrackers, leaderTrackers},
		{"get leader tracker with two secret entries and missing entry - secret does not exist", true, kerrors.IsNotFound(checkTwoSecretError)},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func Test_getUnstructuredResourceSignature(t *testing.T) {
	assetsFolder := getAssetsFolder()
	unstructuredResource, err := getUnstructuredResourceSignature("ltpa", &assetsFolder)
	_, hasKind := unstructuredResource["kind"]
	_, hasAPIVersion := unstructuredResource["apiVersion"]
	tests := []Test{
		{"get unstructured resource signature - get without error", nil, err},
		{"get unstructured resource signature - unstructured resource has kind", true, hasKind},
		{"get unstructured resource signature - unstructured resource has api version", true, hasAPIVersion},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func Test_getUnstructuredResourceSignatureWithInvalidSignature(t *testing.T) {
	testsFolder := getTestsFolder()
	_, err := getUnstructuredResourceSignature("invalid-olo", &testsFolder)
	tests := []Test{
		{"get unstructured resource signature, invalid YAML - get with error", false, err == nil},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func Test_getUnstructuredResourceSignatureWithMissingSignature(t *testing.T) {
	testsFolder := getTestsFolder()
	_, err := getUnstructuredResourceSignature("missing-olo", &testsFolder)
	tests := []Test{
		{"get unstructured resource signature, missing YAML - get with error", false, err == nil},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCreateUnstructuredResourceFromSignature(t *testing.T) {
	testsFolder := getTestsFolder()
	unstructuredLibertyApp, unstructuredLibertyAppName, err := CreateUnstructuredResourceFromSignature("olo", &testsFolder, "olo")
	tests := []Test{
		{"create unstructured resource from signature - no error", nil, err},
		{"create unstructured resource from signature - liberty app kind ", "OpenLibertyApplication", unstructuredLibertyApp.GetKind()},
		{"create unstructured resource from signature - liberty app API version", "apps.openliberty.io/v1", unstructuredLibertyApp.GetAPIVersion()},
		{"create unstructured resource from signature - liberty app name", "olo-managed-olapp", unstructuredLibertyAppName},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCreateUnstructuredResourceFromSignatureWithInvalidSignature(t *testing.T) {
	testsFolder := getTestsFolder()
	unstructuredLibertyApp, unstructuredLibertyAppName, err := CreateUnstructuredResourceFromSignature("invalid-olo", &testsFolder, "olo")
	var nilUnstructuredLibertyApp *unstructured.Unstructured
	tests := []Test{
		{"create unstructured resource from signature, invalid YAML - has error", false, err == nil},
		{"create unstructured resource from signature, invalid YAML - liberty app is nil", nilUnstructuredLibertyApp, unstructuredLibertyApp},
		{"create unstructured resource from signature, invalid YAML - liberty app name empty", "", unstructuredLibertyAppName},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// check when more arguments are provided than replacement tokens in olo-signature.yaml
	unstructuredLibertyApp, unstructuredLibertyAppName, err = CreateUnstructuredResourceFromSignature("olo", &testsFolder, "olo", "one", "two", "three", "four")
	tests = []Test{
		{"create unstructured resource from signature, invalid name replacement 1 - has error", false, err == nil},
		{"create unstructured resource from signature, invalid name replacement 1 - liberty app is nil", nilUnstructuredLibertyApp, unstructuredLibertyApp},
		{"create unstructured resource from signature, invalid name replacement 1 - liberty app name empty", "", unstructuredLibertyAppName},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// check when less arguments are provided than replacement tokens in olo-signature.yaml
	unstructuredLibertyApp, unstructuredLibertyAppName, err = CreateUnstructuredResourceFromSignature("invalid-olo", &testsFolder)
	tests = []Test{
		{"create unstructured resource from signature, invalid name replacement 2 - has error", false, err == nil},
		{"create unstructured resource from signature, invalid name replacement 2 - liberty app is nil", nilUnstructuredLibertyApp, unstructuredLibertyApp},
		{"create unstructured resource from signature, invalid name replacement 2 - liberty app name empty", "", unstructuredLibertyAppName},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCreateUnstructuredResourceListFromSignature(t *testing.T) {
	testsFolder := getTestsFolder()
	unstructuredLibertyAppList, err := CreateUnstructuredResourceListFromSignature("olo", &testsFolder, "olo")
	tests := []Test{
		{"create unstructured resource list from signature - no error", nil, err},
		{"create unstructured resource list from signature - liberty app list kind ", "OpenLibertyApplication", unstructuredLibertyAppList.GetKind()},
		{"create unstructured resource list from signature - liberty app list API version", "apps.openliberty.io/v1", unstructuredLibertyAppList.GetAPIVersion()},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCreateUnstructuredResourceListFromSignatureWithInvalidSignature(t *testing.T) {
	testsFolder := getTestsFolder()
	unstructuredLibertyAppList, err := CreateUnstructuredResourceListFromSignature("invalid-olo", &testsFolder, "olo")
	var nilUnstructuredLibertyAppList *unstructured.UnstructuredList
	tests := []Test{
		{"create unstructured resource list from signature, invalid YAML - has error", false, err == nil},
		{"create unstructured resource list from signature, invalid YAML - liberty app is nil", nilUnstructuredLibertyAppList, unstructuredLibertyAppList},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCreateUnstructuredResourceFromSignatureWithUserError(t *testing.T) {
	testsFolder := getTestsFolder()
	unstructuredLibertyApp, unstructuredLibertyAppName, err := CreateUnstructuredResourceFromSignature("user-error-olo", &testsFolder)
	var nilUnstructuredLibertyApp *unstructured.Unstructured
	tests := []Test{
		{"create unstructured resource from signature, invalid YAML - has error", false, err == nil},
		{"create unstructured resource from signature, invalid YAML - liberty app is nil", nilUnstructuredLibertyApp, unstructuredLibertyApp},
		{"create unstructured resource from signature, invalid YAML - liberty app name empty", "", unstructuredLibertyAppName},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCreateUnstructuredResourceListFromSignatureWithUserError(t *testing.T) {
	testsFolder := getTestsFolder()
	unstructuredLibertyAppList, err := CreateUnstructuredResourceListFromSignature("user-error-olo", &testsFolder)
	var nilUnstructuredLibertyAppList *unstructured.UnstructuredList
	tests := []Test{
		{"create unstructured resource from signature, invalid YAML - has error", false, err == nil},
		{"create unstructured resource from signature, invalid YAML - liberty app is nil", nilUnstructuredLibertyAppList, unstructuredLibertyAppList},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func getUtilsFolder() string {
	cwd, err := os.Getwd()
	if err != nil || !strings.HasSuffix(cwd, "/utils") {
		return "utils"
	}
	return cwd
}

func getAssetsFolder() string {
	return getUtilsFolder() + "/../controllers/assets"
}

func getTestsFolder() string {
	return getUtilsFolder() + "/../controllers/tests"
}

func createMock1LeaderTracker() LeaderTracker {
	return LeaderTracker{
		Name:      "-12345",
		Owner:     "test",
		Path:      "v1_0_0.hello.world",
		PathIndex: "v1_0_0.0",
		Sublease:  "0",
	}
}

func createMock2LeaderTracker() LeaderTracker {
	return LeaderTracker{
		Name:      "-67890",
		Owner:     "test-2",
		Path:      "v1_0_0.hello.world",
		PathIndex: "v1_0_0.1",
		Sublease:  "0",
	}
}

func ignoreSubleases(leaderTracker map[string][]byte) map[string][]byte {
	delete(leaderTracker, ResourceSubleasesKey)
	return leaderTracker
}
