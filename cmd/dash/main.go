package main

import (
	"botDashboard/internal/command"
	"botDashboard/internal/config"
	"botDashboard/internal/event"
	"botDashboard/internal/http"
	"botDashboard/internal/store"
	"fmt"
	"log"
	"time"
)

func main() {
	c := config.LoadConfig()
	store.ConfigureChatMaxMessages(c.Env["CHAT_MAX_MESSAGES"])
	store.ConfigureChatAudio(c.Env["CHAT_AUDIO_DIR"], c.Env["CHAT_AUDIO_MAX_SECONDS"], c.Env["CHAT_AUDIO_MAX_MB"])
	store.InitStore()
	startChatAudioCleanupLoop()

	// Запускаем брокер для сообщений из вне
	if c.Env["NATS_URL"] != "" {
		event.RunBroker()
	}

	server := http.GetServer(fmt.Sprintf(":%s", c.Env["PORT"]))
	err := server.StartHandle()
	if err != nil {
		log.Print(err)
	}

	(&command.Initial{}).Execute()
}

func startChatAudioCleanupLoop() {
	repo := store.GetChatRepository()
	runCleanup := func() {
		if _, err := repo.CleanupExpiredAudioMessages(); err != nil {
			log.Printf("chat audio cleanup failed: %v", err)
		}
	}

	runCleanup()

	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			runCleanup()
		}
	}()
}
