package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	tree "github.com/OpenLiberty/open-liberty-operator/utils/tree"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Validates the resource decision tree YAML and generates the leader tracking state (Secret) for maintaining multiple shared resources
func (r *ReconcileOpenLibertyTrace) reconcileResourceTrackingState(instance *olv1.OpenLibertyTrace, leaderTrackerType string) (lutils.LeaderTrackerMetadataList, error) {
	treeMap, replaceMap, err := tree.ParseDecisionTree(leaderTrackerType, nil)
	if err != nil {
		return nil, err
	}

	latestOperandVersion, err := tree.GetLatestOperandVersion(treeMap, "")
	if err != nil {
		return nil, err
	}

	// persist or create a Secret to store the shared resources' state
	err = r.reconcileLeaderTracker(instance, treeMap, replaceMap, latestOperandVersion, leaderTrackerType, nil)
	if err != nil {
		return nil, err
	}

	if leaderTrackerType == TRACE_RESOURCE_SHARING_FILE_NAME {
		traceMetadataList, err := r.reconcileTraceMetadata(instance, treeMap, latestOperandVersion, nil)
		if err != nil {
			return nil, err
		}
		return traceMetadataList, nil
	}
	return nil, fmt.Errorf("a leaderTrackerType was not provided when running reconcileResourceTrackingState")
}

// If shouldElectNewLeader is set to true, the OpenLibertyTrace instance will be set and returned as the resource leader
// Otherwise, returns the current shared resource leader
func (r *ReconcileOpenLibertyTrace) reconcileLeader(instance *olv1.OpenLibertyTrace, leaderMetadata lutils.LeaderTrackerMetadata, leaderTrackerType string, shouldElectNewLeader bool, removeDanglingResources bool) (string, bool, string, error) {
	leaderTracker, leaderTrackers, err := lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, leaderTrackerType, r.GetClient())
	if err != nil {
		return "", false, "", err
	}
	return r.reconcileLeaderWithState(instance, leaderTracker, leaderTrackers, leaderMetadata, shouldElectNewLeader, removeDanglingResources)
}

func (r *ReconcileOpenLibertyTrace) reconcileLeaderWithState(instance *olv1.OpenLibertyTrace, leaderTracker *corev1.Secret, leaderTrackers *[]lutils.LeaderTracker, leaderMetadata lutils.LeaderTrackerMetadata, shouldElectNewLeader bool, removeDanglingResources bool) (string, bool, string, error) {
	initialLeaderIndex := -1
	for i, tracker := range *leaderTrackers {
		if tracker.Name == leaderMetadata.GetName() {
			initialLeaderIndex = i
		}
	}
	if removeDanglingResources {
		defer func() {
			removeList := []int{}
			n := len(*leaderTrackers)
			for i := range *leaderTrackers {
				j := n - i - 1
				if (*leaderTrackers)[j].Owner == "" {
					removeList = append(removeList, j)
				}
			}
			for _, ri := range removeList {
				*leaderTrackers = append((*leaderTrackers)[:ri], (*leaderTrackers)[ri+1:]...)
			}
			// save the tracker state
			r.SaveLeaderTracker(leaderTracker, leaderTrackers)
		}()
	}

	// if the tracked resource does not exist in resources labels, this instance is leader
	if initialLeaderIndex == -1 {
		if !shouldElectNewLeader {
			return "", false, "", nil
		}
		// clear instance.Name from ownership of any prior resources
		for i := range *leaderTrackers {
			(*leaderTrackers)[i].ClearOwnerIfMatchingAndSharesLastPathParent(instance.Name, leaderMetadata.GetPath())
		}
		// make instance.Name the new leader
		newLeader := lutils.LeaderTracker{
			Name:      leaderMetadata.GetName(),
			Owner:     instance.Name,
			PathIndex: leaderMetadata.GetPathIndex(),
			Path:      leaderMetadata.GetPath(),
			// Sublease:  fmt.Sprint(time.Now().Unix()),
		}
		// append it to the list of leaders
		*leaderTrackers = append(*leaderTrackers, newLeader)
		// save the tracker state
		if err := r.SaveLeaderTracker(leaderTracker, leaderTrackers); err != nil {
			return "", false, "", err
		}
		return instance.Name, true, leaderMetadata.GetPathIndex(), nil
	}
	// otherwise, the resource is being tracked
	// if the leader of the tracked resource is non empty decide whether or not to return the resource owner
	candidateLeader := (*leaderTrackers)[initialLeaderIndex].Owner
	if len(candidateLeader) > 0 {
		// Return this other instance as the leader (the "other" instance could also be this instance)
		// Before returning, if the candidate instance is not this instance, this instance must clean up its old owner references to avoid an resource owner cycle.
		// A resource owner cycle can occur when instance A points to resource A and instance B points to resource B but then both instance A and B swap pointing to each other's resource.
		if candidateLeader != instance.Name {
			// clear instance.Name from ownership of any prior resources and evict the owner if the sublease has expired
			for i := range *leaderTrackers {
				(*leaderTrackers)[i].ClearOwnerIfMatchingAndSharesLastPathParent(instance.Name, leaderMetadata.GetPath())
				// (*leaderTrackers)[i].EvictOwnerIfSubleaseHasExpired()
			}
		}
		// else {
		// candidate is this instance, so renew the sublease
		// (*leaderTrackers)[initialLeaderIndex].RenewSublease()
		// }

		// If the current owner has been evicted, use this instance as the new owner
		currentOwner := (*leaderTrackers)[initialLeaderIndex].Owner
		if currentOwner == "" {
			currentOwner = instance.Name
			(*leaderTrackers)[initialLeaderIndex].SetOwner(currentOwner)
		}
		// save this new owner list
		if err := r.SaveLeaderTracker(leaderTracker, leaderTrackers); err != nil {
			return "", false, "", err
		}
		return currentOwner, currentOwner == instance.Name, (*leaderTrackers)[initialLeaderIndex].PathIndex, nil
	}
	if !shouldElectNewLeader {
		return "", false, "", nil
	}
	// there is either no leader (empty string) or the leader was deleted so now this instance is leader
	pathIndex := ""
	for i := range *leaderTrackers {
		if i == initialLeaderIndex {
			pathIndex = (*leaderTrackers)[i].PathIndex
			(*leaderTrackers)[i].SetOwner(instance.Name)
		} else {
			(*leaderTrackers)[i].ClearOwnerIfMatchingAndSharesLastPathParent(instance.Name, leaderMetadata.GetPath())
		}
	}
	// save this new owner list
	if err := r.SaveLeaderTracker(leaderTracker, leaderTrackers); err != nil {
		return "", false, "", err
	}
	return instance.Name, true, pathIndex, nil
}

func (r *ReconcileOpenLibertyTrace) createNewLeaderTrackerList(instance *olv1.OpenLibertyTrace, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, leaderTrackerType string, assetsFolder *string) (*[]lutils.LeaderTracker, error) {
	var resourcesMatrix []*unstructured.UnstructuredList
	var resourcesRootNameList []string

	leaderTracker := make([]lutils.LeaderTracker, 0)
	if leaderTrackerType == TRACE_RESOURCE_SHARING_FILE_NAME {
		resourcesList, resourceRootName, traceErr := r.GetTraceResources(instance, treeMap, replaceMap, latestOperandVersion, assetsFolder, TRACE_RESOURCE_SHARING_FILE_NAME)
		if traceErr != nil {
			return nil, traceErr
		}
		resourcesMatrix = append(resourcesMatrix, resourcesList)
		resourcesRootNameList = append(resourcesRootNameList, resourceRootName)
	}
	for i, resourcesList := range resourcesMatrix {
		r.UpdateLeaderTrackersFromUnstructuredList(&leaderTracker, resourcesList, treeMap, replaceMap, latestOperandVersion, resourcesRootNameList[i])
	}
	return &leaderTracker, nil
}

// Reconciles the latest LeaderTracker state to be used by the operator
func (r *ReconcileOpenLibertyTrace) reconcileLeaderTracker(instance *olv1.OpenLibertyTrace, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, leaderTrackerType string, assetsFolder *string) error {
	leaderTracker, _, err := lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, leaderTrackerType, r.GetClient())
	// If the Leader Tracker is missing, create from scratch
	if err != nil && kerrors.IsNotFound(err) {
		leaderTracker.Labels[lutils.LeaderVersionLabel] = latestOperandVersion
		leaderTracker.ResourceVersion = ""
		leaderTrackers, err := r.createNewLeaderTrackerList(instance, treeMap, replaceMap, latestOperandVersion, leaderTrackerType, assetsFolder)
		if err != nil {
			return err
		}
		return r.SaveLeaderTracker(leaderTracker, leaderTrackers)
	} else if err != nil {
		return err
	}
	// If the Leader Tracker is outdated, delete it so that it gets recreated in another reconcile
	if leaderTracker.Labels[lutils.LeaderVersionLabel] != latestOperandVersion {
		if err := r.DeleteResource(leaderTracker); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileOpenLibertyTrace) SaveLeaderTracker(leaderTracker *corev1.Secret, trackerList *[]lutils.LeaderTracker) error {
	return r.CreateOrUpdate(leaderTracker, nil, func() error {
		lutils.CustomizeLeaderTracker(leaderTracker, trackerList)
		return nil
	})
}

// Removes the instance as leader if instance is the leader and if no leaders are being tracked then delete the leader tracking Secret
func (r *ReconcileOpenLibertyTrace) RemoveLeader(instance *olv1.OpenLibertyTrace, leaderTracker *corev1.Secret, leaderTrackers *[]lutils.LeaderTracker) error {
	changeDetected := false
	noOwners := true
	// If the instance is being tracked, remove it
	for i := range *leaderTrackers {
		if (*leaderTrackers)[i].ClearOwnerIfMatching(instance.Name) {
			changeDetected = true
		}
		if (*leaderTrackers)[i].Owner != "" {
			noOwners = false
		}
	}
	if noOwners {
		if err := r.DeleteResource(leaderTracker); err != nil {
			return err
		}
	} else if changeDetected {
		if err := r.SaveLeaderTracker(leaderTracker, leaderTrackers); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileOpenLibertyTrace) UpdateLeaderTrackersFromUnstructuredList(leaderTrackers *[]lutils.LeaderTracker, resourceList *unstructured.UnstructuredList, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, resourceRootName string) error {
	for i, resource := range resourceList.Items {
		labelsMap, _, err := unstructured.NestedMap(resource.Object, "metadata", "labels")
		if err != nil {
			return err
		}
		if pathIndexInterface, found := labelsMap[lutils.ResourcePathIndexLabel]; found {
			pathIndex := pathIndexInterface.(string)
			// Skip this resource if path index does not contain a period separating delimeter
			if !strings.Contains(pathIndex, ".") {
				continue
			}
			labelVersionArray := strings.Split(pathIndex, ".")
			// Skip this resource if the path index is not a tuple representing the version and index
			if len(labelVersionArray) != 2 {
				continue
			}
			leader := lutils.LeaderTracker{
				PathIndex: pathIndex,
			}
			indexIntVal, _ := strconv.ParseInt(labelVersionArray[1], 10, 64)
			path, pathErr := tree.GetPathFromLeafIndex(treeMap, labelVersionArray[0], int(indexIntVal))
			// If path comes from a different operand version, the path needs to be upgraded/downgraded to the latestOperandVersion
			if pathErr == nil && labelVersionArray[0] != latestOperandVersion {
				// If user error has occurred, based on whether or not a dev deleted the decision tree structure of an older version
				// allow this condition to error (when err != nil) so that a deleted (older) revision of the decision tree that may be missing
				// won't halt the operator when the ReplacePath validation is performed
				if path, err := tree.ReplacePath(path, latestOperandVersion, treeMap, replaceMap); err == nil {
					newPathIndex := strings.Split(path, ".")[0] + "." + strconv.FormatInt(int64(tree.GetLeafIndex(treeMap, path)), 10)
					leader.PathIndex = newPathIndex
					leader.Path = path
					// the path may have changed so the path index reference needs to be updated directly in the resource
					if err := r.CreateOrUpdate(&resourceList.Items[i], nil, func() error {
						labelsMap, _, err := unstructured.NestedMap(resourceList.Items[i].Object, "metadata", "labels")
						if err != nil {
							return err
						}
						labelsMap[lutils.ResourcePathIndexLabel] = newPathIndex
						if err := unstructured.SetNestedMap(resourceList.Items[i].Object, labelsMap, "metadata", "labels"); err != nil {
							return err
						}
						return nil
					}); err != nil {
						return err
					}
				}
			} else if pathErr == nil { // only update the path metadata if this operator's decision tree structure recognizes the resource
				leader.Path = path
			} else {
				// A valid decision tree path could not be found, so it will not be used by the operator and this resource will not be tracked
				continue
			}
			nameString, _, err := unstructured.NestedString(resource.Object, "metadata", "name")
			if err != nil {
				return err
			}
			leader.Name = nameString[len(resourceRootName):]
			leader.EvictOwner()
			lutils.InsertIntoSortedLeaderTrackers(leaderTrackers, &leader)
		}
	}
	return nil
}

func (r *ReconcileOpenLibertyTrace) RemoveLeaderTrackerReference(instance *olv1.OpenLibertyTrace, resourceSharingFileName string) error {
	leaderTracker, leaderTrackers, err := lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, resourceSharingFileName, r.GetClient())
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return r.RemoveLeader(instance, leaderTracker, leaderTrackers)
}

// Search the cluster namespace for existing Trace CRs
func (r *ReconcileOpenLibertyTrace) GetTraceResources(instance *olv1.OpenLibertyTrace, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, assetsFolder *string, fileName string) (*unstructured.UnstructuredList, string, error) {
	traceResourceList, _, err := lutils.CreateUnstructuredResourceListFromSignature(fileName, assetsFolder, OperatorShortName)
	if err != nil {
		return nil, "", err
	}
	if err := r.GetClient().List(context.TODO(), traceResourceList, client.InNamespace(instance.GetNamespace())); err != nil {
		return nil, "", err
	}
	return traceResourceList, "", nil
}
