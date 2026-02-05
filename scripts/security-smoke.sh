#!/usr/bin/env sh
set -eu

COMPOSE_FILE="${COMPOSE_FILE:-docker/docker-compose.yml}"
PROJECT_DIR="${PROJECT_DIR:-.}"

cd "$PROJECT_DIR"

echo "==> Starting stack"
docker compose -f "$COMPOSE_FILE" up -d

echo "==> Wait for peer to be ready"
# Simple wait loop for health endpoint inside container
for i in $(seq 1 20); do
  if docker exec docker-peer-1 sh -lc "wget -qO- http://127.0.0.1:8081/healthz >/dev/null 2>&1"; then
    echo "healthz OK"
    break
  fi
  sleep 1
  if [ "$i" -eq 20 ]; then
    echo "healthz not ready"
    exit 1
  fi
done

echo "==> Check perms (run/data/keys)"
docker exec docker-peer-1 sh -lc "ls -la /var/lib/ardents/run && ls -la /var/lib/ardents/data && ls -la /var/lib/ardents/data/identity && ls -la /var/lib/ardents/data/keys"

echo "==> Check metrics endpoint"
docker exec docker-peer-1 sh -lc "wget -qO- http://127.0.0.1:9090 | head -n 20"

echo "==> Ensure IPC token/socket present"
docker exec docker-peer-1 sh -lc "ls -la /var/lib/ardents/run/peer.token /var/lib/ardents/run/peer.sock"

echo "==> Negative IPC auth test"
docker compose -f "$COMPOSE_FILE" stop integration integration-2 integration-3

docker exec docker-peer-1 sh -lc 'set -e; TOK=$(cat /var/lib/ardents/run/peer.token); echo "$TOK" > /tmp/peer.token.bak; echo badtoken > /var/lib/ardents/run/peer.token; /usr/local/bin/integration-ipc --home /var/lib/ardents --upstream http://127.0.0.1:8080 --service test.service.v1 --job test.service.v1 >/tmp/ipc_bad.log 2>&1 || true; head -n 5 /tmp/ipc_bad.log; mv /tmp/peer.token.bak /var/lib/ardents/run/peer.token'

echo "==> Negative IPC ACL test (bad perms on peer.token)"
docker exec docker-peer-1 sh -lc 'set -e; chmod 0666 /var/lib/ardents/run/peer.token; /usr/local/bin/integration-ipc --home /var/lib/ardents --upstream http://127.0.0.1:8080 --service test.service.v1 --job test.service.v1 >/tmp/ipc_acl.log 2>&1 || true; head -n 5 /tmp/ipc_acl.log; chmod 0600 /var/lib/ardents/run/peer.token'

docker compose -f "$COMPOSE_FILE" start integration integration-2 integration-3

echo "==> Done"
