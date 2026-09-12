# Wan 3.0 through Aijiau

The public request now follows the [official Wan 3.0 API reference](https://docs.bailian.console.aliyun.com/zh/model-studio/wan3-video-generation-api-reference) and [generation guide](https://docs.bailian.console.aliyun.com/zh/model-studio/wan3-video-generation-guide).
See [the complete website contract](wan-unified.md) for fields, defaults, media constraints, examples, billing and recovery.

## Channel setup and translation

- Type: OpenAI; base URL: `https://tokens.aijiakefu.com` (trailing `/v1` accepted).
- Credential: Aijiau API key authorized for `wan3.0-video`.
- Models: `wan3.0-video` and website alias `wan3.0`; no client model mapping.
- Provider endpoints: `/v1/videos/generations`, `/{id}` and `/{id}/content` beneath it.
- Website endpoints: `POST /v1/videos`, `GET /v1/videos/{id}`, `GET /v1/videos/{id}/content`.
- Aijiau dashboard and [API documentation](https://api.aijiau.com/docs/api) are at `api.aijiau.com`.

The adapter converts `input.prompt` and `input.media` to Aijiau's top-level `prompt` and `media`, preserving media order and type. `parameters.ratio` becomes `aspect_ratio`; resolution, duration, audio, seed, prompt_extend and watermark become top-level provider fields. The provider also receives the matching string `seconds`. Website webhook fields stay in the gateway. Defaults are explicitly sent so provider defaults cannot diverge from billing. No DashScope key or async header is needed for website clients.

This channel supports the standard model. Prime is not enabled, and Alibaba workspace-scoped `oss://` URLs are rejected; callers can use public HTTP(S) URLs or image Base64 data URLs. File contents and actual media duration are validated by the upstream service. The published Aijiau studio uses this flat payload, but its implementation is not proof that every official media format has passed a live task.

## Billing and migration

Configure the 720P per-second base price and group multiplier before enabling requests. Resolution multipliers remain 480P = 0.25/0.30, 720P = 1, 1080P = 0.35/0.30. The official default is now 1080P, adaptive ratio, 5 seconds. Use explicit 480P and 2 seconds when testing.

Fixed duration charges requested output seconds. Automatic `parameters.duration=-1` reserves 30 output seconds, then settles the provider's `video.duration` bounded to 2–30 seconds using the saved price snapshot. Missing/invalid duration settles at the 2-second minimum and emits an accounting error; failed tasks refund. The maximum reserve must never become a maximum charge merely because metadata is absent. Quota conversion uses checked saturation and carries the marker into settlement logs.

The public flat fields (`mode`, `speed`, `seconds`, `size`, duration/resolution/ratio aliases, reference lists and frame fields) are removed. Put prompt/media inside `input` and all generation controls inside `parameters`. Both public model names use the same validator. Saved task IDs remain queryable through the existing task endpoints.

## Earlier verification record (before the nested request migration)

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


## Nested request verification scope

Local contract tests cover official defaults, all seven media types and combination rules, media-only input, Unicode prompt truncation, pointer zero values, URL/Base64 validation, retired flat fields, duration/seed bounds, automatic reserve/settlement, public task identity and website webhooks. Documentation and copied examples use the same request builders.

The earlier paid tasks above used the previous flat public request. They establish provider media generation, download and webhook behavior for that version, not live verification of every newly exposed parameter. Release verification for the nested contract is recorded separately after deployment.
