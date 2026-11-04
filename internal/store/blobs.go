package store

import (
	"database/sql"
	"errors"
	"time"
)

// Blob is a processed, user-uploaded image (icon or wallpaper) kept inside the database.
type Blob struct {
	ID          string
	Ext         string
	ContentType string
	Data        []byte
}

// URL is the public path the blob is served from; services/settings store this string.
func (b *Blob) URL() string { return "/icons/" + b.ID + "." + b.Ext }

func (s *Store) PutBlob(id, ext, contentType string, data []byte) error {
	_, err := s.db.Exec(`INSERT INTO blobs(id,ext,content_type,data,created_at) VALUES(?,?,?,?,?)`, id, ext, contentType, data, now())
	return err
}

func (s *Store) GetBlob(id string) (*Blob, error) {
	var b Blob
	err := s.db.QueryRow(`SELECT id,ext,content_type,data FROM blobs WHERE id=?`, id).Scan(&b.ID, &b.Ext, &b.ContentType, &b.Data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &b, err
}

// PurgeOrphanBlobs deletes blobs that no service icon or the wallpaper setting
// references. The grace period keeps a freshly uploaded image alive while the
// admin is still filling in the form that will reference it.
func (s *Store) PurgeOrphanBlobs(grace time.Duration) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM blobs
		WHERE created_at < ?
		  AND '/icons/' || id || '.' || ext NOT IN (SELECT icon FROM services)
		  AND '/icons/' || id || '.' || ext NOT IN (SELECT value FROM settings WHERE key='wallpaper')`,
		now()-int64(grace.Seconds()))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
