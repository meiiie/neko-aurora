#!/bin/sh
# Local-only terminal for an owner to configure Neko Core directly.
# No key is accepted by this script or placed in arguments/logs/Git.
# Core stores its profile locally; this is NOT an encrypted or permanent vault.
set -eu
umask 077
[ "$(uname -s)" = Linux ] && [ "$(uname -m)" = x86_64 ] || {
  echo 'This development helper requires Linux x86-64.' >&2; exit 1;
}
base="${NEKO_TERMINAL_HOME:-$HOME/.local/share/neko-provider-terminal}"
mkdir -p "$base/bin" "$base/home/.neko-core" "$base/work"
verify() { printf '%s  %s\n' "$1" "$2" | sha256sum -c - >/dev/null; }
core_hash=a71c2ada73b6eece75d58759826805ab831113d5e2357e5bb9edc20895cd8a4f
ttyd_hash=8a217c968aba172e0dbf3f34447218dc015bc4d5e59bf51db2f2cd12b7be4f55
if [ ! -e "$base/bin/neko" ]; then
  curl -fL --connect-timeout 20 --max-time 180 \
    https://github.com/meiiie/neko-core/releases/download/v1.8.0/neko-linux-x64.gz \
    -o "$base/bin/neko.gz"
  verify 1cc4df3734d74f95ae81cce28cb263b1d3bdfba2d4733f677e4d380b8e6c40d5 "$base/bin/neko.gz"
  gzip -dc "$base/bin/neko.gz" > "$base/bin/neko"
fi
verify "$core_hash" "$base/bin/neko"
if [ ! -e "$base/bin/ttyd" ]; then
  curl -fL --connect-timeout 20 --max-time 90 \
    https://github.com/tsl0922/ttyd/releases/download/1.7.7/ttyd.x86_64 \
    -o "$base/bin/ttyd"
fi
verify "$ttyd_hash" "$base/bin/ttyd"
chmod 700 "$base/bin/neko" "$base/bin/ttyd"
config="$base/home/.neko-core/config.json"
if [ ! -e "$config" ]; then
  cat > "$config" <<'JSON'
{"active_profile":"zai","auto_update":false,"auto_update_check":false,"profiles":{"zai":{"provider":"anthropic","base_url":"https://api.z.ai/api/anthropic","model":"glm-5.3","auth":"api_key"}}}
JSON
fi
echo 'Open the local terminal at http://127.0.0.1:18767/ in this machine.'
echo 'The owner enters the key through Neko Core /login. Do not paste keys in chat.'
exec env HOME="$base/home" USERPROFILE="$base/home" NEKO_HOME="$base/home" \
  "$base/bin/ttyd" -i 127.0.0.1 -p 18767 -W -O -m 1 -d 3 \
  -w "$base/work" "$base/bin/neko" --profile zai
