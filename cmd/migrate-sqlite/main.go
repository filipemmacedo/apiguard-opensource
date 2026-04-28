// migrate-sqlite copies data from a local SQLite usage.db into the Postgres
// database used by the api-guard service. It is idempotent: if Postgres already
// contains data in a given table, that table is skipped.
//
// Usage:
//
//	migrate-sqlite --sqlite <path> --postgres <dsn>
//
// Environment variable fallbacks:
//
//	SQLITE_PATH    (default: usage.db)
//	DATABASE_DSN   (Postgres connection string)
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func main() {
	sqlitePath := flag.String("sqlite", envOr("SQLITE_PATH", "usage.db"), "path to SQLite database")
	pgDSN := flag.String("postgres", envOr("DATABASE_DSN", ""), "Postgres connection string")
	flag.Parse()

	if *pgDSN == "" {
		log.Fatal("--postgres / DATABASE_DSN is required")
	}

	// Open SQLite
	sqliteDB, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_pragma=journal_mode(WAL)", *sqlitePath))
	if err != nil {
		log.Printf("SQLite database not found at %s — nothing to migrate", *sqlitePath)
		os.Exit(0)
	}
	if err := sqliteDB.Ping(); err != nil {
		log.Printf("SQLite database not reachable at %s — nothing to migrate", *sqlitePath)
		os.Exit(0)
	}
	defer sqliteDB.Close()

	// Open Postgres
	pgDB, err := sql.Open("pgx", *pgDSN)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	if err := pgDB.Ping(); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}
	defer pgDB.Close()

	tables := []tableSpec{
		{
			name: "usage_records",
			cols: []string{"tenant_id", "timestamp_utc", "request_id", "model", "status", "latency_ms", "prompt_tokens", "completion_tokens", "total_tokens"},
		},
		{
			name: "tenant_api_keys",
			cols: []string{"tenant_id", "display_name", "lookup_key", "key_format", "key_id", "secret_hash", "secret_mask", "status", "created_by", "created_at", "revoked_by", "revoked_at", "deleted_by", "deleted_at", "last_used_at"},
		},
		{
			name: "provider_credentials",
			cols: []string{"provider_type", "display_name", "base_url", "secret_ciphertext", "secret_key_version", "secret_mask", "status", "last_validated_at", "last_validation_error", "created_by", "created_at", "updated_by", "updated_at"},
		},
		{
			name: "provider_models",
			cols: []string{"provider_credential_id", "provider_type", "provider_model_id", "display_name", "enabled", "sync_state", "metadata_json", "last_synced_at"},
		},
		{
			name: "audit_events",
			cols: []string{"actor_id", "actor_role", "action", "resource_type", "resource_id", "resource_label", "status", "request_id", "details_json", "created_at"},
		},
	}

	// Additional tables that might exist in newer SQLite DBs
	for _, extra := range []tableSpec{
		{
			name: "pii_findings",
			cols: []string{"tenant_id", "request_id", "timestamp_utc", "direction", "entity_type", "action", "fingerprint", "finding_count"},
		},
		{
			name: "guardrail_outcomes",
			cols: []string{"tenant_id", "request_id", "timestamp_utc", "guardrail_type", "action", "matched_policy_id"},
		},
		{
			name: "pii_guardrail_policies",
			cols: []string{"entity_type", "enabled", "action", "updated_by", "updated_at"},
		},
		{
			name: "nsfw_blocked_terms",
			cols: []string{"term", "normalized_term", "enabled", "created_by", "created_at", "updated_by", "updated_at"},
		},
	} {
		if sqliteTableExists(sqliteDB, extra.name) {
			tables = append(tables, extra)
		}
	}

	migrated := 0
	for _, t := range tables {
		if !sqliteTableExists(sqliteDB, t.name) {
			log.Printf("[%s] table does not exist in SQLite — skip", t.name)
			continue
		}
		if !pgTableExists(pgDB, t.name) {
			log.Printf("[%s] table does not exist in Postgres — skip (proxy creates it on boot)", t.name)
			continue
		}

		pgCount := tableCount(pgDB, t.name)
		if pgCount > 0 {
			log.Printf("[%s] Postgres already has %d rows — skip", t.name, pgCount)
			continue
		}

		sqliteCount := tableCount(sqliteDB, t.name)
		if sqliteCount == 0 {
			log.Printf("[%s] SQLite has 0 rows — skip", t.name)
			continue
		}

		n, err := migrateTable(sqliteDB, pgDB, t)
		if err != nil {
			log.Printf("[%s] ERROR: %v", t.name, err)
			continue
		}
		log.Printf("[%s] migrated %d rows", t.name, n)
		migrated += n
	}

	if migrated > 0 {
		log.Printf("migration complete: %d total rows copied", migrated)
	} else {
		log.Printf("nothing to migrate")
	}
}

type tableSpec struct {
	name string
	cols []string
}

func migrateTable(src, dst *sql.DB, t tableSpec) (int, error) {
	colList := strings.Join(t.cols, ", ")
	rows, err := src.Query(fmt.Sprintf("SELECT %s FROM %s", colList, t.name))
	if err != nil {
		return 0, fmt.Errorf("read sqlite: %w", err)
	}
	defer rows.Close()

	// Build insert with $1, $2, ... placeholders
	placeholders := make([]string, len(t.cols))
	for i := range t.cols {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		t.name, colList, strings.Join(placeholders, ", "))

	tx, err := dst.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return 0, fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		vals := make([]any, len(t.cols))
		ptrs := make([]any, len(t.cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return count, fmt.Errorf("scan row %d: %w", count, err)
		}
		if _, err := stmt.Exec(vals...); err != nil {
			return count, fmt.Errorf("insert row %d: %w", count, err)
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return count, nil
}

func sqliteTableExists(db *sql.DB, name string) bool {
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&n)
	return err == nil && n > 0
}

func pgTableExists(db *sql.DB, name string) bool {
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_name=$1", name).Scan(&n)
	return err == nil && n > 0
}

func tableCount(db *sql.DB, name string) int64 {
	var n int64
	_ = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", name)).Scan(&n)
	return n
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
