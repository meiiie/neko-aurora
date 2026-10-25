FROM node:24-bookworm-slim

ENV NODE_ENV=production \
    PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    DATA_DIR=/data \
    PORT=8080 \
    HOME=/home/aurora

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev \
    && apt-get update \
    && apt-get install -y --no-install-recommends gosu tini ca-certificates \
    && npx playwright install --with-deps chromium \
    && useradd --create-home --uid 10001 --shell /usr/sbin/nologin aurora \
    && install -d -m 0700 -o aurora -g aurora /data \
    && npm cache clean --force \
    && rm -rf /var/lib/apt/lists/*

COPY --chown=root:root . ./
RUN chmod 0755 /app/entrypoint.sh

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD node -e "fetch('http://127.0.0.1:'+process.env.PORT+'/healthz').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"
ENTRYPOINT ["/usr/bin/tini", "--", "/app/entrypoint.sh"]
