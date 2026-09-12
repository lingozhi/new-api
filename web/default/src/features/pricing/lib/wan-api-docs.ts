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
import { videoWebhookGuide } from './video-webhook-docs'

export function isWanModel(model: string): boolean {
  return model === 'wan3.0'
}

export const WAN_PARAMETERS = [
  { name: 'webhook_url', type: 'string', value: 'HTTPS :443; ≤2048 bytes' },
  { name: 'webhook_secret', type: 'string', value: '≤512 bytes' },
  { name: 'model', type: 'string *', value: 'wan3.0' },
  { name: 'prompt', type: 'string *', value: 'string (length > 0)' },
  {
    name: 'mode',
    type: 'string',
    value: 'auto | general | reference | frames; default: auto',
  },
  { name: 'speed', type: 'string', value: 'standard; default: standard' },
  { name: 'seconds', type: 'integer string', value: '2–30; default: 5' },
  { name: 'duration', type: 'integer', value: '= seconds; 2–30' },
  { name: 'size', type: 'string', value: '480P | 720P | 1080P; default: 720P' },
  { name: 'resolution', type: 'string', value: '= size' },
  {
    name: 'aspect_ratio',
    type: 'string',
    value: '16:9 | 9:16 | 1:1; default: 16:9',
  },
  { name: 'ratio', type: 'string', value: '= aspect_ratio' },
  { name: 'n', type: 'integer', value: '1' },
  { name: 'prompt_extend', type: 'boolean', value: 'true | false' },
  {
    name: 'first_frame',
    type: 'string',
    value: 'https://example.com/first.jpg',
  },
  { name: 'last_frame', type: 'string', value: 'https://example.com/last.jpg' },
  { name: 'reference_images', type: 'object[]', value: '≤10; {url, role?}' },
  {
    name: 'reference_images[].url',
    type: 'string *',
    value: 'https://example.com/image.jpg',
  },
  {
    name: 'reference_images[].role',
    type: 'string',
    value: 'reference_image | first_frame | last_frame',
  },
  { name: 'reference_videos', type: 'object[]', value: '≤5; {url}' },
  {
    name: 'reference_videos[].url',
    type: 'string *',
    value: 'https://example.com/reference.mp4',
  },
  { name: 'reference_audios', type: 'object[]', value: '≤5; {url}' },
  {
    name: 'reference_audios[].url',
    type: 'string *',
    value: 'https://example.com/reference.mp3',
  },
]

export function wanRequest(): Record<string, unknown> {
  return {
    model: 'wan3.0',
    prompt: 'A blue ball on a white table, fixed camera.',
    mode: 'auto',
    speed: 'standard',
    seconds: '2',
    size: '480P',
    aspect_ratio: '16:9',
    prompt_extend: false,
  }
}

export const WAN_EXAMPLES = [
  wanRequest(),
  {
    ...wanRequest(),
    mode: 'general',
    reference_images: [{ url: 'https://example.com/reference.jpg' }],
  },
  {
    ...wanRequest(),
    mode: 'reference',
    reference_images: [{ url: 'https://example.com/reference.jpg' }],
    reference_videos: [{ url: 'https://example.com/reference.mp4' }],
    reference_audios: [{ url: 'https://example.com/reference.mp3' }],
  },
  {
    ...wanRequest(),
    mode: 'frames',
    first_frame: 'https://example.com/first.jpg',
    last_frame: 'https://example.com/last.jpg',
  },
]

export function wanPythonExample(): string {
  return `import json, os, time
from pathlib import Path
import requests

base = os.environ["NEW_API_BASE_URL"].rstrip("/")
headers = {"Authorization": "Bearer " + os.environ["NEW_API_KEY"]}
body = json.loads(${JSON.stringify(JSON.stringify(wanRequest()))})
r = requests.post(base + "/v1/videos", headers=headers, json=body, timeout=120)
r.raise_for_status()
job = r.json()
task_id = job["id"]
Path("wan-task.json").write_text(json.dumps(job))
print("Saved task:", task_id)
for _ in range(120):
    r = requests.get(base + "/v1/videos/" + task_id, headers=headers, timeout=60)
    r.raise_for_status()
    job = r.json()
    state = job["status"]
    if state == "completed":
        with requests.get(base + "/v1/videos/" + task_id + "/content",
                          headers=headers, stream=True, timeout=300) as media:
            media.raise_for_status()
            with open("wan.mp4", "wb") as output:
                for chunk in media.iter_content(1024 * 1024):
                    output.write(chunk)
        break
    if state == "failed":
        raise RuntimeError(job)
    if state not in ("queued", "in_progress"):
        raise RuntimeError(job)
    time.sleep(15)
else:
    raise TimeoutError("Query the saved ID later; do not create a duplicate task.")`
}

export function buildWanAiIntegrationGuide(origin: string): string {
  const base = origin.replace(/\/$/, '')
  return [
    '# Unified Wan 3.0 website API',
    `API origin: ${base} (without /v1); public model is always wan3.0.`,
    'Use a website key from the official group in NEW_API_KEY. Never put real keys in frontend code or logs.',
    'Authorization: Bearer <NEW_API_KEY>; Content-Type: application/json.',
    `POST ${base}/v1/videos`,
    `GET ${base}/v1/videos/{id}`,
    `GET ${base}/v1/videos/{id}/content`,
    '',
    '## Current official channel: modes and speed',
    '| mode | speed | Required input |',
    '| --- | --- | --- |',
    '| general | standard | prompt; reference lists optional |',
    '| reference | standard | prompt + at least one reference image, video or audio |',
    '| frames | standard | prompt + first_frame + last_frame |',
    '- mode defaults to auto: any first_frame/last_frame selects frames, otherwise general. Reference lists alone do not select reference mode.',
    '- Every mode defaults to standard speed. Omit speed or use standard; fast is unavailable and returns HTTP 400. Keep model=wan3.0 for every mode.',
    '',
    '## Complete field inventory (* required; nested * applies when item exists)',
    '| Field | Type | Values |',
    '| --- | --- | --- |',
    ...WAN_PARAMETERS.map(
      (p) => `| ${p.name} | ${p.type} | ${p.value.replaceAll('|', '/')} |`
    ),
    '',
    '## Parameter rules',
    '- model=wan3.0 and non-empty prompt are required even with media. Omit unused optional fields; null and unknown top-level fields are rejected.',
    '- seconds is an integer string such as "2"; duration is an integer alias. Generated duration 2–30, default 5. No fractions, negative values or -1. Aliases must agree.',
    '- size/resolution: 480P,720P,1080P case-insensitive, default 720P; aliases must agree. 4K/pixel dimensions are rejected.',
    '- aspect_ratio/ratio: 16:9,9:16,1:1 only, default 16:9; aliases must agree.',
    '- n is optional and only 1 is valid. prompt_extend is optional boolean; false disables provider prompt enhancement. Omit it to use the provider default; false is preserved.',
    '- Frames requires both first_frame and last_frame HTTPS URLs, with no non-empty reference lists. general/reference must not contain these top-level frame fields. Do not send media: the server builds it.',
    '- general/reference use the same reference_images (max 10), reference_videos (max 5), reference_audios (max 5) object arrays. Each entry requires a public HTTPS url without embedded credentials.',
    '- In general mode, reference_images[].role optionally supports reference_image,first_frame,last_frame. In reference mode only reference_image or omitted is valid; never convert frame roles silently.',
    '- reference_videos[].duration is unsupported in every mode and returns HTTP 400. Use top-level duration or seconds only for generated output length.',
    '- Nested media fields are limited to url and optional image role. Unknown nested fields are rejected. URLs must remain accessible until generation completes; no private-only URLs, local paths, base64 data URLs or multipart uploads. Provider checks actual file availability, formats and media limits; gateway count/duration/resolution bounds are not a guarantee that every media combination will generate.',
    '- stream, response_format, callback_url, seed, negative_prompt, file_id, input_reference, image, image_end, end_image_url and media are not unified API fields. Do not import fields from another provider or legacy workflow.',
    '',
    videoWebhookGuide(),
    '## Responses and recovery',
    '- Creation HTTP 200: id,task_id,request_id use the same public task ID; model remains wan3.0. Save id immediately. Creation status is provider-supplied and may be pending; use GET to read normalized state.',
    '- GET query states: queued,in_progress,completed,failed. Only id/task_id/request_id, model and status are normalized. Other fields are optional provider metadata: progress may be absent or below 100 even when completed, and completed_at may be an RFC3339 string. Do not require integer timestamps or video.url. Handle failed even without an error object. The webhook body has its own documented terminal schema.',
    '- Poll every 10–15 seconds with per-request timeouts and a finite deadline. On query 429/5xx/network errors respect Retry-After/backoff and resume the SAME ID.',
    '- Never automatically retry a timed-out POST: it may already be accepted and repeating it can charge twice. If no ID was received, inspect website task logs.',
    '- Download before completion returns 409. Range/If-Range allow resuming a saved partial file (206); an unsatisfiable range returns 416. A changed If-Range may return the entire file (200). curl --continue-at - can resume downloads.',
    '- Download promptly. The current channel has no verified result-retention period; do not assume 48 hours or permanent storage.',
    '- On completed, download /content with the same token and save the MP4; follow ordinary redirects without forwarding credentials to a different host. Do not assume video.url exists.',
    '- HTTP 400 invalid_request/invalid_duration/invalid_resolution/invalid_n/invalid_media: fix input. Creation validation errors use top-level code/message/data; authentication and content errors may use error.message instead. Check HTTP status first. Auth/group/quota errors: check key, official group and balance. Provider generation failure is not permission to auto-create another paid task.',
    '- invalid_webhook / invalid_webhook_url: correct the callback URL, field types or secret before creating a task.',
    '## Billing',
    '- The website currently charges requested output seconds at the effective USD-per-second price and group multiplier; successful tasks keep that amount and failed tasks refund it. Input reference-video duration is not added to the website charge and actual output duration is not used to resettle Wan. Check current website pricing; do not infer website charges from upstream pricing.',
    '## Four alternative request examples',
    ...WAN_EXAMPLES.flatMap((request) => [
      '```json',
      JSON.stringify(request, null, 2),
      '```',
    ]),
    '## Python: create once, save ID, poll and download',
    `Requires requests; set NEW_API_BASE_URL=${base} and NEW_API_KEY=<website key>. Running the example creates one paid task.`,
    '```python',
    wanPythonExample(),
    '```',
    '## Resume a saved task (do not rerun creation)',
    '```bash',
    `export NEW_API_BASE_URL='${base}'`,
    'export NEW_API_KEY="<NEW_API_KEY>"',
    'TASK_ID="task_example"',
    'curl --fail-with-body "$NEW_API_BASE_URL/v1/videos/$TASK_ID" -H "Authorization: Bearer $NEW_API_KEY"',
    '# Only after completed:',
    'curl --fail-with-body --location "$NEW_API_BASE_URL/v1/videos/$TASK_ID/content" -H "Authorization: Bearer $NEW_API_KEY" --output wan.mp4',
    '```',
    'The previous official Wan channel is disabled. Its fast/reference/frame model IDs are not active compatibility routes on the current channel. Use wan3.0 and the fields above for new integrations.',
  ].join('\n')
}
