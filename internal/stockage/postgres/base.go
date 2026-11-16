// Package postgres stocke durablement les joueurs, les parties et leur journal d'événements.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/felix-marchenay/coinchenet-core/internal/donnees"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

var (
	_ table.Journal      = (*Base)(nil)
	_ table.Archiviste   = (*Base)(nil)
	_ table.Reprise      = (*Base)(nil)
	_ donnees.Joueurs    = (*Base)(nil)
	_ donnees.Historique = (*Base)(nil)
)

//go:embed migrations/*.sql
var fichiersMigrations embed.FS

// cleVerrouMigrations identifie le verrou consultatif qui sérialise les migrations entre instances.
const cleVerrouMigrations = 4_242_000_001

type Base struct {
	pool *pgxpool.Pool
}

func Ouvrir(ctx context.Context, url string) (*Base, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("configuration Postgres invalide : %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connexion à Postgres : %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("base Postgres injoignable : %w", err)
	}
	return &Base{pool: pool}, nil
}

func (b *Base) Fermer() {
	b.pool.Close()
}

func (b *Base) Ping(ctx context.Context) error {
	return b.pool.Ping(ctx)
}

type migration struct {
	version int
	nom     string
	sql     string
}

func lireMigrations() ([]migration, error) {
	entrees, err := fs.ReadDir(fichiersMigrations, "migrations")
	if err != nil {
		return nil, err
	}
	var ms []migration
	for _, e := range entrees {
		prefixe, _, ok := strings.Cut(e.Name(), "_")
		version, err := strconv.Atoi(prefixe)
		if !ok || err != nil {
			return nil, fmt.Errorf("nom de migration invalide %q", e.Name())
		}
		contenu, err := fichiersMigrations.ReadFile(path.Join("migrations", e.Name()))
		if err != nil {
			return nil, err
		}
		ms = append(ms, migration{version: version, nom: e.Name(), sql: string(contenu)})
	}
	slices.SortFunc(ms, func(a, b migration) int { return a.version - b.version })
	return ms, nil
}

// Migrer applique les migrations manquantes, chacune dans sa transaction, sous verrou consultatif.
func (b *Base) Migrer(ctx context.Context) error {
	ms, err := lireMigrations()
	if err != nil {
		return fmt.Errorf("lecture des migrations : %w", err)
	}

	conn, err := b.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("connexion pour les migrations : %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "select pg_advisory_lock($1)", cleVerrouMigrations); err != nil {
		return fmt.Errorf("verrou des migrations : %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), "select pg_advisory_unlock($1)", cleVerrouMigrations)
	}()

	_, err = conn.Exec(ctx, `create table if not exists schema_migrations (
		version integer primary key,
		appliquee_le timestamptz not null default now()
	)`)
	if err != nil {
		return fmt.Errorf("création de schema_migrations : %w", err)
	}

	lignes, err := conn.Query(ctx, "select version from schema_migrations")
	if err != nil {
		return fmt.Errorf("lecture des migrations appliquées : %w", err)
	}
	appliquees, err := pgx.CollectRows(lignes, pgx.RowTo[int])
	if err != nil {
		return fmt.Errorf("lecture des migrations appliquées : %w", err)
	}

	for _, m := range ms {
		if slices.Contains(appliquees, m.version) {
			continue
		}
		err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, m.sql); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "insert into schema_migrations (version) values ($1)", m.version)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s : %w", m.nom, err)
		}
	}
	return nil
}

func violationUnicite(err error) (contrainte string, ok bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName, true
	}
	return "", false
}
