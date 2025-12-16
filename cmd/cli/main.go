package main

import (
	"botDashboard/internal/command"
	"botDashboard/internal/config"
	"botDashboard/internal/store"
	"fmt"
)

func main() {
	config.LoadConfig()
	store.InitStore()

	fmt.Println((&command.Executor{}).Execute())
}
