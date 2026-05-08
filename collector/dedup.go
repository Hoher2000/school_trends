package collector

import (
	"crypto/sha256"
	"encoding/hex"

	bolt "go.etcd.io/bbolt"
)

type Deduplicator struct {
	db     *bolt.DB
	bucket []byte
}

func NewDeduplicator(dbPath string) (*Deduplicator, error) {
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, err
	}
	// Создаём бакет, если нет
	err = db.Update(func(tx *bolt.Tx) error {
		_, e := tx.CreateBucketIfNotExists([]byte("published"))
		return e
	})
	if err != nil {
		return nil, err
	}
	return &Deduplicator{db: db, bucket: []byte("published")}, nil
}

// IsPublished возвращает true, если статья с такой ссылкой уже была опубликована.
func (d *Deduplicator) IsPublished(link string) bool {
	hash := hashLink(link)
	var found bool
	d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(d.bucket)
		if b.Get([]byte(hash)) != nil {
			found = true
		}
		return nil
	})
	return found
}

// MarkPublished запоминает ссылку как опубликованную.
func (d *Deduplicator) MarkPublished(link string) error {
	hash := hashLink(link)
	return d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(d.bucket)
		return b.Put([]byte(hash), []byte(link))
	})
}

func (d *Deduplicator) Close() error {
	return d.db.Close()
}

func hashLink(link string) string {
	h := sha256.Sum256([]byte(link))
	return hex.EncodeToString(h[:])
}
