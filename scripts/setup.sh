#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="$REPO_ROOT/.env.compose"
ENV_EXAMPLE="$REPO_ROOT/.env.compose.example"

# ── Seed users ────────────────────────────────────────────────────────
ADMIN_NAME="Admin"
ADMIN_EMAIL="admin@apiguard.local"
ADMIN_PASSWORD="AdminPass123!"
ADMIN_ORG="APIGuard Admin"

DASH_NAME="Dashboard User"
DASH_EMAIL="dash@apiguard.local"
DASH_PASSWORD="DashPass123!"
DASH_ORG="Dashboard Dev"

AUTH_URL="http://localhost:3001"
MAILPIT_API="http://localhost:8025/api/v1"

# ── Helpers ───────────────────────────────────────────────────────────

info()  { printf "\033[1;34m==>\033[0m %s\n" "$1"; }
ok()    { printf "\033[1;32m  ✓\033[0m %s\n" "$1"; }
warn()  { printf "\033[1;33m  !\033[0m %s\n" "$1"; }
fail()  { printf "\033[1;31m  ✗\033[0m %s\n" "$1"; exit 1; }

wait_for_url() {
  local url=$1 timeout=${2:-120} elapsed=0
  while ! curl -so /dev/null --max-time 2 "$url" 2>/dev/null; do
    sleep 3
    elapsed=$((elapsed + 3))
    if [ "$elapsed" -ge "$timeout" ]; then
      fail "Timed out waiting for $url"
    fi
  done
}

try_login() {
  local email=$1 password=$2
  local status
  status=$(curl -sf -o /dev/null -w "%{http_code}" -X POST "$AUTH_URL/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$email\",\"password\":\"$password\"}" 2>/dev/null || true)
  [ "$status" = "200" ]
}

mailpit_get_code() {
  local email=$1
  local attempts=0 max_attempts=5 code=""
  while [ $attempts -lt $max_attempts ]; do
    code=$(curl -sf "$MAILPIT_API/search?query=to:$email" 2>/dev/null \
      | grep -oE '"Snippet":"[^"]*"' \
      | tail -1 \
      | grep -oE '[0-9]{6}' \
      | tail -1 || true)
    if [ -n "$code" ]; then
      echo "$code"
      return 0
    fi
    sleep 2
    attempts=$((attempts + 1))
  done
  fail "Could not get verification code for $email from Mailpit"
}

mailpit_delete_messages() {
  curl -sf -X DELETE "$MAILPIT_API/messages" > /dev/null 2>&1 || true
}

register_and_verify() {
  local name=$1 email=$2 password=$3 org_name=$4

  if try_login "$email" "$password"; then
    ok "$email already set up — skipping"
    return 0
  fi

  # Clear mailpit so we get a clean code
  mailpit_delete_messages

  # Register
  local reg_status
  reg_status=$(curl -sf -o /dev/null -w "%{http_code}" -X POST "$AUTH_URL/auth/register" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$name\",\"email\":\"$email\",\"password\":\"$password\",\"org_name\":\"$org_name\"}" 2>/dev/null || true)

  if [ "$reg_status" = "409" ]; then
    warn "$email already registered but login failed — attempting password reset"
    # Trigger password reset
    curl -sf -X POST "$AUTH_URL/auth/forgot-password" \
      -H "Content-Type: application/json" \
      -d "{\"email\":\"$email\"}" > /dev/null 2>&1
    sleep 1
    local reset_code
    reset_code=$(mailpit_get_code "$email")
    curl -sf -X POST "$AUTH_URL/auth/reset-password" \
      -H "Content-Type: application/json" \
      -d "{\"email\":\"$email\",\"code\":\"$reset_code\",\"new_password\":\"$password\"}" > /dev/null 2>&1
    ok "$email password reset"
    return 0
  fi

  if [ "$reg_status" != "200" ] && [ "$reg_status" != "201" ]; then
    fail "Register $email failed with status $reg_status"
  fi
  ok "Registered $email"

  # Get verification code from Mailpit
  sleep 1
  local code
  code=$(mailpit_get_code "$email")
  ok "Got verification code for $email"

  # Verify email
  curl -sf -X POST "$AUTH_URL/auth/verify-email" \
    -H "Content-Type: application/json" \
    -d "{\"code\":\"$code\"}" > /dev/null 2>&1
  ok "Verified $email"
}

# ── 1. Env file ──────────────────────────────────────────────────────

info "Checking environment file"

if [ -f "$ENV_FILE" ]; then
  ok ".env.compose already exists — keeping it"
else
  if [ ! -f "$ENV_EXAMPLE" ]; then
    fail ".env.compose.example not found"
  fi
  cp "$ENV_EXAMPLE" "$ENV_FILE"

  # Generate secrets
  access_secret=$(openssl rand -hex 32)
  refresh_secret=$(openssl rand -hex 32)
  master_key=$(openssl rand -hex 32)

  # Replace placeholder secrets
  sed -i.bak "s|change-me-access-secret-at-least-32-chars|$access_secret|g" "$ENV_FILE"
  sed -i.bak "s|change-me-refresh-secret-at-least-32-chars|$refresh_secret|g" "$ENV_FILE"
  sed -i.bak "s|change-me-before-real-use|$master_key|g" "$ENV_FILE"

  # Set multi-tenant mode and SMTP for Mailpit
  sed -i.bak 's|^GOORG_MODE=.*|GOORG_MODE=multi_tenant|' "$ENV_FILE"
  sed -i.bak 's|^GOORG_EMAIL_PROVIDER=.*|GOORG_EMAIL_PROVIDER=smtp|' "$ENV_FILE"

  # Uncomment and set SMTP settings for Mailpit
  sed -i.bak 's|^# GOORG_SMTP_HOST=.*|GOORG_SMTP_HOST=mailpit|' "$ENV_FILE"
  sed -i.bak 's|^# GOORG_SMTP_PORT=.*|GOORG_SMTP_PORT=1025|' "$ENV_FILE"
  sed -i.bak 's|^# GOORG_SMTP_USER=.*|GOORG_SMTP_USER=|' "$ENV_FILE"
  sed -i.bak 's|^# GOORG_SMTP_PASSWORD=.*|GOORG_SMTP_PASSWORD=|' "$ENV_FILE"
  sed -i.bak 's|^# GOORG_SMTP_FROM=.*|GOORG_SMTP_FROM=noreply@apiguard.local|' "$ENV_FILE"

  # Add frontend URL if not present
  if ! grep -q "GOORG_FRONTEND_URL" "$ENV_FILE"; then
    echo "GOORG_FRONTEND_URL=http://localhost:3000" >> "$ENV_FILE"
  fi

  rm -f "$ENV_FILE.bak"
  ok "Created .env.compose with generated secrets"
fi

# ── 2. Docker Compose ────────────────────────────────────────────────

info "Starting Docker Compose stack"
cd "$REPO_ROOT"
docker compose --env-file "$ENV_FILE" up --build -d

info "Waiting for services to become healthy"

printf "  Waiting for auth service..."
wait_for_url "$AUTH_URL/healthz" 120
printf " ready\n"

printf "  Waiting for Mailpit..."
wait_for_url "http://localhost:8025" 60
printf " ready\n"

printf "  Waiting for API Guard..."
wait_for_url "http://localhost:8080" 120
printf " ready\n"

printf "  Waiting for Dashboard..."
wait_for_url "http://localhost:3000" 120
printf " ready\n"

ok "All services are up"

# ── 3. Seed data ─────────────────────────────────────────────────────

info "Setting up admin user"
register_and_verify "$ADMIN_NAME" "$ADMIN_EMAIL" "$ADMIN_PASSWORD" "$ADMIN_ORG"

# Set super_admin flag
docker compose --env-file "$ENV_FILE" exec -T postgres \
  psql -U apiguard -d apiguard -c \
  "UPDATE goorg_users SET is_super_admin = true WHERE email = '$ADMIN_EMAIL';" \
  > /dev/null 2>&1
ok "Super admin flag set for $ADMIN_EMAIL"

info "Setting up dashboard user"
register_and_verify "$DASH_NAME" "$DASH_EMAIL" "$DASH_PASSWORD" "$DASH_ORG"

# ── 4. Summary ───────────────────────────────────────────────────────

echo ""
info "Setup complete!"
echo ""
echo "  Services:"
echo "    Dashboard        http://localhost:3000"
echo "    Admin Panel      http://localhost:3002"
echo "    Auth API         http://localhost:3001"
echo "    API Guard        http://localhost:8080"
echo "    Mailpit (email)  http://localhost:8025"
echo ""
echo "  Credentials:"
echo "    Admin:     $ADMIN_EMAIL / $ADMIN_PASSWORD"
echo "    Dashboard: $DASH_EMAIL / $DASH_PASSWORD"
echo ""
echo "  Useful commands:"
echo "    npm run dev:logs    Follow service logs"
echo "    npm run dev:down    Stop all services"
echo "    npm run dev:build   Rebuild and restart"
echo ""
