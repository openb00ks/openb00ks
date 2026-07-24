package db

import (
	"context"
	"time"
)

type MileageSummaryRow struct {
	Month      time.Time `db:"month"`
	TotalMiles float64   `db:"total_miles"`
	TripCount  int       `db:"trip_count"`
	Year       int
	RateCents  int
	ReimbCents int64
	HasRate    bool
}

func (s *MileageStore) SummaryByMonth(ctx context.Context, entityID string, start, end time.Time, rates *MileageRateStore) ([]MileageSummaryRow, error) {
	rows := []MileageSummaryRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT date_trunc('month', m.date)::date AS month,
		       SUM(m.distance_miles)::float8 AS total_miles,
		       COUNT(*)::int AS trip_count
		FROM mileage_logs m
		WHERE m.entity_id = $1
		  AND m.date >= $2 AND m.date <= $3
		GROUP BY 1
		ORDER BY 1
	`, entityID, start, end)
	if err != nil {
		return nil, err
	}
	if rates == nil {
		return rows, nil
	}
	for i := range rows {
		year := rows[i].Month.Year()
		rate, err := rates.Get(ctx, year)
		if err == nil {
			rows[i].Year = year
			rows[i].RateCents = rate.RateCentsPerMile
			rows[i].ReimbCents = int64(rows[i].TotalMiles*float64(rate.RateCentsPerMile) + 0.5)
			rows[i].HasRate = true
		} else {
			rows[i].Year = year
		}
	}
	return rows, nil
}
