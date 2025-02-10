package tree

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	"gopkg.in/yaml.v3"
)

func castMap(someInterface interface{}) (map[string]interface{}, bool) {
	theMap, isMap := someInterface.(map[string]interface{})
	return theMap, isMap
}

func castList(someInterface interface{}) ([]interface{}, bool) {
	theList, isList := someInterface.([]interface{})
	return theList, isList
}

func castBool(someInterface interface{}) (bool, bool) {
	theBool, isBool := someInterface.(bool)
	return theBool, isBool
}

func castString(someInterface interface{}) (string, bool) {
	theString, isString := someInterface.(string)
	return theString, isString
}

func castReplaceVersionMap(replaceVersionMap interface{}) (map[string]map[string]string, error) {
	vmap := make(map[string]map[string]string)
	if castedVersionMap, isMap := castMap(replaceVersionMap); isMap {
		for versionTag, labelMap := range castedVersionMap {
			if castedLabelMap, isMap := castMap(labelMap); isMap {
				if theLabelReplaceMap, errLabelReplaceList := castLabelReplaceMap(castedLabelMap); errLabelReplaceList == nil {
					vmap[versionTag] = theLabelReplaceMap
				} else {
					return vmap, errLabelReplaceList
				}
			} else {
				return vmap, fmt.Errorf("castReplaceVersionMap: a value in the castedVersionMap is not a map")
			}
		}

	} else {
		return vmap, fmt.Errorf("castReplaceVersionMap: the replaceVersionMap is not a map")
	}
	return vmap, nil
}

// Strictly casts interface{} to map[string]string returning a list of key-value pairs representing labels
// Example: interface{"": "v1_4_0.managePasswordEncryption:false"} -> map[string]string{"": "v1_4_0.managePasswordEncryption:false"}
func castLabelReplaceMap(replaceMap interface{}) (map[string]string, error) {
	labels := make(map[string]string)
	if theMap, isMap := castMap(replaceMap); isMap {
		for k, v := range theMap {
			if theString, isString := castString(v); isString {
				labels[k] = theString
			} else {
				return labels, fmt.Errorf("castLabelReplaceMap: a value in the replaceMap is not a string")
			}
		}
	} else {
		return labels, fmt.Errorf("castLabelReplaceMap: an element of the replaceMap is not a map")
	}
	return labels, nil
}

func CastReplaceMap(decisionTreeYAML map[string]interface{}) (map[string]map[string]string, error) {
	vmap := map[string]map[string]string{}
	if replaceMap, found := decisionTreeYAML["replace"]; found {
		if castedReplaceMap, isMap := castMap(replaceMap); isMap {
			if vmap, errValidVMapList := castReplaceVersionMap(castedReplaceMap); errValidVMapList == nil {
				return vmap, nil
			} else {
				return vmap, errValidVMapList
			}
		}
	}
	return vmap, fmt.Errorf("could not find element '.replace' in LTPA decision tree YAML")
}

func CastTreeMap(decisionTreeYAML map[string]interface{}) (map[string]interface{}, error) {
	if treeMap, found := decisionTreeYAML["tree"]; found {
		if castedTreeMap, isMap := castMap(treeMap); isMap {
			return castedTreeMap, nil
		}
		return nil, fmt.Errorf("could not cast '.tree' into a map[string]interface{}")
	}
	return nil, fmt.Errorf("could not find element '.tree' in decision tree YAML")
}

func ValidateMaps(treeMap map[string]interface{}, replaceMap map[string]map[string]string) error {
	// 0. For each version in treeMap, there must be at least one leaf
	for version := range treeMap {
		_, err := GetPathFromLeafIndex(treeMap, version, 0)
		if err != nil {
			return fmt.Errorf("could not validate maps: version " + version + " in .tree does not have a single terminating leaf")
		}
	}

	// 1. replaceMap keys must exist in treeMap
	for replaceVersion := range replaceMap {
		if _, found := treeMap[replaceVersion]; !found {
			return fmt.Errorf("could not validate maps: a key in .replace does not exist in .tree")
		}
	}
	// 2. replaceMap label key-values should be able to walk treeMap
	for _, replaceLabels := range replaceMap {
		for k, v := range replaceLabels {
			if _, err := CanTraverseTreeStrict(treeMap, k); err != nil {
				return err
			}
			if _, err := CanTraverseTreeStrict(treeMap, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func CanTraverseTreeStrict(treeMap map[string]interface{}, path string) (string, error) {
	return CanTraverseTree(treeMap, path, false)
}

// returns the valid subpath of treeMap and potential error
func CanTraverseTree(treeMap map[string]interface{}, path string, allowSubPaths bool) (string, error) {
	n := len(path)
	if n == 0 {
		return path, nil
	}
	dotLoc := strings.LastIndex(path, ".")
	if dotLoc < 0 || dotLoc+1 >= n {
		return "", fmt.Errorf("the path '" + path + "' is not a valid key-value pair")
	}
	pathKey := path[:dotLoc]
	pathValue := path[dotLoc+1:]
	if !strings.Contains(pathKey, ".") {
		if _, found := treeMap[pathKey]; found {
			return "", nil
		}
		return "", fmt.Errorf("key '" + path + "' does not exist in .tree")
	}
	// pathKey must contain a ".", so j >= 2
	paths := strings.Split(pathKey, ".")
	i := 0
	j := len(paths)
	currMap := treeMap
	currPathString := ""
	for i < j-1 {
		nextMap, found := currMap[paths[i]]
		if !found {
			return currPathString, fmt.Errorf("while traversing path '" + pathKey + "' the element '" + paths[i] + "' could not be found in .tree")
		}
		currPathString += paths[i] + "."
		if castedNextMap, isMap := castMap(nextMap); isMap {
			currMap = castedNextMap
		} else {
			if allowSubPaths {
				// Reached an element that is not a map, so it could be the end of a subpath. Just need to check that it terminates with a boolean or string or valid list element
				// bool check
				if castedBool, isBool := castBool(nextMap); isBool {
					return currPathString + strconv.FormatBool(castedBool), nil
				}
				// string check
				if castedString, isString := castString(nextMap); isString {
					return currPathString + castedString, nil
				}
				// list check
				if castedList, isList := castList(nextMap); isList {
					element := ""
					if i+1 < len(paths) {
						element = paths[i+1]
					} else if i+1 == len(paths) {
						element = pathValue
					}
					return getCastedListElement(castedList, currPathString[:len(currPathString)-1], element)
				}
				return currPathString, fmt.Errorf("while traversing path '" + currPathString + "' the last element was not a boolean, string or list")
			}
			return currPathString, fmt.Errorf("while traversing path '" + pathKey + "' the element '" + paths[i] + "' did not extend to another map[string]interface{} in .tree")
		}
		i += 1
	}
	// i == j-1 (last map element)
	mapLastElement, found := currMap[paths[i]]
	if !found {
		return currPathString, fmt.Errorf("while traversing path '" + pathKey + "' the last element could not be found in .tree")
	}
	currPathString += paths[i]
	// bool check
	if castedBool, isBool := castBool(mapLastElement); isBool {
		if pathValue == "true" && castedBool || pathValue == "false" && !castedBool {
			return currPathString + "." + strconv.FormatBool(castedBool), nil
		}
		return currPathString, fmt.Errorf("while traversing path '" + pathKey + "' the last element in .tree expected a bool but the path last element was not a matching boolean")
	}
	// string check
	if castedString, isString := castString(mapLastElement); isString {
		if isString && castedString == pathValue {
			return currPathString + "." + castedString, nil
		}
		return currPathString, fmt.Errorf("while traversing path '" + pathKey + "' the last element in .tree expected a string but the path last element was not a matching string")
	}
	// list check
	if castedList, isList := castList(mapLastElement); isList {
		return getCastedListElement(castedList, currPathString, pathValue)
	}
	// map check (the last element is not a constant, it is a map)
	if _, isMap := castMap(mapLastElement); isMap {
		return currPathString, fmt.Errorf("while traversing path '" + pathKey + "' the correspond path value '" + pathValue + "' was expected to terminate at a constant value, but received a map element")
	}
	return currPathString, fmt.Errorf("while traversing path '" + pathKey + "' the last element in .tree could not be casted")
}

func getCastedListElement(castedList []interface{}, currPathString string, element string) (string, error) {
	if element != "" {
		checkBooleanTrue, hasBooleanTrue := false, false
		checkBooleanFalse, hasBooleanFalse := false, false
		checkStringEquals, hasStringEquals := false, false
		if element == "true" {
			checkBooleanTrue = true
		} else if element == "false" {
			checkBooleanFalse = true
		} else {
			checkStringEquals = true
		}

		for _, listElement := range castedList {
			// bool check
			if castedBool, isBool := castBool(listElement); isBool {
				if castedBool {
					hasBooleanTrue = true
				} else {
					hasBooleanFalse = true
				}
			}
			// string check
			if castedString, isString := castString(listElement); isString {
				if isString && castedString == element {
					hasStringEquals = true
				}
			}
		}
		if checkBooleanTrue {
			if !hasBooleanTrue {
				return currPathString, fmt.Errorf("while traversing path '" + currPathString + "' the last element was supposed to be a true boolean, but .tree.*.[] has none")
			}
			return currPathString + ".true", nil
		}
		if checkBooleanFalse {
			if !hasBooleanFalse {
				return currPathString, fmt.Errorf("while traversing path '" + currPathString + "' the last element was supposed to be a false boolean, but .tree.*.[] has none")
			}
			return currPathString + ".false", nil
		}
		if checkStringEquals {
			if !hasStringEquals {
				return currPathString, fmt.Errorf("while traversing path '" + currPathString + "' the last element '" + element + "' was supposed to be present, but .tree.*.[] has none")
			}
			return currPathString + "." + element, nil
		}
		return currPathString, fmt.Errorf("while traversing path '" + currPathString + "' the last element did not match to a boolean, string or list")
	}
	return currPathString, fmt.Errorf("while traversing path '" + currPathString + "' the last element was not specified")
}

type TreeNode struct {
	parentPath string // the path leading up to node value
	value      interface{}
}

type AnnotatedTreeNode struct {
	node       TreeNode
	index      int
	sortedKeys []string
}

func isLeafNode(node interface{}) (string, bool) {
	castedBool, found1 := castBool(node)
	castedString, found2 := castString(node)
	leafString := ""
	if found1 {
		leafString = strconv.FormatBool(castedBool)
	} else if found2 {
		leafString = castedString
	}
	return leafString, found1 || found2
}

func GetPathFromLeafIndex(treeMap map[string]interface{}, version string, index int) (string, error) {
	if index < 0 {
		return "", fmt.Errorf("no leaf exists in treeMap with a negative index")
	}
	stack := make([]AnnotatedTreeNode, 0)
	rootNode := AnnotatedTreeNode{
		node: TreeNode{
			parentPath: version,
			value:      treeMap[version],
		},
		index:      -1, // index represents that root.value[index] has already been pushed on the stack, by default the count starts at -1 since nothing has been pushed yet
		sortedKeys: getSortedKeysFromMapInterface(treeMap[version]),
	}
	stack = append(stack, rootNode)
	leafCount := -1
	for len(stack) > 0 {
		top := stack[len(stack)-1]   // read
		stack = stack[:len(stack)-1] // pop
		if leafValue, isLeaf := isLeafNode(top.node.value); isLeaf {
			leafCount += 1
			// fmt.Println("found leaf index " + fmt.Sprint(leafCount) + " with path " + top.node.parentPath + "." + leafValue)
			if leafCount == index {
				return top.node.parentPath + "." + leafValue, nil
			}
		} else {
			if childMap, found := castMap(top.node.value); found {
				// traverse a map, in alphabetical key order
				if top.index < len(childMap)-1 {
					// The parent controls the iteration through the child map so increment the parent node's index and place it back on the stack first
					top.index += 1
					// push parent onto stack
					stack = append(stack, top)
					// create the child node
					key := top.sortedKeys[top.index]
					nextPairNode := AnnotatedTreeNode{
						node: TreeNode{
							parentPath: top.node.parentPath + "." + key,
							value:      childMap[key],
						},
						index:      -1,
						sortedKeys: getSortedKeysFromMapInterface(childMap[key]),
					}
					// push the child node
					stack = append(stack, nextPairNode)
				}
			} else if childList, found := castList(top.node.value); found {
				// traverse the list in order
				if top.index < len(childList)-1 {
					// The parent controls the iteration through the child list so increment the parent node's index and place it back on the stack first
					top.index += 1
					// push the parent onto the stack
					stack = append(stack, top)
					// create the child node
					nextPairNode := AnnotatedTreeNode{
						node: TreeNode{
							parentPath: top.node.parentPath,
							value:      childList[top.index],
						},
						index:      -1,
						sortedKeys: getSortedKeysFromMapInterface(childList[top.index]),
					}
					// push the child onto the stack
					stack = append(stack, nextPairNode)
				}
			}
		}
	}
	return "", fmt.Errorf("could not find leaf index " + fmt.Sprint(index) + " in treeMap")
}

// returns sorted keys from value if it is a map, else returns an empty string array
func getSortedKeysFromMapInterface(value interface{}) []string {
	if _, isLeaf := isLeafNode(value); !isLeaf {
		if childMap, found := castMap(value); found {
			keys := make([]string, len(childMap))
			i := 0
			for k := range childMap {
				keys[i] = k
				i += 1
			}
			sort.Strings(keys)
			return keys
		}
	}
	return []string{}
}

// Returns the index of the leaf represented by validPath in an inorder tree traversal of treeMap where map keys are sorted in alphabetical order, else -1 when not found
// Precondition: validPath must be a valid sub path in treeMap
func GetLeafIndex(treeMap map[string]interface{}, validPath string) int {
	if len(validPath) == 0 {
		return -1
	}
	// if the root is a matching leaf, return 0, otherwise it is not found
	if !strings.Contains(validPath, ".") {
		if _, found := treeMap[validPath]; found {
			return 0
		}
		return -1
	}
	// Using a stack to perform an iterative inorder tree traversal
	pathSegments := strings.Split(validPath, ".")
	stack := make([]AnnotatedTreeNode, 0)
	firstKey := pathSegments[0]
	value, found := treeMap[firstKey]
	if !found {
		return -1
	}
	rootNode := AnnotatedTreeNode{
		node: TreeNode{
			parentPath: firstKey,
			value:      value,
		},
		index:      -1, // index represents that root.value[index] has already been pushed on the stack, by default the count starts at -1 since nothing has been pushed yet
		sortedKeys: getSortedKeysFromMapInterface(value),
	}
	stack = append(stack, rootNode)
	leafCount := -1 // tracks every time a leaf node has been seen, the traversal order is always the same, so this count is unique for each leaf node visited.
	for len(stack) > 0 {
		top := stack[len(stack)-1]   // read
		stack = stack[:len(stack)-1] // pop
		if leafValue, isLeaf := isLeafNode(top.node.value); isLeaf {
			leafCount += 1
			// fmt.Println("found leaf index " + fmt.Sprint(leafCount) + " with path " + top.node.parentPath + "." + leafValue)
			if top.node.parentPath+"."+leafValue == validPath {
				return leafCount
			}
		} else {
			if childMap, found := castMap(top.node.value); found {
				// traverse a map, in alphabetical key order
				if top.index < len(childMap)-1 {
					// The parent controls the iteration through the child map so increment the parent node's index and place it back on the stack first
					top.index += 1
					// push parent onto stack
					stack = append(stack, top)
					// create the child node
					key := top.sortedKeys[top.index]
					nextPairNode := AnnotatedTreeNode{
						node: TreeNode{
							parentPath: top.node.parentPath + "." + key,
							value:      childMap[key],
						},
						index:      -1,
						sortedKeys: getSortedKeysFromMapInterface(childMap[key]),
					}
					// push the child node
					stack = append(stack, nextPairNode)
				}
			} else if childList, found := castList(top.node.value); found {
				// traverse the list in order
				if top.index < len(childList)-1 {
					// The parent controls the iteration through the child list so increment the parent node's index and place it back on the stack first
					top.index += 1
					// push the parent onto the stack
					stack = append(stack, top)
					// create the child node
					nextPairNode := AnnotatedTreeNode{
						node: TreeNode{
							parentPath: top.node.parentPath,
							value:      childList[top.index],
						},
						index:      -1,
						sortedKeys: getSortedKeysFromMapInterface(childList[top.index]),
					}
					// push the child onto the stack
					stack = append(stack, nextPairNode)
				}
			}
		}
	}
	return -1
}

func GetLabelFromDecisionPath(operandVersionString string, pathOptions []string, pathChoices []string) (string, error) {
	no := len(pathOptions)
	nc := len(pathChoices)
	if no == 0 || nc == 0 {
		return "", fmt.Errorf("expected decision tree path lists to be non-empty but got arrays of length " + fmt.Sprint(no) + " and " + fmt.Sprint(nc))
	}
	if no != nc {
		return "", fmt.Errorf("expected decision tree path list to be the same length but got arrays of length " + fmt.Sprint(no) + " and " + fmt.Sprint(nc))
	}
	label := operandVersionString + "."
	n := len(pathOptions)
	for i, option := range pathOptions {
		label += option + "." + pathChoices[i]
		if i < n-1 && pathChoices[i] != "" {
			label += "."
		}
	}
	return label, nil
}

func ParseDecisionTree(leaderTrackerType string, fileName *string) (map[string]interface{}, map[string]map[string]string, error) {
	// First check the in-memory cache to see if a cached decision tree already exists
	cachedTreeMap, cachedReplaceMap := TreeCache.Maps(leaderTrackerType)
	if cachedTreeMap != nil && cachedReplaceMap != nil {
		return cachedTreeMap, cachedReplaceMap, nil
	}

	var tree []byte
	var err error
	// Allow specifying custom file for testing
	if fileName != nil {
		tree, err = os.ReadFile(*fileName)
		if err != nil {
			return nil, nil, err
		}
	} else {
		tree, err = os.ReadFile("internal/controller/assets/" + leaderTrackerType + "-decision-tree.yaml")
		if err != nil {
			return nil, nil, err
		}
	}
	ltpaDecisionTreeYAML := make(map[string]interface{})
	err = yaml.Unmarshal(tree, ltpaDecisionTreeYAML)
	if err != nil {
		return nil, nil, err
	}

	treeMap, err := CastTreeMap(ltpaDecisionTreeYAML)
	if err != nil {
		return nil, nil, err
	}

	replaceMap, err := CastReplaceMap(ltpaDecisionTreeYAML)
	if err != nil {
		return nil, nil, err
	}

	if err := ValidateMaps(treeMap, replaceMap); err != nil {
		return nil, nil, err
	}

	TreeCache.SetDecisionTree(leaderTrackerType, treeMap, replaceMap)

	return treeMap, replaceMap, nil
}

func getLatestTreeMapOperandVersion(treeMap map[string]interface{}, maxVersion string) (string, error) {
	maxTreeVersion := "v0_0_0"
	for version := range treeMap {
		if !lutils.IsOperandVersionString(version) {
			return "", fmt.Errorf("the tree map contained a key without a valid semantic version string of format 'va_b_c'")
		}
		if lutils.CompareOperandVersion(maxTreeVersion, version) < 0 && lutils.CompareOperandVersion(maxVersion, version) >= 0 {
			maxTreeVersion = version
		}
	}
	if _, found := treeMap[maxTreeVersion]; !found {
		return "", fmt.Errorf("could not find a valid key in the tree map when searching for an operand version string less than or equal to version %s", maxVersion)
	}
	return maxTreeVersion, nil
}

func GetLatestOperandVersion(treeMap map[string]interface{}, currentOperandVersion string) (string, error) {
	operandVersionString := ""
	if len(currentOperandVersion) == 0 {
		versionString, err := lutils.GetOperandVersionString()
		if err != nil {
			return "", err
		}
		operandVersionString = versionString
	} else {
		operandVersionString = currentOperandVersion
	}
	latestOperandVersionString, err := getLatestTreeMapOperandVersion(treeMap, operandVersionString)
	if err != nil {
		return "", err
	}
	return latestOperandVersionString, nil
}

// migrates path to a valid path existing in operator version targetVersion
func ReplacePath(path string, targetVersion string, treeMap map[string]interface{}, replaceMap map[string]map[string]string) (string, error) {
	// determine upgrade/downgrade strategy
	currPath := path
	currVersion := strings.Split(currPath, ".")[0]
	sortedKeys := getSortedKeysFromReplaceMap(replaceMap)
	if !lutils.IsValidOperandVersion(currVersion) || !lutils.IsValidOperandVersion(targetVersion) {
		return "", fmt.Errorf("there was no valid upgrade path to migrate the LTPA Secret on path %s, this may occur when the LTPA decision tree was modified from the history of an older multi-LTPA Liberty operator, first delete the affected/outdated LTPA Secret(s) then delete the olo-managed-leader-tracking-ltpa ConfigMap to resolve the operator state", path)
	}
	cmp := lutils.CompareOperandVersion(currVersion, targetVersion)
	latestOperandVersion, _ := GetLatestOperandVersion(treeMap, targetVersion)
	// currVersion > targetVersion - downgrade
	if cmp > 0 {
		// fmt.Println("downgrading..." + path + " to " + targetVersion + " latest version " + latestOperandVersion)
		lastVersion := ""
		_, found := replaceMap[currVersion]
		for found && lastVersion != currVersion {
			lastVersion = currVersion
			for prevPath, nextPath := range replaceMap[currVersion] {
				potentialNextVersion := strings.Split(prevPath, ".")[0]
				// only continue traversing the path if the new potential path is less than the version of the current path (this prevents cycles)
				// also, the potential next path must be at least as large as the latest operand (decision tree) version for the target version
				if nextPath == currPath && lutils.CompareOperandVersion(currVersion, potentialNextVersion) > 0 && lutils.CompareOperandVersion(potentialNextVersion, latestOperandVersion) >= 0 {
					currPath = prevPath
					currVersion = strings.Split(currPath, ".")[0]
					break
				}
			}
			_, found = replaceMap[currVersion]
		}
	}
	// currVersion < targetVersion - upgrade
	if cmp < 0 {
		// fmt.Println("upgrading..." + path + " to " + targetVersion + " with latest version " + latestOperandVersion)
		i := -1
		for _, version := range sortedKeys {
			i += 1
			if lutils.CompareOperandVersion(version, currVersion) > 0 {
				break
			}
		}
		if i >= 0 {
			for currVersion != targetVersion && i < len(sortedKeys) {
				for prevPath, nextPath := range replaceMap[sortedKeys[i]] {
					if prevPath == currPath {
						currPath = nextPath
						currVersion = strings.Split(currPath, ".")[0]
						break
					}
				}
				i += 1
			}
		}
	}
	return currPath, nil
}

func getSortedKeysFromReplaceMap(replaceMap map[string]map[string]string) []string {
	keys := []string{}
	for key := range replaceMap {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return lutils.CompareOperandVersion(keys[i], keys[j]) < 0
	})
	return keys
}
