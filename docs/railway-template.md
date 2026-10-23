# Deploy Neko Aurora on Railway (one click)

[![Deploy on Railway](https://railway.com/button.svg)](https://railway.com/new/template?template=https%3A%2F%2Fgithub.com%2Fmeiiie%2Fneko-aurora)

The repo is template-ready: `Dockerfile` builds the Go server + PWA + Neko
Core, `railway.json` sets the start command, `/healthz` health check and
restart policy. State lives on a volume at `/data`.

## Service variables
| Variable | Required | Value |
|---|---|---|
| `ZAI_API_KEY` | yes | z.ai key (server pays; metered per user plan) |
| `AURORA_MODE` | yes | `hosted` (multi-user sign-up). Omit for single-owner. |
| `AURORA_SECRET` | yes | `${{secret(48)}}` — signs sessions, derives per-user vault keys. **Never change it after launch** (vaults become unreadable). |
| `AURORA_PUBLIC_URL` | yes | `https://${{RAILWAY_PUBLIC_DOMAIN}}` |
| `RAILWAY_RUN_UID` | yes | `0` (the volume is root-owned) |
| `AURORA_SMTP_HOST` `AURORA_SMTP_PORT` `AURORA_SMTP_USER` `AURORA_SMTP_PASS` `AURORA_SMTP_FROM` | for email sign-in | any STARTTLS SMTP (Resend, Postmark, Gmail app password) |
| `AURORA_PLAN_FREE_DAILY` / `AURORA_PLAN_PRO_DAILY` | no | messages/day (default 50 / 1000) |
| `AURORA_MAX_USERS` | no | sign-up cap (default 500) |
| `GOOGLE_CLIENT_ID` `GOOGLE_CLIENT_SECRET` | no | enables "Continue with Google"; redirect `https://<domain>/app/auth/google/callback` |
| `APPLE_CLIENT_ID` `APPLE_TEAM_ID` `APPLE_KEY_ID` `APPLE_PRIVATE_KEY` | no | enables "Sign in with Apple" (Services ID, .p8); return URL `https://<domain>/app/auth/apple/callback` |
| `AURORA_COMPUTER` `RAILWAY_TOKEN` `RAILWAY_ENVIRONMENT_ID` | no | `railway` gives each bot a Railway Sandbox VM (billed per second) |
| `AURORA_DEPLOY_URL` | no | link for the in-app "Deploy on Railway" button (set to the published template URL) |

Single-owner mode instead: drop `AURORA_MODE`, set `AURORA_PUBLIC_HOST=<domain>` and `AURORA_OWNER_CODE` (≥12 chars).

## Publish the template (once, by the repo owner)
1. Railway → New Project → Deploy from GitHub repo → `meiiie/neko-aurora` (branch `main`).
2. Service → Settings → Networking → **Generate Domain** (port 8080).
3. Right-click the service → **Attach volume**, mount path `/data`.
4. Variables: add the table above (use `${{secret(48)}}` for `AURORA_SECRET`, reference `${{RAILWAY_PUBLIC_DOMAIN}}`). Mark `ZAI_API_KEY` and SMTP values as required *template* inputs with descriptions, leave secrets empty.
5. Project Settings → **Generate Template from Project** → fill name "Neko Aurora", description, icon (`app/native/internal/web/assets/app/icon-512.png`), category AI → **Publish**.
6. Copy the template URL (`https://railway.com/deploy/<code>`) and replace the button links in `README.md`, this file and `AURORA_DEPLOY_URL` (default in `internal/hub/hub.go`).

## After deploy
Open `https://<domain>/app/` on the iPhone (Safari → Share → Add to Home Screen),
sign in with the emailed code, tap **Bắt đầu**. Backups: enable Railway volume
backups (the in-app 6-hourly archives run only in single-owner mode).
