package tree

import "sync"

var TreeCache *DecisionTreeCache

func init() {
	TreeCache = &DecisionTreeCache{
		decisionTrees: map[string]*decisionTree{},
		mutex:         &sync.Mutex{},
	}
}

type DecisionTreeCache struct {
	decisionTrees map[string]*decisionTree
	mutex         *sync.Mutex
}

type decisionTree struct {
	treeMap    map[string]interface{}
	replaceMap map[string]map[string]string
}

func (dtc *decisionTree) setTreeMap(treeMap map[string]interface{}) {
	dtc.treeMap = treeMap
}

func (dtc *decisionTree) setReplaceMap(replaceMap map[string]map[string]string) {
	dtc.replaceMap = replaceMap
}

func (dtc *decisionTree) clear() {
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

func (dtc *DecisionTreeCache) Maps(key string) (map[string]interface{}, map[string]map[string]string) {
	dtc.mutex.Lock()
	defer dtc.mutex.Unlock()

	if decisionTree, found := dtc.decisionTrees[key]; found {
		return decisionTree.treeMap, decisionTree.replaceMap
	}
	return nil, nil
}

func (dtc *DecisionTreeCache) Clear() {
	dtc.mutex.Lock()
	defer dtc.mutex.Unlock()

	if dtc.decisionTrees == nil {
		return
	}
	for key, decisionTree := range dtc.decisionTrees {
		decisionTree.clear()
		delete(dtc.decisionTrees, key)
	}
}

func (dtc *DecisionTreeCache) SetDecisionTree(decisionTreeName string, treeMap map[string]interface{}, replaceMap map[string]map[string]string) {
	dtc.mutex.Lock()
	defer dtc.mutex.Unlock()
	dtc.decisionTrees[decisionTreeName] = &decisionTree{treeMap, replaceMap}
}
