package utils

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	"github.com/application-stacks/runtime-component-operator/common"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var LeaderTrackerMutexes *sync.Map

func init() {
	LeaderTrackerMutexes = &sync.Map{}
}

// Leader tracking constants
const ResourcesKey = "names"
const ResourceOwnersKey = "owners"
const ResourcePathsKey = "paths"
const ResourcePathIndicesKey = "pathIndices"

// const ResourceSubleasesKey = "subleases"

const LibertyURI = "openlibertyapplications.apps.openliberty.io"
const LeaderVersionLabel = LibertyURI + "/leader-version"
const ResourcePathIndexLabel = LibertyURI + "/resource-path-index"

const ResourceSuffixLength = 5

func GetLastRotationLabelKey(sharedResourceName string) string {
	return LibertyURI + "/" + sharedResourceName + "-last-rotation"
}

func GetTrackedResourceName(sharedResourceName string) string {
	return kebabToCamelCase(sharedResourceName) + "TrackedResourceName"
}

type LeaderTracker struct {
	Name      string
	Owner     string
	PathIndex string
	Path      string
	Sublease  string
}

type LeaderTrackerMetadata interface {
	GetKind() string
	GetAPIVersion() string
	GetName() string
	GetPath() string
	GetPathIndex() string
}

type LeaderTrackerMetadataList interface {
	GetItems() []LeaderTrackerMetadata
}

func RemoveLeaderTracker(leaderTracker *[]LeaderTracker, i int) bool {
	if leaderTracker == nil {
		return false
	}
	if i >= len(*leaderTracker) {
		return false
	}
	*leaderTracker = append((*leaderTracker)[:i], (*leaderTracker)[i+1:]...)
	return true
}

func (tracker *LeaderTracker) RenewSublease() bool {
	if tracker == nil {
		return false
	}
	tracker.Sublease = fmt.Sprint(time.Now().Unix())
	return true
}

func (tracker *LeaderTracker) SetOwner(instance string) bool {
	if tracker == nil {
		return false
	}
	tracker.Owner = instance
	return true
	// return tracker.RenewSublease()
}

// Clears the LeaderTracker owner if it matches instance, returning true if the LeaderTracker has changed and false otherwise
func (tracker *LeaderTracker) ClearOwnerIfMatching(instance string) bool {
	if tracker == nil {
		return false
	}
	if tracker.Owner == instance {
		tracker.Owner = ""
		return true
	}
	return false
}

func (tracker *LeaderTracker) ClearOwnerIfMatchingAndSharesLastPathParent(instance string, path string) bool {
	if tracker == nil || !strings.Contains(path, ".") || !strings.Contains(tracker.Path, ".") {
		return false
	}
	pathArr := strings.Split(path, ".")
	trackerPathArr := strings.Split(tracker.Path, ".")

	if tracker.Owner == instance && strings.Join(pathArr[:len(pathArr)-1], ".") == strings.Join(trackerPathArr[:len(trackerPathArr)-1], ".") {
		tracker.Owner = ""
		return true
	}
	return false
}

// Removes the Owner and Sublease attribute from LeaderTracker to indicate the resource is no longer being tracked
func (tracker *LeaderTracker) EvictOwner() bool {
	if tracker == nil {
		return false
	}
	tracker.Owner = ""
	// tracker.Sublease = ""
	return true
}

func (tracker *LeaderTracker) EvictOwnerIfSubleaseHasExpired() bool {
	if tracker == nil {
		return false
	}
	// Evict if the sublease could not be parsed
	then, err := strconv.ParseInt(tracker.Sublease, 10, 64)
	if err != nil {
		return tracker.EvictOwner()
	}
	// Evict if the sublease has surpassed the renew time
	now := time.Now().Unix()
	if now-then > 20 {
		return tracker.EvictOwner()
	}
	return false
}

func InsertIntoSortedLeaderTrackers(leaderTrackers *[]LeaderTracker, newLeader *LeaderTracker) {
	insertIndex := -1
	for i, leader := range *leaderTrackers {
		if strings.Compare(newLeader.Name, leader.Name) < 0 {
			insertIndex = i
		}
	}
	if insertIndex == -1 {
		*leaderTrackers = append(*leaderTrackers, *newLeader)
	} else {
		*leaderTrackers = append(*leaderTrackers, LeaderTracker{})
		copy((*leaderTrackers)[insertIndex+1:], (*leaderTrackers)[insertIndex:])
		(*leaderTrackers)[insertIndex] = *newLeader
	}
}

func CustomizeLeaderTracker(leaderTracker *common.LockedBufferSecret, trackerList *[]LeaderTracker) {
	if trackerList == nil {
		if leaderTracker.LockedData == nil {
			leaderTracker.LockedData = make(common.SecretMap)
		}
		leaderTracker.LockedData.Set(ResourceOwnersKey, []byte(""))
		leaderTracker.LockedData.Set(ResourcesKey, []byte(""))
		leaderTracker.LockedData.Set(ResourcePathIndicesKey, []byte(""))
		leaderTracker.LockedData.Set(ResourcePathsKey, []byte(""))
		return
	}
	if leaderTracker.LockedData == nil {
		leaderTracker.LockedData = make(common.SecretMap)
	}

	// pre-calculate size to prevent array reallocation when building the arrays
	ownersSize, namesSize, pathIndicesSize, pathsSize := 0, 0, 0, 0
	n := len(*trackerList)
	for i, tracker := range *trackerList {
		ownersSize += len(tracker.Owner)
		namesSize += len(tracker.Name)
		pathIndicesSize += len(tracker.PathIndex)
		pathsSize += len(tracker.Path)
		if i < n-1 {
			ownersSize++
			namesSize++
			pathIndicesSize++
			pathsSize++
		}
	}

	ownersBytes := make([]byte, 0, ownersSize)
	namesBytes := make([]byte, 0, namesSize)
	pathIndicesBytes := make([]byte, 0, pathIndicesSize)
	pathsBytes := make([]byte, 0, pathsSize)

	for i, tracker := range *trackerList {
		ownersBytes = append(ownersBytes, []byte(tracker.Owner)...)
		namesBytes = append(namesBytes, []byte(tracker.Name)...)
		pathIndicesBytes = append(pathIndicesBytes, []byte(tracker.PathIndex)...)
		pathsBytes = append(pathsBytes, []byte(tracker.Path)...)
		if i < n-1 {
			ownersBytes = append(ownersBytes, ',')
			namesBytes = append(namesBytes, ',')
			pathIndicesBytes = append(pathIndicesBytes, ',')
			pathsBytes = append(pathsBytes, ',')
		}
	}
	leaderTracker.LockedData.Set(ResourceOwnersKey, ownersBytes)
	leaderTracker.LockedData.Set(ResourcesKey, namesBytes)
	leaderTracker.LockedData.Set(ResourcePathIndicesKey, pathIndicesBytes)
	leaderTracker.LockedData.Set(ResourcePathsKey, pathsBytes)
}

func GetLeaderTracker(instance *olv1.OpenLibertyApplication, operatorShortName string, leaderTrackerType string, client client.Client) (*common.LockedBufferSecret, *[]LeaderTracker, error) {
	leaderMutex, mutexFound := LeaderTrackerMutexes.Load(leaderTrackerType)
	if !mutexFound {
		return nil, nil, fmt.Errorf("Could not retrieve %s leader tracker's mutex when attempting to get. Exiting.", leaderTrackerType)
	}
	leaderMutex.(*sync.Mutex).Lock()
	defer leaderMutex.(*sync.Mutex).Unlock()

	leaderTrackerName := operatorShortName + "-managed-leader-tracking-" + leaderTrackerType
	leaderTracker, err := common.GetSecret(client, leaderTrackerName, instance.GetNamespace())

	if err == nil {
		// Get the actual Secret object to access its Data field
		actualSecret := &corev1.Secret{}
		if getErr := client.Get(context.TODO(), types.NamespacedName{Name: leaderTrackerName, Namespace: instance.GetNamespace()}, actualSecret); getErr == nil {
			if leaderTracker.LockedData == nil {
				leaderTracker.LockedData = make(common.SecretMap)
			}
			// Copy Data -> LockedData
			for key, value := range actualSecret.Data {
				leaderTracker.LockedData.Set(key, value)
			}
		}
	}

	if err != nil {
		leaderTracker.Labels = GetRequiredLabels(leaderTracker.Name, "")
		// return a default leaderTracker
		return leaderTracker, nil, err
	}
	// Create the LeaderTracker array
	leaderTrackers := make([]LeaderTracker, 0)
	owners, ownersFound := leaderTracker.LockedData.Get(ResourceOwnersKey)
	names, namesFound := leaderTracker.LockedData.Get(ResourcesKey)
	pathIndices, pathIndicesFound := leaderTracker.LockedData.Get(ResourcePathIndicesKey)
	paths, pathsFound := leaderTracker.LockedData.Get(ResourcePathsKey)

	// subleases, subleasesFound := leaderTracker.LockedData.Get(ResourceSubleasesKey)
	// If flags are out of sync, delete the leader tracker
	if ownersFound != namesFound || pathIndicesFound != pathsFound || namesFound != pathIndicesFound { // || pathIndicesFound != subleasesFound {
		leaderTrackerObject := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: leaderTracker.GetName(), Namespace: leaderTracker.GetNamespace()}}
		if err := client.Delete(context.TODO(), leaderTrackerObject); err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("the resource tracker is out of sync and has been deleted")
	}
	if len(owners) == 0 && len(names) == 0 && len(pathIndices) == 0 && len(paths) == 0 { // && len(subleases) == 0 {
		return leaderTracker, &leaderTrackers, nil
	}
	ownersList := GetCommaSeparatedArray(string(owners))
	namesList := GetCommaSeparatedArray(string(names))
	pathIndicesList := GetCommaSeparatedArray(string(pathIndices))
	pathsList := GetCommaSeparatedArray(string(paths))
	// subleasesList := GetCommaSeparatedArray(string(subleases))
	numOwners := len(ownersList)
	numNames := len(namesList)
	numPathIndices := len(pathIndicesList)
	numPaths := len(pathsList)
	// numSubleases := len(subleasesList)
	// check for array length equivalence
	if numOwners != numNames || numNames != numPathIndices || numPathIndices != numPaths { // || numPaths != numSubleases {
		leaderTrackerObject := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: leaderTracker.GetName(), Namespace: leaderTracker.GetNamespace()}}
		if err := client.Delete(context.TODO(), leaderTrackerObject); err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("the resource tracker does not have array length equivalence and has been deleted")
	}
	// populate the leader trackers array
	for i := range ownersList {
		leaderTrackers = append(leaderTrackers, LeaderTracker{
			Owner:     string(ownersList[i]),
			Name:      string(namesList[i]),
			PathIndex: string(pathIndicesList[i]),
			Path:      string(pathsList[i]),
			// Sublease:  string(subleasesList[i]),
		})
	}
	return leaderTracker, &leaderTrackers, nil
}

func getUnstructuredResourceSignature(leaderTrackerType string, assetsPath *string) (map[string]interface{}, error) {
	var folderPath string
	if assetsPath != nil {
		folderPath = *assetsPath
	} else {
		folderPath = "internal/controller/assets"
	}
	signature, err := os.ReadFile(folderPath + "/" + leaderTrackerType + "-signature.yaml")
	if err != nil {
		return nil, err
	}
	resourceSignatureYAML := make(map[string]interface{})
	err = yaml.Unmarshal(signature, resourceSignatureYAML)
	if err != nil {
		return nil, err
	}
	return resourceSignatureYAML, nil
}

func CreateUnstructuredResourceFromSignature(leaderTrackerType string, assetsFolder *string, args ...string) (*unstructured.Unstructured, string, error) {
	resourceSignatureYAML, err := getUnstructuredResourceSignature(leaderTrackerType, assetsFolder)
	if err != nil {
		return nil, "", err
	}
	apiVersion, apiVersionFound := resourceSignatureYAML["apiVersion"]
	kind, kindFound := resourceSignatureYAML["kind"]
	name, nameFound := resourceSignatureYAML["name"]
	// rootName, rootNameFound := resourceSignatureYAML["rootName"]
	if !apiVersionFound || !kindFound || !nameFound {
		return nil, "", fmt.Errorf("the operator bundled the shared resource '%s' with an invalid signature", leaderTrackerType)
	}
	sharedResource := &unstructured.Unstructured{}
	sharedResource.SetKind(kind.(string))
	sharedResource.SetAPIVersion(apiVersion.(string))
	sharedResourceName, err := parseUnstructuredResourceName(leaderTrackerType, name.(string), args...)
	if err != nil {
		return nil, "", err
	}
	return sharedResource, sharedResourceName, nil
}

func CreateUnstructuredResourceListFromSignature(leaderTrackerType string, assetsFolder *string, args ...string) (*unstructured.UnstructuredList, string, error) {
	resourceSignatureYAML, err := getUnstructuredResourceSignature(leaderTrackerType, assetsFolder)
	if err != nil {
		return nil, "", err
	}
	apiVersion, apiVersionFound := resourceSignatureYAML["apiVersion"]
	kind, kindFound := resourceSignatureYAML["kind"]
	rootName := resourceSignatureYAML["rootName"]
	if !apiVersionFound || !kindFound {
		return nil, "", fmt.Errorf("the operator bundled the shared resource '%s' with an invalid signature", leaderTrackerType)
	}
	sharedResourceList := &unstructured.UnstructuredList{}
	sharedResourceList.SetKind(kind.(string))
	sharedResourceList.SetAPIVersion(apiVersion.(string))

	rootName, rootNameFound := resourceSignatureYAML["rootName"]
	sharedResourceRootName := ""
	if rootNameFound {
		unstructuredResourceRootName, err := parseUnstructuredResourceName(leaderTrackerType, rootName.(string), args[0])
		if err != nil {
			return nil, "", err
		}
		sharedResourceRootName = unstructuredResourceRootName
	}
	return sharedResourceList, sharedResourceRootName, nil
}

// Returns the name of the unstructured resource from the leaderTrackerType signature by parsing and replacing string arguments in the args list
func parseUnstructuredResourceName(leaderTrackerType string, nameStr string, args ...string) (string, error) {
	for i, replacementString := range args {
		replaceToken := fmt.Sprintf("{%d}", i)
		if strings.Contains(nameStr, replaceToken) {
			nameStr = strings.ReplaceAll(nameStr, replaceToken, replacementString)
		} else {
			return "", fmt.Errorf("the operator bundled the shared resource '%s' with an invalid signature; parseUnstructuredResourceName len(args) does not match the number of replacement tokens in the provided signature", leaderTrackerType)
		}
	}
	return nameStr, nil
}
