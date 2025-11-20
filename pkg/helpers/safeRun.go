package helpers

import (
	"fmt"
	"time"
)

func SafeRun[T any](g func() T) T {
	timer := time.Now()
	defer func() {
		fmt.Printf("Time => %s", time.Since(timer))

		if err := recover(); err != nil {
			fmt.Printf("Error => %s", err)
		}
	}()

	return g()
}
