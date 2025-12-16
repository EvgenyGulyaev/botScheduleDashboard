package db

import (
	"log"
	"sync"

	bolt "go.etcd.io/bbolt"
)

type Db struct {
	filename string
	DB       *bolt.DB
}

var instance *Db
var once sync.Once

func Init(filename string) *Db {
	once.Do(func() {
		instance = openDb(filename)
	})
	return instance
}

func (d *Db) EnsureBucket(name []byte) error {
	return d.DB.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(name)
		return err
	})
}

func openDb(filename string) *Db {
	if filename == "" {
		filename = "bot.db"
	}

	db, err := bolt.Open(filename, 0600, nil)
	if err != nil {
		log.Fatalf("failed to open bolt db: %v", err)
	}

	return &Db{filename: filename, DB: db}
}
