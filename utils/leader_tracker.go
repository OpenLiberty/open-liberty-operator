package utils

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Leader tracking constants
const ResourcesKey = "names"
const ResourceOwnersKey = "owners"
const ResourcePathsKey = "paths"
const ResourcePathIndicesKey = "pathIndices"
const ResourceSubleasesKey = "subleases"

const LeaderVersionLabel = "openlibertyapplications.apps.openliberty.io/leader-version"
const ResourcePathIndexLabel = "openlibertyapplications.apps.openliberty.io/resource-path-index"

const ResourceSuffixLength = 5

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
	return tracker.RenewSublease()
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

// Removes the Owner and Sublease attribute from LeaderTracker to indicate the resource is no longer being tracked
func (tracker *LeaderTracker) EvictOwner() bool {
	if tracker == nil {
		return false
	}
	tracker.Owner = ""
	tracker.Sublease = ""
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

func CustomizeLeaderTracker(leaderTracker *corev1.Secret, trackerList *[]LeaderTracker) {
	if trackerList == nil {
		leaderTracker.Data = make(map[string][]byte)
		leaderTracker.Data[ResourceOwnersKey] = []byte("")
		leaderTracker.Data[ResourcesKey] = []byte("")
		leaderTracker.Data[ResourcePathIndicesKey] = []byte("")
		leaderTracker.Data[ResourcePathsKey] = []byte("")
		leaderTracker.Data[ResourceSubleasesKey] = []byte("")
		return
	}
	leaderTracker.Data = make(map[string][]byte)
	owners, names, pathIndices, paths, subleases := "", "", "", "", ""
	n := len(*trackerList)
	for i, tracker := range *trackerList {
		owners += tracker.Owner
		names += tracker.Name
		pathIndices += tracker.PathIndex
		paths += tracker.Path
		subleases += tracker.Sublease
		if i < n-1 {
			owners += ","
			names += ","
			pathIndices += ","
			paths += ","
			subleases += ","
		}
	}
	leaderTracker.Data[ResourceOwnersKey] = []byte(owners)
	leaderTracker.Data[ResourcesKey] = []byte(names)
	leaderTracker.Data[ResourcePathIndicesKey] = []byte(pathIndices)
	leaderTracker.Data[ResourcePathsKey] = []byte(paths)
	leaderTracker.Data[ResourceSubleasesKey] = []byte(subleases)
}

func GetLeaderTracker(instance *olv1.OpenLibertyApplication, operatorShortName string, leaderTrackerType string, client client.Client) (*corev1.Secret, *[]LeaderTracker, error) {
	leaderTracker := &corev1.Secret{}
	leaderTracker.Name = operatorShortName + "-managed-leader-tracking-" + leaderTrackerType
	leaderTracker.Namespace = instance.GetNamespace()
	leaderTracker.Labels = GetRequiredLabels(leaderTracker.Name, "")
	if err := client.Get(context.TODO(), types.NamespacedName{Name: leaderTracker.Name, Namespace: leaderTracker.Namespace}, leaderTracker); err != nil {
		// return a default leaderTracker
		return leaderTracker, nil, err
	}
	// Create the LeaderTracker array
	leaderTrackers := make([]LeaderTracker, 0)
	owners, ownersFound := leaderTracker.Data[ResourceOwnersKey]
	names, namesFound := leaderTracker.Data[ResourcesKey]
	pathIndices, pathIndicesFound := leaderTracker.Data[ResourcePathIndicesKey]
	paths, pathsFound := leaderTracker.Data[ResourcePathsKey]
	subleases, subleasesFound := leaderTracker.Data[ResourceSubleasesKey]
	// If flags are out of sync, delete the leader tracker
	if ownersFound != namesFound || pathIndicesFound != pathsFound || namesFound != pathIndicesFound || pathIndicesFound != subleasesFound {
		if err := client.Delete(context.TODO(), leaderTracker); err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("the resource tracker is out of sync and has been deleted")
	}
	if len(owners) == 0 && len(names) == 0 && len(pathIndices) == 0 && len(paths) == 0 && len(subleases) == 0 {
		return leaderTracker, &leaderTrackers, nil
	}
	ownersList := GetCommaSeparatedArray(string(owners))
	namesList := GetCommaSeparatedArray(string(names))
	pathIndicesList := GetCommaSeparatedArray(string(pathIndices))
	pathsList := GetCommaSeparatedArray(string(paths))
	subleasesList := GetCommaSeparatedArray(string(subleases))
	numOwners := len(ownersList)
	numNames := len(namesList)
	numPathIndices := len(pathIndicesList)
	numPaths := len(pathsList)
	numSubleases := len(subleasesList)
	// check for array length equivalence
	if numOwners != numNames || numNames != numPathIndices || numPathIndices != numPaths || numPaths != numSubleases {
		if err := client.Delete(context.TODO(), leaderTracker); err != nil {
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
			Sublease:  string(subleasesList[i]),
		})
	}
	return leaderTracker, &leaderTrackers, nil
}

func getUnstructuredResourceSignature(leaderTrackerType string, assetsPath *string) (map[string]interface{}, error) {
	var folderPath string
	if assetsPath != nil {
		folderPath = *assetsPath
	} else {
		folderPath = "controllers/assets"
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
	if !apiVersionFound || !kindFound || !nameFound {
		return nil, "", fmt.Errorf("the operator bundled the shared resource '" + leaderTrackerType + "' with an invalid signature")
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

func CreateUnstructuredResourceListFromSignature(leaderTrackerType string, assetsFolder *string, args ...string) (*unstructured.UnstructuredList, error) {
	resourceSignatureYAML, err := getUnstructuredResourceSignature(leaderTrackerType, assetsFolder)
	if err != nil {
		return nil, err
	}
	apiVersion, apiVersionFound := resourceSignatureYAML["apiVersion"]
	kind, kindFound := resourceSignatureYAML["kind"]
	if !apiVersionFound || !kindFound {
		return nil, fmt.Errorf("the operator bundled the shared resource '" + leaderTrackerType + "' with an invalid signature")
	}
	sharedResourceList := &unstructured.UnstructuredList{}
	sharedResourceList.SetKind(kind.(string))
	sharedResourceList.SetAPIVersion(apiVersion.(string))
	return sharedResourceList, nil
}

// Returns the name of the unstructured resource from the leaderTrackerType signature by parsing and replacing string arguments in the args list
func parseUnstructuredResourceName(leaderTrackerType string, nameStr string, args ...string) (string, error) {
	for i, replacementString := range args {
		replaceToken := fmt.Sprintf("{%d}", i)
		if strings.Contains(nameStr, replaceToken) {
			nameStr = strings.ReplaceAll(nameStr, replaceToken, replacementString)
		} else {
			return "", fmt.Errorf("the operator bundled the shared resource '" + leaderTrackerType + "' with an invalid signature; parseUnstructuredResourceName len(args) does not match the number of replacement tokens in the provided signature")
		}
	}
	return nameStr, nil
}
