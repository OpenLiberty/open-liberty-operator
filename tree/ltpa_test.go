package tree

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

type Test struct {
	test     string
	expected interface{}
	actual   interface{}
}

func TestCompareOperandVersion(t *testing.T) {
	tests := []Test{
		{"same version", CompareOperandVersion("v0_0_0", "v0_0_0"), 0},
		{"same version, multiple digits", CompareOperandVersion("v10_10_10", "v10_10_10"), 0},
		{"same version, build tags", CompareOperandVersion("v2_0_0alpha", "v2_0_0alpha"), 0},
		{"different patch version, build tags", CompareOperandVersion("v2_0_10alpha", "v2_0_2alpha"), 8},
		{"different patch version, build tags, reversed", CompareOperandVersion("v2_0_2alpha", "v2_0_10alpha"), -8},
		{"different patch version", CompareOperandVersion("v1_0_0", "v1_0_1"), -1},
		{"different minor version", CompareOperandVersion("v1_0_0", "v1_1_0"), -1},
		{"different major version", CompareOperandVersion("v2_0_0", "v1_0_0"), 1},
		{"minor less than patch", CompareOperandVersion("v1_10_0", "v1_0_5"), 10},
		{"major less than patch", CompareOperandVersion("v2_0_0", "v1_0_10"), 1},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGetCommaSeparatedString(t *testing.T) {
	emptyString, emptyStringErr := GetCommaSeparatedString("", 0)
	oneElement, oneElementErr := GetCommaSeparatedString("one", 0)
	oneElementAOOB, oneElementAOOBErr := GetCommaSeparatedString("one", 1)
	multiElement, multiElementErr := GetCommaSeparatedString("one,two,three,four,five", 3)
	multiElementAOOB, multiElementAOOBErr := GetCommaSeparatedString("one,two,three,four,five", 5)
	tests := []Test{
		{"empty string", "", emptyString},
		{"empty string errors", fmt.Errorf("there is no element"), emptyStringErr},
		{"one element", "one", oneElement},
		{"one element errors", nil, oneElementErr},
		{"one element array out of bounds", "", oneElementAOOB},
		{"one element array out of bounds error", fmt.Errorf("cannot index string list with only one element"), oneElementAOOBErr},
		{"multi element", "four", multiElement},
		{"multi element error", nil, multiElementErr},
		{"multi element array out of bounds", "", multiElementAOOB},
		{"multi element array out of bounds error", fmt.Errorf("element not found"), multiElementAOOBErr},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCommaSeparatedStringContains(t *testing.T) {
	match := CommaSeparatedStringContains("one,two,three,four", "three")
	substringNonMatch := CommaSeparatedStringContains("one,two,three,four", "thre")
	substringNonMatch2 := CommaSeparatedStringContains("one,two,three,four", "threee")
	oneElementMatch := CommaSeparatedStringContains("one", "one")
	noElementNonMatch := CommaSeparatedStringContains("", "one")
	tests := []Test{
		{"single match", 2, match},
		{"substring should not match", -1, substringNonMatch},
		{"substring 2 should not match", -1, substringNonMatch2},
		{"one element match", 0, oneElementMatch},
		{"no element non match", -1, noElementNonMatch},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func verifyTests(tests []Test) error {
	for _, tt := range tests {
		if !reflect.DeepEqual(tt.actual, tt.expected) {
			return fmt.Errorf("%s test expected: (%v) actual: (%v)", tt.test, tt.expected, tt.actual)
		}
	}
	return nil
}

func TestParseLTPADecisionTree(t *testing.T) {
	// Test 2 generations
	fileName := getControllersFolder() + "/tests/ltpa-decision-tree-2-generations.yaml"
	treeMap, replaceMap, err := ParseLTPADecisionTree(&fileName)

	// Expect tree map
	expectedTreeMap := make(map[string]interface{})
	expectedTreeMap["v10_4_0"] = make(map[string]interface{})
	expectedTreeMap["v10_4_0"].(map[string]interface{})["managePasswordEncryption"] = make([]interface{}, 2)
	expectedTreeMap["v10_4_0"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[0] = true
	expectedTreeMap["v10_4_0"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[1] = false
	expectedTreeMap["v10_4_0"].(map[string]interface{})["test"] = "test"
	expectedTreeMap["v10_4_1"] = make(map[string]interface{})
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"] = make(map[string]interface{})
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"] = make(map[string]interface{})
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"].(map[string]interface{})["managePasswordEncryption"] = make([]interface{}, 2)
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[0] = true
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[1] = false
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["xor"] = "type"

	// Expect replace map
	expectedReplaceMap := make(map[string]map[string]string)
	expectedReplaceMap["v10_4_1"] = make(map[string]string)
	expectedReplaceMap["v10_4_1"]["v10_4_0.managePasswordEncryption.true"] = "v10_4_1.type.aes.managePasswordEncryption.true"
	expectedReplaceMap["v10_4_1"]["v10_4_0.managePasswordEncryption.false"] = "v10_4_1.type.aes.managePasswordEncryption.false"
	expectedReplaceMap["v10_4_1"]["v10_4_0.test.test"] = "v10_4_1.type.xor.type"

	tests := []Test{
		{"parse decision tree (2 generations) error", nil, err},
		{"parse decision tree (2 generations) map", expectedTreeMap, treeMap},
		{"parse decision tree (2 generations) replace map", expectedReplaceMap, replaceMap},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Test 3 generations
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-3-generations.yaml"
	treeMap, replaceMap, err = ParseLTPADecisionTree(&fileName)

	// Expect tree map
	expectedTreeMap = make(map[string]interface{})
	expectedTreeMap["v10_3_3"] = make(map[string]interface{})
	expectedTreeMap["v10_3_3"].(map[string]interface{})["manageLTPA"] = true
	expectedTreeMap["v10_4_0"] = make(map[string]interface{})
	expectedTreeMap["v10_4_0"].(map[string]interface{})["managePasswordEncryption"] = make([]interface{}, 3)
	expectedTreeMap["v10_4_0"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[0] = true
	expectedTreeMap["v10_4_0"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[1] = false
	expectedTreeMap["v10_4_0"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[2] = "test"
	expectedTreeMap["v10_4_1"] = make(map[string]interface{})
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"] = make(map[string]interface{})
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"] = make(map[string]interface{})
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"].(map[string]interface{})["managePasswordEncryption"] = make([]interface{}, 3)
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[0] = true
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[1] = false
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["aes"].(map[string]interface{})["managePasswordEncryption"].([]interface{})[2] = "test"
	expectedTreeMap["v10_4_1"].(map[string]interface{})["type"].(map[string]interface{})["xor"] = "type"

	// Expect replace map
	expectedReplaceMap = make(map[string]map[string]string)
	expectedReplaceMap["v10_4_0"] = make(map[string]string)
	expectedReplaceMap["v10_4_0"]["v10_3_3.manageLTPA.true"] = "v10_4_0.managePasswordEncryption.false"
	expectedReplaceMap["v10_4_1"] = make(map[string]string)
	expectedReplaceMap["v10_4_1"]["v10_4_0.managePasswordEncryption.true"] = "v10_4_1.type.aes.managePasswordEncryption.true"
	expectedReplaceMap["v10_4_1"]["v10_4_0.managePasswordEncryption.false"] = "v10_4_1.type.aes.managePasswordEncryption.false"
	expectedReplaceMap["v10_4_1"]["v10_4_0.managePasswordEncryption.test"] = "v10_4_1.type.aes.managePasswordEncryption.test"

	tests = []Test{
		{"parse decision tree (3 generations) error", nil, err},
		{"parse decision tree (3 generations) map", expectedTreeMap, treeMap},
		{"parse decision tree (3 generations) replace map", expectedReplaceMap, replaceMap},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Test 1 generation
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-1-generation.yaml"
	treeMap, replaceMap, err = ParseLTPADecisionTree(&fileName)

	expectedTreeMap = make(map[string]interface{})
	expectedTreeMap["v10_3_3"] = "test"

	expectedReplaceMap = make(map[string]map[string]string)

	tests = []Test{
		{"parse decision tree (1 generation) error", nil, err},
		{"parse decision tree (1 generation) map", expectedTreeMap, treeMap},
		{"parse decision tree (1 generation) replace map", expectedReplaceMap, replaceMap},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGetLabelFromDecisionPath(t *testing.T) {
	labelString, err := GetLabelFromDecisionPath("v10_4_20", []string{"manageLTPA"}, []string{"true"})
	labelString2, err2 := GetLabelFromDecisionPath("v10_4_20", []string{}, []string{"false"})
	labelString3, err3 := GetLabelFromDecisionPath("v10_4_20", []string{"one", "two"}, []string{"one"})
	labelString4, err4 := GetLabelFromDecisionPath("v0_0_0", []string{"one", "two", "three"}, []string{"four", "five", "six"})
	tests := []Test{
		{"get label from decision path - error", nil, err},
		{"get label from decision path - string", "v10_4_20.manageLTPA.true", labelString},
		{"get label from decision path 2 - error", fmt.Errorf("expected decision tree path lists to be non-empty but got arrays of length 0 and 1"), err2},
		{"get label from decision path 2 - string", "", labelString2},
		{"get label from decision path 3 - error", fmt.Errorf("expected decision tree path list to be the same length but got arrays of length 2 and 1"), err3},
		{"get label from decision path 3 - string", "", labelString3},
		{"get label from decision path 4 - error", nil, err4},
		{"get label from decision path 4 - string", "v0_0_0.one.four.two.five.three.six", labelString4},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGetLatestOperandVersion(t *testing.T) {
	// Test 1 generation
	fileName := getControllersFolder() + "/tests/ltpa-decision-tree-1-generation.yaml"
	treeMap, _, _ := ParseLTPADecisionTree(&fileName)
	latestOperandVersion1, err1 := GetLatestOperandVersion(treeMap, "v10_3_3")

	// Test 2 generations
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-2-generations.yaml"
	treeMap, _, _ = ParseLTPADecisionTree(&fileName)
	latestOperandVersion2, err2 := GetLatestOperandVersion(treeMap, "v10_3_999")

	// Test 3 generations
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-3-generations.yaml"
	treeMap, _, _ = ParseLTPADecisionTree(&fileName)
	latestOperandVersion3, err3 := GetLatestOperandVersion(treeMap, "v10_4_1")

	// Test complex
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, _, _ = ParseLTPADecisionTree(&fileName)
	latestOperandVersion4, err4 := GetLatestOperandVersion(treeMap, "v10_4_499")

	tests := []Test{
		{"get label from decision path 1 - error", nil, err1},
		{"get label from decision path 1 - string", "v10_3_3", latestOperandVersion1},
		{"get label from decision path 2 - error", fmt.Errorf("could not find a valid key in the tree map when searching for an operand version string less than or equal to version v10_3_999"), err2},
		{"get label from decision path 2 - string", "", latestOperandVersion2},
		{"get label from decision path 3 - error", nil, err3},
		{"get label from decision path 3 - string", "v10_4_1", latestOperandVersion3},
		{"get label from decision path 4 - error", nil, err4},
		{"get label from decision path 4 - string", "v10_4_20", latestOperandVersion4},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestReplacePath(t *testing.T) {
	// Test 1 generation - empty replace map
	fileName := getControllersFolder() + "/tests/ltpa-decision-tree-1-generation.yaml"
	treeMap, replaceMap, _ := ParseLTPADecisionTree(&fileName)
	newPath1a, err1a := ReplacePath("v10_3_3", "v10_3_3", treeMap, replaceMap)

	// Test 1 generation - empty replace map and attempting to upgrade to an invalid version
	newPath1b, err1b := ReplacePath("v10_3_3", "v20_30_30", treeMap, replaceMap)

	// Test 2 generations - upgrade from first to second generation
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-2-generations.yaml"
	treeMap, replaceMap, _ = ParseLTPADecisionTree(&fileName)
	newPath2a, err2a := ReplacePath("v10_4_0.managePasswordEncryption.false", "v10_4_1", treeMap, replaceMap)

	// Test 2 generations - upgrade from second to first generation
	newPath2b, err2b := ReplacePath("v10_4_1.type.aes.managePasswordEncryption.true", "v10_4_0", treeMap, replaceMap)

	// Test 3 generations - upgrade from first to third generation
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-3-generations.yaml"
	treeMap, replaceMap, _ = ParseLTPADecisionTree(&fileName)
	newPath3a, err3a := ReplacePath("v10_3_3.manageLTPA.true", "v10_4_1", treeMap, replaceMap)

	// Test 3 generations - upgrade from third to first generation
	newPath3b, err3b := ReplacePath("v10_4_1.type.aes.managePasswordEncryption.false", "v10_3_3", treeMap, replaceMap)

	// Test 3 generations - upgrade from third to first generation
	// However, it can only upgrade up to the second generation because no valid upgrade path exists from second to first generation
	newPath3c, err3c := ReplacePath("v10_4_1.type.aes.managePasswordEncryption.true", "v10_3_3", treeMap, replaceMap)

	// Test 3 generations - same as 3c but upgrading from second to first generation
	newPath3d, err3d := ReplacePath("v10_4_0.managePasswordEncryption.true", "v10_3_3", treeMap, replaceMap)

	// Test complex
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, _ = ParseLTPADecisionTree(&fileName)
	newPath4a, err4a := ReplacePath("v10_4_500.a.f.g.i.bar", "v10_4_21", treeMap, replaceMap)

	newPath4b, err4b := ReplacePath("v10_4_500.a.f.g.i.bar", "v10_4_500", treeMap, replaceMap)

	tests := []Test{
		// upgrades
		{"get replaced path 1a - error", nil, err1a},
		{"get replaced path 1a - new path", "v10_3_3", newPath1a},
		{"get replaced path 1b - error", nil, err1b},
		{"get replaced path 1b - new path", "v10_3_3", newPath1b},
		{"get replaced path 2a - error", nil, err2a},
		{"get replaced path 2a - new path", "v10_4_1.type.aes.managePasswordEncryption.false", newPath2a},
		{"get replaced path 3a - error", nil, err3a},
		{"get replaced path 3a - new path", "v10_4_1.type.aes.managePasswordEncryption.false", newPath3a},
		// downgrades
		{"get replaced path 2b - error", nil, err2b},
		{"get replaced path 2b - new path", "v10_4_0.managePasswordEncryption.true", newPath2b},
		{"get replaced path 3b - error", nil, err3b},
		{"get replaced path 3b - new path", "v10_3_3.manageLTPA.true", newPath3b},
		{"get replaced path 3c - error", nil, err3c},
		{"get replaced path 3c - new path", "v10_4_0.managePasswordEncryption.true", newPath3c},
		{"get replaced path 3d - error", nil, err3d},
		{"get replaced path 3d - new path", "v10_4_0.managePasswordEncryption.true", newPath3d},
		{"get replaced path 4a - error", nil, err4a},
		{"get replaced path 4a - new path", "v10_4_20.a.f.g.i.bar", newPath4a},
		{"get replaced path 4b - error", nil, err4b},
		{"get replaced path 4b - new path", "v10_4_500.a.f.g.i.bar", newPath4b},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGetLeafIndex(t *testing.T) {
	// Test 1 generation - empty path
	fileName := getControllersFolder() + "/tests/ltpa-decision-tree-1-generation.yaml"
	treeMap, _, _ := ParseLTPADecisionTree(&fileName)
	leafIndex1a := GetLeafIndex(treeMap, "")
	leafIndex1b := GetLeafIndex(treeMap, "v10_3_3.test")

	// Test 2 generations
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-2-generations.yaml"
	treeMap, _, _ = ParseLTPADecisionTree(&fileName)
	// valid paths
	leafIndex2 := GetLeafIndex(treeMap, "")
	leafIndex2a := GetLeafIndex(treeMap, "v10_4_0.managePasswordEncryption.true")
	leafIndex2b := GetLeafIndex(treeMap, "v10_4_0.managePasswordEncryption.false")
	leafIndex2c := GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.true")
	leafIndex2d := GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.false")
	leafIndex2e := GetLeafIndex(treeMap, "v10_4_1.type.xor.type")
	// invalid paths
	leafIndex2f := GetLeafIndex(treeMap, "v10_4_0.managePasswordEncryption")
	leafIndex2g := GetLeafIndex(treeMap, "v10_4_0.managePasswordEncryption.random")
	leafIndex2h := GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption")
	leafIndex2i := GetLeafIndex(treeMap, "v10_4_1.type.aes.")
	leafIndex2j := GetLeafIndex(treeMap, "v10_4_1.type.aes")
	leafIndex2k := GetLeafIndex(treeMap, "v10_4_1.type")
	leafIndex2l := GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.true.false")
	// syntax errors, incorrect elements
	leafIndex2m := GetLeafIndex(treeMap, "v10_4_1.type.aes.")
	leafIndex2n := GetLeafIndex(treeMap, "v10_4_1.type.")
	leafIndex2o := GetLeafIndex(treeMap, "v10_4_1.ty")

	// Test 3 generations
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-3-generations.yaml"
	treeMap, _, _ = ParseLTPADecisionTree(&fileName)
	leafIndex3 := GetLeafIndex(treeMap, "")
	// valid paths
	leafIndex3a := GetLeafIndex(treeMap, "v10_3_3.manageLTPA.true")
	leafIndex3b := GetLeafIndex(treeMap, "v10_4_0.managePasswordEncryption.true")
	leafIndex3c := GetLeafIndex(treeMap, "v10_4_0.managePasswordEncryption.false")
	leafIndex3d := GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.true")
	leafIndex3e := GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.false")
	leafIndex3f := GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.test")
	leafIndex3g := GetLeafIndex(treeMap, "v10_4_1.type.xor.type")
	// invalid paths
	leafIndex3h := GetLeafIndex(treeMap, "v10_4_1.type.xor.type.test")
	leafIndex3i := GetLeafIndex(treeMap, "v10_4_1.type.aes")
	leafIndex3j := GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.true.v10_3_3.manageLTPA.true")
	leafIndex3k := GetLeafIndex(treeMap, "v10_4_1.v10_3_3.manageLTPA.true")
	leafIndex3l := GetLeafIndex(treeMap, "v10_3_3.manageLTPA.true.v10_4_0")

	// Test complex
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, _, _ = ParseLTPADecisionTree(&fileName)
	leafIndex4 := GetLeafIndex(treeMap, "")
	// valid paths
	leafIndex4a := GetLeafIndex(treeMap, "v10_3_3.a.b")
	leafIndex4b := GetLeafIndex(treeMap, "v10_4_1.a.b.c.true")
	leafIndex4c := GetLeafIndex(treeMap, "v10_4_1.a.b.d.true")
	leafIndex4d := GetLeafIndex(treeMap, "v10_4_1.a.b.e.true")
	leafIndex4e := GetLeafIndex(treeMap, "v10_4_1.a.b.e.false")
	leafIndex4f := GetLeafIndex(treeMap, "v10_4_20.a.b.c.true")
	leafIndex4g := GetLeafIndex(treeMap, "v10_4_20.a.b.d.false")
	leafIndex4h := GetLeafIndex(treeMap, "v10_4_20.a.b.e.foo")
	leafIndex4i := GetLeafIndex(treeMap, "v10_4_20.a.f.g.i.bar")
	leafIndex4j := GetLeafIndex(treeMap, "v10_4_20.a.f.h.element")
	leafIndex4k := GetLeafIndex(treeMap, "v10_4_500.a.b.b.true")
	leafIndex4l := GetLeafIndex(treeMap, "v10_4_500.a.b.c.true")
	leafIndex4m := GetLeafIndex(treeMap, "v10_4_500.a.b.d.false")
	leafIndex4n := GetLeafIndex(treeMap, "v10_4_500.a.b.e.foo")
	leafIndex4o := GetLeafIndex(treeMap, "v10_4_500.a.f.g.i.bar")
	leafIndex4p := GetLeafIndex(treeMap, "v10_4_500.a.f.h.element")

	tests := []Test{
		{"get leaf index - 1 generation a", -1, leafIndex1a},
		{"get leaf index - 1 generation b", 0, leafIndex1b},
		{"get leaf index - 2 generations", -1, leafIndex2},
		{"get leaf index - 2 generations a", 0, leafIndex2a},
		{"get leaf index - 2 generations b", 1, leafIndex2b},
		{"get leaf index - 2 generations c", 0, leafIndex2c},
		{"get leaf index - 2 generations d", 1, leafIndex2d},
		{"get leaf index - 2 generations e", 2, leafIndex2e},
		{"get leaf index - 2 generations f", -1, leafIndex2f},
		{"get leaf index - 2 generations g", -1, leafIndex2g},
		{"get leaf index - 2 generations h", -1, leafIndex2h},
		{"get leaf index - 2 generations i", -1, leafIndex2i},
		{"get leaf index - 2 generations j", -1, leafIndex2j},
		{"get leaf index - 2 generations k", -1, leafIndex2k},
		{"get leaf index - 2 generations l", -1, leafIndex2l},
		{"get leaf index - 2 generations m", -1, leafIndex2m},
		{"get leaf index - 2 generations n", -1, leafIndex2n},
		{"get leaf index - 2 generations o", -1, leafIndex2o},
		{"get leaf index - 3 generations", -1, leafIndex3},
		{"get leaf index - 3 generations a", 0, leafIndex3a},
		{"get leaf index - 3 generations b", 0, leafIndex3b},
		{"get leaf index - 3 generations c", 1, leafIndex3c},
		{"get leaf index - 3 generations d", 0, leafIndex3d},
		{"get leaf index - 3 generations e", 1, leafIndex3e},
		{"get leaf index - 3 generations f", 2, leafIndex3f},
		{"get leaf index - 3 generations g", 3, leafIndex3g},
		{"get leaf index - 3 generations h", -1, leafIndex3h},
		{"get leaf index - 3 generations i", -1, leafIndex3i},
		{"get leaf index - 3 generations j", -1, leafIndex3j},
		{"get leaf index - 3 generations k", -1, leafIndex3k},
		{"get leaf index - 3 generations l", -1, leafIndex3l},
		{"get leaf index - complex generations", -1, leafIndex4},
		{"get leaf index - complex generations a", 0, leafIndex4a},
		{"get leaf index - complex generations b", 0, leafIndex4b},
		{"get leaf index - complex generations c", 1, leafIndex4c},
		{"get leaf index - complex generations d", 2, leafIndex4d},
		{"get leaf index - complex generations e", 3, leafIndex4e},
		{"get leaf index - complex generations f", 0, leafIndex4f},
		{"get leaf index - complex generations g", 1, leafIndex4g},
		{"get leaf index - complex generations h", 2, leafIndex4h},
		{"get leaf index - complex generations i", 3, leafIndex4i},
		{"get leaf index - complex generations j", 4, leafIndex4j},
		{"get leaf index - complex generations k", 0, leafIndex4k},
		{"get leaf index - complex generations l", 1, leafIndex4l},
		{"get leaf index - complex generations m", 2, leafIndex4m},
		{"get leaf index - complex generations n", 3, leafIndex4n},
		{"get leaf index - complex generations o", 4, leafIndex4o},
		{"get leaf index - complex generations p", 5, leafIndex4p},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGetPathFromLeafIndex(t *testing.T) {
	// Test 1 generation
	fileName := getControllersFolder() + "/tests/ltpa-decision-tree-1-generation.yaml"
	treeMap, _, _ := ParseLTPADecisionTree(&fileName)
	tests := []Test{}
	validPaths := []string{"v10_3_3.test"}
	for i, path := range validPaths {
		errTest, pathTest := testGetPathFromLeafIndex("get path from leaf index - 1 generation", treeMap, path, i)
		tests = append(tests, errTest)
		tests = append(tests, pathTest)
	}

	// Test 2 generations
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-2-generations.yaml"
	treeMap, _, _ = ParseLTPADecisionTree(&fileName)
	// valid paths
	validPaths = []string{"v10_4_0.managePasswordEncryption.true", "v10_4_0.managePasswordEncryption.false",
		"v10_4_1.type.aes.managePasswordEncryption.true", "v10_4_1.type.aes.managePasswordEncryption.false", "v10_4_1.type.xor.type"}
	for i, path := range validPaths {
		errTest, pathTest := testGetPathFromLeafIndex("get path from leaf index - 2 generations", treeMap, path, i)
		tests = append(tests, errTest)
		tests = append(tests, pathTest)
	}

	// Test 3 generations
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-3-generations.yaml"
	treeMap, _, _ = ParseLTPADecisionTree(&fileName)
	// valid paths
	validPaths = []string{"v10_3_3.manageLTPA.true", "v10_4_0.managePasswordEncryption.true", "v10_4_0.managePasswordEncryption.false",
		"v10_4_1.type.aes.managePasswordEncryption.true", "v10_4_1.type.aes.managePasswordEncryption.false", "v10_4_1.type.xor.type"}
	for i, path := range validPaths {
		errTest, pathTest := testGetPathFromLeafIndex("get path from leaf index - 3 generations", treeMap, path, i)
		tests = append(tests, errTest)
		tests = append(tests, pathTest)
	}

	// Test complex
	fileName = getControllersFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, _, _ = ParseLTPADecisionTree(&fileName)
	// valid paths
	validPaths = []string{"v10_2_2.test", "v10_3_3.a.b", "v10_4_1.a.b.c.true", "v10_4_1.a.b.d.true", "v10_4_1.a.b.e.true", "v10_4_1.a.b.e.false",
		"v10_4_20.a.b.c.true", "v10_4_20.a.b.d.false", "v10_4_20.a.b.e.foo", "v10_4_20.a.f.g.i.bar", "v10_4_20.a.f.h.element",
		"v10_4_500.a.b.b.true", "v10_4_500.a.b.c.true", "v10_4_500.a.b.d.false", "v10_4_500.a.b.e.foo", "v10_4_500.a.f.g.i.bar", "v10_4_500.a.f.h.element"}
	for i, path := range validPaths {
		errTest, pathTest := testGetPathFromLeafIndex("get path from leaf index - complex generations", treeMap, path, i)
		tests = append(tests, errTest)
		tests = append(tests, pathTest)
	}

	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCanTraverseTreeSubPath(t *testing.T) {
	fileName := getControllersFolder() + "/tests/ltpa-decision-tree-3-generations.yaml"
	treeMap, _, _ := ParseLTPADecisionTree(&fileName)

	subPath1, err1 := CanTraverseTree(treeMap, "v10_4_1.type.xor.type.a.b", true)
	subPath2, err2 := CanTraverseTree(treeMap, "v10_4_1.type.aes.managePasswordEncryption.true.a.b", true)
	subPath3, err3 := CanTraverseTree(treeMap, "v10_4_1.type.aes.managePasswordEncryption.true.a", true)
	subPath4, err4 := CanTraverseTree(treeMap, "v10_4_1.type.aes.managePasswordEncryption.true.", true) // syntax error
	subPath5, err5 := CanTraverseTree(treeMap, "v10_4_1.type.aes.managePasswordEncryption.false.a.b", true)
	subPath6, err6 := CanTraverseTree(treeMap, "v10_4_1.type.aes.managePasswordEncryption.false.a", true)
	subPath7, err7 := CanTraverseTree(treeMap, "v10_4_1.type.aes.managePasswordEncryption.false.", true) // syntax error
	subPath8, err8 := CanTraverseTree(treeMap, "v10_4_1.type.aes.managePasswordEncryption.test.a.b", true)
	subPath9, err9 := CanTraverseTree(treeMap, "v10_4_1.type.aes.managePasswordEncryption.test.a", true)
	subPath10, err10 := CanTraverseTree(treeMap, "v10_4_1.type.aes.managePasswordEncryption.test.", true) // syntax error

	tests := []Test{
		{"can traverse tree sub path - 3 generations error 1", nil, err1},
		{"can traverse tree sub path - 3 generations sub path 1", "v10_4_1.type.xor.type", subPath1},
		{"can traverse tree sub path - 3 generations error 2", nil, err2},
		{"can traverse tree sub path - 3 generations sub path 2", "v10_4_1.type.aes.managePasswordEncryption.true", subPath2},
		{"can traverse tree sub path - 3 generations error 3", nil, err3},
		{"can traverse tree sub path - 3 generations sub path 3", "v10_4_1.type.aes.managePasswordEncryption.true", subPath3},
		{"can traverse tree sub path - 3 generations error 4", fmt.Errorf("the path 'v10_4_1.type.aes.managePasswordEncryption.true.' is not a valid key-value pair"), err4},
		{"can traverse tree sub path - 3 generations sub path 4", "", subPath4},
		{"can traverse tree sub path - 3 generations error 5", nil, err5},
		{"can traverse tree sub path - 3 generations sub path 5", "v10_4_1.type.aes.managePasswordEncryption.false", subPath5},
		{"can traverse tree sub path - 3 generations error 6", nil, err6},
		{"can traverse tree sub path - 3 generations sub path 6", "v10_4_1.type.aes.managePasswordEncryption.false", subPath6},
		{"can traverse tree sub path - 3 generations error 7", fmt.Errorf("the path 'v10_4_1.type.aes.managePasswordEncryption.false.' is not a valid key-value pair"), err7},
		{"can traverse tree sub path - 3 generations sub path 7", "", subPath7},
		{"can traverse tree sub path - 3 generations error 8", nil, err8},
		{"can traverse tree sub path - 3 generations sub path 8", "v10_4_1.type.aes.managePasswordEncryption.test", subPath8},
		{"can traverse tree sub path - 3 generations error 9", nil, err9},
		{"can traverse tree sub path - 3 generations sub path 9", "v10_4_1.type.aes.managePasswordEncryption.test", subPath9},
		{"can traverse tree sub path - 3 generations error 10", fmt.Errorf("the path 'v10_4_1.type.aes.managePasswordEncryption.test.' is not a valid key-value pair"), err10},
		{"can traverse tree sub path - 3 generations sub path 10", "", subPath10},
	}

	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func testGetPathFromLeafIndex(testName string, treeMap map[string]interface{}, validPath string, i int) (Test, Test) {
	path, err := GetPathFromLeafIndex(treeMap, strings.Split(validPath, ".")[0], GetLeafIndex(treeMap, validPath))
	return Test{
			test:     testName + " error " + fmt.Sprint(i),
			expected: nil,
			actual:   err,
		}, Test{
			test:     testName + " path " + fmt.Sprint(i),
			expected: validPath,
			actual:   path,
		}
}

func getControllersFolder() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "/../controllers"
	}
	return cwd + "/../controllers"
}
