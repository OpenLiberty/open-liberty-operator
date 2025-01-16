package controller

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type WorkerCache struct {
	store *sync.Map
}

const WORKER_KEY = "worker"
const CERTMANAGER_WORKER_KEY = "cm-worker"
const ALLOWED_CERTMANAGER_WORKER_KEY = "allowed-" + CERTMANAGER_WORKER_KEY
const ALLOWED_WORKER_KEY = "allowed-" + WORKER_KEY
const MAX_WORKERS = 15
const MAX_CERTMANAGER_WORKERS = 10

const DELAY_WORKER_TIME_KEY = "delay-worker-time"
const DELAY_WORKER_COUNT_KEY = "delay-worker-count"
const WORKER_DELAY = 3
const MAX_WORKER_PER_DELAY = 5

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
	now := time.Now().Unix()
	// worker should have delay
	if worker == WORKER {
		if lastTime, ok := wc.store.Load(DELAY_WORKER_TIME_KEY); ok {
			lastCount, countOk := wc.store.Load(DELAY_WORKER_COUNT_KEY)
			if countOk {
				// exit if worker is queued too early
				if now-lastTime.(int64) < WORKER_DELAY && lastCount.(int) >= MAX_WORKER_PER_DELAY {
					return false
				}
			} else {
				// exit if worker is queued too early
				if now-lastTime.(int64) < WORKER_DELAY {
					return false
				}
			}
		}
	}

	workerKey := getWorkerKey(worker, namespace, name)
	if _, ok := wc.store.Load(workerKey); ok {
		if worker == WORKER {
			// save the last worked time
			if lastTime, ok := wc.store.Load(DELAY_WORKER_TIME_KEY); ok {
				if now-lastTime.(int64) < WORKER_DELAY {
					val, _ := wc.store.Load(DELAY_WORKER_COUNT_KEY)
					wc.store.Store(DELAY_WORKER_COUNT_KEY, val.(int)+1)
				} else {
					wc.store.Store(DELAY_WORKER_TIME_KEY, now)
					wc.store.Store(DELAY_WORKER_COUNT_KEY, 0)
				}
			} else {
				wc.store.Store(DELAY_WORKER_TIME_KEY, now)
				wc.store.Store(DELAY_WORKER_COUNT_KEY, 0)
			}
		}
		return true
	}
	if wc.GetTotalWorkers(worker) < wc.GetMaxWorkers(worker) {
		wc.store.Store(workerKey, 0)
		if worker == WORKER {
			// save the last worked time
			if lastTime, ok := wc.store.Load(DELAY_WORKER_TIME_KEY); ok {
				if now-lastTime.(int64) < WORKER_DELAY {
					val, _ := wc.store.Load(DELAY_WORKER_COUNT_KEY)
					wc.store.Store(DELAY_WORKER_COUNT_KEY, val.(int)+1)
				} else {
					wc.store.Store(DELAY_WORKER_TIME_KEY, now)
					wc.store.Store(DELAY_WORKER_COUNT_KEY, 0)
				}
			} else {
				wc.store.Store(DELAY_WORKER_TIME_KEY, now)
				wc.store.Store(DELAY_WORKER_COUNT_KEY, 0)
			}
		}
		return true
	}
	return false
}

// Release this instance from the worker queue and allow this worker to bypass queue on a subsequent cache lookup
func (wc *WorkerCache) ReleaseWorkingInstance(worker Worker, namespace, name string) {
	wc.store.Delete(getWorkerKey(worker, namespace, name))
	wc.store.Store(getAllowedWorkerKey(worker, namespace, name), 0)
}
