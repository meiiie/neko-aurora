# Neko Aurora — container build: Go control plane + /app/ PWA + Neko Core (Bun)
# so agent turns run through Core (GLM-5.3 on z.ai) inside the container.
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
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 neko && mkdir -p /data && chown neko /data
COPY --from=core /opt/neko-core /opt/neko-core
COPY --from=build /out/neko-aurora /usr/local/bin/neko-aurora
USER neko
ENV TZ=Asia/Ho_Chi_Minh PORT=8080 NEKO_CORE_DIR=/opt/neko-core
VOLUME ["/data"]
EXPOSE 8080
# Binds 0.0.0.0:$PORT only when AURORA_PUBLIC_HOST and AURORA_OWNER_CODE (>=12 chars) are set;
# remote peers can then reach only the owner-login /app/ surface.
# Set AURORA_RUNTIME=http to skip Core and call z.ai over HTTP instead.
ENTRYPOINT []
CMD ["neko-aurora", "-state", "/data"]
