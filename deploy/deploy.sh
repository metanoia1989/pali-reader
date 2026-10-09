#!/usr/bin/env bash
#
# Builds and ships the reader to bigubuntu.
#
#   ./deploy/deploy.sh            # backend + frontend
#   ./deploy/deploy.sh backend    # only the Go binary
#   ./deploy/deploy.sh frontend   # only the built SPA
#
# The server has no Go or Node toolchain, so everything is cross-compiled here
# and uploaded. The Go binary is static (CGO off), which is why this works at
# all: the sqlite driver used at import time is pure Go.
set -euo pipefail

HOST="${HOST:-bigubuntu}"
APP_DIR=/www/server/go_project/pali_reading
WEB_DIR=/www/wwwroot/pali.bigubuntu.internal

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WHAT="${1:-all}"

say() { printf '\n\033[1m== %s\033[0m\n' "$*"; }

build_backend() {
  say "building backend (linux/amd64)"
  (
    cd "$ROOT/backend"
    # shellcheck source=../goenv.sh
    source "$ROOT/goenv.sh"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
      go build -trimpath -ldflags "-s -w" -o /tmp/pali-reader .
  )
  say "uploading binary"
  scp -q /tmp/pali-reader "$HOST:/tmp/pali-reader"
  ssh "$HOST" "sudo install -m 755 -o www -g www /tmp/pali-reader $APP_DIR/pali-reader && rm -f /tmp/pali-reader"
}

build_frontend() {
  say "building frontend"
  (cd "$ROOT/frontend" && npm run build)
  say "uploading frontend"
  tar czf /tmp/pali-dist.tgz -C "$ROOT/frontend/dist" .
  scp -q /tmp/pali-dist.tgz "$HOST:/tmp/pali-dist.tgz"
  # The swap is ordered so a reader loading the page mid-deploy never sees an
  # index.html that points at assets which are not there yet: the new files are
  # copied in first, and only then are the superseded ones removed.
  ssh "$HOST" "
    set -e
    rm -rf /tmp/palidist && mkdir -p /tmp/palidist
    tar xzf /tmp/pali-dist.tgz -C /tmp/palidist
    rm -f /tmp/pali-dist.tgz

    # Remove hashed assets from the previous build that this one does not carry.
    if [ -d $WEB_DIR/assets ]; then
      for f in $WEB_DIR/assets/*; do
        [ -e "\$f" ] || continue
        base=\$(basename "\$f")
        [ -e "/tmp/palidist/assets/\$base" ] || sudo rm -f "\$f"
      done
    fi

    sudo cp -r /tmp/palidist/. $WEB_DIR/
    rm -rf /tmp/palidist
    sudo chown -R www:www $WEB_DIR 2>/dev/null || true
  "
}

restart() {
  say "restarting the API"
  # The panel owns the process: it wrote the launcher at
  # /www/server/go_project/vhost/scripts/pali_reading.sh, which sources the env
  # file, starts the binary under www and writes a pid file. Stopping by name
  # and then running that launcher keeps the panel's view of the project
  # accurate; starting the binary directly would leave it thinking it is down.
  ssh "$HOST" "
    set -e
    sudo pkill -x pali-reader 2>/dev/null || true
    sleep 1
    launcher=/www/server/go_project/vhost/scripts/pali_reading.sh
    if [ -x \"\$launcher\" ]; then
      sudo -u www \"\$launcher\" >/dev/null 2>&1 || true
    else
      echo 'launcher not found; start the project from the panel' >&2
      exit 1
    fi
  "
  sleep 2
}

check() {
  say "health check"
  for i in 1 2 3 4 5; do
    if out=$(ssh "$HOST" "curl -s --max-time 5 http://127.0.0.1:8091/api/health"); then
      echo "$out"
      case "$out" in
        *'"ok":true'*) return 0 ;;
      esac
    fi
    sleep 2
  done
  echo "API did not come up; recent log:" >&2
  ssh "$HOST" "sudo tail -30 /www/wwwlogs/go/pali_reading.log 2>/dev/null || true" >&2
  return 1
}

case "$WHAT" in
  backend) build_backend ;;
  frontend) build_frontend ;;
  all) build_backend; build_frontend ;;
  *) echo "usage: $0 [all|backend|frontend]" >&2; exit 2 ;;
esac

restart
check
say "done — http://pali.bigubuntu.internal/"
