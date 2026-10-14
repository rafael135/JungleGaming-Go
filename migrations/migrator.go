package migrations

import (
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed *.sql
var Files embed.FS

var ErrNoChange = migrate.ErrNoChange

func newMigrate(databaseURL string) (*migrate.Migrate, error) {
	sourceDriver, err := iofs.New(Files, ".")
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar migrations embutidas (iofs): %w", err)
	}

	// O driver pgx/v5 do golang-migrate é registrado sob o esquema 'pgx5://'
	migURL := databaseURL
	if strings.HasPrefix(migURL, "postgres://") {
		migURL = "pgx5://" + strings.TrimPrefix(migURL, "postgres://")
	} else if strings.HasPrefix(migURL, "postgresql://") {
		migURL = "pgx5://" + strings.TrimPrefix(migURL, "postgresql://")
	}

	m, err := migrate.NewWithSourceInstance("iofs", sourceDriver, migURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao inicializar golang-migrate: %w", err)
	}

	return m, nil
}

// Up aplica todas as migrations pendentes usando o golang-migrate.
func Up(databaseURL string) error {
	m, err := newMigrate(databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			return ErrNoChange
		}
		return fmt.Errorf("erro ao executar migrations up: %w", err)
	}

	return nil
}

// Down reverte a última migration aplicada usando o golang-migrate.
func Down(databaseURL string) error {
	m, err := newMigrate(databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Steps(-1); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			return ErrNoChange
		}
		return fmt.Errorf("erro ao reverter migration (down): %w", err)
	}

	return nil
}

// Version retorna a versão atual e se o banco está em estado 'dirty'.
func Version(databaseURL string) (uint, bool, error) {
	m, err := newMigrate(databaseURL)
	if err != nil {
		return 0, false, err
	}
	defer m.Close()

	v, dirty, err := m.Version()
	if err != nil {
		return 0, false, err
	}

	return v, dirty, nil
}
