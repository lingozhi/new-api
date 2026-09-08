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

export function lxmoneSeedanceRequest(model: string) {
  return {
    model,
    prompt: 'A blue ball on a white table, fixed camera.',
    seconds: model === 'seedance-2-fast' || model === 'seedance-2-mini' ? 5 : 4,
    resolution: '480p',
    aspect_ratio: '16:9',
  }
}

export function lxmoneSeedancePythonExample(model: string): string {
  return `import json, os, time
from pathlib import Path
import requests

base = os.environ["NEW_API_BASE_URL"].rstrip("/")
headers = {"Authorization": "Bearer " + os.environ["NEW_API_KEY"]}
payload = json.loads(${JSON.stringify(JSON.stringify(lxmoneSeedanceRequest(model)))})
response = requests.post(base + "/v1/videos", headers=headers, json=payload, timeout=120)
response.raise_for_status()
job = response.json()
task_id = job["id"]
Path("video-task.json").write_text(json.dumps(job))
print("Saved task:", task_id)
for _ in range(120):
    response = requests.get(base + "/v1/videos/" + task_id, headers=headers, timeout=60)
    response.raise_for_status()
    job = response.json()
    if job["status"] == "completed":
        with requests.get(base + "/v1/videos/" + task_id + "/content",
                          headers=headers, stream=True, timeout=300) as media:
            media.raise_for_status()
            with open("video.mp4", "wb") as output:
                for chunk in media.iter_content(1024 * 1024):
                    output.write(chunk)
        break
    if job["status"] in ("failed", "cancelled", "expired"):
        raise RuntimeError(job)
    if job["status"] not in ("queued", "in_progress"):
        raise RuntimeError(job)
    time.sleep(15)
else:
    raise TimeoutError("Query the saved task ID later; do not submit again.")`
}

export function buildLxmoneSeedanceAiIntegrationGuide(
  model: string,
  baseUrl: string
): string {
  const base = baseUrl.replace(/\/$/, '')
  const request = lxmoneSeedanceRequest(model)
  return [
    '# Seedance website API integration guide',
    '',
    'Implement against this website contract. Do not substitute another provider SDK or the legacy Seedance channel protocol. Do not invent undocumented optional fields.',
    `Selected public model: ${model}`,
    `API origin (without /v1): ${base}`,
    '',
    '## Authentication and endpoints',
    '- Use a website API key in the official group. Keep it server-side in NEW_API_KEY; never put a real key in frontend code, logs, or this guide.',
    '- Authorization: Bearer <NEW_API_KEY>',
    '- Content-Type: application/json for creation.',
    `- Create: POST ${base}/v1/videos`,
    `- Query: GET ${base}/v1/videos/{id}`,
    `- Download completed MP4: GET ${base}/v1/videos/{id}/content`,
    '- Use the same website token for creation, polling and download. The base origin has no /v1 suffix; the endpoint paths already contain /v1.',
    '- Use JSON requests and public media URLs. The old /v1/media/uploads and /v1/videos/generations paths belong to the legacy channel and are not supported by this integration. The legacy interface rejects webhook_url/webhook_secret with HTTP 400 unsupported_webhook.',
    '',
    '## Models and limits',
    '| Public model | Internal upstream mapping (do not use in client requests) | Seconds | Effective resolutions |',
    '| --- | --- | --- | --- |',
    '| seedance-2 | seedance-2-pro | 4–15 integers | 480p, 720p, 1080p, 4k |',
    '| seedance-2.5 | seedance-2.5-pro | 4–30 integers | 480p, 720p, 1080p |',
    '| seedance-2-fast | seedance-2-fast | 5 or 10 only | 480p, 720p |',
    '| seedance-2-mini | seedance-2-mini | 5 or 10 only | 480p, 720p |',
    '- Defaults for all four: 5 seconds, 720p, 16:9. Examples below explicitly choose the lowest-cost duration and 480p.',
    '- Fast/Mini accept 1080p but normalize it to 720p; 4k is rejected. Seedance 2.5 accepts 4k but normalizes it to 1080p. Output and billing use the normalized resolution.',
    '',
    '## Seedance 2.5 fixed-spec exception',
    '- Public seedance-2.5 at effective 720p and exactly 30 seconds ALWAYS selects sd-2.5 internally, including size/seconds aliases or omitted resolution (default 720p). Keep the public model name; do not send sd-2.5 yourself. Other resolutions and durations use the original upstream model.',
    '- This combination accepts prompt and at most 10 reference_images (URL strings or {url} objects). The gateway converts these to the upstream images array. Frame fields, reference_videos, reference_audios, sound_effects, no_music and other unsupported options are rejected with HTTP 400 unsupported_sd25_input. There is no silent fallback or discarded media.',
    '- Send only model, prompt, seconds/duration, resolution/size, aspect_ratio/ratio, reference_images and optional webhook_url/webhook_secret for this combination. The following general Seedance media rules apply to other specifications.',
    '- SD-2.5 generates a fixed 30-second 720p result. The website keeps its current 720p-per-second rate for 30 seconds and normal failure refunds; provider duration metadata does not change this fixed charge. Public task IDs/model names, polling, downloads and webhooks remain unchanged.',
    '',
    '## Complete supported request field inventory',
    'Omit unused optional fields; do not send null, empty media placeholders or every optional field at once. Use one spelling per alias pair. The webhook section defines the optional empty-secret behavior.',
    '| Field | Type | Required/default | Contract |',
    '| --- | --- | --- | --- |',
    `| model | string | required | Use ${model}; public aliases are listed above. |`,
    '| prompt | string | required | Non-empty text, including when media is supplied. Describe the requested video. |',
    '| seconds | integer or integer string | optional; 5 | Generated duration in seconds; use the selected model limits above. No fractional values, zero or -1. |',
    '| duration | integer | optional | Alias of seconds. If both are present they must be equal. |',
    '| resolution | string | optional; 720p | 480p, 720p, 1080p, 4k subject to model limits/normalization; case-insensitive. Pixel dimensions are not accepted. |',
    '| size | string | optional | Alias of resolution. If both are present they must agree before normalization, ignoring case. |',
    '| aspect_ratio | string | optional; 16:9 | Exactly 16:9, 9:16 or 1:1. adaptive, 4:3, 3:4 and 21:9 are unsupported. |',
    '| ratio | string | optional | Alias of aspect_ratio. If both are present they must agree. |',
    '| input_reference | URL string or {"url": string} | optional | First-frame image URL. Cannot be combined with reference lists. |',
    '| image | URL string or {"url": string} | optional | Alternative first-frame field; use instead of input_reference. |',
    '| image_end | URL string or {"url": string} | optional | Last-frame image URL; requires a first frame. |',
    '| end_image_url | URL string or {"url": string} | optional | Alternative last-frame field; use instead of image_end. |',
    '| reference_images | array of URL strings (also accepts {"url": string}) | optional; omitted | Reference image URLs, in order. |',
    '| reference_videos | array of URL strings (also accepts {"url": string}) | optional; omitted | Reference video URLs. |',
    '| reference_audios | array of URL strings (also accepts {"url": string}) | optional; omitted | Reference audio URLs. See audio dependency below. |',
    '| webhook_url | string | optional | Public HTTPS :443; maximum 2048 bytes. |',
    '| webhook_secret | string | optional | Maximum 512 bytes; requires webhook_url. |',
    '| sound_effects | boolean | optional; provider default | false disables generated sound effects; explicit false is preserved. |',
    '| no_music | boolean | optional; provider default | true disables generated sound effects. Prefer one option; if both are present they must be opposite. |',
    '',
    '## Media combinations and provider validation',
    '- Text-only: model + prompt and optional duration/resolution/aspect ratio.',
    '- First frame: add input_reference OR image. A last frame is optional.',
    '- First and last frames: add image_end OR end_image_url together with a first frame. A last frame alone is invalid.',
    '- Multimodal reference: use reference_images/reference_videos/reference_audios in any supported combination, without any first/last-frame fields.',
    '- Seedance 2, Fast and Mini: reference_audios requires at least one reference image or video. Seedance 2.5 allows audio-only references, but prompt remains required.',
    '- Gateway-enforced maximum counts: Seedance 2/Fast/Mini 9 images, 3 videos, 3 audios; Seedance 2.5 30 images, 10 videos, 10 audios.',
    '- Media URLs must use HTTP or HTTPS without embedded credentials and be accessible to the provider, not localhost/private-only files. Replace example.com placeholders with real assets. The gateway validates counts and URL syntax, including matching frame aliases. The provider validates public reachability, actual formats and media duration/size. Reference and frame object inputs must contain only url and are normalized to URL strings. When both frame aliases are supplied, their normalized URLs must match.',
    '- Reference prompts may use @Image1, @Video1 and @Audio1 in array order. Seedance 2/Fast/Mini reference videos are typically 2–15 seconds and ≤50 MB; audio typically 2–15 seconds and ≤15 MB. Seedance 2.5 reference videos are typically 2–30 seconds and ≤200 MB. These are provider guidance, not locally measured limits.',
    '- These media example shapes are documented integration inputs, not a claim that every media combination has passed a live generation test.',
    '',
    '## Rejected and undocumented options',
    '- The gateway rejects the presence of stream, n and response_format, even stream=false or n=1. Omit these keys entirely.',
    '- generate_audio, seed, negative_prompt, file_id, callback_url, start_image and reference role/type extensions are not a supported documented contract for this channel. Do not copy optional fields from legacy Argolink or other video APIs.',
    '- Streaming, batching, remix, cancellation and uploads are not documented here. Authenticated polling and content download remain available alongside webhooks.',
    '',
    videoWebhookGuide(),
    '## Responses, polling and recovery',
    '- Creation returns HTTP 200 with id, task_id and request_id identifying the same public task. Save id immediately; do not use upstream task identifiers.',
    '- Query response status is queued or in_progress while pending, completed on success, or failed with an error. progress and error details depend on the provider response and may be missing; status is authoritative, not a guessed completion time. Handle status=failed even without an error object, and never require a specific error.code.',
    '- Poll every 15 seconds with a finite deadline and per-request timeout. Respect Retry-After when present. Transient query/network errors do not justify a new creation request; retry querying the saved ID with backoff.',
    '- Download returns 409 before completion. Range/If-Range are forwarded for these tasks; a satisfiable range returns 206, an unsatisfiable range 416. If If-Range no longer matches, the provider may return the full file (200). With curl use --continue-at - to resume a saved partial file.',
    '- The provider advertises a default 48-hour result retention period, configurable by its administrator. This website does not promise permanent storage; download promptly after completion.',
    '- Download /content only after completed, with the same token. Follow normal download redirects without forwarding the token to a different host. Store the MP4 locally or in your application storage.',
    '- Never automatically repeat POST after a timeout, connection loss or 5xx: acceptance may be uncertain and a duplicate may incur another charge. Preserve any returned task ID and inspect website task logs if no ID was received.',
    '- Example creation response:',
    '```json',
    JSON.stringify(
      {
        id: 'task_example',
        task_id: 'task_example',
        request_id: 'task_example',
        object: 'video.generation',
        model,
        status: 'queued',
        progress: 0,
      },
      null,
      2
    ),
    '```',
    '- A completed query uses the same public id/model with status="completed"; do not require progress=100 to recognize completion. Use the authenticated content endpoint rather than assuming a video.url field exists.',
    '- Example failure fields (other task fields may also be present):',
    '```json',
    JSON.stringify(
      {
        id: 'task_example',
        model,
        status: 'failed',
        error: {
          message: 'Video generation failed',
        },
      },
      null,
      2
    ),
    '```',
    '',
    '## Errors and billing',
    '- invalid_request / invalid_duration / invalid_resolution / invalid_media: fix the request before submitting again.',
    '- model_price_error: administrator must configure the selected channel price. Do not switch silently to a legacy provider.',
    '- Authentication, group or quota errors: check the website key, official group access and remaining balance. HTTP 429: respect Retry-After/backoff.',
    '- status=failed: inspect available task error details; do not require a fixed error code or run automatic paid retries.',
    '- invalid_webhook / invalid_webhook_url: correct callback options; unsupported_webhook: use the documented new interface or omit callback fields on the legacy interface.',
    '- Prices are USD per second of generated video at the effective resolution, with the applicable user-group multiplier. Read current prices from the model page; do not hardcode an old provider multiplier.',
    '- Creation reserves the requested-duration amount. Delivered duration is used for settlement when returned; otherwise the original amount remains. Failed tasks refund the website reservation.',
    '',
    '## Minimal text-to-video request',
    '```json',
    JSON.stringify(request, null, 2),
    '```',
    '## Alternative: first and last frames',
    '```json',
    JSON.stringify(
      {
        ...request,
        input_reference: 'https://example.com/first.jpg',
        image_end: 'https://example.com/last.jpg',
      },
      null,
      2
    ),
    '```',
    '## Alternative: image, video and audio references',
    '```json',
    JSON.stringify(
      {
        ...request,
        reference_images: ['https://example.com/reference.jpg'],
        reference_videos: ['https://example.com/reference.mp4'],
        reference_audios: ['https://example.com/reference.mp3'],
      },
      null,
      2
    ),
    '```',
    '## Python end-to-end example',
    '- Requires Python 3 and requests. Set NEW_API_BASE_URL to the origin above and NEW_API_KEY to a website key. Running this example creates one paid task.',
    '- The example stops on HTTP errors and saves video-task.json after acceptance. On a polling error or deadline, resume querying that ID; do not rerun the whole script to resume.',
    '```python',
    lxmoneSeedancePythonExample(model),
    '```',
    '## Resume an existing task without creating another task',
    '```bash',
    `export NEW_API_BASE_URL='${base}'`,
    'export NEW_API_KEY="<NEW_API_KEY>"',
    'TASK_ID="task_example"',
    'curl --fail-with-body --max-time 60 "$NEW_API_BASE_URL/v1/videos/$TASK_ID" -H "Authorization: Bearer $NEW_API_KEY"',
    '# Only after status=completed:',
    'curl --fail-with-body --location --max-time 300 "$NEW_API_BASE_URL/v1/videos/$TASK_ID/content" -H "Authorization: Bearer $NEW_API_KEY" --output video.mp4',
    '```',
  ].join('\n')
}
