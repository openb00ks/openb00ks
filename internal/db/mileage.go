package db

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/openb00ks/openb00ks/internal/models"
)

const mileageColumns = "m.id, m.entity_id, m.user_id, m.receipt_id, m.date, m.distance_miles, m.start_location, m.end_location, m.purpose, m.created_at, m.updated_at"

type MileageStore struct {
	db *DB
}

func NewMileageStore(db *DB) *MileageStore {
	return &MileageStore{db: db}
}

func (s *MileageStore) Create(ctx context.Context, log models.MileageLog) (models.MileageLog, error) {
	var userID sql.NullString
	var receiptID sql.NullString
	var start sql.NullString
	var end sql.NullString
	var purpose sql.NullString
	if log.UserID != "" {
		userID = sql.NullString{String: log.UserID, Valid: true}
	}
	if log.ReceiptID != "" {
		receiptID = sql.NullString{String: log.ReceiptID, Valid: true}
	}
	if log.StartLocation != "" {
		start = sql.NullString{String: log.StartLocation, Valid: true}
	}
	if log.EndLocation != "" {
		end = sql.NullString{String: log.EndLocation, Valid: true}
	}
	if log.Purpose != "" {
		purpose = sql.NullString{String: log.Purpose, Valid: true}
	}

	var id string
	err := s.db.GetContext(ctx, &id, `
		INSERT INTO mileage_logs (entity_id, user_id, receipt_id, date, distance_miles, start_location, end_location, purpose)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, log.EntityID, userID, receiptID, log.Date, log.DistanceMiles, start, end, purpose)
	if err != nil {
		return models.MileageLog{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *MileageStore) Update(ctx context.Context, id string, log models.MileageLog) (models.MileageLog, error) {
	var receiptID sql.NullString
	var start sql.NullString
	var end sql.NullString
	var purpose sql.NullString
	if log.ReceiptID != "" {
		receiptID = sql.NullString{String: log.ReceiptID, Valid: true}
	}
	if log.StartLocation != "" {
		start = sql.NullString{String: log.StartLocation, Valid: true}
	}
	if log.EndLocation != "" {
		end = sql.NullString{String: log.EndLocation, Valid: true}
	}
	if log.Purpose != "" {
		purpose = sql.NullString{String: log.Purpose, Valid: true}
	}

	var updatedID string
	err := s.db.GetContext(ctx, &updatedID, `
		UPDATE mileage_logs
		SET date = $2,
		    distance_miles = $3,
		    start_location = $4,
		    end_location = $5,
		    purpose = $6,
		    receipt_id = $7,
		    updated_at = now()
		WHERE id = $1
		RETURNING id
	`, id, log.Date, log.DistanceMiles, start, end, purpose, receiptID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.MileageLog{}, ErrNotFound
		}
		return models.MileageLog{}, err
	}
	return s.GetByID(ctx, updatedID)
}

func (s *MileageStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM mileage_logs
		WHERE id = $1
	`, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MileageStore) GetEntityID(ctx context.Context, id string) (string, error) {
	var entityID string
	err := s.db.GetContext(ctx, &entityID, `
		SELECT m.entity_id
		FROM mileage_logs m
		WHERE m.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return entityID, nil
}

func (s *MileageStore) GetByID(ctx context.Context, id string) (models.MileageLog, error) {
	row := MileageLogRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+mileageColumns+`
		FROM mileage_logs m
		WHERE m.id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.MileageLog{}, ErrNotFound
		}
		return models.MileageLog{}, err
	}
	return mileageFromRow(row), nil
}

func (s *MileageStore) List(ctx context.Context, entityID string, start, end *time.Time, limit int) ([]models.MileageLog, error) {
	if limit <= 0 {
		limit = 100
	}
	rows := []MileageLogRow{}
	query := `
		SELECT ` + mileageColumns + `
		FROM mileage_logs m
		WHERE m.entity_id = $1
	`
	args := []interface{}{entityID}
	if start != nil {
		args = append(args, *start)
		query += " AND m.date >= $" + strconv.Itoa(len(args))
	}
	if end != nil {
		args = append(args, *end)
		query += " AND m.date <= $" + strconv.Itoa(len(args))
	}
	args = append(args, limit)
	query += " ORDER BY m.date DESC, m.created_at DESC LIMIT $" + strconv.Itoa(len(args))

	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]models.MileageLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, mileageFromRow(row))
	}
	return out, nil
}

func (s *MileageStore) Export(ctx context.Context, entityID string, start, end *time.Time) ([]models.MileageLog, error) {
	return s.List(ctx, entityID, start, end, 10000)
}

func mileageFromRow(row MileageLogRow) models.MileageLog {
	log := models.MileageLog{
		ID:            row.ID,
		EntityID:      row.EntityID,
		Date:          row.Date,
		DistanceMiles: row.DistanceMiles,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
	if row.UserID.Valid {
		log.UserID = row.UserID.String
	}
	if row.ReceiptID.Valid {
		log.ReceiptID = row.ReceiptID.String
	}
	if row.StartLocation.Valid {
		log.StartLocation = row.StartLocation.String
	}
	if row.EndLocation.Valid {
		log.EndLocation = row.EndLocation.String
	}
	if row.Purpose.Valid {
		log.Purpose = row.Purpose.String
	}
	return log
}
