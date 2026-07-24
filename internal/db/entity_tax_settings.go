package db

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"
)

type EntityTaxSettings struct {
	EntityID                       string
	TaxYear                        int
	HomeOfficeSqFt                 sql.NullInt64
	HomeTotalSqFt                  sql.NullInt64
	CellPhoneBusinessUsePercent    sql.NullInt64
	HomeInternetBusinessUsePercent sql.NullInt64
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
}

type EntityTaxSettingsStore struct {
	db *DB
}

func NewEntityTaxSettingsStore(db *DB) *EntityTaxSettingsStore {
	return &EntityTaxSettingsStore{db: db}
}

func (s *EntityTaxSettingsStore) Get(ctx context.Context, entityID string, taxYear int) (EntityTaxSettings, error) {
	row := EntityTaxSettingsRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT entity_id, tax_year, home_office_sqft, home_total_sqft, cell_phone_business_use_percent, home_internet_business_use_percent, created_at, updated_at
		FROM entity_tax_settings
		WHERE entity_id = $1 AND tax_year = $2
	`, entityID, taxYear)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EntityTaxSettings{EntityID: entityID, TaxYear: taxYear}, nil
		}
		return EntityTaxSettings{}, err
	}
	return entityTaxSettingsFromRow(row), nil
}

func (s *EntityTaxSettingsStore) Upsert(ctx context.Context, entityID string, taxYear int, homeOfficeSqFt, homeTotalSqFt, cellPhoneBusinessUsePercent, homeInternetBusinessUsePercent sql.NullInt64) (EntityTaxSettings, error) {
	row := EntityTaxSettingsRow{}
	err := s.db.GetContext(ctx, &row, `
		INSERT INTO entity_tax_settings (
			entity_id,
			tax_year,
			home_office_sqft,
			home_total_sqft,
			cell_phone_business_use_percent,
			home_internet_business_use_percent
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (entity_id, tax_year) DO UPDATE
		SET home_office_sqft = EXCLUDED.home_office_sqft,
		    home_total_sqft = EXCLUDED.home_total_sqft,
		    cell_phone_business_use_percent = EXCLUDED.cell_phone_business_use_percent,
		    home_internet_business_use_percent = EXCLUDED.home_internet_business_use_percent,
		    updated_at = now()
		RETURNING entity_id, tax_year, home_office_sqft, home_total_sqft, cell_phone_business_use_percent, home_internet_business_use_percent, created_at, updated_at
	`, entityID, taxYear, homeOfficeSqFt, homeTotalSqFt, cellPhoneBusinessUsePercent, homeInternetBusinessUsePercent)
	if err != nil {
		return EntityTaxSettings{}, err
	}
	return entityTaxSettingsFromRow(row), nil
}

func entityTaxSettingsFromRow(row EntityTaxSettingsRow) EntityTaxSettings {
	return EntityTaxSettings(row)
}

func UtilitiesBusinessUsePercent(homeOfficeSqFt, homeTotalSqFt sql.NullInt64) (int, bool) {
	if !homeOfficeSqFt.Valid || !homeTotalSqFt.Valid || homeTotalSqFt.Int64 <= 0 {
		return 0, false
	}
	if homeOfficeSqFt.Int64 < 0 || homeTotalSqFt.Int64 < 0 {
		return 0, false
	}
	percent := math.Round((float64(homeOfficeSqFt.Int64) / float64(homeTotalSqFt.Int64)) * 100)
	if percent < 0 {
		return 0, false
	}
	return int(percent), true
}
