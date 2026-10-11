package db

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week11/your-monitor/migrations"
)

// region: migrate

// migrateLockID names the advisory lock that Migrate holds. Any number
// works, as long as nothing else in the database uses the same one.
const migrateLockID = 8_080_001

// Migrate applies every migration in the migrations package that the
// database has not recorded yet, in order of version.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	return migrate(ctx, pool, migrations.FS)
}

func migrate(ctx context.Context, pool *pgxpool.Pool, files fs.FS) error {
	// The api and the worker start at the same moment and both migrate.
	// The advisory lock makes the second one wait until the first is done.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "select pg_advisory_lock($1)", migrateLockID); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer conn.Exec(context.WithoutCancel(ctx), "select pg_advisory_unlock($1)", migrateLockID)

	if _, err := conn.Exec(ctx, `create table if not exists schema_migrations (
		version    int primary key,
		applied_at timestamptz not null default now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	// endregion: migrate

	// region: apply
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return err
	}
	slices.Sort(names) // 001_init.sql, 002_jobs.sql, ...
	for _, name := range names {
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %s: name must start with a number, as in 001_init.sql", name)
		}
		var applied bool
		err = conn.QueryRow(ctx,
			"select exists (select 1 from schema_migrations where version = $1)", version).Scan(&applied)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		sql, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		// The file and its schema_migrations row commit together or not
		// at all: a migration that fails halfway leaves no trace.
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "insert into schema_migrations (version) values ($1)", version)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		log.Printf("migrated version=%d file=%s", version, name)
	}
	return nil
}

// endregion: apply
