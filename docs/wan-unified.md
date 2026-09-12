# Unified Wan 3.0

This contract describes the current official Aijiau channel. The previous
Lxmone Wan channel is disabled on this deployment. Use `model: "wan3.0"` for every
workflow and a **website API key** in the official group (`官方渠道`). Do not use
the provider key in client requests.

## Endpoints and authentication

Set `NEW_API_BASE_URL` to the website origin, for example `https://api.opwan.ai`,
without `/v1`. All endpoints require `Authorization: Bearer <NEW_API_KEY>`.

| Operation | Method and path |
| --- | --- |
| Create one paid task | `POST /v1/videos`, `Content-Type: application/json` |
| Query saved public ID | `GET /v1/videos/{id}` |
| Download completed MP4 | `GET /v1/videos/{id}/content` |

The unified model does not accept multipart uploads, streaming, or the provider's
`/v1/videos/generations` path. The gateway handles provider routing internally.

## Modes

| mode | speed | Required input in addition to prompt |
| --- | --- | --- |
| general | standard | none; reference lists optional |
| reference | standard | at least one reference image, video or audio |
| frames | standard | both first_frame and last_frame |

`mode` defaults to `auto`: either top-level frame field selects `frames`, otherwise
`general`. Reference lists alone do not select `reference`. Every mode defaults to
`standard`; explicit `fast` returns HTTP 400 before quota reservation. Frames
cannot contain non-empty reference lists. General/reference cannot contain
these top-level frame fields.

## Complete request fields

Omit unused optional fields. Unknown fields and explicit nulls are rejected.
Required nested fields apply when an array entry exists.

| Field | Type | Required / accepted values |
| --- | --- | --- |
| model | string | required; `wan3.0` |
| prompt | string | required; non-empty text, including media requests |
| mode | string | `auto` (default), `general`, `reference`, `frames` |
| speed | string | `standard` only (default) |
| seconds | string | integer string, e.g. `"2"`; 2–30, default 5 |
| duration | integer | alias of seconds; 2–30 |
| size | string | `480P`, `720P` (default), `1080P`, case-insensitive |
| resolution | string | alias of size |
| aspect_ratio | string | `16:9` (default), `9:16`, `1:1` |
| ratio | string | alias of aspect_ratio |
| n | integer | `1` only |
| prompt_extend | boolean | optional; false disables enhancement; omission uses provider default |
| first_frame | string | HTTPS image URL; required with last_frame in frames mode |
| last_frame | string | HTTPS image URL; required with first_frame in frames mode |
| reference_images | object[] | at most 10; required url, optional role |
| reference_images[].url | string | HTTPS image URL |
| reference_images[].role | string | general: reference_image, first_frame, last_frame; reference: reference_image only; omission means reference image |
| reference_videos | object[] | at most 5; required url only |
| reference_videos[].url | string | HTTPS video URL |
| reference_audios | object[] | at most 5; required url only |
| reference_audios[].url | string | HTTPS audio URL |
| webhook_url | string | optional public HTTPS callback, port 443, at most 2048 bytes |
| webhook_secret | string | optional signing secret, at most 512 bytes; requires webhook_url |

Aliases must agree when both are supplied. Fractions, -1/automatic duration,
4K/pixel-dimension resolutions and other aspect ratios are unsupported.
Explicit `prompt_extend: false` is preserved.

Media URLs must be public HTTPS URLs without embedded credentials and remain
accessible until generation completes. Local paths, base64 data URLs and uploads
are unsupported. The gateway checks structure and counts; the provider checks
file availability, formats and actual media limits. Accepted gateway bounds do
not guarantee that every duration/resolution/media combination will generate.

`reference_videos[].duration` is unsupported in **every** mode. Only top-level
`seconds`/`duration` controls generated length. Nested objects accept only the
listed fields. Do not send provider fields such as `media`, `seed`, `audio`,
`watermark`, `negative_prompt`, `input_reference`, `image`, `image_end`, `file_id`,
`callback_url`, `response_format` or `stream`.

## Four alternative requests

These are separate requests. Replace example URLs with real files; do not combine
every optional field. The website API tab and copied AI guide share these four
workflows and the complete parameter inventory.

Text to video:

```json
{"model":"wan3.0","prompt":"A blue ball on a white table, fixed camera.","mode":"auto","speed":"standard","seconds":"2","size":"480P","aspect_ratio":"16:9","prompt_extend":false}
```

General with an image reference:

```json
{"model":"wan3.0","prompt":"A slow camera move around the subject.","mode":"general","speed":"standard","duration":2,"resolution":"480P","reference_images":[{"url":"https://example.com/reference.jpg"}]}
```

Reference media (one or more lists is sufficient):

```json
{"model":"wan3.0","prompt":"Animate the subject using the supplied references.","mode":"reference","speed":"standard","seconds":"2","size":"480P","reference_images":[{"url":"https://example.com/reference.jpg"}],"reference_videos":[{"url":"https://example.com/reference.mp4"}],"reference_audios":[{"url":"https://example.com/reference.mp3"}]}
```

First and last frames:

```json
{"model":"wan3.0","prompt":"A smooth transition between the two frames.","mode":"frames","speed":"standard","seconds":"2","size":"480P","first_frame":"https://example.com/first.jpg","last_frame":"https://example.com/last.jpg"}
```

## Create once, query and download

Save a chosen JSON example as `request.json`. The following creates one paid task;
save `create.json` immediately. Creation returns HTTP 200 with the same public ID
in `id`, `task_id`, `request_id`, and `model: "wan3.0"`. Creation status is
provider-supplied and may be `pending`; read normalized state with GET.

```sh
export NEW_API_BASE_URL='https://api.opwan.ai'
export NEW_API_KEY='<website API key>'
curl --fail-with-body --max-time 120 "$NEW_API_BASE_URL/v1/videos" -H "Authorization: Bearer $NEW_API_KEY" -H 'Content-Type: application/json' --data-binary @request.json -o create.json
```

Save the returned `id` as `TASK_ID`. Query every 10–15 seconds with a finite
polling deadline and per-request timeout:

```sh
TASK_ID='task_replace_with_returned_id'
curl --fail-with-body --max-time 60 "$NEW_API_BASE_URL/v1/videos/$TASK_ID" -H "Authorization: Bearer $NEW_API_KEY"
```

| GET status | Client action |
| --- | --- |
| queued | wait and query the same ID |
| in_progress | wait and query the same ID |
| completed | download and save MP4 |
| failed | stop; inspect error details and website task logs |

Illustrative completed query, with optional metadata omitted:

```json
{"id":"task_example","task_id":"task_example","request_id":"task_example","model":"wan3.0","status":"completed"}
```

Only public IDs, model and query status are normalized. Other fields are optional
provider metadata: progress and error may be absent, and progress need not reach
100 when completed. Timestamps may be RFC3339 strings. Do not require integer
timestamps, a fixed error object or `video.url`. Webhooks have their own fixed
terminal schema. After `completed`, download with the same website key:

```sh
curl --fail-with-body --location --max-time 300 "$NEW_API_BASE_URL/v1/videos/$TASK_ID/content" -H "Authorization: Bearer $NEW_API_KEY" --output wan.mp4
```

Before completion, content returns 409. Range/If-Range support partial content
(206), unsatisfiable ranges (416), and full responses (200, including an If-Range
mismatch). Use `curl --continue-at -` with a saved partial file to resume. If the
resource changed, download a fresh file instead of appending a 200 response.
Do not forward authorization to another host when following redirects.
Download promptly: no retention period is verified for the current channel, and
the website does not guarantee permanent storage. Do not assume 48 hours.

## Errors and recovery

Check HTTP status before parsing success fields. Create validation errors return
HTTP 400 with top-level code/message/data, for example:

```json
{"code":"invalid_duration","message":"duration must be between 2 and 30 seconds","data":null}
```

`invalid_request`, `invalid_duration`, `invalid_resolution`, `invalid_n` and
`invalid_media` need input correction. `invalid_webhook` and `invalid_webhook_url`
need callback corrections. Authentication and download errors may instead use
`error.message`; check the website key, group and balance for authentication,
routing or quota errors.

For GET network/429/5xx errors, respect Retry-After/backoff and resume the **same**
ID. A polling timeout is not proof of task failure. Never automatically retry a
timed-out POST: it may already be accepted and repeating it can charge twice.
If no ID arrived, inspect website task logs before creating another task. The
website API tab and copied AI guide include Python code that saves the ID,
polls with a deadline and downloads the result.

## Billing

The website reserves requested output seconds at the configured per-second
resolution price and group multiplier. Successful tasks keep that charge;
failed tasks refund the reservation. Input video length is not added, and actual
output length does not resettle Wan. Validation occurs before reservation.

At the current USD 0.30 base price and group multiplier 1, 480P/720P/1080P cost
USD 0.25/0.30/0.35 per requested second. For example, 2 seconds at 480P costs
USD 0.50. Always read current website pricing; this is not an upstream price table.

## Optional webhooks

Add `webhook_url` and optionally `webhook_secret` to any valid request. They are
website-only fields, stripped before forwarding upstream. See [video webhooks](video-webhooks.md)
for the complete payload, HMAC verification, retries and deduplication. Polling
remains available when a callback fails.

## Provider configuration and legacy deployments

See [Aijiau setup](wan3-aijiau.md) for the current channel and verification scope.
The website API tab and AI-copy guide describe that active channel. The adapter
still contains [Lxmone legacy support](wan3-lxmone.md) for other deployments;
that does not make its disabled model routes available here. Restoring another
provider requires reviewing modes, pricing and public documentation together.
No client-side provider model mapping is required.
