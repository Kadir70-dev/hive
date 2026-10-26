#!/usr/bin/env bash
# Re-select existing SHA-tagged app images. DOES NOT downgrade database schema.
# Refuses to proceed until an operator confirms schema compatibility.
set -euo pipefail
: "${ENV_FILE:?Set ENV_FILE to the private Hostinger env file}"
: "${PREVIOUS_TAG:?Set PREVIOUS_TAG to a prior deployed git SHA}"
[[ "${CONFIRM_SCHEMA_COMPATIBLE:-}" == "YES" ]] || { echo "Review DB migrations first; set CONFIRM_SCHEMA_COMPATIBLE=YES only if safe."; exit 2; }
[[ "$PREVIOUS_TAG" =~ ^[0-9a-f]{12,40}$ ]] || { echo "Tag must be a 12-40 character git SHA"; exit 2; }
[[ -f "$ENV_FILE" ]] || { echo "Environment file not found"; exit 2; }
for image in hive-api hive-web; do
  docker image inspect "$image:$PREVIOUS_TAG" >/dev/null || { echo "Missing image $image:$PREVIOUS_TAG; rollback aborted"; exit 1; }
done
export HIVE_IMAGE_TAG="$PREVIOUS_TAG"
docker compose --env-file "$ENV_FILE" -f docker-compose.yml -f docker-compose.hostinger.yml \
  up -d --no-build --no-deps api web
echo "Images selected: $PREVIOUS_TAG. Verify application health; no schema rollback was attempted."
