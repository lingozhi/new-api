# Wan 3.0 through Aijiau

Provider documentation: [API 调用文档](https://api.aijiau.com/docs/api).
The documented API base is `https://tokens.aijiakefu.com/v1`; `api.aijiau.com`
hosts the dashboard and documentation.

## Channel setup

- Channel type: OpenAI.
- Base URL: `https://tokens.aijiakefu.com` (a trailing `/v1` is also accepted
  by the video adapter).
- Key: an Aijiau API key authorized for video generation.
- Models: `wan3.0` and/or `wan3.0-video`.
- Model mapping: not required. The default unified `wan3.0` request becomes
  the documented upstream `wan3.0-video` model.

The unified API converts general/reference/frame inputs into this provider's
typed `media` array, including `reference_audio` for audio references. All modes
use the same upstream `wan3.0-video` model. Aijiau has no separate fast model:
omit `speed` or use `standard`; explicit `fast` requests are rejected before
billing. Reference-video `duration` metadata is not supported. The existing
unified frame requirement (both first and last frame) remains unchanged.

Configure the gateway's per-second 720p model price and group multiplier before
enabling paid requests. Existing Wan duration/resolution multipliers apply;
this integration does not set prices or infer Aijiau's upstream cost. The public
documentation does not specify a complete price table or guarantee every legacy
Wan workflow. Enable other upstream model IDs only after verifying their
availability with the provider key.

## Gateway request

```sh
curl "$NEW_API_BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "wan3.0",
    "prompt": "海边日出，镜头缓慢向前移动",
    "resolution": "720P",
    "duration": 5,
    "aspect_ratio": "16:9"
  }'
```

Save the returned public task ID, then query `GET /v1/videos/{id}` and download
`GET /v1/videos/{id}/content` using the same gateway key. The gateway routes
submission, polling and download to Aijiau's `/v1/videos/generations` endpoint
family, forwarding the provider key only upstream. Existing unified Wan
validation, billing, public task IDs and webhooks remain available; see
[Unified Wan 3.0](wan-unified.md).

## Verification and deployment record

On 2026-09-12, production enabled Aijiau channel 149 and disabled Lxmone Wan
channel 147. GPT Image and Seedance channels were unchanged. The configured
models are `wan3.0` and `wan3.0-video`; the provider key's model list confirmed
`wan3.0-video`. Former fast/reference/frame model IDs are not enabled here.

A live `wan3.0` text request for 2 seconds at 480P completed successfully: playable
854×480 H.264/AAC MP4 (2.02 seconds), HTTP 200 full download, and a matching HTTP
206 byte-range download. Net website charge: USD 0.50 at group multiplier 1.
Zero duration and fast speed returned HTTP 400 without a charge. The temporary
test token was revoked after verification.

Three additional live tasks on 2026-09-12 used the minimum output settings
(2 requested seconds, 480P tier), with no repeated creation requests:

| Workflow | Inputs | Aspect ratio | Actual output | Website charge |
| --- | --- | --- | --- | --- |
| general | one reference image | 1:1 | 640×640, 2.02 seconds | USD 0.50 |
| reference | image + 2-second video + 2-second WAV audio | 9:16 | 480×854, 2.02 seconds | USD 0.50 |
| frames | first and last frame | 16:9 | 854×480, 2.066 seconds | USD 0.50 |

All tasks reached SUCCESS on channel 149. The provider fetched every supplied
asset over HTTPS. Each result was playable H.264/AAC MP4, preserved the blue-ball
reference subject, and passed authenticated full download (200) plus matching
byte-range download (206). The 480P tier uses 640×640 for square output; callers
must not assume its output height is always exactly 480 pixels.

All three signed completion webhooks reached the HTTPS receiver. One receiver
response deliberately returned 503; the gateway retried about 30 seconds later
and received 204. The retry retained the delivery ID and identical payload bytes;
HMAC verification passed and receiver deduplication applied the task once. The
database confirmed all three outbox entries delivered. Callback retries added no
generation charge. Total additional website charge was USD 1.50; the temporary
token and HTTPS receiver were removed after verification.

The exact spending cap initially left zero token quota, which the current v1 video
query/download authentication rejects. Keeping one **unspent** quota unit allowed
reads and did not change the USD 1.50 charge. Clients should keep their token active
with positive remaining quota while retrieving existing video tasks.

Local fixtures additionally cover unsafe duration/count/alias inputs, status
aliases, RFC3339 metadata, failed callbacks and durable outbox behavior. Higher
resolutions, longer durations, every media format/combination and perceptual audio
reference fidelity were not exhaustively tested live. No provider retention
period has been verified.

Aijiau's live polling responses use `pending` and `running`; `running` is an
in-progress state, not a failed task. The adapter also accepts its client-side
terminal aliases `succeeded`/`success` and `canceled`/`rejected`.
Completion timestamps use RFC3339 strings. Polling parses state independently
of provider-specific video metadata and timestamps so successful tasks settle
and become downloadable.
