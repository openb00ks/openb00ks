package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/openb00ks/openb00ks/internal/models"
)

const mileageRateColumns = "r.year, r.rate_cents_per_mile, r.created_at, r.updated_at"

type MileageRateStore struct {
	db *DB
}

func NewMileageRateStore(db *DB) *MileageRateStore {
	return &MileageRateStore{db: db}
}

func (s *MileageRateStore) Get(ctx context.Context, year int) (models.MileageRate, error) {
	row := MileageRateRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+mileageRateColumns+`
		FROM mileage_rates r
		WHERE r.year = $1
	`, year)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.MileageRate{}, ErrNotFound
		}
		return models.MileageRate{}, err
	}
	return mileageRateFromRow(row), nil
}

func (s *MileageRateStore) Upsert(ctx context.Context, year int, rateCents int) (models.MileageRate, error) {
	var outYear int
	err := s.db.GetContext(ctx, &outYear, `
		INSERT INTO mileage_rates (year, rate_cents_per_mile)
		VALUES ($1, $2)
		ON CONFLICT (year) DO UPDATE
		SET rate_cents_per_mile = EXCLUDED.rate_cents_per_mile,
		    updated_at = now()
		RETURNING year
	`, year, rateCents)
	if err != nil {
		return models.MileageRate{}, err
	}
	return s.Get(ctx, outYear)
}

func (s *MileageRateStore) List(ctx context.Context) ([]models.MileageRate, error) {
	rows := []MileageRateRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT `+mileageRateColumns+`
		FROM mileage_rates r
		ORDER BY r.year DESC
	`)
	if err != nil {
		return nil, err
	}
	out := make([]models.MileageRate, 0, len(rows))
	for _, row := range rows {
		out = append(out, mileageRateFromRow(row))
	}
	return out, nil
}

func mileageRateFromRow(row MileageRateRow) models.MileageRate {
	return models.MileageRate{
		Year:             row.Year,
		RateCentsPerMile: row.RateCentsPerMile,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
