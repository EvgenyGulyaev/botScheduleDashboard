package db

import (
	"botDashboard/pkg/singleton"
	"os"

	"go.etcd.io/bbolt"
)

type Repository struct {
	db *Db
}

func GetRepository() *Repository {
	return singleton.GetInstance("bolt-repo", func() interface{} {
		return &Repository{db: Init(os.Getenv("DB_NAME_FILE"))}
	}).(*Repository)
}

func (r *Repository) EnsureBuckets(buckets [][]byte) error {
	for _, bucket := range buckets {
		if err := r.db.EnsureBucket(bucket); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) Update(fn func(*bbolt.Tx) error) error {
	return r.db.DB.Update(fn)
}

func (r *Repository) View(fn func(*bbolt.Tx) error) error {
	return r.db.DB.View(fn)
}
