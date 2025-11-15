package db

import (
	bolt "go.etcd.io/bbolt"
	"log"
	"os"
	"sync"
)

type Db struct {
	filename string
	DB       *bolt.DB
}

var instance *Db

var once sync.Once

func Init() *Db {
	once.Do(func() {
		instance = initDb(os.Getenv("DB_NAME_FILE"))
	})
	return instance
}

var UserBucket = []byte("Users")

func initDb(filename string) *Db {
	db, err := bolt.Open(filename, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(UserBucket)
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
	return &Db{filename: filename, DB: db}
}
