#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
guard="$script_dir/verify-production-build-source.sh"
valid_sha=0123456789abcdef0123456789abcdef01234567
production_project_id=aabd5c5d-f858-45b7-8fb0-6798d7fb6766
production_environment_id=5c24d515-a1e0-484c-b98d-64ccac58c404
production_service_id=fe183840-fc81-41d5-9de0-b3239f695681

RAILWAY_PROJECT_ID=another-project "$guard"

env \
  RAILWAY_PROJECT_ID="$production_project_id" \
  RAILWAY_ENVIRONMENT_ID="$production_environment_id" \
  RAILWAY_SERVICE_ID="$production_service_id" \
  RAILWAY_ENVIRONMENT_NAME=renamed-production \
  RAILWAY_GIT_REPO_OWNER=lingozhi \
  RAILWAY_GIT_REPO_NAME=new-api \
  RAILWAY_GIT_BRANCH=main \
  RAILWAY_GIT_COMMIT_SHA="$valid_sha" \
  "$guard"

if env \
  RAILWAY_PROJECT_ID="$production_project_id" \
  RAILWAY_ENVIRONMENT_ID="$production_environment_id" \
  RAILWAY_SERVICE_ID="$production_service_id" \
  RAILWAY_GIT_REPO_OWNER=lingozhi \
  RAILWAY_GIT_REPO_NAME=new-api \
  RAILWAY_GIT_BRANCH=main \
  "$guard" >/dev/null 2>&1; then
  echo "error: guard accepted production without a commit SHA" >&2
  exit 1
fi

if env \
  RAILWAY_PROJECT_ID="$production_project_id" \
  RAILWAY_ENVIRONMENT_ID="$production_environment_id" \
  RAILWAY_SERVICE_ID="$production_service_id" \
  RAILWAY_GIT_REPO_OWNER=lingozhi \
  RAILWAY_GIT_REPO_NAME=new-api \
  RAILWAY_GIT_BRANCH=feature \
  RAILWAY_GIT_COMMIT_SHA="$valid_sha" \
  "$guard" >/dev/null 2>&1; then
  echo "error: guard accepted a non-main production branch" >&2
  exit 1
fi

if env \
  RAILWAY_PROJECT_ID="$production_project_id" \
  RAILWAY_ENVIRONMENT_ID="$production_environment_id" \
  RAILWAY_SERVICE_ID="$production_service_id" \
  RAILWAY_GIT_REPO_OWNER=someone \
  RAILWAY_GIT_REPO_NAME=new-api \
  RAILWAY_GIT_BRANCH=main \
  RAILWAY_GIT_COMMIT_SHA="$valid_sha" \
  "$guard" >/dev/null 2>&1; then
  echo "error: guard accepted the wrong production repository" >&2
  exit 1
fi

echo "production build source guard tests passed"
