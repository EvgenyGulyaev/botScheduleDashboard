package singleton

import (
	"sync"
)

var (
	instances = make(map[string]interface{})
	mu        sync.Mutex
)

func GetInstance(key string, factory func() interface{}) interface{} {
	mu.Lock()
	defer mu.Unlock()

	if instance, exists := instances[key]; exists {
		return instance
	}

	instance := factory()
	instances[key] = instance
	return instance
}
