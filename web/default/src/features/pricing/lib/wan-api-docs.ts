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
  return (
    model === 'wan3.0' ||
    model === 'wan3.0-video' ||
    model === 'wan3.0-video-prime'
  )
}

export const WAN_PARAMETERS = [
  {
    name: 'model',
    type: 'string *',
    value: 'wan3.0-video | wan3.0-video-prime | wan3.0',
  },
  { name: 'input', type: 'object *', value: '{prompt?, media?}' },
  {
    name: 'input.prompt',
    type: 'string',
    value: '≤20000; prompt | media',
  },
  { name: 'input.media', type: 'object[]', value: '≤20; {type, url}' },
  {
    name: 'input.media[].type',
    type: 'string *',
    value:
      'first_frame | last_frame | reference_image | reference_video | reference_audio | file | link',
  },
  {
    name: 'input.media[].url',
    type: 'string *',
    value: 'HTTP(S); data:image/...;base64,...',
  },
  { name: 'parameters', type: 'object', value: '{}' },
  {
    name: 'parameters.resolution',
    type: 'string',
    value: '480P | 720P | 1080P; default: 1080P',
  },
  {
    name: 'parameters.ratio',
    type: 'string',
    value: 'adaptive | 16:9 | 4:3 | 1:1 | 3:4 | 9:16; default: adaptive',
  },
  {
    name: 'parameters.duration',
    type: 'integer',
    value: '-1 | 2–30; default: 5',
  },
  {
    name: 'parameters.audio',
    type: 'boolean',
    value: 'true | false; default: true',
  },
  {
    name: 'parameters.seed',
    type: 'integer',
    value: '-1 | 0–2147483647; default: -1',
  },
  {
    name: 'parameters.prompt_extend',
    type: 'boolean',
    value: 'true | false; default: true',
  },
  {
    name: 'parameters.watermark',
    type: 'boolean',
    value: 'true | false; default: false',
  },
  { name: 'webhook_url', type: 'string', value: 'HTTPS :443; ≤2048 bytes' },
  { name: 'webhook_secret', type: 'string', value: '≤512 bytes' },
]

export function wanRequest(modelName = 'wan3.0-video') {
  return {
    model: modelName,
    input: { prompt: 'A blue ball on a white table, fixed camera.' },
    parameters: {
      resolution: '480P',
      duration: 2,
      ratio: '16:9',
      audio: false,
      seed: 0,
      prompt_extend: false,
      watermark: false,
    },
  }
}

export const WAN_EXAMPLES = [
  wanRequest(),
  {
    ...wanRequest(),
    input: {
      prompt: 'Animate this first frame with a slow camera move.',
      media: [{ type: 'first_frame', url: 'https://example.com/first.jpg' }],
    },
  },
  {
    ...wanRequest(),
    input: {
      prompt: 'A smooth transition from the first frame to the last frame.',
      media: [
        { type: 'first_frame', url: 'https://example.com/first.jpg' },
        { type: 'last_frame', url: 'https://example.com/last.jpg' },
      ],
    },
  },
  {
    ...wanRequest(),
    input: {
      prompt:
        'Use the subject in 图1, the movement in 视频1 and the sound in 音频1.',
      media: [
        { type: 'reference_image', url: 'https://example.com/reference.jpg' },
        { type: 'reference_video', url: 'https://example.com/reference.mp4' },
        { type: 'reference_audio', url: 'https://example.com/reference.mp3' },
      ],
    },
    parameters: { ...wanRequest().parameters, audio: true },
  },
  {
    ...wanRequest(),
    input: {
      prompt: 'Create a short product introduction from this document.',
      media: [{ type: 'file', url: 'https://example.com/product.pdf' }],
    },
  },
  {
    ...wanRequest(),
    input: {
      prompt: 'Summarize this public article as a short video.',
      media: [{ type: 'link', url: 'https://example.com/article' }],
    },
  },
  {
    ...wanRequest(),
    input: {
      prompt: 'Edit 视频1: replace the background with a sunny beach.',
      media: [
        { type: 'reference_video', url: 'https://example.com/reference.mp4' },
      ],
    },
    parameters: { ...wanRequest().parameters, ratio: 'adaptive' },
  },
  {
    ...wanRequest(),
    input: {
      prompt:
        'Extend 视频1 forward: the camera continues moving toward the subject.',
      media: [
        { type: 'reference_video', url: 'https://example.com/reference.mp4' },
      ],
    },
    parameters: { ...wanRequest().parameters, ratio: 'adaptive' },
  },
]

export function wanExamples(modelName = 'wan3.0-video') {
  return WAN_EXAMPLES.map((request) => ({ ...request, model: modelName }))
}

export function wanPythonExample(modelName = 'wan3.0-video'): string {
  return `import json, os, time
from pathlib import Path
import requests

base = os.environ["NEW_API_BASE_URL"].rstrip("/")
headers = {"Authorization": "Bearer " + os.environ["NEW_API_KEY"]}
body = json.loads(${JSON.stringify(JSON.stringify(wanRequest(modelName)))})
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

export function buildWanAiIntegrationGuide(
  origin: string,
  modelName = 'wan3.0-video'
): string {
  const base = origin.replace(/\/$/, '')
  return [
    '# Wan 3.0 website API',
    'Request fields follow the official Wan 3.0 input / parameters / media structure. Aijiau remains the upstream channel.',
    'Sources: [official API reference](https://docs.bailian.console.aliyun.com/zh/model-studio/wan3-video-generation-api-reference) and [generation guide](https://docs.bailian.console.aliyun.com/zh/model-studio/wan3-video-generation-guide).',
    '',
    '## Website endpoints and authentication',
    `API origin: ${base} (without /v1). Use a website API key in the official group (官方渠道).`,
    'Authorization: Bearer <NEW_API_KEY>; Content-Type: application/json.',
    `POST ${base}/v1/videos`,
    `GET ${base}/v1/videos/{id}`,
    `GET ${base}/v1/videos/{id}/content`,
    'The website uses its own endpoints, key, public task IDs and response schema. The DashScope endpoint, SDK response format and X-DashScope-Async header are not required here. Do not send a provider key from clients.',
    `Examples on this page use model ${modelName}. wan3.0-video-prime is the high-speed version, with the same official input/parameters/media contract as wan3.0-video. wan3.0 aliases the standard model. Prime requires an upstream key/channel authorized for Prime and is never silently mapped to standard. Select the workflow through media and prompt, without a mode or speed field.`,
    '',
    '## Complete field inventory (* required; nested * applies when an item exists)',
    '| Field | Type | Values |',
    '| --- | --- | --- |',
    ...WAN_PARAMETERS.map(
      (p) => `| ${p.name} | ${p.type} | ${p.value.replaceAll('|', '/')} |`
    ),
    '',
    '## Input and parameters',
    '- model and input are required. input must contain a non-empty prompt or at least one media item; media-only requests are accepted. Prompt supports Chinese and English and is truncated to 20,000 Unicode characters.',
    '- parameters is optional. Defaults are explicitly forwarded: resolution=1080P, ratio=adaptive, duration=5, audio=true, seed=-1, prompt_extend=true, watermark=false. Use explicit 480P and duration=2 for a minimum-cost test.',
    '- duration accepts integer -1 (automatic) or 2–30. With reference videos, input-video total seconds plus output seconds must not exceed 30. The provider checks actual input duration; do not add duration to media objects.',
    '- resolution is exactly 480P, 720P or 1080P. ratio accepts adaptive,16:9,4:3,1:1,3:4,9:16. adaptive lets the model choose from media and prompt intent.',
    '- audio=false generates without an audio track; changing this switch does not change the rate. prompt_extend=false disables prompt rewriting. watermark=true enables a watermark. Explicit false values and seed=0 are preserved.',
    '- seed accepts -1 or 0–2147483647. -1 or omission chooses a random seed; even the same seed does not guarantee identical output.',
    '- Omit unused fields. Explicit nulls, unknown fields and wrong types are rejected, including in input, parameters and each media object. No multipart, streaming, n, negative_prompt, or extra provider fields.',
    '',
    '## Media combinations and workflows',
    '- Each input.media item has exactly type and url. At most 20 items total, retaining array order. 图1, 视频1 and 音频1 in prompt count reference images, videos and audios separately in their respective array order.',
    '- Text to video: prompt with no media. Image to video: one first_frame; optionally add one last_frame. A last_frame alone is invalid. Frame media cannot mix with reference_image, reference_video, reference_audio, file or link.',
    '- Reference generation: up to 10 reference_image, 5 reference_video and 5 reference_audio, combined as needed. Video total duration ≤15 seconds; audio total duration ≤15 seconds.',
    '- Document / webpage generation: at most one file OR one link. Either can combine with reference media, but not frames. file and link are mutually exclusive.',
    '- Video editing: include reference_video and describe the edit in prompt. Video extension: include reference_video and explicitly ask to extend forward/backward or continue the clip. Both use the same model, and ratio=adaptive is recommended. Automatic duration (-1) is optional; see billing below.',
    '',
    '## Media URLs and file constraints',
    '- Use publicly accessible HTTP(S) URLs without embedded credentials, valid until completion. Images also accept data:image/jpeg;base64,..., data:image/png;base64,..., data:image/bmp;base64,... or data:image/webp;base64,... . Base64 is image-only. No local paths or direct multipart uploads.',
    '- DashScope temporary oss:// URLs depend on the owning Alibaba workspace and are not accepted on this Aijiau channel. Use a public URL or image data URL instead.',
    '- Images: JPEG/JPG/PNG (no alpha)/BMP/WEBP, at most 20 MB each, each side 240–8000 px, aspect ratio at most 8:1.',
    '- Videos: MP4/MOV, 1–15 seconds each, total ≤15 seconds, at least 16 fps, each side 240–4096 px, aspect ratio at most 8:1, at most 100 MB each.',
    '- Audio: WAV/MP3, 1–15 seconds each, total ≤15 seconds, at most 15 MB each.',
    '- Documents: DOCX/DOC/XLSX/XLS/PPTX/PPT/PDF/TXT/KEY/PAGES/NUMBERS/MD, at most 100 MB. PDF/DOCX/DOC/PPTX/PPT/KEY/PAGES are limited to 50 pages. link must be a public page that needs no login.',
    '- The gateway checks fields, scalar bounds, item counts, combinations and URL/Base64 structure. The provider checks actual file contents, availability, dimensions, formats, page counts and media duration; acceptance by the gateway does not guarantee generation success.',
    '',
    '## Billing',
    '- Fixed duration: reserve and charge the requested output seconds at the website resolution rate and group multiplier. Reference-video length and delivered length do not change this fixed-duration charge. Failed tasks refund the reservation.',
    '- Automatic duration (-1): reserve 30 output seconds at the selected rate; success settles the provider-reported video.duration, bounded to 2–30 seconds, and refunds the unused reserve. If duration metadata is missing or invalid, charge the minimum 2 seconds, release the rest and log the anomaly. Failures refund the reservation.',
    modelName === 'wan3.0-video-prime'
      ? '- Prime has independent pricing: 480P/720P/1080P multipliers are 0.5/1/2 relative to its configured 720P base price. At USD 0.90 base and group multiplier 1, rates are USD 0.45/0.90/1.80 per second: 2-second 480P costs USD 0.90; automatic 480P reserves USD 13.50; default 5-second 1080P reserves USD 9.00. Check the actual website model and group prices before use.'
      : '- Standard at group multiplier 1 and USD 0.30 base price: 480P/720P/1080P cost USD 0.25/0.30/0.35 per second. A 2-second 480P task costs USD 0.50; automatic 480P reserves USD 7.50; default 5-second 1080P reserves USD 1.75. Prime uses separate model pricing and resolution multipliers (0.5/1/2). Check actual website pricing; these are website charges, not upstream prices.',
    '',
    '## Responses and recovery',
    '- Creation returns HTTP 200. Save id immediately; id/task_id/request_id refer to the same public task ID and model is the requested website model. Creation status may be pending. GET normalizes status to queued,in_progress,completed,failed.',
    '- Only public IDs, model and GET status are normalized. Optional provider metadata may be absent: progress need not reach 100, error details may be missing, timestamps may be RFC3339 strings. Do not require video.url or the Alibaba output.task_id wrapper.',
    '- Poll the same ID every 10–15 seconds with request timeouts and a finite deadline. Resume the saved ID after a deadline or query failure; respect Retry-After/backoff on 429/5xx. Never automatically repeat a timed-out creation POST: it may already be accepted and repeating it may charge again. If no ID arrived, inspect task logs.',
    '- Download /content only after completed, with the same active website token and positive remaining quota. Before completion it returns 409. If a token uses its last quota, restore a small positive allowance to resume reads; do not recreate the task.',
    '- Download supports Range/If-Range: 206 partial content, 416 unsatisfiable range, and 200 full content including a changed If-Range. Do not append a full 200 response to a partial file. Follow redirects without forwarding credentials to a different host.',
    '- Download promptly. Alibaba documents 24-hour task IDs for its direct API; that period is not a verified Aijiau retention guarantee. This website does not guarantee permanent media storage.',
    '- Check HTTP status first. Validation errors return 400 with code/message/data (invalid_request,invalid_duration,invalid_resolution,invalid_media,invalid_webhook,invalid_webhook_url). Authentication/download errors may use error.message. Correct fields, key, group or balance before retrying.',
    '',
    videoWebhookGuide(),
    '## Eight separate examples',
    'In order: text, single first frame, first/last frames, mixed references, document, webpage, edit, extension. Replace all media URLs. These examples use fixed 2-second 480P output; each POST creates a separate paid task. Do not merge all examples.',
    ...wanExamples(modelName).flatMap((request) => [
      '```json',
      JSON.stringify(request, null, 2),
      '```',
    ]),
    '## Python: create once, save ID, poll and download',
    `Requires requests; set NEW_API_BASE_URL=${base} and NEW_API_KEY=<website key>. Running this creates one paid task.`,
    '```python',
    wanPythonExample(modelName),
    '```',
    '## Resume without creating another task',
    '```bash',
    `export NEW_API_BASE_URL='${base}'`,
    'export NEW_API_KEY="<website key>"',
    'TASK_ID="task_example"',
    'curl --fail-with-body "$NEW_API_BASE_URL/v1/videos/$TASK_ID" -H "Authorization: Bearer $NEW_API_KEY"',
    '# Only after completed:',
    'curl --fail-with-body --location "$NEW_API_BASE_URL/v1/videos/$TASK_ID/content" -H "Authorization: Bearer $NEW_API_KEY" --output wan.mp4',
    '```',
    '## Migration',
    'Replace the previous flat request with input.prompt/input.media and parameters. mode, speed, seconds, size, top-level duration/resolution/aspect_ratio/ratio, n, first_frame/last_frame and reference_images/reference_videos/reference_audios are removed and return 400. audio/seed/prompt_extend/watermark belong inside parameters. Retired wan3.0-prime-r2v and wan3.0-i2v remain unsupported. wan3.0-video-prime uses the official nested contract and requires a Prime-enabled channel. Existing saved public task IDs can still be queried and downloaded.',
  ].join('\n')
}
