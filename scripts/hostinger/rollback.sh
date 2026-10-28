#!/usr/bin/env bash
# Re-select existing SHA-tagged app images. DOES NOT downgrade database schema.
# Refuses to proceed until an operator confirms schema compatibility.
set -euo pipefail
: "${ENV_FILE:?Set ENV_FILE to the private Hostinger env file}"
: "${PREVIOUS_TAG:?Set PREVIOUS_TAG to a prior deployed git SHA}"
[[ "${CONFIRM_SCHEMA_COMPATIBLE:-}" == "YES" ]] || { echo "Review DB migrations first; set CONFIRM_SCHEMA_COMPATIBLE=YES only if safe."; exit 2; }
[[ "$PREVIOUS_TAG" =~ ^[0-9a-f]{12,40}$ ]] || { echo "Tag must be a 12-40 character git SHA"; exit 2; }
[[ -f "$ENV_FILE" ]] || { echo "Environment file not found"; exit 2; }
# Layout settings come from the env file so a rollback targets the same stack.
envval() { grep -E "^$1=" "$ENV_FILE" | tail -1 | cut -d= -f2- | tr -d "\"'" || true; }
web_suffix="$(envval HIVE_WEB_IMAGE_SUFFIX)"
dc=(docker compose --env-file "$ENV_FILE" -f docker-compose.yml -f docker-compose.hostinger.yml)
[[ -z "$(envval HIVE_EDGE_ALIAS)" ]] || dc+=(-f docker-compose.edge.yml)
for image in hive-api "hive-web${web_suffix}"; do
  docker image inspect "$image:$PREVIOUS_TAG" >/dev/null || { echo "Missing image $image:$PREVIOUS_TAG; rollback aborted"; exit 1; }
done
export HIVE_IMAGE_TAG="$PREVIOUS_TAG"
"${dc[@]}" up -d --no-build --no-deps api web
# Restarting the web container is asynchronous. Do not declare success before
# the private Next.js healthcheck responds.
ready=0
for attempt in $(seq 1 45); do
  if "${dc[@]}" exec -T web node -e 'fetch("http://127.0.0.1:3000/auth").then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))' >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 2
done
[[ "$ready" == 1 ]] || { echo "Rollback app did not become healthy. Investigate immediately."; exit 1; }
echo "Images selected and Next.js ready: $PREVIOUS_TAG. No schema rollback was attempted."
