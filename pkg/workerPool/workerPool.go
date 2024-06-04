package workerPool

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
)

type WorkerPool struct {
	config    Config
	taskQueue chan Task
}

type Config struct {
	WorkerCount  int
	GlobalBuffer int
}

type Room struct {
	taskCounter          atomic.Uint64
	result               []TaskResult
	resultMutex          sync.Mutex
	asyncCollectorWait   sync.WaitGroup
	asyncCollectorActive atomic.Bool
	bufferSize           int
	resultChan           chan TaskResult
	wg                   sync.WaitGroup
	wp                   *WorkerPool
}

type Task struct {
	run        func() interface{}
	room       *Room
	taskNumber uint64
}

type TaskResult struct {
	taskNumber uint64
	result     interface{}
}

func NewWorkerPool(config Config) *WorkerPool {
	if config.WorkerCount < 1 {
		numberOfCPUs := runtime.NumCPU()
		numberOfWorkers := (numberOfCPUs * 3)
		config.WorkerCount = numberOfWorkers
	}

	if config.GlobalBuffer < 1 {
		config.GlobalBuffer = 10000
	}

	taskQueue := make(chan Task, config.GlobalBuffer)

	wp := &WorkerPool{
		config:    config,
		taskQueue: taskQueue,
	}

	for i := 0; i < config.WorkerCount; i++ {
		go wp.Worker()
	}

	return wp
}

func (wp *WorkerPool) Worker() {
	for t := range wp.taskQueue {
		t.room.resultChan <- TaskResult{
			taskNumber: t.taskNumber,
			result:     t.run(),
		}
		t.room.wg.Done()
	}
}

func (wp *WorkerPool) CreateRoom(size int) *Room {
	return &Room{
		bufferSize:         size,
		resultChan:         make(chan TaskResult, size),
		wp:                 wp,
		asyncCollectorWait: sync.WaitGroup{},
	}
}

func (ro *Room) NewTaskWaitForFreeSlot(job func() interface{}) (taskID uint64) {
	task := Task{
		run:        job,
		room:       ro,
		taskNumber: ro.taskCounter.Add(1),
	}

	ro.wg.Add(1)
	ro.wp.taskQueue <- task
	return task.taskNumber
}

func (ro *Room) NewTask(job func() interface{}) (taskID uint64, err error) {
	if len(ro.wp.taskQueue) == cap(ro.wp.taskQueue) {
		return 0, fmt.Errorf("Global buffer is full. Please wait for some tasks to finish. Or increase the buffer size.")
	}

	if len(ro.resultChan) == cap(ro.resultChan) {
		return 0, fmt.Errorf("Room buffer is full. Please wait for some tasks to finish. Or increase the buffer size.")
	}

	taskID = ro.NewTaskWaitForFreeSlot(job)
	return
}

func (ro *Room) Collect() []interface{} {
	go ro.WaitAndClose()
	results := make([]TaskResult, 0)

	for result := range ro.resultChan {
		results = append(results, result)
	}

	return sortResults(results)
}

func (ro *Room) AsyncCollector() {
	if ro.asyncCollectorActive.Load() {
		return
	}

	ro.asyncCollectorActive.Store(true)
	ro.asyncCollectorWait.Add(1)

	go func() {
		defer ro.asyncCollectorActive.Store(false)
		defer ro.asyncCollectorWait.Done()

		ro.resultMutex.Lock()
		for result := range ro.resultChan {
			ro.result = append(ro.result, result)
		}
		ro.resultMutex.Unlock()
	}()
}

func (ro *Room) GetAsyncResults() ([]interface{}, error) {
	go ro.WaitAndClose()
	ro.asyncCollectorWait.Wait()
	ro.resultMutex.Lock()
	defer ro.resultMutex.Unlock()

	return sortResults(ro.result), nil
}

func (ro *Room) WaitAndClose() {
	ro.wg.Wait()
	close(ro.resultChan)
}

func sortResults(tasks []TaskResult) []interface{} {
	results := make([]interface{}, len(tasks))
	for _, task := range tasks {
		results[task.taskNumber-1] = task.result
	}

	return results
}
