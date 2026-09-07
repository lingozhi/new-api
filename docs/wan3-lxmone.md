# Wan 3.0 through lxmone.xyz

The OpenAI channel adapter supports `wan3.0-video`, `wan3.0-video-prime`,
`wan3.0-prime-r2v`, and `wan3.0-i2v` through the asynchronous video API.
Provider documentation: <https://lxmone.xyz/user-api-docs.html#wan>.

## Channel and pricing

Configure an OpenAI channel with base URL `https://lxmone.xyz`, a video-enabled
provider key, and the four model names. Keep the channel disabled until prices
and end-to-end tests are complete. Do not put credentials in repository files.

Set each model's `ModelPrice` to **0.30 USD**, the 720p price per output second.
The adapter applies duration and resolution multipliers; do not use a token
billing expression for this asynchronous task route.

| Resolution | Base price per output second | Multiplier relative to 720p |
| --- | --- | --- |
| 480p | $0.25 | 0.25 / 0.30 |
| 720p | $0.30 | 1 |
| 1080p | $0.35 | 0.35 / 0.30 |

Existing user/group multipliers still apply. The public pricing catalog exposes
the three prices via `video_resolution_prices`. Changing a model's base price
scales all three tiers proportionally.

## Requests

Submit JSON to `POST /v1/videos`, query `GET /v1/videos/{id}`, then retrieve
`GET /v1/videos/{id}/content` with the same gateway API key. Save the public
gateway task ID and avoid resubmitting while a task is pending.

```json
{
  "model": "wan3.0-video",
  "prompt": "A blue ball gently rolling across a white tabletop, static camera.",
  "seconds": "2",
  "size": "480P",
  "aspect_ratio": "16:9",
  "prompt_extend": false
}
```

`seconds` is a string integer; `duration` is an integer alias. Both must agree
when supplied. Durations must be 2–30 seconds. `size` and `resolution` accept
480P, 720P, or 1080P, case-insensitively, and must agree when both are supplied.
Omitted duration and resolution are normalized to 5 seconds and 720P and sent
explicitly upstream, so billing and generation cannot use different defaults.
Only one output per request is supported.

The two `wan3.0-video*` models accept `reference_images`, `reference_videos`, and
`reference_audios`. `wan3.0-prime-r2v` requires `media` with `reference_image`,
`reference_video`, or `audio` entries. `wan3.0-i2v` requires exactly one
`first_frame` and one `last_frame` entry in `media`. References use HTTPS URLs;
limits are 10 images, 5 videos, and 5 audios. Media fields and optional flags
such as `prompt_extend: false` are preserved when forwarding the request.

Validate through the deployed gateway, including invalid-parameter rejection,
task submission, polling, authenticated MP4 download, and the consume log's
duration, resolution, group multiplier, and final charge.
