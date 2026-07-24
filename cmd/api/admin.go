package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/config"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/logging"
	"github.com/openb00ks/openb00ks/internal/models"
)

type resetAdminOptions struct {
	Email      string
	Password   string
	TenantName string
}

func runAdmin(args []string) {
	cfg := config.Load()
	logging.Setup(logging.Config{
		Level:     cfg.LogLevel,
		Format:    cfg.LogFormat,
		AddSource: false,
	})
	if len(args) == 0 {
		slog.Error("usage: openb00ks admin reset-password --email <email> --password <password>")
		os.Exit(1)
	}
	switch args[0] {
	case "reset-password":
		opts, err := parseResetAdminOptions(args[1:])
		if err != nil {
			slog.Error("invalid reset-password options", "err", err)
			os.Exit(1)
		}
		if err := resetAdminPassword(cfg.DatabaseURL, opts); err != nil {
			slog.Error("admin password reset failed", "err", err)
			os.Exit(1)
		}
		fmt.Printf("admin user ready: %s\n", opts.Email)
	default:
		slog.Error("unknown admin command")
		os.Exit(1)
	}
}

func parseResetAdminOptions(args []string) (resetAdminOptions, error) {
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := resetAdminOptions{}
	fs.StringVar(&opts.Email, "email", "", "admin email")
	fs.StringVar(&opts.Password, "password", "", "new admin password")
	fs.StringVar(&opts.TenantName, "tenant-name", "Default Tenant", "tenant name to create if none exists")
	if err := fs.Parse(args); err != nil {
		return resetAdminOptions{}, err
	}
	opts.Email = strings.TrimSpace(opts.Email)
	opts.Password = strings.TrimSpace(opts.Password)
	opts.TenantName = strings.TrimSpace(opts.TenantName)
	if opts.Email == "" || opts.Password == "" {
		return resetAdminOptions{}, fmt.Errorf("email and password are required")
	}
	if opts.TenantName == "" {
		opts.TenantName = "Default Tenant"
	}
	return opts, nil
}

func resetAdminPassword(databaseURL string, opts resetAdminOptions) error {
	if strings.TrimSpace(databaseURL) == "" {
		return db.ErrMissingDSN
	}
	hash, err := auth.HashPassword(opts.Password)
	if err != nil {
		return err
	}
	conn, err := db.Open(databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Warn("database close failed", "err", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := conn.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	userID, tenantID, err := upsertAdminUser(ctx, tx, opts.Email, hash, opts.TenantName)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tenant_memberships (tenant_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, user_id)
		DO UPDATE SET role = EXCLUDED.role
	`, tenantID, userID, string(models.RoleAdmin)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET default_tenant_id = $2
		WHERE id = $1
	`, userID, tenantID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO system_settings (id, setup_complete, setup_completed_at, settings_json, updated_at)
		VALUES (1, true, now(), '{}'::jsonb, now())
		ON CONFLICT (id) DO UPDATE
		SET setup_complete = true,
		    setup_completed_at = COALESCE(system_settings.setup_completed_at, EXCLUDED.setup_completed_at),
		    updated_at = now()
	`); err != nil {
		return err
	}
	return tx.Commit()
}

type resetAdminTx interface {
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func upsertAdminUser(ctx context.Context, tx resetAdminTx, email, passwordHash, tenantName string) (string, string, error) {
	var row struct {
		ID              string         `db:"id"`
		DefaultTenantID sql.NullString `db:"default_tenant_id"`
	}
	err := tx.GetContext(ctx, &row, `
		SELECT id, default_tenant_id
		FROM users
		WHERE email = $1
	`, email)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	userID := row.ID
	defaultTenantID := row.DefaultTenantID
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.GetContext(ctx, &userID, `
			INSERT INTO users (email, password_hash, is_admin)
			VALUES ($1, $2, true)
			RETURNING id
		`, email, passwordHash); err != nil {
			return "", "", err
		}
	} else if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET password_hash = $2,
		    is_admin = true
		WHERE id = $1
	`, userID, passwordHash); err != nil {
		return "", "", err
	}

	tenantID := ""
	if defaultTenantID.Valid {
		tenantID = defaultTenantID.String
	}
	if tenantID == "" {
		err := tx.GetContext(ctx, &tenantID, `
			SELECT tenant_id
			FROM tenant_memberships
			WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT 1
		`, userID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", "", err
		}
	}
	if tenantID == "" {
		err := tx.GetContext(ctx, &tenantID, `
			SELECT id
			FROM tenants
			ORDER BY created_at ASC
			LIMIT 1
		`)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", "", err
		}
	}
	if tenantID == "" {
		if err := tx.GetContext(ctx, &tenantID, `
			INSERT INTO tenants (name)
			VALUES ($1)
			RETURNING id
		`, tenantName); err != nil {
			return "", "", err
		}
	}
	return userID, tenantID, nil
}
