package controller

import (
	"fmt"
	"sync"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	tree "github.com/OpenLiberty/open-liberty-operator/utils/tree"
)

const TRACE_RESOURCE_SHARING_FILE_NAME = "trace"

func init() {
	lutils.LeaderTrackerMutexes.Store(TRACE_RESOURCE_SHARING_FILE_NAME, &sync.Mutex{})
}

func (r *ReconcileOpenLibertyTrace) reconcileTraceMetadata(instance *olv1.OpenLibertyTrace, treeMap map[string]interface{}, latestOperandVersion string, assetsFolder *string) (lutils.LeaderTrackerMetadataList, error) {
	metadataList := &lutils.TraceMetadataList{}
	metadataList.Items = []lutils.LeaderTrackerMetadata{}

	// During runtime, the OpenLibertyApplication instance will decide what Trace related resources to track by populating arrays of pathOptions and pathChoices
	pathOptionsList, pathChoicesList := r.getTracePathOptionsAndChoices(instance, latestOperandVersion)

	for i := range pathOptionsList {
		metadata := &lutils.TraceMetadata{}
		pathOptions := pathOptionsList[i]
		pathChoices := pathChoicesList[i]

		// convert the path options and choices into a labelString, for a path of length n, the labelString is
		// constructed as a weaved array in format "<pathOptions[0]>.<pathChoices[0]>.<pathOptions[1]>.<pathChoices[1]>...<pathOptions[n-1]>.<pathChoices[n-1]>"
		labelString, err := tree.GetLabelFromDecisionPath(latestOperandVersion, pathOptions, pathChoices)
		if err != nil {
			return metadataList, err
		}

		// validate that the decision path such as "v1_4_0.managePasswordEncryption.<pathChoices[n-1]>" is a valid subpath in treeMap
		// an error here indicates a build time error created by the operator developer or pollution of the ltpa-decision-tree.yaml
		// Note: validSubPath is a substring of labelString and a valid path within treeMap; it will always hold that len(validSubPath) <= len(labelString)
		// Also, validSubPath will return wildcard characters as '*' and will not output the full entry provided from labelString so that reverse lookup with GetLeafIndex is possible
		validSubPath, err := tree.CanTraverseTree(treeMap, labelString, true)
		if err != nil {
			return metadataList, err
		}

		// retrieve the Trace leader tracker to re-use an existing name or to create a new metadata.Name
		leaderTracker, _, err := lutils.GetLeaderTracker(instance.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME, r.GetClient())
		if err != nil {
			return metadataList, err
		}

		// if the leaderTracker is on a mismatched version, wait for a subsequent reconcile loop to re-create the leader tracker
		if leaderTracker.Labels[lutils.LeaderVersionLabel] != latestOperandVersion {
			return metadataList, fmt.Errorf("waiting for the Leader Tracker to be updated")
		}

		// to avoid limitation with Kubernetes label values having a max length of 63, translate validSubPath into a path index
		pathIndex := tree.GetLeafIndex(treeMap, validSubPath)
		versionedPathIndex := fmt.Sprintf("%s.%d", latestOperandVersion, pathIndex)
		if n := len(pathChoices); n > 0 {
			metadata.Path = validSubPath
			metadata.PathIndex = versionedPathIndex
			metadata.Name = pathChoices[n-1] // at least for v1_4_2, pathChoices[n-1] will be the wildcard entry * representing the name
			metadataList.Items = append(metadataList.Items, metadata)
		}
	}
	return metadataList, nil
}

func (r *ReconcileOpenLibertyTrace) getTracePathOptionsAndChoices(instance *olv1.OpenLibertyTrace, latestOperandVersion string) ([][]string, [][]string) {
	var pathOptionsList, pathChoicesList [][]string
	if latestOperandVersion == "v1_4_2" {
		// Generate a path option/choice for a leader to manage the Trace CR instance's pods
		pathOptions := []string{"name"}                // ordering matters, it must follow the nodes of the Trace decision tree in trace-decision-tree.yaml
		pathChoices := []string{instance.Spec.PodName} // wildcard entry can be provided as any string
		pathOptionsList = append(pathOptionsList, pathOptions)
		pathChoicesList = append(pathChoicesList, pathChoices)

		prevPodName := instance.GetStatus().GetOperatedResource().GetOperatedResourceName()
		if instance.Spec.PodName != prevPodName && prevPodName != "" {
			pathOptions := []string{"name"}
			pathChoices := []string{prevPodName}
			pathOptionsList = append(pathOptionsList, pathOptions)
			pathChoicesList = append(pathChoicesList, pathChoices)
		}
	}
	return pathOptionsList, pathChoicesList
}
