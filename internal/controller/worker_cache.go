package controller

import (
	"container/heap"
	"fmt"
	"strings"
	"sync"
)

type WorkerCache struct {
	store                 *sync.Map
	pq                    PriorityQueue
	maxWorkers            int
	maxCertManagerWorkers int
	workCount             int
}

const WORKER_KEY = "worker"
const CERTMANAGER_WORKER_KEY = "cm-worker"
const ALLOWED_CERTMANAGER_WORKER_KEY = "allowed-" + CERTMANAGER_WORKER_KEY
const ALLOWED_WORKER_KEY = "allowed-" + WORKER_KEY

type Worker int

const (
	WORKER             Worker = iota
	CERTMANAGER_WORKER Worker = iota
)

func (wc *WorkerCache) Init(maxWorkers, maxCertManagerWorkers int) {
	wc.store = &sync.Map{}
	wc.maxWorkers = maxWorkers
	wc.maxCertManagerWorkers = maxCertManagerWorkers
	wc.pq = make(PriorityQueue, 0)
	wc.workCount = 0
}

func (wc *WorkerCache) GetTotalWorkers(worker Worker) int {
	if worker == CERTMANAGER_WORKER {
		return wc.countWorkers(CERTMANAGER_WORKER_KEY)
	}
	return wc.countWorkers(WORKER_KEY)
}

func (wc *WorkerCache) GetMaxWorkers(worker Worker) int {
	if worker == CERTMANAGER_WORKER {
		return wc.maxCertManagerWorkers
	}
	return wc.maxWorkers
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
		return getNamespacedKey(CERTMANAGER_WORKER_KEY, namespace)
	}
	return getGenericKey(WORKER_KEY, namespace, name)
}

func getAllowedWorkerKey(worker Worker, namespace, name string) string {
	if worker == CERTMANAGER_WORKER {
		return getNamespacedKey(ALLOWED_CERTMANAGER_WORKER_KEY, namespace)
	}
	return getGenericKey(ALLOWED_WORKER_KEY, namespace, name)
}

func getGenericKey(rootKey, namespace, name string) string {
	return fmt.Sprintf("%s-%s-%s", rootKey, namespace, name)
}

func getNamespacedKey(rootKey, namespace string) string {
	return fmt.Sprintf("%s-%s", rootKey, namespace)
}

// Reserves space in the cache for a working instance
func (wc *WorkerCache) ReserveWorkingInstance(worker Worker, namespace, name string) bool {
	allowedWorkerKey := getAllowedWorkerKey(worker, namespace, name)
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
	wc.store.Store(getAllowedWorkerKey(worker, namespace, name), 0)
}

func createItem(namespace, name string, resource *Resource) *Item {
	return &Item{resource: resource, namespace: namespace, name: name, priority: resource.priority, index: 0}
}

func (wc *WorkerCache) CreateWork(namespace, name string, resource *Resource) bool {
	wc.workCount += 1
	heap.Push(&wc.pq, createItem(namespace, name, resource))
	return true
}

func (wc *WorkerCache) GetWork() *Item {
	if wc.workCount > 0 {
		wc.workCount -= 1
		return heap.Pop(&wc.pq).(*Item)
	}
	return nil
}
