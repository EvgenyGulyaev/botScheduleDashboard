package main

import (
	"botDashboard/internal/config"
	"botDashboard/internal/http"
	"botDashboard/pkg/db"
	"fmt"
	"log"
)

func main() {
	c := config.LoadConfig()
	db.Init()

	server := http.GetServer(fmt.Sprintf(":%s", c.Env["PORT"]))
	err := server.StartHandle()
	if err != nil {
		log.Print(err)
	}

}
