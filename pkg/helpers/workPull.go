package helpers

import (
	"context"
	"sync"
)

func MultipleWork[I, O any](data []I, workerCount int, callback func(I) O) []O {
	ctx := context.Background()

	resChan := WorkerPool[I, O](ctx, workerCount, Generator[I](ctx, data, workerCount), callback)

	res := make([]O, 0, len(data))

	for {
		select {
		case <-ctx.Done():
			return res
		case val, ok := <-resChan:
			if !ok {
				return res
			}
			res = append(res, val)
		}
	}
}

func Generator[T any](ctx context.Context, data []T, size int) <-chan T {
	ch := make(chan T, size)
	go func() {
		defer close(ch)

		for i := 0; i < len(data); i++ {
			select {
			case ch <- data[i]:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

func WorkerPool[I, O any](c context.Context, workerCount int, input <-chan I, callback func(I) O) <-chan O {
	result := make(chan O)
	wg := new(sync.WaitGroup)

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-c.Done():
					return
				case i, ok := <-input:
					if !ok {
						return
					}
					select {
					case <-c.Done():
						return
					case result <- callback(i):
					}
				}
			}

		}()
	}

	go func() {
		defer close(result)
		wg.Wait()
	}()
	return result
}
