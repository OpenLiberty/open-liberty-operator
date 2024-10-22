package tree

var TreeCache DecisionTreeCache

type DecisionTreeCache struct {
	decisionTrees   map[string]DecisionTree
	operatorVersion string
}

type DecisionTree struct {
	treeMap    map[string]interface{}
	replaceMap map[string]map[string]string
}

func (dtc *DecisionTree) SetTreeMap(treeMap map[string]interface{}) {
	dtc.treeMap = treeMap
}

func (dtc *DecisionTree) SetReplaceMap(replaceMap map[string]map[string]string) {
	dtc.replaceMap = replaceMap
}

func (dtc *DecisionTree) TreeMap() map[string]interface{} {
	return dtc.treeMap
}

func (dtc *DecisionTree) ReplaceMap() map[string]map[string]string {
	return dtc.replaceMap
}

func (dtc *DecisionTree) Clear() {
	if dtc.treeMap != nil {
		for k, _ := range dtc.treeMap {
			delete(dtc.treeMap, k)
		}
	}
	if dtc.replaceMap != nil {
		for k, _ := range dtc.replaceMap {
			delete(dtc.replaceMap, k)
		}
	}
}

func (dtc *DecisionTreeCache) TreeMap(key string) map[string]interface{} {
	if decisionTree, found := dtc.decisionTrees[key]; found {
		return decisionTree.TreeMap()
	}
	return nil
}

func (dtc *DecisionTreeCache) ReplaceMap(key string) map[string]map[string]string {
	if decisionTree, found := dtc.decisionTrees[key]; found {
		return decisionTree.ReplaceMap()
	}
	return nil
}

func (dtc *DecisionTreeCache) Clear() {
	if dtc.decisionTrees == nil {
		return
	}
	for k, decisionTree := range dtc.decisionTrees {
		decisionTree.Clear()
		delete(dtc.decisionTrees, k)
	}
}

func (dtc *DecisionTreeCache) SetOperatorVersion(operatorVersion string) {
	// clear the cache if the operatorVersion has changed
	if dtc.operatorVersion != operatorVersion {
		dtc.Clear()
	}
	dtc.operatorVersion = operatorVersion
}
