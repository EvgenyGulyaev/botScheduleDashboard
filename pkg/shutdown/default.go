package shutdown

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var instance *Shutdown

var once sync.Once

type Shutdown struct {
	cancels []func()
}

func (s *Shutdown) Add(cancel func()) {
	s.cancels = append(s.cancels, cancel)
}

func (s *Shutdown) Wait() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	for _, cancel := range s.cancels {
		cancel()
	}
	os.Exit(0)
}

func Get() *Shutdown {
	once.Do(func() {
		instance = &Shutdown{}
	})
	return instance
}
