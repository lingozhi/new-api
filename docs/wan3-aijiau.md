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

Tests use local fixtures for the documented request, task status and content
download protocols. A real provider key is still required to verify generation,
model availability and charges end to end.

Aijiau's live polling responses use `pending` and `running`; `running` is an
in-progress state, not a failed task. The adapter also accepts its client-side
terminal aliases `succeeded`/`success` and `canceled`/`rejected`.
