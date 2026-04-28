# Docker One-Command Setup

Run the entire API Guard stack with a single command:

```bash
docker compose up -d
```

This starts **6 services**:

| Service | Port | Description |
|---------|------|-------------|
| **postgres** | internal | PostgreSQL 17 database |
| **mailpit** | [localhost:8025](http://localhost:8025) | Email testing UI + SMTP server |
| **auth** | [localhost:3001](http://localhost:3001) | Authentication & org management API |
| **api-guard** | [localhost:8080](http://localhost:8080) | LLM proxy |
| **dashboard** | [localhost:3000](http://localhost:3000) | Tenant dashboard (Vite dev) |
| **admin-frontend** | [localhost:3002](http://localhost:3002) | Admin panel |

## Quick Access

- **Dashboard** — http://localhost:3000 (register, login, manage your org)
- **Admin Panel** — http://localhost:3002 (super admin: manage tenants)
- **Mailpit** — http://localhost:8025 (view all emails: verification codes, invites)
- **API Proxy** — http://localhost:8080 (LLM proxy endpoint)

## Email (Mailpit)

All emails (verification codes, invite links, password resets) are captured by [Mailpit](https://mailpit.axllent.org/) instead of being sent to real inboxes.

Open http://localhost:8025 to see them.

The auth service connects to Mailpit's SMTP on port 1025 (internal docker network). No authentication needed — Mailpit accepts all mail.

### Environment

These are the default `.env` values for email:

```env
GOORG_EMAIL_PROVIDER=smtp
GOORG_SMTP_HOST=mailpit
GOORG_SMTP_PORT=1025
GOORG_SMTP_USER=
GOORG_SMTP_PASSWORD=
GOORG_SMTP_FROM=noreply@apiguard.local
```

## Tenant Invite Flow

1. **Admin creates a tenant** at http://localhost:3002/tenants (click "Add Tenant")
   - Provide: name, admin email, plan, permissions
   - Tenant is created with status **pending**

2. **Invite email is sent** — check http://localhost:8025
   - The email contains a link like: `http://localhost:3000/accept-invite?token=...`

3. **Invited user accepts** — clicks the link in Mailpit
   - Sets their name and password
   - Automatically logged in and redirected to the dashboard
   - Tenant status changes from **pending** to **active**

4. **User can now log in** at http://localhost:3000/login with the credentials they set

## Super Admin Setup

To bootstrap a super admin account (required for the admin panel), set this in `.env`:

```env
GOORG_SUPER_ADMIN_EMAIL=admin@example.com
```

Then register at http://localhost:3000/register with that email. The account will automatically get super admin privileges. Verify the email via the code in Mailpit.

Log in to the admin panel at http://localhost:3002 with the same credentials.

## Useful Commands

```bash
# Start everything
docker compose up -d

# View logs
docker compose logs -f auth        # auth service logs
docker compose logs -f dashboard    # dashboard logs

# Rebuild after code changes
docker compose up -d --build

# Stop everything
docker compose down

# Stop and wipe database
docker compose down -v
```

## Ports Summary

| Port | Service |
|------|---------|
| 3000 | Dashboard (tenant-facing) |
| 3001 | Auth API |
| 3002 | Admin panel |
| 8025 | Mailpit web UI |
| 8080 | LLM proxy |

## Switching to a Real Email Provider

To use Resend instead of Mailpit:

```env
GOORG_EMAIL_PROVIDER=resend
GOORG_RESEND_API_KEY=re_xxxxxxxxxxxx
GOORG_RESEND_FROM=noreply@yourdomain.com
```

To use a real SMTP server:

```env
GOORG_EMAIL_PROVIDER=smtp
GOORG_SMTP_HOST=smtp.gmail.com
GOORG_SMTP_PORT=587
GOORG_SMTP_USER=you@gmail.com
GOORG_SMTP_PASSWORD=app-password
GOORG_SMTP_FROM=you@gmail.com
```

## Frontend URL Configuration

Invite links in emails point to the dashboard frontend, not the auth API. This is configured via:

```env
GOORG_FRONTEND_URL=http://localhost:3000
```

If deploying to a different domain, update this to match your dashboard's public URL.
