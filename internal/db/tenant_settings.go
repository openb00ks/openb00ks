package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type TenantSettingsRow struct {
	TenantID     string    `db:"tenant_id"`
	SettingsJSON []byte    `db:"settings_json"`
	UpdatedAt    time.Time `db:"updated_at"`
}

type TenantSettings struct {
	TenantID     string          `json:"tenant_id"`
	SettingsJSON json.RawMessage `json:"settings_json"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type TenantSettingsStore struct {
	db *DB
}

func NewTenantSettingsStore(db *DB) *TenantSettingsStore {
	return &TenantSettingsStore{db: db}
}

// Get returns tenant settings for the given tenant ID.
// If no settings exist, returns empty settings (not an error).
func (s *TenantSettingsStore) Get(ctx context.Context, tenantID string) (TenantSettings, error) {
	row := TenantSettingsRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT tenant_id, settings_json, updated_at
		FROM tenant_settings
		WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TenantSettings{
				TenantID:     tenantID,
				SettingsJSON: json.RawMessage(`{}`),
				UpdatedAt:    time.Time{},
			}, nil
		}
		return TenantSettings{}, err
	}
	return settingsFromRow(row), nil
}

// Upsert creates or updates tenant settings.
func (s *TenantSettingsStore) Upsert(ctx context.Context, tenantID string, settingsJSON json.RawMessage) (TenantSettings, error) {
	row := TenantSettingsRow{}
	err := s.db.GetContext(ctx, &row, `
		INSERT INTO tenant_settings (tenant_id, settings_json)
		VALUES ($1, $2)
		ON CONFLICT (tenant_id) DO UPDATE
		SET settings_json = EXCLUDED.settings_json,
		    updated_at = now()
		RETURNING tenant_id, settings_json, updated_at
	`, tenantID, settingsJSON)
	if err != nil {
		return TenantSettings{}, err
	}
	return settingsFromRow(row), nil
}

func settingsFromRow(row TenantSettingsRow) TenantSettings {
	return TenantSettings{
		TenantID:     row.TenantID,
		SettingsJSON: json.RawMessage(row.SettingsJSON),
		UpdatedAt:    row.UpdatedAt,
	}
}
