package server

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/alexedwards/scs/sqlite3store"
)

// sessionStore keeps sign-ins in the database (the sessions table) through scs's
// SQLite store, so a deploy or restart signs nobody out. It stores each session
// under a SHA-256 hash of its token, so the
// database, its backups and the dev copies hold nothing that signs anyone in: the
// token itself lives only in the browser's cookie.
type sessionStore struct {
	rows *sqlite3store.SQLite3Store
}

// newSessionStore opens the store on the write pool. Expired sessions are swept
// once an hour.
func newSessionStore(db *sql.DB) sessionStore {
	return sessionStore{rows: sqlite3store.NewWithConfig(db, sqlite3store.Config{
		CleanUpInterval: time.Hour,
		TableName:       "sessions",
	})}
}

func sessionKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s sessionStore) Find(token string) ([]byte, bool, error) {
	return s.rows.Find(sessionKey(token))
}

func (s sessionStore) Commit(token string, b []byte, expiry time.Time) error {
	return s.rows.Commit(sessionKey(token), b, expiry)
}

func (s sessionStore) Delete(token string) error {
	return s.rows.Delete(sessionKey(token))
}
