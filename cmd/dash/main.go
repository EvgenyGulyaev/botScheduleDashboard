package main

import (
	"botDashboard/internal/config"
	"botDashboard/internal/event"
	"botDashboard/internal/http"
	store "botDashboard/internal/store"
	"fmt"
	"log"
)

func main() {
	c := config.LoadConfig()
	store.InitStore()

	// Запускаем брокер для сообщений из вне
	if c.Env["NATS_URL"] != "" {
		event.RunBroker()
	}

	server := http.GetServer(fmt.Sprintf(":%s", c.Env["PORT"]))
	err := server.StartHandle()
	if err != nil {
		log.Print(err)
	}

}
