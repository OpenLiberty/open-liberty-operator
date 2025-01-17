package controller

import (
	"container/heap"
	"fmt"
	"strings"
	"sync"
	"time"
)

type WorkerCache struct {
	store                 *sync.Map
	issuerQueue           PriorityQueue
	certificateQueue      PriorityQueue
	maxWorkers            int
	maxCertManagerWorkers int
	issuerWorkCount       int
	certificateWorkCount  int
	visited               *sync.Map
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
	wc.issuerQueue = make(PriorityQueue, 0)
	wc.certificateQueue = make(PriorityQueue, 0)
	wc.issuerWorkCount = 0
	wc.certificateWorkCount = 0
	wc.visited = &sync.Map{}
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
	resource.namespace = namespace
	resource.name = name
	return &Item{resource: resource, namespace: namespace, name: name, priority: resource.priority, index: 0}
}

func getWorkKey(namespace, name string, resource *Resource) string {
	return fmt.Sprintf("%s-%s-%s", resource.resourceName, namespace, name)
}

func (wc *WorkerCache) CreateIssuerWork(namespace, name string, resource *Resource) bool {
	workKey := getWorkKey(namespace, name, resource)
	if _, ok := wc.visited.Load(workKey); !ok {
		wc.visited.Store(workKey, time.Now().Unix())
		wc.issuerWorkCount += 1
		heap.Push(&wc.issuerQueue, createItem(namespace, name, resource))
		return true
	}
	return false
}

func (wc *WorkerCache) PeekIssuerWork() *Item {
	if wc.issuerWorkCount > 0 {
		item := wc.GetIssuerWork()
		wc.CreateIssuerWork(item.namespace, item.name, item.resource)
	}
	return nil
}

func (wc *WorkerCache) GetIssuerWork() *Item {
	if wc.issuerWorkCount > 0 {
		wc.issuerWorkCount -= 1
		item := heap.Pop(&wc.issuerQueue).(*Item)
		wc.visited.Delete(getWorkKey(item.namespace, item.name, item.resource))
		return item
	}
	return nil
}

func (wc *WorkerCache) CreateCertificateWork(namespace, name string, resource *Resource) bool {
	workKey := getWorkKey(namespace, name, resource)
	if _, ok := wc.visited.Load(workKey); !ok {
		wc.visited.Store(workKey, time.Now().Unix())
		wc.certificateWorkCount += 1
		heap.Push(&wc.certificateQueue, createItem(namespace, name, resource))
		return true
	}
	return false
}

func (wc *WorkerCache) PeekCertificateWork() *Item {
	if wc.certificateWorkCount > 0 {
		item := wc.GetCertificateWork()
		wc.CreateCertificateWork(item.namespace, item.name, item.resource)
	}
	return nil
}

func (wc *WorkerCache) GetCertificateWork() *Item {
	if wc.certificateWorkCount > 0 {
		wc.certificateWorkCount -= 1
		item := heap.Pop(&wc.certificateQueue).(*Item)
		wc.visited.Delete(getWorkKey(item.namespace, item.name, item.resource))
		return item
	}
	return nil
}
