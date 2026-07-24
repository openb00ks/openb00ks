package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type SystemSettingsRow struct {
	ID               int          `db:"id"`
	SetupComplete    bool         `db:"setup_complete"`
	SetupCompletedAt sql.NullTime `db:"setup_completed_at"`
	SettingsJSON     []byte       `db:"settings_json"`
	UpdatedAt        time.Time    `db:"updated_at"`
}

type SystemSettings struct {
	SetupComplete    bool            `json:"setup_complete"`
	SetupCompletedAt time.Time       `json:"setup_completed_at"`
	SettingsJSON     json.RawMessage `json:"settings_json"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type SystemSettingsStore struct {
	db *DB
}

func NewSystemSettingsStore(db *DB) *SystemSettingsStore {
	return &SystemSettingsStore{db: db}
}

func (s *SystemSettingsStore) Get(ctx context.Context) (SystemSettings, error) {
	if s.db == nil {
		return SystemSettings{}, ErrUnavailable
	}
	row := SystemSettingsRow{}
	err := s.db.GetContext(ctx, &row, `
        SELECT id, setup_complete, setup_completed_at, settings_json, updated_at
        FROM system_settings
        LIMIT 1
    `)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || isMissingSystemSettingsTableErr(err) {
			return SystemSettings{
				SetupComplete:    false,
				SettingsJSON:     json.RawMessage(`{}`),
				SetupCompletedAt: time.Time{},
				UpdatedAt:        time.Time{},
			}, nil
		}
		return SystemSettings{}, err
	}
	return systemSettingsFromRow(row), nil
}

func (s *SystemSettingsStore) SetSetupComplete(ctx context.Context, completedAt time.Time) (SystemSettings, error) {
	if s.db == nil {
		return SystemSettings{}, ErrUnavailable
	}
	row := SystemSettingsRow{}
	err := s.db.GetContext(ctx, &row, `
        UPDATE system_settings
        SET setup_complete = true,
            setup_completed_at = $1,
            updated_at = now()
        WHERE id = 1
        RETURNING id, setup_complete, setup_completed_at, settings_json, updated_at
    `, completedAt)
	if err == nil {
		return systemSettingsFromRow(row), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return SystemSettings{}, err
	}
	err = s.db.GetContext(ctx, &row, `
        INSERT INTO system_settings (id, setup_complete, setup_completed_at, settings_json, updated_at)
        VALUES (1, true, $1, '{}'::jsonb, now())
        RETURNING id, setup_complete, setup_completed_at, settings_json, updated_at
    `, completedAt)
	if err != nil {
		return SystemSettings{}, err
	}
	return systemSettingsFromRow(row), nil
}

func (s *SystemSettingsStore) UpsertSettings(ctx context.Context, settingsJSON json.RawMessage) (SystemSettings, error) {
	if s.db == nil {
		return SystemSettings{}, ErrUnavailable
	}
	if settingsJSON == nil {
		settingsJSON = json.RawMessage(`{}`)
	}
	row := SystemSettingsRow{}
	err := s.db.GetContext(ctx, &row, `
        INSERT INTO system_settings (id, settings_json, updated_at)
        VALUES (1, $1, now())
        ON CONFLICT (id) DO UPDATE
        SET settings_json = EXCLUDED.settings_json,
            updated_at = now()
        RETURNING id, setup_complete, setup_completed_at, settings_json, updated_at
    `, settingsJSON)
	if err != nil {
		return SystemSettings{}, err
	}
	return systemSettingsFromRow(row), nil
}

func systemSettingsFromRow(row SystemSettingsRow) SystemSettings {
	out := SystemSettings{
		SetupComplete: row.SetupComplete,
		SettingsJSON:  json.RawMessage(row.SettingsJSON),
		UpdatedAt:     row.UpdatedAt,
	}
	if row.SetupCompletedAt.Valid {
		out.SetupCompletedAt = row.SetupCompletedAt.Time
	}
	return out
}

func isMissingSystemSettingsTableErr(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P01"
	}
	return false
}
