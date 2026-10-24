# Neko Aurora — container build: Go control plane + /app/ PWA + Neko Core (Bun)
# (GLM-5.3 on z.ai; Core is the fallback runtime) inside the container.
# Free to build locally; deploy target is any container host (Railway, Fly, Render...).
FROM golang:1.27-alpine AS build
WORKDIR /src/app/native
COPY app/native/go.mod ./
RUN go mod download || true
COPY app/native/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/neko-aurora ./cmd/neko-aurora

# Neko Core runtime: production dependencies only, from the committed lockfile.
FROM oven/bun:1.3-alpine AS core
WORKDIR /opt/neko-core
COPY core/ ./
RUN rm -rf docs evals experiments browser-extension cloudflare test tests \
 && bun install --frozen-lockfile --production \
 && bun bin/neko.ts --help >/dev/null

FROM oven/bun:1.3-alpine
RUN apk add --no-cache ca-certificates tzdata bubblewrap util-linux-misc chromium font-dejavu && adduser -D -u 10001 neko && mkdir -p /data && chown neko /data
COPY --from=core /opt/neko-core /opt/neko-core
COPY --from=build /out/neko-aurora /usr/local/bin/neko-aurora
USER neko
ENV TZ=Asia/Ho_Chi_Minh PORT=8080 NEKO_CORE_DIR=/opt/neko-core AURORA_COMPUTER=off AURORA_BROWSER_NOSANDBOX=1
# Hosted multi-user mode (Railway template): AURORA_MODE=hosted, AURORA_SECRET (>=32 chars),
# ZAI_API_KEY, AURORA_SMTP_* for sign-in mail; AURORA_COMPUTER=railway + RAILWAY_TOKEN +
# RAILWAY_ENVIRONMENT_ID gives every bot a Railway Sandbox. See docs/railway-template.md.
# State lives in /data: attach a Railway volume there (Railway rejects the VOLUME keyword).
EXPOSE 8080
# Binds 0.0.0.0:$PORT only when AURORA_PUBLIC_HOST and AURORA_OWNER_CODE (>=12 chars) are set;
# remote peers can then reach only the owner-login /app/ surface.
# Turns stream straight from z.ai over HTTP; Core is the fallback. AURORA_RUNTIME=core forces Core, =http skips it.
# AURORA_SEND_PER_MIN caps model-spending messages (default 20/min).
# Snapshots: /data/backups/aurora-*.tar.gz every AURORA_BACKUP_EVERY (default 6h, "off" disables), newest 14 kept.
# vault.key is never archived; restore: neko-aurora -state /new/dir -aurora-restore <archive>, then copy aurora/vault.key.
ENTRYPOINT []
CMD ["neko-aurora", "-state", "/data"]
