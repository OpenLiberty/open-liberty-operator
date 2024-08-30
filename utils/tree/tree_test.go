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

func verifyTests(tests []Test) error {
	for _, tt := range tests {
		if !reflect.DeepEqual(tt.actual, tt.expected) {
			return fmt.Errorf("%s test expected: (%v) actual: (%v)", tt.test, tt.expected, tt.actual)
		}
	}
	return nil
}

func TestParseDecisionTree(t *testing.T) {
	// Test 2 generations
	fileName := getControllerFolder() + "/tests/ltpa-decision-tree-2-generations.yaml"
	treeMap, replaceMap, err := ParseDecisionTree("ltpa", &fileName)

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
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-3-generations.yaml"
	treeMap, replaceMap, err = ParseDecisionTree("ltpa", &fileName)

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
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-1-generation.yaml"
	treeMap, replaceMap, err = ParseDecisionTree("ltpa", &fileName)

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
	fileName := getControllerFolder() + "/tests/ltpa-decision-tree-1-generation.yaml"
	treeMap, _, _ := ParseDecisionTree("ltpa", &fileName)
	latestOperandVersion1, err1 := GetLatestOperandVersion(treeMap, "v10_3_3")

	// Test 2 generations
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-2-generations.yaml"
	treeMap, _, _ = ParseDecisionTree("ltpa", &fileName)
	latestOperandVersion2, err2 := GetLatestOperandVersion(treeMap, "v10_3_999")

	// Test 3 generations
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-3-generations.yaml"
	treeMap, _, _ = ParseDecisionTree("ltpa", &fileName)
	latestOperandVersion3, err3 := GetLatestOperandVersion(treeMap, "v10_4_1")

	// Test complex
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, _, _ = ParseDecisionTree("ltpa", &fileName)
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
	fileName := getControllerFolder() + "/tests/ltpa-decision-tree-1-generation.yaml"
	treeMap, replaceMap, _ := ParseDecisionTree("ltpa", &fileName)
	newPath1a, err1a := ReplacePath("v10_3_3", "v10_3_3", treeMap, replaceMap)

	// Test 1 generation - empty replace map and attempting to upgrade to an invalid version
	newPath1b, err1b := ReplacePath("v10_3_3", "v20_30_30", treeMap, replaceMap)

	// Test 2 generations - upgrade from first to second generation
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-2-generations.yaml"
	treeMap, replaceMap, _ = ParseDecisionTree("ltpa", &fileName)
	newPath2a, err2a := ReplacePath("v10_4_0.managePasswordEncryption.false", "v10_4_1", treeMap, replaceMap)

	// Test 2 generations - upgrade from second to first generation
	newPath2b, err2b := ReplacePath("v10_4_1.type.aes.managePasswordEncryption.true", "v10_4_0", treeMap, replaceMap)

	// Test 3 generations - upgrade from first to third generation
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-3-generations.yaml"
	treeMap, replaceMap, _ = ParseDecisionTree("ltpa", &fileName)
	newPath3a, err3a := ReplacePath("v10_3_3.manageLTPA.true", "v10_4_1", treeMap, replaceMap)

	// Test 3 generations - upgrade from third to first generation
	newPath3b, err3b := ReplacePath("v10_4_1.type.aes.managePasswordEncryption.false", "v10_3_3", treeMap, replaceMap)

	// Test 3 generations - upgrade from third to first generation
	// However, it can only upgrade up to the second generation because no valid upgrade path exists from second to first generation
	newPath3c, err3c := ReplacePath("v10_4_1.type.aes.managePasswordEncryption.true", "v10_3_3", treeMap, replaceMap)

	// Test 3 generations - same as 3c but upgrading from second to first generation
	newPath3d, err3d := ReplacePath("v10_4_0.managePasswordEncryption.true", "v10_3_3", treeMap, replaceMap)

	// Test complex
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, replaceMap, _ = ParseDecisionTree("ltpa", &fileName)
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
	fileName := getControllerFolder() + "/tests/ltpa-decision-tree-1-generation.yaml"
	treeMap, _, _ := ParseDecisionTree("ltpa", &fileName)
	tests := []Test{
		{"get leaf index - 1 generation a", -1, GetLeafIndex(treeMap, "")},
		{"get leaf index - 1 generation b", 0, GetLeafIndex(treeMap, "v10_3_3.test")},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}

	// Test 2 generations
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-2-generations.yaml"
	treeMap, _, _ = ParseDecisionTree("ltpa", &fileName)
	tests = []Test{
		{"get leaf index - 2 generations", -1, GetLeafIndex(treeMap, "")},
		// valid paths
		{"get leaf index - 2 generations a", 0, GetLeafIndex(treeMap, "v10_4_0.managePasswordEncryption.true")},
		{"get leaf index - 2 generations b", 1, GetLeafIndex(treeMap, "v10_4_0.managePasswordEncryption.false")},
		{"get leaf index - 2 generations c", 0, GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.true")},
		{"get leaf index - 2 generations d", 1, GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.false")},
		{"get leaf index - 2 generations e", 2, GetLeafIndex(treeMap, "v10_4_1.type.xor.type")},
		// invalid paths
		{"get leaf index - 2 generations f", -1, GetLeafIndex(treeMap, "v10_4_0.managePasswordEncryption")},
		{"get leaf index - 2 generations g", -1, GetLeafIndex(treeMap, "v10_4_0.managePasswordEncryption.random")},
		{"get leaf index - 2 generations h", -1, GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption")},
		{"get leaf index - 2 generations i", -1, GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption")},
		{"get leaf index - 2 generations j", -1, GetLeafIndex(treeMap, "v10_4_1.type.aes")},
		{"get leaf index - 2 generations k", -1, GetLeafIndex(treeMap, "v10_4_1.type")},
		{"get leaf index - 2 generations l", -1, GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.true.false")},
		// syntax errors, incorrect elements
		{"get leaf index - 2 generations m", -1, GetLeafIndex(treeMap, "v10_4_1.type.aes.")},
		{"get leaf index - 2 generations n", -1, GetLeafIndex(treeMap, "v10_4_1.type.")},
		{"get leaf index - 2 generations o", -1, GetLeafIndex(treeMap, "v10_4_1.ty")},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
	// Test 3 generations
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-3-generations.yaml"
	treeMap, _, _ = ParseDecisionTree("ltpa", &fileName)
	tests = []Test{
		{"get leaf index - 3 generations", -1, GetLeafIndex(treeMap, "")},
		// valid paths
		{"get leaf index - 3 generations a", 0, GetLeafIndex(treeMap, "v10_3_3.manageLTPA.true")},
		{"get leaf index - 3 generations b", 0, GetLeafIndex(treeMap, "v10_4_0.managePasswordEncryption.true")},
		{"get leaf index - 3 generations c", 1, GetLeafIndex(treeMap, "v10_4_0.managePasswordEncryption.false")},
		{"get leaf index - 3 generations d", 0, GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.true")},
		{"get leaf index - 3 generations e", 1, GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.false")},
		{"get leaf index - 3 generations f", 2, GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.test")},
		{"get leaf index - 3 generations g", 3, GetLeafIndex(treeMap, "v10_4_1.type.xor.type")},
		// invalid paths
		{"get leaf index - 3 generations h", -1, GetLeafIndex(treeMap, "v10_4_1.type.xor.type.test")},
		{"get leaf index - 3 generations i", -1, GetLeafIndex(treeMap, "v10_4_1.type.aes")},
		{"get leaf index - 3 generations j", -1, GetLeafIndex(treeMap, "v10_4_1.type.aes.managePasswordEncryption.true.v10_3_3.manageLTPA.true")},
		{"get leaf index - 3 generations k", -1, GetLeafIndex(treeMap, "v10_4_1.v10_3_3.manageLTPA.true")},
		{"get leaf index - 3 generations l", -1, GetLeafIndex(treeMap, "v10_3_3.manageLTPA.true.v10_4_0")},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
	// Test complex
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, _, _ = ParseDecisionTree("ltpa", &fileName)
	tests = []Test{
		{"get leaf index - complex generations", -1, GetLeafIndex(treeMap, "")},
		{"get leaf index - complex generations a", 0, GetLeafIndex(treeMap, "v10_3_3.a.b")},
		{"get leaf index - complex generations b", 0, GetLeafIndex(treeMap, "v10_4_1.a.b.c.true")},
		{"get leaf index - complex generations c", 1, GetLeafIndex(treeMap, "v10_4_1.a.b.d.true")},
		{"get leaf index - complex generations d", 2, GetLeafIndex(treeMap, "v10_4_1.a.b.e.true")},
		{"get leaf index - complex generations e", 3, GetLeafIndex(treeMap, "v10_4_1.a.b.e.false")},
		{"get leaf index - complex generations f", 0, GetLeafIndex(treeMap, "v10_4_20.a.b.c.true")},
		{"get leaf index - complex generations g", 1, GetLeafIndex(treeMap, "v10_4_20.a.b.d.false")},
		{"get leaf index - complex generations h", 2, GetLeafIndex(treeMap, "v10_4_20.a.b.e.foo")},
		{"get leaf index - complex generations i", 3, GetLeafIndex(treeMap, "v10_4_20.a.f.g.i.bar")},
		{"get leaf index - complex generations j", 4, GetLeafIndex(treeMap, "v10_4_20.a.f.h.element")},
		{"get leaf index - complex generations k", 0, GetLeafIndex(treeMap, "v10_4_500.a.b.b.true")},
		{"get leaf index - complex generations l", 1, GetLeafIndex(treeMap, "v10_4_500.a.b.c.true")},
		{"get leaf index - complex generations m", 2, GetLeafIndex(treeMap, "v10_4_500.a.b.d.false")},
		{"get leaf index - complex generations n", 3, GetLeafIndex(treeMap, "v10_4_500.a.b.e.foo")},
		{"get leaf index - complex generations o", 4, GetLeafIndex(treeMap, "v10_4_500.a.f.g.i.bar")},
		{"get leaf index - complex generations p", 5, GetLeafIndex(treeMap, "v10_4_500.a.f.h.element")},
	}
	if err := verifyTests(tests); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestGetPathFromLeafIndex(t *testing.T) {
	// Test 1 generation
	fileName := getControllerFolder() + "/tests/ltpa-decision-tree-1-generation.yaml"
	treeMap, _, _ := ParseDecisionTree("ltpa", &fileName)
	tests := []Test{}
	validPaths := []string{"v10_3_3.test"}
	for i, path := range validPaths {
		errTest, pathTest := testGetPathFromLeafIndex("get path from leaf index - 1 generation", treeMap, path, i)
		tests = append(tests, errTest)
		tests = append(tests, pathTest)
	}

	// Test 2 generations
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-2-generations.yaml"
	treeMap, _, _ = ParseDecisionTree("ltpa", &fileName)
	// valid paths
	validPaths = []string{"v10_4_0.managePasswordEncryption.true", "v10_4_0.managePasswordEncryption.false",
		"v10_4_1.type.aes.managePasswordEncryption.true", "v10_4_1.type.aes.managePasswordEncryption.false", "v10_4_1.type.xor.type"}
	for i, path := range validPaths {
		errTest, pathTest := testGetPathFromLeafIndex("get path from leaf index - 2 generations", treeMap, path, i)
		tests = append(tests, errTest)
		tests = append(tests, pathTest)
	}

	// Test 3 generations
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-3-generations.yaml"
	treeMap, _, _ = ParseDecisionTree("ltpa", &fileName)
	// valid paths
	validPaths = []string{"v10_3_3.manageLTPA.true", "v10_4_0.managePasswordEncryption.true", "v10_4_0.managePasswordEncryption.false",
		"v10_4_1.type.aes.managePasswordEncryption.true", "v10_4_1.type.aes.managePasswordEncryption.false", "v10_4_1.type.xor.type"}
	for i, path := range validPaths {
		errTest, pathTest := testGetPathFromLeafIndex("get path from leaf index - 3 generations", treeMap, path, i)
		tests = append(tests, errTest)
		tests = append(tests, pathTest)
	}

	// Test complex
	fileName = getControllerFolder() + "/tests/ltpa-decision-tree-complex.yaml"
	treeMap, _, _ = ParseDecisionTree("ltpa", &fileName)
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
	fileName := getControllerFolder() + "/tests/ltpa-decision-tree-3-generations.yaml"
	treeMap, _, _ := ParseDecisionTree("ltpa", &fileName)

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

func getControllerFolder() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "/../../internal/controller"
	}
	return cwd + "/../../internal/controller"
}
