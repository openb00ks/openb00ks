package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/openb00ks/openb00ks/internal/models"
)

const preferencesColumns = "p.user_id, p.default_entity_id, p.theme, p.created_at, p.updated_at"

type PreferencesStore struct {
	db *DB
}

func NewPreferencesStore(db *DB) *PreferencesStore {
	return &PreferencesStore{db: db}
}

func (s *PreferencesStore) Get(ctx context.Context, userID string) (models.UserPreferences, error) {
	row := UserPreferencesRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+preferencesColumns+`
		FROM user_preferences p
		WHERE p.user_id = $1
	`, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.UserPreferences{}, ErrNotFound
		}
		return models.UserPreferences{}, err
	}
	return preferencesFromRow(row), nil
}

func (s *PreferencesStore) Upsert(ctx context.Context, userID string, defaultEntityID string, theme string) (models.UserPreferences, error) {
	var defaultEntity sql.NullString
	if defaultEntityID != "" {
		defaultEntity = sql.NullString{String: defaultEntityID, Valid: true}
	}
	var id string
	err := s.db.GetContext(ctx, &id, `
		INSERT INTO user_preferences (user_id, default_entity_id, theme)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE
		SET default_entity_id = EXCLUDED.default_entity_id,
		    theme = EXCLUDED.theme,
		    updated_at = now()
		RETURNING user_id
	`, userID, defaultEntity, theme)
	if err != nil {
		return models.UserPreferences{}, err
	}
	return s.Get(ctx, id)
}

func preferencesFromRow(row UserPreferencesRow) models.UserPreferences {
	prefs := models.UserPreferences{
		UserID:    row.UserID,
		Theme:     row.Theme,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	if row.DefaultEntityID.Valid {
		prefs.DefaultEntityID = row.DefaultEntityID.String
	}
	return prefs
}
