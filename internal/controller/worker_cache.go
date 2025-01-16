package controller

import (
	"fmt"
	"strings"
	"sync"
)

type WorkerCache struct {
	store *sync.Map
}

const WORKER_KEY = "worker"
const CERTMANAGER_WORKER_KEY = "cm-worker"
const ALLOWED_WORKER_KEY = "allowed-worker"
const MAX_WORKERS = 10
const MAX_CERTMANAGER_WORKERS = 5

type Worker int

const (
	WORKER             Worker = iota
	CERTMANAGER_WORKER Worker = iota
)

func (wc *WorkerCache) Init() {
	wc.store = &sync.Map{}
}

func (wc *WorkerCache) GetTotalWorkers(worker Worker) int {
	if worker == CERTMANAGER_WORKER {
		return wc.countWorkers(CERTMANAGER_WORKER_KEY)
	}
	return wc.countWorkers(WORKER_KEY)
}

func (wc *WorkerCache) GetMaxWorkers(worker Worker) int {
	if worker == CERTMANAGER_WORKER {
		return MAX_CERTMANAGER_WORKERS
	}
	return MAX_WORKERS
}

func (wc *WorkerCache) countWorkers(workerKey string) int {
	workers := 0
	wc.store.Range(func(key, value any) bool {
		if strings.HasPrefix(key.(string), workerKey) {
			workers += 1
		}
		return true
	})
	return workers
}

func getWorkerKey(worker Worker, namespace, name string) string {
	if worker == CERTMANAGER_WORKER {
		return getGenericKey(CERTMANAGER_WORKER_KEY, namespace, name)
	}
	return getGenericKey(WORKER_KEY, namespace, name)
}

func getCertManagerWorkerKey(namespace, name string) string {
	return getGenericKey(WORKER_KEY, namespace, name)
}

func getAllowedWorkerKey(namespace, name string) string {
	return getGenericKey(ALLOWED_WORKER_KEY, namespace, name)
}

func getGenericKey(rootKey, namespace, name string) string {
	return fmt.Sprintf("%s-%s-%s", rootKey, namespace, name)
}

// Reserves space in the cache for a working instance
func (wc *WorkerCache) ReserveWorkingInstance(worker Worker, namespace, name string) bool {
	allowedWorkerKey := getAllowedWorkerKey(namespace, name)
	if _, ok := wc.store.Load(allowedWorkerKey); ok {
		return true
	}
	workerKey := getWorkerKey(worker, namespace, name)
	if _, ok := wc.store.Load(workerKey); ok {
		return true
	}
	if wc.GetTotalWorkers(worker) < wc.GetMaxWorkers(worker) {
		wc.store.Store(workerKey, 0)
		return true
	}
	return false
}

// Release this instance from the worker queue and allow this worker to bypass queue on a subsequent cache lookup
func (wc *WorkerCache) ReleaseWorkingInstance(worker Worker, namespace, name string) {
	wc.store.Delete(getWorkerKey(worker, namespace, name))
	wc.store.Store(getAllowedWorkerKey(namespace, name), 0)
}
