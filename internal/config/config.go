package config

import (
	"botDashboard/pkg/singleton"
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type Config struct {
	IsLoaded bool
	Env      map[string]string
}

func LoadConfig() *Config {
	return singleton.GetInstance("config", func() interface{} {
		return load()
	}).(*Config)
}

func load() *Config {
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	rootPath := filepath.Join(pwd, ".env")

	err = godotenv.Load(rootPath)
	if err != nil {
		log.Print(filepath.Join(pwd, ".env"))
		log.Fatal("Error loading .env file in ", rootPath)
	}
	env, err := godotenv.Read(rootPath)
	if err != nil {
		log.Fatal("Error cannot read .env file")
	}
	return &Config{IsLoaded: true, Env: env}
}
