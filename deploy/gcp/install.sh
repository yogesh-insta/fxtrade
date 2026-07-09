#!/bin/bash
# One-time setup for fxtrade on Ubuntu (e2-micro or similar).
# Run on the VM as a user with sudo. Does not start trading until .credentials exists.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
INSTALL_ROOT="${FXTRADE_HOME:-/opt/fxtrade}"
BINARY_SRC=""
NIFTY_PULSE_SRC=""
AFL_PULSE_SRC=""
AFL_PULSE_PREGAME_SRC=""
HEALTH_WATCH_SRC=""
BOT_DAILY_EMAIL_SRC=""
BOT_ANALYZE_SRC=""
BOT_WEEKLY_EMAIL_SRC=""
CREDENTIALS_SRC=""
ENABLE_ALL=false
ENABLE_BOT=""
DRY_RUN=false

usage() {
  cat <<'EOF'
Usage: sudo ./deploy/gcp/install.sh [options]

Options:
  --binary PATH         Linux amd64 fxtrade binary (default: build on VM or copy separately)
  --nifty-pulse PATH    Linux amd64 nifty-pulse binary (default: deploy via CI or build manually)
  --afl-pulse PATH      Linux amd64 afl-pulse binary (default: deploy via CI or build manually)
  --afl-pulse-pregame PATH  Linux amd64 afl-pulse-pregame binary
  --health-watch PATH   Linux amd64 health-watch binary (watchdog alerts)
  --bot-daily-email PATH  Linux amd64 bot-daily-email binary (all-bot daily summary)
  --bot-analyze PATH      Linux amd64 bot-analyze binary (trade analysis CLI)
  --bot-weekly-email PATH Linux amd64 bot-weekly-email binary (weekly bot summary)
  --credentials PATH    Local .credentials to install (default: skip; use fetch-credentials.sh)
  --enable-all          Enable fxtrade.service (all bots from .credentials)
  --enable-bot ID       Enable fxtrade@ID.service (e.g. universe_scanner, fx_sentiment)
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
    --nifty-pulse) NIFTY_PULSE_SRC="$2"; shift 2 ;;
    --afl-pulse) AFL_PULSE_SRC="$2"; shift 2 ;;
    --afl-pulse-pregame) AFL_PULSE_PREGAME_SRC="$2"; shift 2 ;;
    --health-watch) HEALTH_WATCH_SRC="$2"; shift 2 ;;
    --bot-daily-email) BOT_DAILY_EMAIL_SRC="$2"; shift 2 ;;
    --bot-analyze) BOT_ANALYZE_SRC="$2"; shift 2 ;;
    --bot-weekly-email) BOT_WEEKLY_EMAIL_SRC="$2"; shift 2 ;;
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

mkdir -p "$INSTALL_ROOT"/{bin,data,logs,scripts,deploy/gcp}
mkdir -p /etc/fxtrade

cp -f "$ROOT/deploy/gcp/"*.service /etc/systemd/system/
cp -f "$ROOT/deploy/gcp/"*.timer /etc/systemd/system/ 2>/dev/null || true
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

if [[ -n "$NIFTY_PULSE_SRC" ]]; then
  install -m 755 "$NIFTY_PULSE_SRC" "$INSTALL_ROOT/bin/nifty-pulse"
elif [[ ! -x "$INSTALL_ROOT/bin/nifty-pulse" ]]; then
  echo "note: no binary at $INSTALL_ROOT/bin/nifty-pulse yet — deploy via GitHub Actions or:"
  echo "  GOOS=linux GOARCH=amd64 go build -o nifty-pulse ./cmd/nifty-pulse"
  echo "  scp nifty-pulse user@vm:/tmp/nifty-pulse && sudo install -m 755 /tmp/nifty-pulse $INSTALL_ROOT/bin/nifty-pulse"
fi

if [[ -n "$AFL_PULSE_SRC" ]]; then
  install -m 755 "$AFL_PULSE_SRC" "$INSTALL_ROOT/bin/afl-pulse"
elif [[ ! -x "$INSTALL_ROOT/bin/afl-pulse" ]]; then
  echo "note: no binary at $INSTALL_ROOT/bin/afl-pulse yet — deploy via GitHub Actions or:"
  echo "  GOOS=linux GOARCH=amd64 go build -o afl-pulse ./cmd/afl-pulse"
  echo "  scp afl-pulse user@vm:/tmp/afl-pulse && sudo install -m 755 /tmp/afl-pulse $INSTALL_ROOT/bin/afl-pulse"
fi

if [[ -n "$AFL_PULSE_PREGAME_SRC" ]]; then
  install -m 755 "$AFL_PULSE_PREGAME_SRC" "$INSTALL_ROOT/bin/afl-pulse-pregame"
elif [[ ! -x "$INSTALL_ROOT/bin/afl-pulse-pregame" ]]; then
  echo "note: no binary at $INSTALL_ROOT/bin/afl-pulse-pregame yet — build manually:"
  echo "  GOOS=linux GOARCH=amd64 go build -o afl-pulse-pregame ./cmd/afl-pulse-pregame"
  echo "  scp afl-pulse-pregame user@vm:/tmp/ && sudo install -m 755 /tmp/afl-pulse-pregame $INSTALL_ROOT/bin/afl-pulse-pregame"
fi

if [[ -n "$HEALTH_WATCH_SRC" ]]; then
  install -m 755 "$HEALTH_WATCH_SRC" "$INSTALL_ROOT/bin/health-watch"
elif [[ ! -x "$INSTALL_ROOT/bin/health-watch" ]]; then
  echo "note: no binary at $INSTALL_ROOT/bin/health-watch yet — build manually:"
  echo "  GOOS=linux GOARCH=amd64 go build -o health-watch ./cmd/health-watch"
  echo "  scp health-watch user@vm:/tmp/ && sudo install -m 755 /tmp/health-watch $INSTALL_ROOT/bin/health-watch"
fi

if [[ -n "$BOT_DAILY_EMAIL_SRC" ]]; then
  install -m 755 "$BOT_DAILY_EMAIL_SRC" "$INSTALL_ROOT/bin/bot-daily-email"
elif [[ ! -x "$INSTALL_ROOT/bin/bot-daily-email" ]]; then
  echo "note: no binary at $INSTALL_ROOT/bin/bot-daily-email yet — build manually:"
  echo "  GOOS=linux GOARCH=amd64 go build -o bot-daily-email ./cmd/bot-daily-email"
  echo "  scp bot-daily-email user@vm:/tmp/ && sudo install -m 755 /tmp/bot-daily-email $INSTALL_ROOT/bin/bot-daily-email"
fi

if [[ -n "$BOT_ANALYZE_SRC" ]]; then
  install -m 755 "$BOT_ANALYZE_SRC" "$INSTALL_ROOT/bin/bot-analyze"
elif [[ ! -x "$INSTALL_ROOT/bin/bot-analyze" ]]; then
  echo "note: no binary at $INSTALL_ROOT/bin/bot-analyze yet — build manually:"
  echo "  GOOS=linux GOARCH=amd64 go build -o bot-analyze ./cmd/bot-analyze"
  echo "  scp bot-analyze user@vm:/tmp/ && sudo install -m 755 /tmp/bot-analyze $INSTALL_ROOT/bin/bot-analyze"
fi

if [[ -n "$BOT_WEEKLY_EMAIL_SRC" ]]; then
  install -m 755 "$BOT_WEEKLY_EMAIL_SRC" "$INSTALL_ROOT/bin/bot-weekly-email"
elif [[ ! -x "$INSTALL_ROOT/bin/bot-weekly-email" ]]; then
  echo "note: no binary at $INSTALL_ROOT/bin/bot-weekly-email yet — build manually:"
  echo "  GOOS=linux GOARCH=amd64 go build -o bot-weekly-email ./cmd/bot-weekly-email"
  echo "  scp bot-weekly-email user@vm:/tmp/ && sudo install -m 755 /tmp/bot-weekly-email $INSTALL_ROOT/bin/bot-weekly-email"
fi

install -m 755 "$ROOT/deploy/gcp/check-scheduled-jobs.sh" "$INSTALL_ROOT/scripts/check-scheduled-jobs.sh"
install -m 755 "$ROOT/deploy/gcp/run-health-watch.sh" "$INSTALL_ROOT/scripts/run-health-watch.sh"
install -m 755 "$ROOT/deploy/gcp/run-bot-daily-email.sh" "$INSTALL_ROOT/scripts/run-bot-daily-email.sh"
install -m 755 "$ROOT/deploy/gcp/run-bot-weekly-email.sh" "$INSTALL_ROOT/scripts/run-bot-weekly-email.sh"
install -m 644 "$ROOT/deploy/gcp/fxtrade-watch.cron" /etc/cron.d/fxtrade-watch
chmod 644 /etc/cron.d/fxtrade-watch
echo "Installed watchdog cron (/etc/cron.d/fxtrade-watch) — emails on failure via .credentials SMTP"

if [[ -d "$ROOT/data/afl" ]]; then
  mkdir -p "$INSTALL_ROOT/data/afl"
  cp -f "$ROOT/data/afl/"*.json "$INSTALL_ROOT/data/afl/"
  chown -R fxtrade:fxtrade "$INSTALL_ROOT/data/afl"
fi

if [[ -f "$ROOT/watchlist.txt" ]]; then
  install -o fxtrade -g fxtrade -m 644 "$ROOT/watchlist.txt" "$INSTALL_ROOT/watchlist.txt"
elif [[ ! -f "$INSTALL_ROOT/watchlist.txt" ]]; then
  echo "note: no watchlist at $INSTALL_ROOT/watchlist.txt — copy watchlist.txt from repo root"
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

systemctl enable --now nifty-pulse.timer
echo "Enabled nifty-pulse.timer (18:00 Australia/Sydney, Sun–Fri)"

systemctl enable --now afl-pulse.timer
echo "Enabled afl-pulse.timer (18:00 Australia/Melbourne, Thursday)"

systemctl enable --now afl-pulse-pregame.timer
echo "Enabled afl-pulse-pregame.timer (every 5 minutes, T-45 pregame)"

if $ENABLE_ALL; then
  systemctl enable --now fxtrade.service
  echo "Enabled fxtrade.service (all bots from .credentials)"
elif [[ -n "$ENABLE_BOT" ]]; then
  # Per-bot units need unique health ports if more than one runs at once.
  case "$ENABLE_BOT" in
    universe_scanner) port=":8081" ;;
    fx_sentiment|range_trend) port=":8082" ;;
    btc_cfd) port=":8083" ;;
    *) port=":8080" ;;
  esac
  extra_env=""
  if [[ "$ENABLE_BOT" == "btc_cfd" || "$ENABLE_BOT" == "fx_sentiment" || "$ENABLE_BOT" == "universe_scanner" ]]; then
    # Override global FXTRADE_DRY_RUN so OANDA practice orders are placed.
    extra_env=$'FXTRADE_DRY_RUN=\n'
  fi
  cat >"/etc/fxtrade/fxtrade@${ENABLE_BOT}.env" <<EOF
FXTRADE_HEALTH_ADDR=$port
${extra_env}EOF
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
