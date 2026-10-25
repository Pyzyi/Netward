#!/usr/bin/env bash
# Container integration test: runs the image with the hardening flags from
# docker-compose.yml on an isolated bridge network next to a few "devices",
# then checks privilege drop, file ownership, discovery and the API.
#
#   e2e/container-test.sh [image]
set -euo pipefail

IMAGE="${1:-netzwart:local}"
NET="nwtest-$$"
SUBNET="172.31.$((RANDOM % 200 + 20)).0/24"
WORK="$(mktemp -d)"
PASS="Sonnenblume-im-Garten7"
FAILED=0
CONTAINERS=()

cleanup() {
  for c in "${CONTAINERS[@]}"; do docker rm -f "$c" >/dev/null 2>&1 || true; done
  docker network rm "$NET" >/dev/null 2>&1 || true
  rm -rf "$WORK" 2>/dev/null || sudo rm -rf "$WORK" 2>/dev/null || true
}
trap cleanup EXIT
if [ -n "${GITHUB_ACTIONS:-}" ]; then
  trap 'echo "::error title=container-test::line $LINENO: $BASH_COMMAND"' ERR
fi

ok()   { echo "  ok   $*"; }
fail() { echo "  FAIL $*"; [ -n "${GITHUB_ACTIONS:-}" ] && echo "::error title=container-test::$*"; FAILED=$((FAILED + 1)); }

docker network create --subnet "$SUBNET" "$NET" >/dev/null
GW="$(docker network inspect -f '{{(index .IPAM.Config 0).Gateway}}' "$NET")"
for i in 1 2 3; do
  c="nwdev-$$-$i"
  docker run -d --rm --network "$NET" --name "$c" alpine:3.22 sleep 3600 >/dev/null
  CONTAINERS+=("$c")
done

HARDEN=(--cap-drop ALL --cap-add CHOWN --cap-add SETUID --cap-add SETGID --cap-add NET_RAW
        --security-opt no-new-privileges:true --read-only)

# start NAME [docker run args...] -> prints container IP
start() {
  local name="$1"; shift
  docker run -d --name "$name" --network "$NET" "$@" "$IMAGE" >/dev/null
  CONTAINERS+=("$name")
  for _ in $(seq 1 60); do
    st="$(docker inspect -f '{{.State.Status}}' "$name")"
    [ "$st" = "running" ] || break
    ip="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$name")"
    if [ -n "$ip" ] && curl -fsS "http://$ip:8840/healthz" >/dev/null 2>&1; then echo "$ip"; return 0; fi
    sleep 1
  done
  docker logs "$name" >&2 || true
  return 1
}

status_of() { cat "/proc/$(docker inspect -f '{{.State.Pid}}' "$1")/status"; }

api() { # api IP METHOD PATH [JSON] [cookiejar]
  local ip="$1" method="$2" path="$3" body="${4:-}" jar="${5:-$WORK/jar}"
  local args=(-fsS -X "$method" -b "$jar" -c "$jar" -H "Origin: http://$ip:8840" -H "Content-Type: application/json")
  [ -f "$WORK/csrf" ] && args+=(-H "X-CSRF-Token: $(cat "$WORK/csrf")")
  [ -n "$body" ] && args+=(-d "$body")
  curl "${args[@]}" "http://$ip:8840/api/v1$path"
}

echo "== 1. Standard: compose flags, bind mount owned by root"
mkdir -p "$WORK/data1"
IP="$(start nw1-$$ "${HARDEN[@]}" -v "$WORK/data1:/data")" || { fail "container did not become ready"; exit 1; }
ST="$(status_of nw1-$$)"
grep -qE '^Uid:\s+10001\s+10001\s+10001\s+10001' <<<"$ST" && ok "runs as UID 10001" || fail "wrong UID: $(grep ^Uid <<<"$ST")"
grep -qE '^CapEff:\s+0+$' <<<"$ST" && ok "no effective capabilities" || fail "CapEff: $(grep ^CapEff <<<"$ST")"
grep -qE '^CapPrm:\s+0+$' <<<"$ST" && ok "no permitted capabilities" || fail "CapPrm: $(grep ^CapPrm <<<"$ST")"
grep -qE '^NoNewPrivs:\s+1' <<<"$ST" && ok "no_new_privs set" || fail "NoNewPrivs not set"
[ "$(stat -c '%u %a' "$WORK/data1")" = "10001 700" ] && ok "data dir 10001/0700" || fail "data dir: $(stat -c '%u %a' "$WORK/data1")"
[ "$(stat -c '%u %a' "$WORK/data1/netzwart.db")" = "10001 600" ] && ok "database 10001/0600" || fail "db: $(stat -c '%u %a' "$WORK/data1/netzwart.db")"
docker logs nw1-$$ 2>&1 | grep -q 'mode=raw' && ok "raw socket mode" || fail "not in raw mode"
code="$(curl -s -o /dev/null -w '%{http_code}' -H 'Host: evil.example' "http://$IP:8840/api/v1/auth/state")"
[ "$code" = 421 ] && ok "DNS rebinding host rejected (421)" || fail "foreign Host answered $code"
curl -sI "http://$IP:8840/" | grep -qi "content-security-policy: default-src 'none'" && ok "CSP header" || fail "CSP missing"

api "$IP" POST /auth/setup "{\"username\":\"admin\",\"password\":\"$PASS\",\"language\":\"de\",\"timezone\":\"Europe/Berlin\"}" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["csrf"])' > "$WORK/csrf" && ok "setup" || fail "setup failed"
found=0
for _ in $(seq 1 30); do
  found="$(api "$IP" GET /devices | python3 -c 'import json,sys; print(sum(1 for d in json.load(sys.stdin) if d["online"]))')"
  [ "$found" -ge 5 ] && break
  sleep 2
done
[ "$found" -ge 5 ] && ok "discovered $found devices (3 test containers, gateway, itself)" || fail "only $found devices discovered"
api "$IP" GET /devices | python3 -c "
import json,sys
ds=json.load(sys.stdin)
assert any(d['isGateway'] and d['ip']=='$GW' for d in ds), 'gateway not flagged'
assert any(d['isSelf'] for d in ds), 'self not flagged'
" && ok "gateway and self recognised" || fail "gateway/self classification"

echo "== 2. docker exec reset-password while running"
out="$(docker exec nw1-$$ /netzwart reset-password)"
NEWPW="$(sed -n 's/^New password for "admin": //p' <<<"$out")"
[ -n "$NEWPW" ] && ok "reset-password printed a new password" || fail "reset-password: $out"
bad="$(find "$WORK/data1" ! -uid 10001 | wc -l)"
[ "$bad" = 0 ] && ok "no root-owned files created" || fail "$bad files not owned by 10001: $(find "$WORK/data1" ! -uid 10001)"
rm -f "$WORK/jar" "$WORK/csrf"
api "$IP" POST /auth/login "{\"username\":\"admin\",\"password\":\"$NEWPW\"}" >/dev/null && ok "login with new password" || fail "login after reset"
sleep 35
[ "$(docker inspect -f '{{.State.Health.Status}}' nw1-$$)" = healthy ] && ok "Docker healthcheck healthy" || fail "health: $(docker inspect -f '{{.State.Health.Status}}' nw1-$$)"

echo "== 3. Existing data dir owned by UID 1000"
mkdir -p "$WORK/data3" && chown 1000:1000 "$WORK/data3" && chmod 755 "$WORK/data3"
IP3="$(start nw3-$$ "${HARDEN[@]}" -v "$WORK/data3:/data")" && ok "starts with foreign-owned directory" || fail "did not start"
[ "$(stat -c '%u' "$WORK/data3")" = 10001 ] && ok "directory handed to 10001" || fail "owner $(stat -c '%u' "$WORK/data3")"

echo "== 4. No volume (anonymous volume from the image)"
start nw4-$$ "${HARDEN[@]}" >/dev/null && ok "starts without volume" || fail "no-volume start failed"

echo "== 5. PUID/PGID 1000"
mkdir -p "$WORK/data5"
start nw5-$$ "${HARDEN[@]}" -e PUID=1000 -e PGID=1000 -v "$WORK/data5:/data" >/dev/null && ok "starts with PUID=1000" || fail "PUID start failed"
grep -qE '^Uid:\s+1000\s' <<<"$(status_of nw5-$$)" && ok "runs as UID 1000" || fail "PUID not applied"

echo "== 6. Without NET_RAW: unprivileged fallback"
mkdir -p "$WORK/data6"
IP6="$(start nw6-$$ --cap-drop ALL --cap-add CHOWN --cap-add SETUID --cap-add SETGID --security-opt no-new-privileges:true --read-only -v "$WORK/data6:/data")" \
  && ok "starts without NET_RAW" || fail "fallback start failed"
docker logs nw6-$$ 2>&1 | grep -q 'mode=fallback' && ok "fallback mode reported" || fail "fallback mode not reported"

echo "== 7. --user 1000:1000 (no root at all)"
mkdir -p "$WORK/data7" && chown 1000:1000 "$WORK/data7"
start nw7-$$ --user 1000:1000 --cap-drop ALL --security-opt no-new-privileges:true --read-only -v "$WORK/data7:/data" >/dev/null \
  && ok "starts as non-root user" || fail "non-root start failed"

echo
if [ "$FAILED" -gt 0 ]; then echo "$FAILED check(s) failed"; exit 1; fi
echo "all container checks passed"
