#!/bin/sh
set -eu

# Railway mounts /data after image construction. The worker and Chromium run
# as this dedicated user; they do not receive a host project or browser mount.
if [ "${DATA_DIR:-/data}" != /data ]; then
  echo 'DATA_DIR must be /data in the cloud image' >&2
  exit 64
fi
if [ "$#" -eq 0 ]; then
  set -- node /app/server.mjs
fi
if [ "$(id -u)" = 0 ]; then
  install -d -m 0700 -o aurora -g aurora /data
  chown -R --no-dereference aurora:aurora /data
  chmod 0700 /data
  exec gosu aurora "$@"
fi
exec "$@"
