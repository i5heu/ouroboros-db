package main

import (
	"fmt"

	workerpool "github.com/i5heu/ouroboros-db/pkg/workerPool"
)

type funcResult struct {
	bob int
	err error
}

func main() {
	work := []int{}

	for i := 0; i < 50000; i++ {
		work = append(work, i)
	}

	wp := workerpool.NewWorkerPool(workerpool.Config{GlobalBuffer: 1000})
	room := wp.CreateRoom(1000)

	room.AsyncCollector()

	for _, w := range work {
		task := w
		room.NewTaskWaitForFreeSlot(func() interface{} {
			return funcResult{
				bob: task * task,
				err: fmt.Errorf("Hello World"),
			}
		})
	}

	result, err := room.GetAsyncResults()
	if err != nil {
		fmt.Println(err)
	}

	field, ok := result[500].(funcResult)
	if !ok {
		fmt.Println("not ok")
	}

	fmt.Println(len(result), field.err.Error())
}
