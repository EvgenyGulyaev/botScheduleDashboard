package store

import (
	"botDashboard/internal/model"
	"botDashboard/pkg/db"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

type AuditRepository struct {
	repo *db.Repository
}

func GetAuditRepository() *AuditRepository {
	return &AuditRepository{
		repo: db.GetRepository(),
	}
}

func (ar *AuditRepository) Append(entry model.AuditEntry) (model.AuditEntry, error) {
	entry.ActorEmail = strings.TrimSpace(entry.ActorEmail)
	entry.ActorLogin = strings.TrimSpace(entry.ActorLogin)
	entry.Action = strings.TrimSpace(entry.Action)
	entry.Target = strings.TrimSpace(entry.Target)
	entry.Summary = strings.TrimSpace(entry.Summary)
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	if entry.Action == "" {
		return model.AuditEntry{}, fmt.Errorf("audit action is required")
	}

	err := ar.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(AuditBucket)
		if b == nil {
			return fmt.Errorf("audit bucket not found")
		}
		if err := pruneAuditBucketByAge(b, time.Now().UTC().Add(-model.AuditRetention)); err != nil {
			return err
		}
		seq, err := b.NextSequence()
		if err != nil {
			return err
		}
		entry.ID = fmt.Sprintf("%020d", seq)
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if err := b.Put([]byte(entry.ID), data); err != nil {
			return err
		}
		return pruneAuditBucket(b, model.AuditMaxRecentEntries)
	})
	if err != nil {
		return model.AuditEntry{}, err
	}
	return entry, nil
}

func (ar *AuditRepository) ListRecent() ([]model.AuditEntry, error) {
	result := make([]model.AuditEntry, 0, model.AuditMaxRecentEntries)
	err := ar.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(AuditBucket)
		if b == nil {
			return fmt.Errorf("audit bucket not found")
		}
		if err := pruneAuditBucketByAge(b, time.Now().UTC().Add(-model.AuditRetention)); err != nil {
			return err
		}
		cursor := b.Cursor()
		for key, value := cursor.Last(); key != nil && len(result) < model.AuditMaxRecentEntries; key, value = cursor.Prev() {
			var entry model.AuditEntry
			if err := json.Unmarshal(value, &entry); err != nil {
				return err
			}
			result = append(result, entry)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].ID > result[j].ID
	})
	return result, nil
}

func (ar *AuditRepository) ClearAll() error {
	return ar.repo.ClearBucket(AuditBucket)
}

func pruneAuditBucket(bucket *bbolt.Bucket, keep int) error {
	if keep <= 0 {
		keep = model.AuditMaxRecentEntries
	}
	stats := bucket.Stats()
	overLimit := stats.KeyN - keep
	if overLimit <= 0 {
		return nil
	}
	cursor := bucket.Cursor()
	for key, _ := cursor.First(); key != nil && overLimit > 0; key, _ = cursor.First() {
		if err := bucket.Delete(key); err != nil {
			return err
		}
		overLimit--
	}
	return nil
}

func pruneAuditBucketByAge(bucket *bbolt.Bucket, cutoff time.Time) error {
	cursor := bucket.Cursor()
	keysToDelete := make([][]byte, 0)
	for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
		var entry model.AuditEntry
		if err := json.Unmarshal(value, &entry); err != nil {
			return err
		}
		if !entry.CreatedAt.IsZero() && entry.CreatedAt.Before(cutoff) {
			keysToDelete = append(keysToDelete, append([]byte(nil), key...))
		}
	}
	for _, key := range keysToDelete {
		if err := bucket.Delete(key); err != nil {
			return err
		}
	}
	return nil
}
