package tree

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"

	lutils "github.com/OpenLiberty/open-liberty-operator/utils"

	"gopkg.in/yaml.v3"
)

func CompareOperandVersion(a string, b string) int {
	arrA := strings.Split(a[1:], "_")
	arrB := strings.Split(b[1:], "_")
	for i := range arrA {
		intA, _ := strconv.ParseInt(lutils.GetFirstNumberFromString(arrA[i]), 10, 64)
		intB, _ := strconv.ParseInt(lutils.GetFirstNumberFromString(arrB[i]), 10, 64)
		if intA != intB {
			return int(intA - intB)
		}
	}
	return 0
}

func getLatestTreeMapOperandVersion(treeMap map[string]interface{}, maxVersion string) (string, error) {
	maxTreeVersion := "v0_0_0"
	for version := range treeMap {
		if !lutils.IsOperandVersionString(version) {
			return "", fmt.Errorf("the tree map contained a key without a valid semantic version string of format 'va_b_c'")
		}
		if CompareOperandVersion(maxTreeVersion, version) < 0 && CompareOperandVersion(maxVersion, version) >= 0 {
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

var letterNums = []rune("abcdefghijklmnopqrstuvwxyz1234567890")

func GetRandomLowerAlphanumericSuffix(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letterNums[rand.Intn(len(letterNums))]
	}
	return "-" + string(b)
}

func GetCommaSeparatedString(stringList string, index int) (string, error) {
	if stringList == "" {
		return "", fmt.Errorf("there is no element")
	}
	if strings.Contains(stringList, ",") {
		for i, val := range strings.Split(stringList, ",") {
			if index == i {
				return val, nil
			}
		}
	} else {
		if index == 0 {
			return stringList, nil
		}
		return "", fmt.Errorf("cannot index string list with only one element")
	}
	return "", fmt.Errorf("element not found")
}

func IsLowerAlphanumericSuffix(suffix string) bool {
	for _, ch := range suffix {
		numCheck := int(ch - '0')
		lowerAlphaCheck := int(ch - 'a')
		if !((numCheck >= 0 && numCheck <= 9) || (lowerAlphaCheck >= 0 && lowerAlphaCheck <= 25)) {
			return false
		}
	}
	return true
}

func GetCommaSeparatedArray(stringList string) []string {
	if strings.Contains(stringList, ",") {
		return strings.Split(stringList, ",")
	}
	return []string{stringList}
}

// returns the index of the contained value in stringList or else -1
func CommaSeparatedStringContains(stringList string, value string) int {
	if strings.Contains(stringList, ",") {
		for i, label := range strings.Split(stringList, ",") {
			if value == label {
				return i
			}
		}
	} else if stringList == value {
		return 0
	}
	return -1
}

func IsValidOperandVersion(version string) bool {
	if len(version) == 0 {
		return false
	}
	if version[0] != 'v' {
		return false
	}
	if !strings.Contains(version[1:], "_") {
		return false
	}
	versions := strings.Split(version[1:], "_")
	if len(versions) != 3 {
		return false
	}
	for _, version := range versions {
		if len(lutils.GetFirstNumberFromString(version)) == 0 {
			return false
		}
	}

	return true
}

// migrates path to a valid path existing in operator version targetVersion
func ReplacePath(path string, targetVersion string, treeMap map[string]interface{}, replaceMap map[string]map[string]string) (string, error) {
	// determine upgrade/downgrade strategy
	currPath := path
	currVersion := strings.Split(currPath, ".")[0]
	sortedKeys := getSortedKeysFromReplaceMap(replaceMap)
	if !IsValidOperandVersion(currVersion) || !IsValidOperandVersion(targetVersion) {
		return "", fmt.Errorf("there was no valid upgrade path to migrate the LTPA Secret on path %s, this may occur when the LTPA decision tree was modified from the history of an older multi-LTPA Liberty operator, first delete the affected/outdated LTPA Secret(s) then delete the olo-managed-leader-tracking-ltpa ConfigMap to resolve the operator state", path)
	}
	cmp := CompareOperandVersion(currVersion, targetVersion)
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
				if nextPath == currPath && CompareOperandVersion(currVersion, potentialNextVersion) > 0 && CompareOperandVersion(potentialNextVersion, latestOperandVersion) >= 0 {
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
			if CompareOperandVersion(version, currVersion) > 0 {
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
		return CompareOperandVersion(keys[i], keys[j]) < 0
	})
	return keys
}

func ParseLTPADecisionTree(fileName *string) (map[string]interface{}, map[string]map[string]string, error) {
	var tree []byte
	var err error
	// Allow specifying custom file for testing
	if fileName != nil {
		tree, err = os.ReadFile(*fileName)
		if err != nil {
			return nil, nil, err
		}
	} else {
		tree, err = os.ReadFile("controllers/assets/ltpa-decision-tree.yaml")
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

	return treeMap, replaceMap, nil
}
