package migrate

import (
	"database/sql"
	"errors"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/openb00ks/openb00ks/db/migrations"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func newMigrate(dbURL string) (*migrate.Migrate, *sql.DB, error) {
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return nil, nil, err
	}
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	return m, db, nil
}

func Up(dbURL string, steps int) error {
	m, db, err := newMigrate(dbURL)
	if err != nil {
		return err
	}
	defer closeAll(m, db)
	if steps > 0 {
		if err := m.Steps(steps); err != nil && errors.Is(err, migrate.ErrNoChange) {
			return nil
		} else {
			return err
		}
	}
	if err := m.Up(); err != nil && errors.Is(err, migrate.ErrNoChange) {
		return nil
	} else {
		return err
	}
}

func Down(dbURL string, steps int) error {
	m, db, err := newMigrate(dbURL)
	if err != nil {
		return err
	}
	defer closeAll(m, db)
	if steps <= 0 {
		steps = 1
	}
	return m.Steps(-steps)
}

func Goto(dbURL string, version uint) error {
	m, db, err := newMigrate(dbURL)
	if err != nil {
		return err
	}
	defer closeAll(m, db)
	return m.Migrate(version)
}

func Force(dbURL string, version int) error {
	m, db, err := newMigrate(dbURL)
	if err != nil {
		return err
	}
	defer closeAll(m, db)
	return m.Force(version)
}

func Version(dbURL string) (uint, bool, error) {
	m, db, err := newMigrate(dbURL)
	if err != nil {
		return 0, false, err
	}
	defer closeAll(m, db)
	v, dirty, err := m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, dirty, nil
	}
	return v, dirty, err
}

func closeAll(m *migrate.Migrate, db *sql.DB) {
	_, _ = m.Close()
	_ = db.Close()
}
