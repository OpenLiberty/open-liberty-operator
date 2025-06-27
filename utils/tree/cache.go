package tree

import "sync"

var TreeCache *DecisionTreeCache

func init() {
	TreeCache = &DecisionTreeCache{
		decisionTrees:       map[string]*decisionTree{},
		decisionTreeRecords: map[string]*cacheRecord{}, // for cache invalidation
		mutex:               &sync.Mutex{},
	}
}

type cacheRecord struct {
	fileName         string
	lastModifiedTime int64
}

type DecisionTreeCache struct {
	decisionTrees       map[string]*decisionTree
	mutex               *sync.Mutex
	decisionTreeRecords map[string]*cacheRecord
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

func (dtc *DecisionTreeCache) Maps(key string, treeFileName string, lastModifiedTime int64) (map[string]interface{}, map[string]map[string]string) {
	dtc.mutex.Lock()
	defer dtc.mutex.Unlock()

	decisionTreeRecord, found := dtc.decisionTreeRecords[key]
	if !found || decisionTreeRecord == nil {
		return nil, nil // no cache record, so return nil
	}
	if decisionTreeRecord.fileName != treeFileName || decisionTreeRecord.lastModifiedTime != lastModifiedTime {
		dtc.performCacheClear()
		return nil, nil // cache record mismatch, so invalidate the cache
	}
	// cache record match - return the tree cache if it is written to memory
	if decisionTree, found := dtc.decisionTrees[key]; found {
		return decisionTree.treeMap, decisionTree.replaceMap
	}
	return nil, nil
}

// Performs clear operation on decision tree cache
func (dtc *DecisionTreeCache) performCacheClear() {
	if dtc.decisionTrees == nil {
		return
	}
	keys := []string{}
	for key, decisionTree := range dtc.decisionTrees {
		decisionTree.clear()
		keys = append(keys, key)
	}
	for _, key := range keys {
		delete(dtc.decisionTrees, key)
	}
}

func (dtc *DecisionTreeCache) Clear() {
	dtc.mutex.Lock()
	defer dtc.mutex.Unlock()
	dtc.performCacheClear()
}

func (dtc *DecisionTreeCache) SetDecisionTree(decisionTreeName string, treeMap map[string]interface{}, replaceMap map[string]map[string]string, treeFileName string, lastModifiedTime int64) {
	dtc.mutex.Lock()
	defer dtc.mutex.Unlock()
	dtc.decisionTrees[decisionTreeName] = &decisionTree{treeMap, replaceMap}
	dtc.decisionTreeRecords[decisionTreeName] = &cacheRecord{treeFileName, lastModifiedTime}
}
