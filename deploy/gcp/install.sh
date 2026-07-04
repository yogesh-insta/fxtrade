#!/bin/bash
# One-time setup for fxtrade on Ubuntu (e2-micro or similar).
# Run on the VM as a user with sudo. Does not start trading until .credentials exists.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
INSTALL_ROOT="${FXTRADE_HOME:-/opt/fxtrade}"
BINARY_SRC=""
CREDENTIALS_SRC=""
ENABLE_ALL=false
ENABLE_BOT=""
DRY_RUN=false

usage() {
  cat <<'EOF'
Usage: sudo ./deploy/gcp/install.sh [options]

Options:
  --binary PATH         Linux amd64 fxtrade binary (default: build on VM or copy separately)
  --credentials PATH    Local .credentials to install (default: skip; use fetch-credentials.sh)
  --enable-all          Enable fxtrade.service (all bots from .credentials)
  --enable-bot ID       Enable fxtrade@ID.service (e.g. universe_scanner, range_trend)
  --dry-run             Set FXTRADE_DRY_RUN=--dry-run in /etc/fxtrade/fxtrade.env
  -h, --help            Show this help

Examples:
  sudo ./deploy/gcp/install.sh --enable-all
  sudo ./deploy/gcp/install.sh --binary /tmp/fxtrade --credentials /tmp/.credentials --enable-bot universe_scanner
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BINARY_SRC="$2"; shift 2 ;;
    --credentials) CREDENTIALS_SRC="$2"; shift 2 ;;
    --enable-all) ENABLE_ALL=true; shift ;;
    --enable-bot) ENABLE_BOT="$2"; shift 2 ;;
    --dry-run) DRY_RUN=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1"; usage; exit 1 ;;
  esac
done

if [[ "$(id -u)" -ne 0 ]]; then
  echo "error: run with sudo"
  exit 1
fi

if ! id fxtrade &>/dev/null; then
  useradd --system --home-dir "$INSTALL_ROOT" --shell /usr/sbin/nologin fxtrade
fi

mkdir -p "$INSTALL_ROOT"/{bin,data,logs,deploy/gcp}
mkdir -p /etc/fxtrade

cp -f "$ROOT/deploy/gcp/"*.service /etc/systemd/system/
cp -f "$ROOT/deploy/gcp/fxtrade.env.example" /etc/fxtrade/fxtrade.env
sed -i "s|FXTRADE_HOME=.*|FXTRADE_HOME=$INSTALL_ROOT|" /etc/fxtrade/fxtrade.env
sed -i "s|FXTRADE_CREDENTIALS=.*|FXTRADE_CREDENTIALS=$INSTALL_ROOT/.credentials|" /etc/fxtrade/fxtrade.env

if $DRY_RUN; then
  sed -i 's|^# FXTRADE_DRY_RUN=|FXTRADE_DRY_RUN=|' /etc/fxtrade/fxtrade.env || true
  if grep -q '^FXTRADE_DRY_RUN=' /etc/fxtrade/fxtrade.env; then
    sed -i 's|^FXTRADE_DRY_RUN=.*|FXTRADE_DRY_RUN=--dry-run|' /etc/fxtrade/fxtrade.env
  else
    echo 'FXTRADE_DRY_RUN=--dry-run' >> /etc/fxtrade/fxtrade.env
  fi
fi

if [[ -n "$BINARY_SRC" ]]; then
  install -m 755 "$BINARY_SRC" "$INSTALL_ROOT/bin/fxtrade"
elif [[ ! -x "$INSTALL_ROOT/bin/fxtrade" ]]; then
  echo "note: no binary at $INSTALL_ROOT/bin/fxtrade yet — deploy via GitHub Actions or:"
  echo "  GOOS=linux GOARCH=amd64 go build -o fxtrade ./cmd/fxtrade"
  echo "  scp fxtrade user@vm:/tmp/fxtrade && sudo install -m 755 /tmp/fxtrade $INSTALL_ROOT/bin/fxtrade"
fi

cp -f "$ROOT/deploy/gcp/fetch-credentials.sh" "$INSTALL_ROOT/deploy/gcp/"
chmod 755 "$INSTALL_ROOT/deploy/gcp/fetch-credentials.sh"

if [[ -n "$CREDENTIALS_SRC" ]]; then
  install -o fxtrade -g fxtrade -m 600 "$CREDENTIALS_SRC" "$INSTALL_ROOT/.credentials"
fi

chown -R fxtrade:fxtrade "$INSTALL_ROOT"
chmod 750 "$INSTALL_ROOT"
chmod 755 "$INSTALL_ROOT"/{bin,logs,data} 2>/dev/null || true

systemctl daemon-reload

if $ENABLE_ALL; then
  systemctl enable --now fxtrade.service
  echo "Enabled fxtrade.service (all bots from .credentials)"
elif [[ -n "$ENABLE_BOT" ]]; then
  # Per-bot units need unique health ports if more than one runs at once.
  case "$ENABLE_BOT" in
    universe_scanner) port=":8081" ;;
    range_trend) port=":8082" ;;
    *) port=":8080" ;;
  esac
  cat >"/etc/fxtrade/fxtrade@${ENABLE_BOT}.env" <<EOF
FXTRADE_HEALTH_ADDR=$port
EOF
  systemctl daemon-reload
  systemctl enable --now "fxtrade@${ENABLE_BOT}.service"
  echo "Enabled fxtrade@${ENABLE_BOT}.service (health $port)"
else
  echo "Systemd units installed. Enable one of:"
  echo "  sudo systemctl enable --now fxtrade.service"
  echo "  sudo systemctl enable --now fxtrade@universe_scanner.service"
fi

if [[ ! -f "$INSTALL_ROOT/.credentials" ]]; then
  echo ""
  echo "Next: place $INSTALL_ROOT/.credentials (copy from laptop or Secret Manager)."
  echo "  sudo $INSTALL_ROOT/deploy/gcp/fetch-credentials.sh fxtrade-credentials"
  echo "Then: sudo systemctl restart fxtrade.service  # or fxtrade@BOT"
fi

echo "Install root: $INSTALL_ROOT"
echo "Health (default): curl http://127.0.0.1:8080/health"
