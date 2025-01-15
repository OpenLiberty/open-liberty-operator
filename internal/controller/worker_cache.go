package controller

import (
	"fmt"
	"sync"
)

type WorkerCache struct {
	store *sync.Map
}

const WORKER_KEY = "worker"
const ALLOWED_WORKER_KEY = "allowed-worker"
const MAX_WORKERS = 15

func (wc *WorkerCache) Init() {
	wc.store = &sync.Map{}
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
	return fmt.Sprintf("%s-%s-%s", WORKER_KEY, namespace, name)
}

func getAllowedWorkerKey(namespace, name string) string {
	return fmt.Sprintf("%s-%s-%s", ALLOWED_WORKER_KEY, namespace, name)
}

// Reserves space in the cache for a working instance
func (wc *WorkerCache) ReserveWorkingInstance(namespace, name string) bool {
	allowedWorkerKey := getAllowedWorkerKey(namespace, name)
	if _, ok := wc.store.Load(allowedWorkerKey); ok {
		return true
	}
	workerKey := getWorkerKey(namespace, name)
	if _, ok := wc.store.Load(workerKey); ok {
		return true
	}
	if wc.GetTotalWorkers() < MAX_WORKERS {
		wc.store.Store(workerKey, 0)
		return true
	}
	return false
}

// Release this instance from the worker queue and allow this worker to bypass queue on a subsequent cache lookup
func (wc *WorkerCache) ReleaseWorkingInstance(namespace, name string) {
	wc.store.Delete(getWorkerKey(namespace, name))
	wc.store.Store(getAllowedWorkerKey(namespace, name), 0)
}
