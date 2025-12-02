package helpers

import (
	"context"
	"sync"
)

func FanIn[T any](ctx context.Context, chs []<-chan T) <-chan T {
	out := make(chan T)
	var wg sync.WaitGroup

	for _, ch := range chs {
		wg.Add(1)
		go func(c <-chan T) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case v, ok := <-c:
					if !ok {
						return
					}
					select {
					case out <- v:
					case <-ctx.Done():
						return
					}
				}
			}
		}(ch)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

func FanOut[T any](ctx context.Context, in <-chan T, numChannels int, f func(T) T) []<-chan T {
	chs := make([]<-chan T, numChannels)
	for i := 0; i < numChannels; i++ {
		chs[i] = pipeline(ctx, in, f)
	}
	return chs
}

func pipeline[T any](ctx context.Context, in <-chan T, f func(T) T) <-chan T {
	out := make(chan T)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-in:
				if !ok {
					return
				}
				select {
				case out <- f(v):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}
