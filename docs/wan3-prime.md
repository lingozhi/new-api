# Wan 3.0 Prime

`wan3.0-video-prime` is the high-speed model in the [official API reference](https://docs.bailian.console.aliyun.com/zh/model-studio/wan3-video-generation-api-reference).
It uses the same `input / parameters / media` fields and constraints as standard.
See the [complete website contract](wan-unified.md) for media combinations,
defaults, automatic duration, task responses, downloads and webhooks.

Use the website task endpoint and a website key with access to a Prime-enabled
channel. No `speed` or `mode` field is needed. Prime is never converted to standard.

```json
{
  "model": "wan3.0-video-prime",
  "input": {"prompt": "A blue ball on a white table, fixed camera."},
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

Create with `POST /v1/videos`, save `id`, query `GET /v1/videos/{id}`, then download
`GET /v1/videos/{id}/content`. The returned public model remains
`wan3.0-video-prime`. Fixed-duration tasks charge requested output seconds.
Automatic duration reserves 30 seconds and settles using the saved Prime price.

Prime has independent resolution multipliers: 480P = 0.5, 720P = 1, 1080P = 2.
The configured discounted 720P base price is USD 0.63 with group multiplier 1.
Per-second prices are USD 0.315 / 0.63 / 1.26 for 480P / 720P / 1080P. The
0.7 discount is already included in the base price; do not apply it again.
The example costs USD 0.63; automatic 480P reserves USD 9.45, and default
5-second 1080P costs USD 6.30. Prices are configured in USD as requested.
Check the current website price before submitting.

Aijiau currently lists Prime in its separate `Wan3.0视频Prime` group (32). A key
that only lists standard `wan3.0-video` must not be assumed to authorize Prime.
Configure a separate OpenAI-compatible channel with the Prime group's key,
base URL `https://tokens.aijiakefu.com`, model `wan3.0-video-prime`, and website
group `官方渠道`. The supplied Prime key has been verified to list this exact
model. Set its independent website base price before enabling the channel.
Keep the working standard channel and the retired Lxmone channel’s disabled
state intact when configuring Prime.
