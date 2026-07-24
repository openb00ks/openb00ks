package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

const accountRoleTagColumns = "art.account_id, art.role_tag, art.created_at"

type AccountRoleTagStore struct {
	db *DB
}

func NewAccountRoleTagStore(db *DB) *AccountRoleTagStore {
	return &AccountRoleTagStore{db: db}
}

func (s *AccountRoleTagStore) ListByAccountID(ctx context.Context, accountID string) ([]string, error) {
	rows := []AccountRoleTagRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT `+accountRoleTagColumns+`
		FROM account_role_tags art
		WHERE art.account_id = $1
		ORDER BY art.role_tag
	`, accountID)
	if err != nil {
		return nil, err
	}
	tags := make([]string, 0, len(rows))
	for _, row := range rows {
		tags = append(tags, row.RoleTag)
	}
	return tags, nil
}

func (s *AccountRoleTagStore) setForAccountTx(ctx context.Context, tx sqlxTx, accountID string, roleTags []string) error {
	if tx == nil {
		return errors.New("nil transaction")
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM account_role_tags
		WHERE account_id = $1
	`, accountID); err != nil {
		return err
	}
	for _, roleTag := range normalizeAccountRoleTags(roleTags) {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO account_role_tags (account_id, role_tag)
			VALUES ($1, $2)
		`, accountID, roleTag); err != nil {
			return err
		}
	}
	return nil
}

type sqlxTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func normalizeAccountRoleTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = normalizeAccountRoleTag(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func normalizeAccountRoleTag(tag string) string {
	tag = strings.TrimSpace(strings.ToLower(tag))
	tag = strings.ReplaceAll(tag, "-", "_")
	tag = strings.Join(strings.Fields(tag), "_")
	return tag
}
