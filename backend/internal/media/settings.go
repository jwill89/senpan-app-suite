package media

import (
	"context"
	"database/sql"
	"errors"

	"app-suite/internal/db"
	"app-suite/internal/store"
)

type storeSettings struct{ st *store.Store }

// StoreSettings is the settings table of st as the Settings the fonts keep their
// metadata in.
func StoreSettings(st *store.Store) Settings { return storeSettings{st} }

func (s storeSettings) Get(ctx context.Context, key string) (string, error) {
	v, err := db.New(s.st.ReadDB()).GetSetting(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s storeSettings) Set(ctx context.Context, key, value string) error {
	return db.New(s.st.DB()).SetSetting(ctx, db.SetSettingParams{Key: key, Value: value})
}
