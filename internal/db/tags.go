package db

import (
	"context"
	"strings"

	"github.com/openb00ks/openb00ks/internal/models"
)

const tagColumns = "t.id, t.entity_id, t.name, t.created_at"

type TagStore struct {
	db *DB
}

func NewTagStore(db *DB) *TagStore {
	return &TagStore{db: db}
}

func (s *TagStore) Ensure(ctx context.Context, entityID, name string) (models.Tag, error) {
	name = normalizeTag(name)
	var id string
	err := s.db.GetContext(ctx, &id, `
		INSERT INTO tags (entity_id, name)
		VALUES ($1, $2)
		ON CONFLICT (entity_id, name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, entityID, name)
	if err != nil {
		return models.Tag{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *TagStore) GetByID(ctx context.Context, id string) (models.Tag, error) {
	row := TagRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+tagColumns+`
		FROM tags t
		WHERE t.id = $1
	`, id)
	if err != nil {
		return models.Tag{}, err
	}
	return tagFromRow(row), nil
}

func (s *TagStore) ListByReceiptID(ctx context.Context, receiptID string) ([]models.Tag, error) {
	rows := []TagRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT `+tagColumns+`
		FROM receipt_tags rt
		JOIN tags t ON t.id = rt.tag_id
		WHERE rt.receipt_id = $1
		ORDER BY t.name
	`, receiptID)
	if err != nil {
		return nil, err
	}
	out := make([]models.Tag, 0, len(rows))
	for _, row := range rows {
		out = append(out, tagFromRow(row))
	}
	return out, nil
}

func (s *TagStore) SetReceiptTags(ctx context.Context, receiptID string, tags []models.Tag) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `
		DELETE FROM receipt_tags
		WHERE receipt_id = $1
	`, receiptID); err != nil {
		return err
	}
	for _, tag := range tags {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO receipt_tags (receipt_id, tag_id)
			VALUES ($1, $2)
		`, receiptID, tag.ID); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func tagFromRow(row TagRow) models.Tag {
	return models.Tag{
		ID:        row.ID,
		EntityID:  row.EntityID,
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
	}
}

func normalizeTag(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.Join(strings.Fields(name), " ")
	return name
}
