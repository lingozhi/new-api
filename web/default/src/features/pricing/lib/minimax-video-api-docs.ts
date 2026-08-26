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
import { ENDPOINT_TYPES } from '../constants'
import type { PricingModel } from '../types'
import type { ImageSampleLanguage } from './image-api-docs'
import type { SupportedParameter } from './mock-stats'

export const MINIMAX_VIDEO_V2_ENDPOINT_TYPE = ENDPOINT_TYPES.MINIMAX_VIDEO_V2
export const MINIMAX_VIDEO_V2_QUERY_PATH =
  '/v2/query/video_generation/{task_id}'

export type MiniMaxVideoSampleContext = {
  baseUrl: string
  apiKeyEnv: string
  modelName: string
  endpointPath: string
}

export function supportsMiniMaxVideoV2Endpoint(model: PricingModel): boolean {
  return (
    model.supported_endpoint_types?.includes(MINIMAX_VIDEO_V2_ENDPOINT_TYPE) ??
    false
  )
}

export function buildMiniMaxVideoParameters(
  modelName: string
): SupportedParameter[] {
  return [
    {
      name: 'model',
      type: 'string',
      required: true,
      defaultValue: modelName,
      descriptionKey: 'Exact model identifier for MiniMax-H3 video generation',
    },
    {
      name: 'content',
      type: 'array',
      required: true,
      range: '1 ~ 16',
      descriptionKey:
        'Ordered text and reference media items; at least one non-empty text item is required',
    },
    {
      name: 'resolution',
      type: 'enum',
      required: true,
      defaultValue: '768P',
      enumValues: ['768P'],
      descriptionKey: 'Output resolution; only 768P is currently supported',
    },
    {
      name: 'duration',
      type: 'integer',
      required: true,
      range: '4 ~ 15',
      descriptionKey:
        'Requested video duration in whole seconds; billing uses this value',
    },
    {
      name: 'ratio',
      type: 'enum',
      required: true,
      enumValues: ['16:9', '9:16', '1:1'],
      descriptionKey:
        'Explicit output ratio; 1:1 is unavailable when reference audio is used',
    },
    {
      name: 'callback_url',
      type: 'string',
      range: '≤ 2048 characters',
      descriptionKey:
        'Optional public HTTPS URL; it must echo the create-time challenge within 3 seconds and receives terminal task results',
    },
    {
      name: 'aigc_watermark',
      type: 'boolean',
      defaultValue: false,
      enumValues: ['false'],
      descriptionKey:
        'Watermark option reserved by MiniMax V2; the current implementation requires false',
    },
  ]
}

function miniMaxVideoRequestBody(context: MiniMaxVideoSampleContext) {
  return {
    model: context.modelName,
    content: [
      {
        type: 'text',
        text: 'A cinematic tracking shot of a paper boat sailing through a neon city at night.',
      },
    ],
    resolution: '768P',
    duration: 6,
    ratio: '16:9',
    aigc_watermark: false,
  }
}

function buildCurlSample(context: MiniMaxVideoSampleContext): string {
  const requestBody = JSON.stringify(miniMaxVideoRequestBody(context), null, 2)

  return [
    '# Bash, curl, and Python 3 are required.',
    `: "\${${context.apiKeyEnv}:?Set ${context.apiKeyEnv} before running}"`,
    ': "${MINIMAX_IDEMPOTENCY_KEY:?Set a stable business-operation ID before running}"',
    `BASE_URL="${context.baseUrl}"`,
    `SUBMIT_URL="$BASE_URL${context.endpointPath}"`,
    'QUERY_BASE_URL="$BASE_URL/v2/query/video_generation"',
    'umask 077',
    'WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/minimax-video.XXXXXX") || { echo "Could not create a private temporary directory" >&2; exit 1; }',
    'HEADERS_FILE="$WORK_DIR/headers"',
    'BODY_FILE="$WORK_DIR/body.json"',
    'cleanup() {',
    '  rm -f -- "$HEADERS_FILE" "$BODY_FILE"',
    '  rmdir -- "$WORK_DIR" 2>/dev/null || true',
    '}',
    'trap cleanup EXIT',
    "trap 'exit 1' HUP INT TERM",
    '',
    'submit_deadline=$((SECONDS + 300))',
    'while true; do',
    '  http_code=$(curl --silent --show-error --request POST "$SUBMIT_URL" \\',
    `    -H "Authorization: Bearer $${context.apiKeyEnv}" \\`,
    '    -H "Content-Type: application/json" \\',
    '    -H "Idempotency-Key: $MINIMAX_IDEMPOTENCY_KEY" \\',
    '    --connect-timeout 30 \\',
    '    --max-time 120 \\',
    '    --dump-header "$HEADERS_FILE" \\',
    '    --output "$BODY_FILE" \\',
    "    --write-out '%{http_code}' \\",
    `    -d '${requestBody.replaceAll('\n', '\n       ')}')`,
    '  curl_exit=$?',
    '  if [ "$curl_exit" -ne 0 ]; then',
    '    if [ "$SECONDS" -ge "$submit_deadline" ]; then',
    '      echo "Video submission remained uncertain for 5 minutes; rerun with the same MINIMAX_IDEMPOTENCY_KEY" >&2',
    '      exit 1',
    '    fi',
    '    sleep 5',
    '    continue',
    '  fi',
    '  if [ "$http_code" = "429" ]; then',
    '    if [ "$SECONDS" -ge "$submit_deadline" ]; then',
    '      echo "Video submission was rate-limited for 5 minutes; rerun with the same MINIMAX_IDEMPOTENCY_KEY" >&2',
    '      exit 1',
    '    fi',
    `    retry_after=$(awk 'tolower($1) == "retry-after:" { print $2 }' "$HEADERS_FILE" | tr -d '\\r')`,
    '    sleep "${retry_after:-15}"',
    '    continue',
    '  fi',
    '  if [ "$http_code" = "200" ]; then',
    '    break',
    '  fi',
    '  echo "Video submission failed with HTTP $http_code" >&2',
    '  cat "$BODY_FILE" >&2',
    '  exit 1',
    'done',
    '',
    `initial_retry_after=$(awk 'tolower($1) == "retry-after:" { print $2 }' "$HEADERS_FILE" | tr -d '\\r')`,
    'if [ -n "$initial_retry_after" ]; then',
    '  sleep "$initial_retry_after"',
    'fi',
    `task_id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["task_id"])' < "$BODY_FILE")`,
    'deadline=$((SECONDS + 900))',
    '',
    'while [ "$SECONDS" -lt "$deadline" ]; do',
    '  query_timeout=$((deadline - SECONDS))',
    '  if [ "$query_timeout" -gt 60 ]; then',
    '    query_timeout=60',
    '  fi',
    '  http_code=$(curl --silent --show-error --request GET "$QUERY_BASE_URL/$task_id" \\',
    `    -H "Authorization: Bearer $${context.apiKeyEnv}" \\`,
    '    --connect-timeout 30 \\',
    '    --max-time "$query_timeout" \\',
    '    --dump-header "$HEADERS_FILE" \\',
    '    --output "$BODY_FILE" \\',
    "    --write-out '%{http_code}')",
    '  curl_exit=$?',
    '  if [ "$curl_exit" -ne 0 ]; then',
    '    if [ "$SECONDS" -ge "$deadline" ]; then',
    '      break',
    '    fi',
    '    sleep 5',
    '    continue',
    '  fi',
    '  if [ "$http_code" = "429" ]; then',
    `    retry_after=$(awk 'tolower($1) == "retry-after:" { print $2 }' "$HEADERS_FILE" | tr -d '\\r')`,
    '    sleep "${retry_after:-15}"',
    '    continue',
    '  fi',
    '  if [ "$http_code" != "200" ]; then',
    '    echo "Video query failed with HTTP $http_code" >&2',
    '    cat "$BODY_FILE" >&2',
    '    exit 1',
    '  fi',
    `  task_status=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["task"]["status"])' < "$BODY_FILE")`,
    '  case "$task_status" in',
    '    succeeded)',
    `      python3 -c 'import json,sys; print(json.load(sys.stdin)["task"]["content"]["url"])' < "$BODY_FILE"`,
    '      exit 0',
    '      ;;',
    '    failed|cancelled)',
    '      echo "Video task ended with status $task_status" >&2',
    '      cat "$BODY_FILE" >&2',
    '      exit 1',
    '      ;;',
    '    queued|running)',
    '      ;;',
    '    *)',
    '      echo "Unknown task status: $task_status" >&2',
    '      exit 1',
    '      ;;',
    '  esac',
    `  retry_after=$(awk 'tolower($1) == "retry-after:" { print $2 }' "$HEADERS_FILE" | tr -d '\\r')`,
    '  sleep "${retry_after:-15}"',
    'done',
    '',
    'echo "Video task did not finish within 15 minutes" >&2',
    'exit 1',
  ].join('\n')
}

function buildPythonSample(context: MiniMaxVideoSampleContext): string {
  const submitUrl = `${context.baseUrl}${context.endpointPath}`
  const queryBaseUrl = `${context.baseUrl}/v2/query/video_generation`

  return [
    'import os',
    'import time',
    '',
    'import requests',
    '',
    `api_key = os.environ.get("${context.apiKeyEnv}")`,
    'if not api_key:',
    `    raise RuntimeError("Set ${context.apiKeyEnv} before running")`,
    'idempotency_key = os.environ.get("MINIMAX_IDEMPOTENCY_KEY")',
    'if not idempotency_key or len(idempotency_key) > 256:',
    '    raise RuntimeError("Set MINIMAX_IDEMPOTENCY_KEY to a stable value of at most 256 characters")',
    '',
    'headers = {',
    '    "Authorization": f"Bearer {api_key}",',
    '    "Content-Type": "application/json",',
    '    "Idempotency-Key": idempotency_key,',
    '}',
    'payload = {',
    `    "model": "${context.modelName}",`,
    '    "content": [',
    '        {',
    '            "type": "text",',
    '            "text": "A cinematic tracking shot of a paper boat sailing through a neon city at night.",',
    '        }',
    '    ],',
    '    "resolution": "768P",',
    '    "duration": 6,',
    '    "ratio": "16:9",',
    '    "aigc_watermark": False,',
    '}',
    '',
    'submit_deadline = time.monotonic() + 300',
    'while True:',
    '    try:',
    `        response = requests.post("${submitUrl}", headers=headers, json=payload, timeout=120)`,
    '    except (requests.ConnectionError, requests.Timeout) as error:',
    '        if time.monotonic() >= submit_deadline:',
    '            raise TimeoutError("Video submission remained uncertain for 5 minutes; rerun with the same MINIMAX_IDEMPOTENCY_KEY") from error',
    '        time.sleep(5)',
    '        continue',
    '    if response.status_code == 429:',
    '        if time.monotonic() >= submit_deadline:',
    '            raise TimeoutError("Video submission was rate-limited for 5 minutes; rerun with the same MINIMAX_IDEMPOTENCY_KEY")',
    '        time.sleep(max(int(response.headers.get("Retry-After", "15")), 1))',
    '        continue',
    '    response.raise_for_status()',
    '    break',
    '',
    'task_id = response.json()["task_id"]',
    'initial_retry_after = max(int(response.headers.get("Retry-After", "0")), 0)',
    'if initial_retry_after:',
    '    time.sleep(initial_retry_after)',
    'deadline = time.monotonic() + 900',
    '',
    'while time.monotonic() < deadline:',
    `    response = requests.get(f"${queryBaseUrl}/{task_id}", headers={"Authorization": f"Bearer {api_key}"}, timeout=60)`,
    '    if response.status_code == 429:',
    '        time.sleep(max(int(response.headers.get("Retry-After", "15")), 1))',
    '        continue',
    '    response.raise_for_status()',
    '    task = response.json()["task"]',
    '    status = task["status"]',
    '    if status == "succeeded":',
    '        print(task["content"]["url"])',
    '        break',
    '    if status in {"failed", "cancelled"}:',
    '        raise RuntimeError(f"Video task ended with status {status}: {task.get(\'error\')}")',
    '    if status not in {"queued", "running"}:',
    '        raise RuntimeError(f"Unknown task status: {status}")',
    '    retry_after = int(response.headers.get("Retry-After", "15"))',
    '    time.sleep(retry_after)',
    'else:',
    '    raise TimeoutError("Video task did not finish within 15 minutes")',
  ].join('\n')
}

function buildNodeSample(
  language: 'typescript' | 'javascript',
  context: MiniMaxVideoSampleContext
): string {
  const isTypeScript = language === 'typescript'
  const submitUrl = `${context.baseUrl}${context.endpointPath}`
  const queryBaseUrl = `${context.baseUrl}/v2/query/video_generation`
  const createResponseLine = isTypeScript
    ? 'const { task_id: taskId } = (await response.json()) as { task_id: string }'
    : 'const { task_id: taskId } = await response.json()'
  const queryResponseLine = isTypeScript
    ? '  const { task } = (await response.json()) as { task: VideoTask }'
    : '  const { task } = await response.json()'
  const typeBlock = isTypeScript
    ? [
        'type VideoTask = {',
        "  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled'",
        '  content?: { url: string }',
        '  error?: { code: string; message: string }',
        '}',
        '',
      ]
    : []

  return [
    ...typeBlock,
    `const apiKey = process.env.${context.apiKeyEnv}`,
    `if (!apiKey) throw new Error('Set ${context.apiKeyEnv} before running')`,
    'const idempotencyKey = process.env.MINIMAX_IDEMPOTENCY_KEY',
    "if (!idempotencyKey || idempotencyKey.length > 256) throw new Error('Set MINIMAX_IDEMPOTENCY_KEY to a stable value of at most 256 characters')",
    '',
    isTypeScript
      ? 'const sleep = (milliseconds: number) =>\n  new Promise<void>((resolve) => setTimeout(resolve, milliseconds))'
      : 'const sleep = (milliseconds) =>\n  new Promise((resolve) => setTimeout(resolve, milliseconds))',
    '',
    'const payload = {',
    `    model: '${context.modelName}',`,
    '    content: [',
    '      {',
    "        type: 'text',",
    "        text: 'A cinematic tracking shot of a paper boat sailing through a neon city at night.',",
    '      },',
    '    ],',
    "    resolution: '768P',",
    '    duration: 6,',
    "    ratio: '16:9',",
    '    aigc_watermark: false,',
    '}',
    '',
    isTypeScript ? 'let response: Response' : 'let response',
    'const submitDeadline = Date.now() + 300_000',
    'while (true) {',
    '  try {',
    `    response = await fetch('${submitUrl}', {`,
    "      method: 'POST',",
    '      headers: {',
    '        Authorization: `Bearer ${apiKey}`,',
    "        'Content-Type': 'application/json',",
    "        'Idempotency-Key': idempotencyKey,",
    '      },',
    '      body: JSON.stringify(payload),',
    '      signal: AbortSignal.timeout(120_000),',
    '    })',
    '  } catch (error) {',
    '    if (Date.now() >= submitDeadline) {',
    "      throw new Error('Video submission remained uncertain for 5 minutes; rerun with the same MINIMAX_IDEMPOTENCY_KEY', { cause: error })",
    '    }',
    '    await sleep(5_000)',
    '    continue',
    '  }',
    '  if (response.status === 429) {',
    '    if (Date.now() >= submitDeadline) {',
    "      throw new Error('Video submission was rate-limited for 5 minutes; rerun with the same MINIMAX_IDEMPOTENCY_KEY')",
    '    }',
    "    const retryAfter = Math.max(Number(response.headers.get('Retry-After') ?? 15) || 15, 1)",
    '    await sleep(retryAfter * 1_000)',
    '    continue',
    '  }',
    '  if (!response.ok) {',
    '    throw new Error(`Video submission failed (${response.status}): ${await response.text()}`)',
    '  }',
    '  break',
    '}',
    createResponseLine,
    "const initialRetryAfter = Math.max(Number(response.headers.get('Retry-After') ?? 0) || 0, 0)",
    'if (initialRetryAfter > 0) await sleep(initialRetryAfter * 1_000)',
    'const deadline = Date.now() + 900_000',
    isTypeScript ? 'let videoUrl: string | undefined' : 'let videoUrl',
    '',
    'while (Date.now() < deadline) {',
    `  response = await fetch('${queryBaseUrl}/' + encodeURIComponent(taskId), {`,
    '    headers: { Authorization: `Bearer ${apiKey}` },',
    '    signal: AbortSignal.timeout(60_000),',
    '  })',
    '  if (response.status === 429) {',
    "    const retryAfter = Math.max(Number(response.headers.get('Retry-After') ?? 15) || 15, 1)",
    '    await sleep(retryAfter * 1_000)',
    '    continue',
    '  }',
    '  if (!response.ok) {',
    '    throw new Error(`Video query failed (${response.status}): ${await response.text()}`)',
    '  }',
    queryResponseLine,
    "  if (task.status === 'succeeded') {",
    "    if (!task.content?.url) throw new Error('Succeeded task has no content URL')",
    '    videoUrl = task.content.url',
    '    break',
    '  }',
    "  if (task.status === 'failed' || task.status === 'cancelled') {",
    "    throw new Error(`Video task ended with status ${task.status}: ${task.error?.message ?? 'no error details'}`)",
    '  }',
    "  if (task.status !== 'queued' && task.status !== 'running') {",
    '    throw new Error(`Unknown task status: ${task.status}`)',
    '  }',
    "  const retryAfter = Number(response.headers.get('Retry-After') ?? 15)",
    '  await sleep(retryAfter * 1_000)',
    '}',
    '',
    "if (!videoUrl) throw new Error('Video task did not finish within 15 minutes')",
    'console.log(videoUrl)',
  ].join('\n')
}

export function buildMiniMaxVideoSample(
  language: ImageSampleLanguage,
  context: MiniMaxVideoSampleContext
): string {
  if (language === 'curl' || language === 'bash') {
    return buildCurlSample(context)
  }
  if (language === 'python') {
    return buildPythonSample(context)
  }
  return buildNodeSample(language, context)
}

export function buildMiniMaxVideoAiIntegrationGuide(
  context: MiniMaxVideoSampleContext
): string {
  return [
    '# Opwan MiniMax-H3 Video Generation V2 integration guide',
    '',
    'Use this document as the source of truth for the implemented MiniMax V2-compatible subset.',
    '',
    '## Endpoints and authentication',
    `- Submit: POST ${context.baseUrl}${context.endpointPath}`,
    `- Query: GET ${context.baseUrl}${MINIMAX_VIDEO_V2_QUERY_PATH}`,
    `- Authentication: Authorization: Bearer $${context.apiKeyEnv}`,
    '- Submit JSON with Content-Type: application/json.',
    '',
    '## Request contract',
    `- model must be exactly ${context.modelName}.`,
    '- content contains 1 to 16 items and must include at least one non-empty text item. Each text item is limited to 7000 Unicode characters.',
    '- resolution=768P is required. duration is an integer from 4 to 15, inclusive.',
    '- ratio must be explicit: 16:9 and 9:16 work for every supported mode; 1:1 works only without reference audio.',
    '- Use role=reference_image for at most 9 reference_image items.',
    '- Use role=reference_audio for at most 3 reference_audio items, and include at least one reference image. The combined prompt is then limited to 10000 characters.',
    '- Media accepts a publicly reachable HTTPS URL on the default port or an allowed base64 data URI.',
    '- callback_url is optional. When present, it must be a publicly reachable HTTPS URL of at most 2048 characters.',
    '',
    '## Unsupported options',
    '- 2K, adaptive, 21:9, 4:3, and 3:4 are unsupported.',
    '- first_frame, last_frame, reference_video, and aigc_watermark=true are unsupported.',
    '',
    '## Optional terminal callback',
    '- During submission, the gateway sends callback_url a POST with {"challenge":"..."}. The receiver must return a 2xx JSON response containing the identical challenge value within 3 seconds. The task is accepted only after verification succeeds.',
    '- After verification, processing continues in the background. This compatibility layer sends callbacks only for terminal succeeded, failed, or cancelled tasks, not for every queued or running transition.',
    '- The terminal callback body is exactly the same {"task":{...}} response returned by the query endpoint.',
    '- A terminal delivery is acknowledged by any 2xx response. Redirects, non-2xx responses, network errors, and timeouts are retried up to five total attempts, with 30, 60, 120, and 240 second delays after the initial attempt.',
    '- Terminal deliveries include X-Webhook-Delivery-Id and X-Webhook-Timestamp. Delivery is at least once, so deduplicate retries by the stable delivery ID.',
    '- No callback signature is sent. Protect the endpoint with an unguessable URL and confirm sensitive state through the authenticated query endpoint.',
    '- Callback delivery failure does not change task status or billing. The authenticated query endpoint remains the fallback for seven days.',
    '',
    '## Async task lifecycle',
    '- A successful submit returns HTTP 200 with {"task_id":"task_..."}. It is not an HTTP 202 contract.',
    '- Processing continues in the background. Clients may poll the query endpoint with the same user token until queued or running becomes succeeded, failed, or cancelled.',
    '- On success, task.content.url is an authenticated gateway URL. Download it with the same user token and store the result promptly.',
    '- Tasks can be queried for seven days after submission.',
    '- Respect Retry-After when present; otherwise wait 15 seconds. Apply a client-side deadline.',
    '',
    '## Idempotency and billing',
    '- Send a unique Idempotency-Key of at most 256 characters. Reuse the same key and identical request when retrying an uncertain submission.',
    '- Persist the key before the first POST. The examples read MINIMAX_IDEMPOTENCY_KEY so restarting the client does not create a new paid operation.',
    '- The same key and canonical request replay the original task without another charge; a different request returns HTTP 409.',
    '- If a submit response includes task_id and Retry-After, query that task instead of submitting with a new key.',
    '- Billing uses the requested duration in seconds. Read the current per-second price and group multiplier from the model plaza.',
    '',
    '## Quick start',
    '- This polling example deliberately omits callback_url. Add it only after deploying a receiver that passes the challenge handshake.',
    '```bash',
    buildCurlSample(context),
    '```',
  ].join('\n')
}
