# Wan 3.0 website API
Request fields follow the official Wan 3.0 input / parameters / media structure. Aijiau remains the upstream channel.
Sources: [official API reference](https://docs.bailian.console.aliyun.com/zh/model-studio/wan3-video-generation-api-reference) and [generation guide](https://docs.bailian.console.aliyun.com/zh/model-studio/wan3-video-generation-guide).

## Website endpoints and authentication
API origin: https://api.opwan.ai (without /v1). Use a website API key in the official group (官方渠道).
Authorization: Bearer <NEW_API_KEY>; Content-Type: application/json.
POST https://api.opwan.ai/v1/videos
GET https://api.opwan.ai/v1/videos/{id}
GET https://api.opwan.ai/v1/videos/{id}/content
The website uses its own endpoints, key, public task IDs and response schema. The DashScope endpoint, SDK response format and X-DashScope-Async header are not required here. Do not send a provider key from clients.
Use model wan3.0-video (standard); wan3.0 is a website alias with identical validation. The official Prime model is not enabled on the current Aijiau channel. Do not select a separate model or mode for different media workflows.

## Complete field inventory (* required; nested * applies when an item exists)
| Field | Type | Values |
| --- | --- | --- |
| model | string * | wan3.0-video / wan3.0 |
| input | object * | {prompt?, media?} |
| input.prompt | string | ≤20000; prompt / media |
| input.media | object[] | ≤20; {type, url} |
| input.media[].type | string * | first_frame / last_frame / reference_image / reference_video / reference_audio / file / link |
| input.media[].url | string * | HTTP(S); data:image/...;base64,... |
| parameters | object | {} |
| parameters.resolution | string | 480P / 720P / 1080P; default: 1080P |
| parameters.ratio | string | adaptive / 16:9 / 4:3 / 1:1 / 3:4 / 9:16; default: adaptive |
| parameters.duration | integer | -1 / 2–30; default: 5 |
| parameters.audio | boolean | true / false; default: true |
| parameters.seed | integer | -1 / 0–2147483647; default: -1 |
| parameters.prompt_extend | boolean | true / false; default: true |
| parameters.watermark | boolean | true / false; default: false |
| webhook_url | string | HTTPS :443; ≤2048 bytes |
| webhook_secret | string | ≤512 bytes |

## Input and parameters
- model and input are required. input must contain a non-empty prompt or at least one media item; media-only requests are accepted. Prompt supports Chinese and English and is truncated to 20,000 Unicode characters.
- parameters is optional. Defaults are explicitly forwarded: resolution=1080P, ratio=adaptive, duration=5, audio=true, seed=-1, prompt_extend=true, watermark=false. Use explicit 480P and duration=2 for a minimum-cost test.
- duration accepts integer -1 (automatic) or 2–30. With reference videos, input-video total seconds plus output seconds must not exceed 30. The provider checks actual input duration; do not add duration to media objects.
- resolution is exactly 480P, 720P or 1080P. ratio accepts adaptive,16:9,4:3,1:1,3:4,9:16. adaptive lets the model choose from media and prompt intent.
- audio=false generates without an audio track; changing this switch does not change the rate. prompt_extend=false disables prompt rewriting. watermark=true enables a watermark. Explicit false values and seed=0 are preserved.
- seed accepts -1 or 0–2147483647. -1 or omission chooses a random seed; even the same seed does not guarantee identical output.
- Omit unused fields. Explicit nulls, unknown fields and wrong types are rejected, including in input, parameters and each media object. No multipart, streaming, n, negative_prompt, or extra provider fields.

## Media combinations and workflows
- Each input.media item has exactly type and url. At most 20 items total, retaining array order. 图1, 视频1 and 音频1 in prompt count reference images, videos and audios separately in their respective array order.
- Text to video: prompt with no media. Image to video: one first_frame; optionally add one last_frame. A last_frame alone is invalid. Frame media cannot mix with reference_image, reference_video, reference_audio, file or link.
- Reference generation: up to 10 reference_image, 5 reference_video and 5 reference_audio, combined as needed. Video total duration ≤15 seconds; audio total duration ≤15 seconds.
- Document / webpage generation: at most one file OR one link. Either can combine with reference media, but not frames. file and link are mutually exclusive.
- Video editing: include reference_video and describe the edit in prompt. Video extension: include reference_video and explicitly ask to extend forward/backward or continue the clip. Both use the same model, and ratio=adaptive is recommended. Automatic duration (-1) is optional; see billing below.

## Media URLs and file constraints
- Use publicly accessible HTTP(S) URLs without embedded credentials, valid until completion. Images also accept data:image/jpeg;base64,..., data:image/png;base64,..., data:image/bmp;base64,... or data:image/webp;base64,... . Base64 is image-only. No local paths or direct multipart uploads.
- DashScope temporary oss:// URLs depend on the owning Alibaba workspace and are not accepted on this Aijiau channel. Use a public URL or image data URL instead.
- Images: JPEG/JPG/PNG (no alpha)/BMP/WEBP, at most 20 MB each, each side 240–8000 px, aspect ratio at most 8:1.
- Videos: MP4/MOV, 1–15 seconds each, total ≤15 seconds, at least 16 fps, each side 240–4096 px, aspect ratio at most 8:1, at most 100 MB each.
- Audio: WAV/MP3, 1–15 seconds each, total ≤15 seconds, at most 15 MB each.
- Documents: DOCX/DOC/XLSX/XLS/PPTX/PPT/PDF/TXT/KEY/PAGES/NUMBERS/MD, at most 100 MB. PDF/DOCX/DOC/PPTX/PPT/KEY/PAGES are limited to 50 pages. link must be a public page that needs no login.
- The gateway checks fields, scalar bounds, item counts, combinations and URL/Base64 structure. The provider checks actual file contents, availability, dimensions, formats, page counts and media duration; acceptance by the gateway does not guarantee generation success.

## Billing
- Fixed duration: reserve and charge the requested output seconds at the website resolution rate and group multiplier. Reference-video length and delivered length do not change this fixed-duration charge. Failed tasks refund the reservation.
- Automatic duration (-1): reserve 30 output seconds at the selected rate; success settles the provider-reported video.duration, bounded to 2–30 seconds, and refunds the unused reserve. If duration metadata is missing or invalid, charge the minimum 2 seconds, release the rest and log the anomaly. Failures refund the reservation.
- At the current group multiplier 1 and USD 0.30 base price, 480P/720P/1080P cost USD 0.25/0.30/0.35 per second. A 2-second 480P task costs USD 0.50; automatic 480P reserves USD 7.50. Defaults reserve USD 1.75 for 5 seconds at 1080P. Check current website pricing before use; these are website charges, not Alibaba or Aijiau upstream prices.

## Responses and recovery
- Creation returns HTTP 200. Save id immediately; id/task_id/request_id refer to the same public task ID and model is the requested website model. Creation status may be pending. GET normalizes status to queued,in_progress,completed,failed.
- Only public IDs, model and GET status are normalized. Optional provider metadata may be absent: progress need not reach 100, error details may be missing, timestamps may be RFC3339 strings. Do not require video.url or the Alibaba output.task_id wrapper.
- Poll the same ID every 10–15 seconds with request timeouts and a finite deadline. Resume the saved ID after a deadline or query failure; respect Retry-After/backoff on 429/5xx. Never automatically repeat a timed-out creation POST: it may already be accepted and repeating it may charge again. If no ID arrived, inspect task logs.
- Download /content only after completed, with the same active website token and positive remaining quota. Before completion it returns 409. If a token uses its last quota, restore a small positive allowance to resume reads; do not recreate the task.
- Download supports Range/If-Range: 206 partial content, 416 unsatisfiable range, and 200 full content including a changed If-Range. Do not append a full 200 response to a partial file. Follow redirects without forwarding credentials to a different host.
- Download promptly. Alibaba documents 24-hour task IDs for its direct API; that period is not a verified Aijiau retention guarantee. This website does not guarantee permanent media storage.
- Check HTTP status first. Validation errors return 400 with code/message/data (invalid_request,invalid_duration,invalid_resolution,invalid_media,invalid_webhook,invalid_webhook_url). Authentication/download errors may use error.message. Correct fields, key, group or balance before retrying.

## Optional website webhooks (polling remains available)
- webhook_url: optional string, maximum 2048 bytes, public HTTPS endpoint on port 443 without embedded credentials. Private destinations are rejected. Omit the field when unused; null/empty is rejected.
- webhook_secret: optional string, maximum 512 bytes; requires webhook_url. Use a strong independent secret, not the website API key. Omitted/empty means no signature. Null is rejected.
- These are WEBSITE fields. They are persisted in the gateway outbox and removed before forwarding to the provider. No provider callback support is required.
- Merge these two optional fields into ONE of the valid creation requests:
```json
{
  "webhook_url": "https://your-app.example/webhooks/video",
  "webhook_secret": "<YOUR_WEBHOOK_SECRET>"
}
```
- The website sends POST application/json after completed or failed. No intermediate progress events. It continues polling the provider internally. You can still query the original task ID.
- Success body:
```json
{
  "id": "task_example",
  "task_id": "task_example",
  "model": "<REQUESTED_MODEL>",
  "status": "completed",
  "progress": 100,
  "content_url": "/v1/videos/task_example/content"
}
```
- model is your public requested model, not the placeholder above. On failure status=failed, error={"message":"..."}, and content_url is absent. id and task_id are the same public ID; progress=100 means terminal, not necessarily successful.
- content_url is a relative path on the WEBSITE API origin. Download it with the website API key after success; never send that key to the webhook endpoint.
- Headers: X-Webhook-Delivery-Id (stable public task ID), X-Webhook-Timestamp (Unix seconds), X-Webhook-Signature (only when secret is non-empty).
- Signature: v1=<lowercase hexadecimal HMAC-SHA256>. Sign the exact bytes of "v1." + timestamp + "." + delivery_id + "." + RAW_HTTP_BODY with webhook_secret. Verify before parsing JSON using a constant-time comparison, reject timestamps outside a short tolerance (e.g. 5 minutes), and deduplicate the Delivery ID atomically.
- Reply with HTTP 2xx promptly after durably recording the event. Redirects are rejected. Timeout is 15 seconds per delivery; failures get at most 5 total attempts with 30/60/120/240-second retry delays, subject to scheduler timing. Delivery is at least once and retries reuse Delivery ID.
- Callback delivery failure does not change generation status or create another paid task. Keep polling as fallback. Do not retry creation just because a callback is missing.
## Eight separate examples
In order: text, single first frame, first/last frames, mixed references, document, webpage, edit, extension. Replace all media URLs. These examples use fixed 2-second 480P output; each POST creates a separate paid task. Do not merge all examples.
```json
{
  "model": "wan3.0-video",
  "input": {
    "prompt": "A blue ball on a white table, fixed camera."
  },
  "parameters": {
    "resolution": "480P",
    "duration": 2,
    "ratio": "16:9",
    "audio": false,
    "seed": 0,
    "prompt_extend": false,
    "watermark": false
  }
}
```
```json
{
  "model": "wan3.0-video",
  "input": {
    "prompt": "Animate this first frame with a slow camera move.",
    "media": [
      {
        "type": "first_frame",
        "url": "https://example.com/first.jpg"
      }
    ]
  },
  "parameters": {
    "resolution": "480P",
    "duration": 2,
    "ratio": "16:9",
    "audio": false,
    "seed": 0,
    "prompt_extend": false,
    "watermark": false
  }
}
```
```json
{
  "model": "wan3.0-video",
  "input": {
    "prompt": "A smooth transition from the first frame to the last frame.",
    "media": [
      {
        "type": "first_frame",
        "url": "https://example.com/first.jpg"
      },
      {
        "type": "last_frame",
        "url": "https://example.com/last.jpg"
      }
    ]
  },
  "parameters": {
    "resolution": "480P",
    "duration": 2,
    "ratio": "16:9",
    "audio": false,
    "seed": 0,
    "prompt_extend": false,
    "watermark": false
  }
}
```
```json
{
  "model": "wan3.0-video",
  "input": {
    "prompt": "Use the subject in 图1, the movement in 视频1 and the sound in 音频1.",
    "media": [
      {
        "type": "reference_image",
        "url": "https://example.com/reference.jpg"
      },
      {
        "type": "reference_video",
        "url": "https://example.com/reference.mp4"
      },
      {
        "type": "reference_audio",
        "url": "https://example.com/reference.mp3"
      }
    ]
  },
  "parameters": {
    "resolution": "480P",
    "duration": 2,
    "ratio": "16:9",
    "audio": true,
    "seed": 0,
    "prompt_extend": false,
    "watermark": false
  }
}
```
```json
{
  "model": "wan3.0-video",
  "input": {
    "prompt": "Create a short product introduction from this document.",
    "media": [
      {
        "type": "file",
        "url": "https://example.com/product.pdf"
      }
    ]
  },
  "parameters": {
    "resolution": "480P",
    "duration": 2,
    "ratio": "16:9",
    "audio": false,
    "seed": 0,
    "prompt_extend": false,
    "watermark": false
  }
}
```
```json
{
  "model": "wan3.0-video",
  "input": {
    "prompt": "Summarize this public article as a short video.",
    "media": [
      {
        "type": "link",
        "url": "https://example.com/article"
      }
    ]
  },
  "parameters": {
    "resolution": "480P",
    "duration": 2,
    "ratio": "16:9",
    "audio": false,
    "seed": 0,
    "prompt_extend": false,
    "watermark": false
  }
}
```
```json
{
  "model": "wan3.0-video",
  "input": {
    "prompt": "Edit 视频1: replace the background with a sunny beach.",
    "media": [
      {
        "type": "reference_video",
        "url": "https://example.com/reference.mp4"
      }
    ]
  },
  "parameters": {
    "resolution": "480P",
    "duration": 2,
    "ratio": "adaptive",
    "audio": false,
    "seed": 0,
    "prompt_extend": false,
    "watermark": false
  }
}
```
```json
{
  "model": "wan3.0-video",
  "input": {
    "prompt": "Extend 视频1 forward: the camera continues moving toward the subject.",
    "media": [
      {
        "type": "reference_video",
        "url": "https://example.com/reference.mp4"
      }
    ]
  },
  "parameters": {
    "resolution": "480P",
    "duration": 2,
    "ratio": "adaptive",
    "audio": false,
    "seed": 0,
    "prompt_extend": false,
    "watermark": false
  }
}
```
## Python: create once, save ID, poll and download
Requires requests; set NEW_API_BASE_URL=https://api.opwan.ai and NEW_API_KEY=<website key>. Running this creates one paid task.
```python
import json, os, time
from pathlib import Path
import requests

base = os.environ["NEW_API_BASE_URL"].rstrip("/")
headers = {"Authorization": "Bearer " + os.environ["NEW_API_KEY"]}
body = json.loads("{\"model\":\"wan3.0-video\",\"input\":{\"prompt\":\"A blue ball on a white table, fixed camera.\"},\"parameters\":{\"resolution\":\"480P\",\"duration\":2,\"ratio\":\"16:9\",\"audio\":false,\"seed\":0,\"prompt_extend\":false,\"watermark\":false}}")
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
    raise TimeoutError("Query the saved ID later; do not create a duplicate task.")
```
## Resume without creating another task
```bash
export NEW_API_BASE_URL='https://api.opwan.ai'
export NEW_API_KEY="<website key>"
TASK_ID="task_example"
curl --fail-with-body "$NEW_API_BASE_URL/v1/videos/$TASK_ID" -H "Authorization: Bearer $NEW_API_KEY"
# Only after completed:
curl --fail-with-body --location "$NEW_API_BASE_URL/v1/videos/$TASK_ID/content" -H "Authorization: Bearer $NEW_API_KEY" --output wan.mp4
```
## Migration
Replace the previous flat request with input.prompt/input.media and parameters. mode, speed, seconds, size, top-level duration/resolution/aspect_ratio/ratio, n, first_frame/last_frame and reference_images/reference_videos/reference_audios are removed and return 400. audio/seed/prompt_extend/watermark belong inside parameters. Prime/R2V/I2V model routing is not enabled. Existing saved public task IDs can still be queried and downloaded.
