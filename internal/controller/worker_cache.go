package controller

import (
	"fmt"
	"sync"
)

type WorkerCache struct {
	store *sync.Map
}

const WORKERS_KEY = "worker"
const MAX_WORKERS = 15

func (wc *WorkerCache) Init() {
	wc.store = &sync.Map{}
	wc.store.Store(WORKERS_KEY, 0)
}

func (wc *WorkerCache) GetTotalWorkers() int {
	workers := 0
	wc.store.Range(func(key, value any) bool {
		workers += 1
		return true
	})
	return workers
}

func getWorkerKey(namespace, name string) string {
	return fmt.Sprintf("%s-%s-%s", WORKERS_KEY, namespace, name)
}

// Reserves space in the cache for a working instance
func (wc *WorkerCache) ReserveWorkingInstance(namespace, name string) bool {
	workerKey := getWorkerKey(namespace, name)
	_, ok := wc.store.Load(workerKey)
	if ok {
		return true
	}
	if wc.GetTotalWorkers() < MAX_WORKERS {
		wc.store.Store(workerKey, 0)
		return true
	}
	return false
}

func (wc *WorkerCache) ReleaseWorkingInstance(namespace, name string) {
	wc.store.Delete(getWorkerKey(namespace, name))
}
