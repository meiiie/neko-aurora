#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  prepare)
    sudo apt-get update -qq
    sudo apt-get install -y --no-install-recommends bubblewrap
    if [ -e /proc/sys/kernel/apparmor_restrict_unprivileged_userns ]; then
      original="$(sysctl -n kernel.apparmor_restrict_unprivileged_userns)"
      echo "NEKO_CI_APPARMOR_USERNS_ORIGINAL=$original" >> "$GITHUB_ENV"
      sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
    fi
    bwrap --unshare-user --unshare-pid --ro-bind / / --proc /proc --dev /dev /bin/true
    ;;
  restore)
    if [ -n "${NEKO_CI_APPARMOR_USERNS_ORIGINAL:-}" ]; then
      sudo sysctl -w "kernel.apparmor_restrict_unprivileged_userns=$NEKO_CI_APPARMOR_USERNS_ORIGINAL"
    fi
    ;;
  *) exit 2 ;;
esac
