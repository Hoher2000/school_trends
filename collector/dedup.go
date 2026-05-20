package collector

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"log"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	publishedBucket = []byte("published")
	expiryBucket    = []byte("expiry")
)

type Deduplicator struct {
	db          *bolt.DB
	bucket      []byte
	stopCleanup chan struct{} // сигнал остановки фоновой очистки
}

func NewDeduplicator(dbPath string, maxAge time.Duration) (*Deduplicator, error) {
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, err
	}
	// Создаём оба бакета
	err = db.Update(func(tx *bolt.Tx) error {
		if _, e := tx.CreateBucketIfNotExists(publishedBucket); e != nil {
			return e
		}
		_, e := tx.CreateBucketIfNotExists(expiryBucket)
		return e
	})
	if err != nil {
		return nil, err
	}

	d := &Deduplicator{
		db:          db,
		bucket:      publishedBucket,
		stopCleanup: make(chan struct{}),
	}

	// Запускаем фоновую очистку с интервалом 24 часа
	go d.cleanupLoop(maxAge)

	return d, nil
}

// IsPublished возвращает true, если статья с таким заголовком уже была опубликована.
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

// MarkPublished запоминает ссылку как опубликованную и сохраняет временную метку.
func (d *Deduplicator) MarkPublished(link string) error {
	hash := hashLink(link)
	now := time.Now().Unix()
	nowBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(nowBytes, uint64(now))

	return d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(d.bucket)
		if err := b.Put([]byte(hash), []byte(link)); err != nil {
			return err
		}
		eb := tx.Bucket(expiryBucket)
		return eb.Put([]byte(hash), nowBytes)
	})
}

// CleanupOldEntries удаляет записи старше maxAge.
func (d *Deduplicator) CleanupOldEntries(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge).Unix()
	return d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(d.bucket)
		eb := tx.Bucket(expiryBucket)

		cursor := eb.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			ts := int64(binary.BigEndian.Uint64(v))
			if ts < cutoff {
				eb.Delete(k)
				b.Delete(k)
			}
		}
		return nil
	})
}

// cleanupLoop периодически вызывает очистку старых записей.
func (d *Deduplicator) cleanupLoop(maxAge time.Duration) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// При старте тоже очищаем, чтобы сразу уменьшить базу
	if err := d.CleanupOldEntries(maxAge); err != nil {
		log.Printf("Ошибка первичной очистки BoltDB: %v", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := d.CleanupOldEntries(maxAge); err != nil {
				log.Printf("Ошибка очистки BoltDB: %v", err)
			}
		case <-d.stopCleanup:
			return
		}
	}
}

// Close останавливает фоновую очистку и закрывает базу.
func (d *Deduplicator) Close() error {
	close(d.stopCleanup)
	return d.db.Close()
}

func hashLink(link string) string {
	h := sha256.Sum256([]byte(link))
	return hex.EncodeToString(h[:])
}
