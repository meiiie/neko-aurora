# Neko Duet — container build (control plane + /duet/ PWA only).
# Free to build locally; deploy target is any container host (Railway, Fly, Render...).
FROM golang:1.27-alpine AS build
WORKDIR /src/app/native
COPY app/native/go.mod ./
RUN go mod download || true
COPY app/native/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/neko-control ./cmd/neko-control

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 neko && mkdir -p /data && chown neko /data
USER neko
COPY --from=build /out/neko-control /usr/local/bin/neko-control
ENV TZ=Asia/Ho_Chi_Minh PORT=8080
VOLUME ["/data"]
EXPOSE 8080
# Binds 0.0.0.0:$PORT only when DUET_PUBLIC_HOST and DUET_OWNER_CODE (>=12 chars) are set;
# remote peers can then reach only the owner-login /duet/ surface.
CMD ["neko-control", "-state", "/data"]
