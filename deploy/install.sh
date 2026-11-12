#!/usr/bin/env bash
set -euo pipefail

echo "==> NVR install on Ubuntu"

if [ "$(id -u)" -ne 0 ]; then
  echo "Запусти от root: sudo bash deploy/install.sh"
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y ca-certificates curl git

if ! command -v docker >/dev/null 2>&1; then
  echo "==> Installing Docker..."
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker
fi

docker compose version >/dev/null 2>&1 || {
  echo "Docker Compose plugin missing"
  exit 1
}

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

if [ ! -f .env ]; then
  SECRET=$(openssl rand -hex 24 2>/dev/null || head -c 32 /dev/urandom | xxd -p)
  cat > .env <<ENV
JWT_SECRET=${SECRET}
TZ=Europe/Moscow
ENV
  echo "==> Created deploy/.env with random JWT_SECRET"
fi

echo "==> Building and starting..."
docker compose up -d --build

echo ""
echo "Готово."
echo "  UI:  http://$(hostname -I | awk '{print $1}')/"
echo "  Логин: admin / admin"
echo ""
echo "Полезные команды:"
echo "  cd $SCRIPT_DIR && docker compose logs -f"
echo "  cd $SCRIPT_DIR && docker compose ps"
echo "  cd $SCRIPT_DIR && docker compose restart"
