#!/bin/sh

set -eu

production_project_id=aabd5c5d-f858-45b7-8fb0-6798d7fb6766
production_environment_id=5c24d515-a1e0-484c-b98d-64ccac58c404
production_service_id=fe183840-fc81-41d5-9de0-b3239f695681

if [ "${RAILWAY_PROJECT_ID:-}" != "$production_project_id" ] ||
  [ "${RAILWAY_ENVIRONMENT_ID:-}" != "$production_environment_id" ] ||
  [ "${RAILWAY_SERVICE_ID:-}" != "$production_service_id" ]; then
  exit 0
fi

fail() {
  echo "error: production builds must originate from lingozhi/new-api main on GitHub: $1" >&2
  exit 1
}

[ "${RAILWAY_GIT_REPO_OWNER:-}" = "lingozhi" ] || fail "unexpected repository owner"
[ "${RAILWAY_GIT_REPO_NAME:-}" = "new-api" ] || fail "unexpected repository name"
[ "${RAILWAY_GIT_BRANCH:-}" = "main" ] || fail "unexpected branch"

commit_sha=${RAILWAY_GIT_COMMIT_SHA:-}
[ ${#commit_sha} -eq 40 ] || fail "missing or invalid commit SHA"
case "$commit_sha" in
  *[!0-9a-fA-F]*) fail "invalid commit SHA" ;;
esac

echo "Verified production Git source: $RAILWAY_GIT_REPO_OWNER/$RAILWAY_GIT_REPO_NAME@$commit_sha"
