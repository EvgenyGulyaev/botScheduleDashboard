package main

import (
	"botDashboard/internal/command"
	"botDashboard/internal/config"
	"botDashboard/pkg/db"
	"fmt"
)

func main() {
	config.LoadConfig()
	db.Init()

	fmt.Println((&command.Executor{}).Execute())
}
