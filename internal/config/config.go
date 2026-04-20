package config

import (
	"botDashboard/pkg/singleton"
	"errors"
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
	rootPath, err := findEnvPath(pwd)
	if err != nil {
		log.Print(filepath.Join(pwd, ".env"))
		log.Fatal("Error loading .env file in ", filepath.Join(pwd, ".env"))
	}

	err = godotenv.Load(rootPath)
	if err != nil {
		log.Fatal("Error loading .env file in ", rootPath)
	}
	env, err := godotenv.Read(rootPath)
	if err != nil {
		log.Fatal("Error cannot read .env file")
	}
	return &Config{IsLoaded: true, Env: env}
}

func findEnvPath(startDir string) (string, error) {
	current := startDir
	for {
		candidate := filepath.Join(current, ".env")
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}
