package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/openb00ks/openb00ks/internal/models"
)

const accountColumns = "a.id, a.entity_id, a.name, a.type, a.code, a.created_at"

type AccountStore struct {
	db       *DB
	roleTags *AccountRoleTagStore
}

func NewAccountStore(db *DB, roleTags ...*AccountRoleTagStore) *AccountStore {
	var tagStore *AccountRoleTagStore
	if len(roleTags) > 0 {
		tagStore = roleTags[0]
	}
	return &AccountStore{db: db, roleTags: tagStore}
}

func (s *AccountStore) ListForEntity(ctx context.Context, entityID string, limit int) ([]models.Account, error) {
	if limit <= 0 {
		limit = 200
	}
	rows := []AccountRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT `+accountColumns+`
		FROM accounts a
		WHERE a.entity_id = $1
		ORDER BY a.code IS NULL, a.code, a.name
		LIMIT $2
	`, entityID, limit)
	if err != nil {
		return nil, err
	}
	accounts := make([]models.Account, 0, len(rows))
	for _, row := range rows {
		account, err := s.accountFromRow(ctx, row)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

func (s *AccountStore) Create(ctx context.Context, entityID, name, typ, code string, roleTags ...string) (models.Account, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return models.Account{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	var id string
	err = tx.GetContext(ctx, &id, `
		INSERT INTO accounts (entity_id, name, type, code)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		RETURNING id
	`, entityID, name, typ, code)
	if err != nil {
		return models.Account{}, err
	}
	if s.roleTags != nil && roleTags != nil {
		if err = s.roleTags.setForAccountTx(ctx, tx, id, roleTags); err != nil {
			return models.Account{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return models.Account{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *AccountStore) Update(ctx context.Context, accountID, name, typ, code string, roleTags ...string) (models.Account, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return models.Account{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	var id string
	err = tx.GetContext(ctx, &id, `
		UPDATE accounts
		SET name = $1, type = $2, code = NULLIF($3, '')
		WHERE id = $4
		RETURNING id
	`, name, typ, code, accountID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Account{}, ErrNotFound
		}
		return models.Account{}, err
	}
	if s.roleTags != nil && roleTags != nil {
		if err = s.roleTags.setForAccountTx(ctx, tx, id, roleTags); err != nil {
			return models.Account{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return models.Account{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *AccountStore) GetByID(ctx context.Context, accountID string) (models.Account, error) {
	row := AccountRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+accountColumns+`
		FROM accounts a
		WHERE a.id = $1
	`, accountID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Account{}, ErrNotFound
		}
		return models.Account{}, err
	}
	return s.accountFromRow(ctx, row)
}

func (s *AccountStore) Delete(ctx context.Context, accountID string) error {
	// An account with posted journal lines can't be deleted — it would orphan history. Report it cleanly
	// instead of surfacing a raw FK violation.
	var inUse bool
	if err := s.db.GetContext(ctx, &inUse, `SELECT EXISTS(SELECT 1 FROM entries WHERE account_id = $1)`, accountID); err != nil {
		return err
	}
	if inUse {
		return ErrAccountInUse
	}
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM accounts
		WHERE id = $1
	`, accountID)
	if err != nil {
		// Other tables (vendor rules, drafts, import rows) also reference accounts with RESTRICT; treat any
		// remaining FK violation as in-use rather than a 500.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrAccountInUse
		}
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

func (s *AccountStore) GetEntityID(ctx context.Context, accountID string) (string, error) {
	var entityID string
	err := s.db.GetContext(ctx, &entityID, `
		SELECT a.entity_id
		FROM accounts a
		WHERE a.id = $1
	`, accountID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return entityID, nil
}

func (s *AccountStore) FindDefaultCashAccount(ctx context.Context, entityID string) (models.Account, error) {
	row := AccountRow{}
	err := s.db.GetContext(ctx, &row, `
		SELECT `+accountColumns+`
		FROM accounts a
		WHERE a.entity_id = $1
		  AND lower(a.name) = 'cash'
		ORDER BY a.created_at DESC
		LIMIT 1
	`, entityID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Account{}, ErrNotFound
		}
		return models.Account{}, err
	}
	return s.accountFromRow(ctx, row)
}

func (s *AccountStore) accountFromRow(ctx context.Context, row AccountRow) (models.Account, error) {
	account := models.Account{
		ID:        row.ID,
		EntityID:  row.EntityID,
		Name:      row.Name,
		Type:      row.Type,
		Code:      row.Code.String,
		CreatedAt: row.CreatedAt,
	}
	if s.roleTags == nil {
		return account, nil
	}
	tags, err := s.roleTags.ListByAccountID(ctx, row.ID)
	if err != nil {
		return models.Account{}, err
	}
	account.RoleTags = tags
	return account, nil
}
