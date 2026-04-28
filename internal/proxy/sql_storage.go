package proxy

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const (
	sqlDialectPostgres sqlDialect = "postgres"
	sqlDialectSQLite   sqlDialect = "sqlite"
)

type sqlDialect string

type sqlStorage struct {
	db      *sql.DB
	dialect sqlDialect
}

func openSQLStorage(driver, target string) (*sqlStorage, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", "postgres", "pgx":
		return openPostgresStorage(target)
	case "sqlite", "sqlite3":
		return openSQLiteStorage(target)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", driver)
	}
}

func openPostgresStorage(dsn string) (*sqlStorage, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("postgres database dsn is required")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres storage: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres storage: %w", err)
	}

	return &sqlStorage{db: db, dialect: sqlDialectPostgres}, nil
}

func openSQLiteStorage(path string) (*sqlStorage, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("sqlite database path is required")
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite storage: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite storage: %w", err)
	}

	return &sqlStorage{db: db, dialect: sqlDialectSQLite}, nil
}

func (s *sqlStorage) close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *sqlStorage) exec(query string, args ...any) (sql.Result, error) {
	return s.db.Exec(rebindSQL(s.dialect, query), args...)
}

func (s *sqlStorage) query(query string, args ...any) (*sql.Rows, error) {
	return s.db.Query(rebindSQL(s.dialect, query), args...)
}

func (s *sqlStorage) queryRow(query string, args ...any) *sql.Row {
	return s.db.QueryRow(rebindSQL(s.dialect, query), args...)
}

func (s *sqlStorage) begin() (*sqlTx, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	return &sqlTx{tx: tx, dialect: s.dialect}, nil
}

func (s *sqlStorage) insertReturningID(query string, args ...any) (int64, error) {
	var id int64
	if err := s.queryRow(query+" RETURNING id", args...).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *sqlStorage) addColumnIfMissing(table, column, definition string) error {
	if s.dialect != sqlDialectSQLite {
		_, err := s.exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s", table, column, definition))
		return err
	}

	rows, err := s.query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			columnTyp string
			notNull   int
			defaultV  sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &columnTyp, &notNull, &defaultV, &pk); err != nil {
			return err
		}
		if strings.EqualFold(name, column) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = s.exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

type sqlTx struct {
	tx      *sql.Tx
	dialect sqlDialect
}

func (tx *sqlTx) exec(query string, args ...any) (sql.Result, error) {
	return tx.tx.Exec(rebindSQL(tx.dialect, query), args...)
}

func (tx *sqlTx) query(query string, args ...any) (*sql.Rows, error) {
	return tx.tx.Query(rebindSQL(tx.dialect, query), args...)
}

func (tx *sqlTx) commit() error {
	return tx.tx.Commit()
}

func (tx *sqlTx) rollback() error {
	return tx.tx.Rollback()
}

func rebindSQL(dialect sqlDialect, query string) string {
	if dialect != sqlDialectPostgres || !strings.Contains(query, "?") {
		return query
	}

	var builder strings.Builder
	builder.Grow(len(query) + 8)

	argIndex := 1
	for _, r := range query {
		if r != '?' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('$')
		builder.WriteString(strconv.Itoa(argIndex))
		argIndex++
	}

	return builder.String()
}
