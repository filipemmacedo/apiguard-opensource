#!/bin/sh
set -e

# If a SQLite database is mounted, migrate its data to Postgres first.
if [ -f "${SQLITE_PATH:-/data/usage.db}" ]; then
  echo "SQLite file found - running migration check..."
  /app/migrate-sqlite --sqlite "${SQLITE_PATH:-/data/usage.db}" --postgres "${DATABASE_DSN}"
fi

exec /app/apiguard
