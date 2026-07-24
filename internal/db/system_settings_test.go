package db

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsMissingSystemSettingsTableErr(t *testing.T) {
	t.Parallel()

	if !isMissingSystemSettingsTableErr(&pgconn.PgError{Code: "42P01"}) {
		t.Fatal("expected undefined_table to be treated as missing system_settings table")
	}
	if isMissingSystemSettingsTableErr(errors.New("boom")) {
		t.Fatal("expected generic errors to be ignored")
	}
}
