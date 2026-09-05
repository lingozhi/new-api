/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export const SEEDANCE_MODELS = [
  'seedance-2.0',
  'seedance-2.0-fast',
  'seedance-2.5',
] as const

export function isSeedanceModel(name: string): boolean {
  return SEEDANCE_MODELS.some((model) => model === name)
}

export function seedanceRequest(model: string) {
  return {
    model,
    prompt:
      'A peaceful green forest in soft morning sunlight. The camera slowly moves forward.',
    duration: 4,
    resolution: '480p',
    aspect_ratio: '16:9',
    generate_audio: false,
  }
}

export function seedanceGenerationExample(
  model: string,
  baseUrl: string
): string {
  return `# Bash + curl + jq. Set NEW_API_KEY to a token from this site.
set -euo pipefail
: "\${NEW_API_KEY:?Set NEW_API_KEY first}"
BASE_URL="${baseUrl}"
REQUEST_ID=$(curl --fail-with-body --silent --show-error --max-time 120 \\
  "$BASE_URL/v1/videos/generations" \\
  -H "Authorization: Bearer $NEW_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '${JSON.stringify(seedanceRequest(model), null, 2)}' | jq -er '.request_id')
printf 'request_id=%s\n' "$REQUEST_ID"
DEADLINE=$((SECONDS + 900))
while (( SECONDS < DEADLINE )); do
  RESULT=$(curl --fail-with-body --silent --show-error --max-time 60 \\
    "$BASE_URL/v1/videos/$REQUEST_ID" -H "Authorization: Bearer $NEW_API_KEY")
  STATUS=$(printf '%s' "$RESULT" | jq -er '.status')
  case "$STATUS" in
    done)
      curl --fail-with-body --location --max-time 300 \\
        "$BASE_URL/v1/videos/$REQUEST_ID/content" \\
        -H "Authorization: Bearer $NEW_API_KEY" --output video.mp4
      exit 0 ;;
    failed|expired) printf '%s\n' "$RESULT" >&2; exit 1 ;;
    pending) sleep 15 ;;
    *) printf '%s\n' "$RESULT" >&2; exit 1 ;;
  esac
done
printf 'Still pending: query %s again; do not resubmit automatically.\n' "$REQUEST_ID" >&2
exit 1`
}

export function seedanceUploadExample(model: string, baseUrl: string): string {
  return `# Bash + curl + jq. Upload bytes directly to object storage.
set -euo pipefail
: "\${NEW_API_KEY:?Set NEW_API_KEY first}"
BASE_URL="${baseUrl}"
FILE="first-frame.png"
SIZE=$(wc -c < "$FILE")
TICKET=$(curl --fail-with-body --silent --show-error --max-time 60 \\
  "$BASE_URL/v1/media/uploads" \\
  -H "Authorization: Bearer $NEW_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d "{\\"model\\":\\"${model}\\",\\"type\\":\\"image\\",\\"content_type\\":\\"image/png\\",\\"size_bytes\\":$SIZE}")
UPLOAD_URL=$(printf '%s' "$TICKET" | jq -er '.upload_url')
MEDIA_URL=$(printf '%s' "$TICKET" | jq -er '.media_url')
curl --fail-with-body --request PUT --max-time 300 \\
  "$UPLOAD_URL" -H "Content-Type: image/png" --data-binary "@$FILE"
jq -n --arg model '${model}' --arg url "$MEDIA_URL" \\
  '{model:$model,prompt:"Animate this landscape with a slow camera movement.",start_image:{url:$url},aspect_ratio:"adaptive",duration:4,resolution:"480p"}' \\
  | curl --fail-with-body --silent --show-error --max-time 120 \\
      "$BASE_URL/v1/videos/generations" \\
      -H "Authorization: Bearer $NEW_API_KEY" \\
      -H "Content-Type: application/json" --data-binary @-`
}
