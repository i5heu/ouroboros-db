package workerpool

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
)

type WorkerPool struct {
	config         Config
	taskQueue      chan Task
	resultRegistry map[int]chan interface{}
}

type Config struct {
	WorkerCount  int
	GlobalBuffer int
}

type Room struct {
	result               []interface{}
	resultMutex          sync.Mutex
	asyncCollectorWait   sync.WaitGroup
	asyncCollectorActive atomic.Bool
	bufferSize           int
	resultChan           chan interface{}
	wg                   sync.WaitGroup
	wp                   *WorkerPool
}

type Task struct {
	run  func() interface{}
	room *Room
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
		t.room.resultChan <- t.run()
		t.room.wg.Done()
	}
}

func (wp *WorkerPool) CreateRoom(size int) *Room {
	return &Room{
		bufferSize:         size,
		resultChan:         make(chan interface{}, size),
		wp:                 wp,
		asyncCollectorWait: sync.WaitGroup{},
	}
}

func (ro *Room) NewTaskWaitForFreeSlot(job func() interface{}) {
	task := Task{
		run:  job,
		room: ro,
	}
	ro.wg.Add(1)
	ro.wp.taskQueue <- task
}

func (ro *Room) NewTask(job func() interface{}) error {
	if len(ro.wp.taskQueue) == cap(ro.wp.taskQueue) {
		return fmt.Errorf("Global buffer is full. Please wait for some tasks to finish. Or increase the buffer size.")
	}

	if len(ro.resultChan) == cap(ro.resultChan) {
		return fmt.Errorf("Room buffer is full. Please wait for some tasks to finish. Or increase the buffer size.")
	}

	ro.NewTaskWaitForFreeSlot(job)

	return nil
}

func (ro *Room) Collect() []interface{} {
	go ro.WaitAndClose()
	results := make([]interface{}, 0)

	for result := range ro.resultChan {
		results = append(results, result)
	}

	return results
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

	return ro.result, nil
}

func (ro *Room) WaitAndClose() {
	ro.wg.Wait()
	close(ro.resultChan)
}
