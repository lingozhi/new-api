# DepthMedia API 调用说明

DepthMedia 是统一的异步媒体处理接口。深度视频、视频去字幕、视频去背景、视频放大、图片去背景和图片高清放大都通过
`POST /v1/media/jobs` 提交。客户端会立即获得公开 `task_id`，随后可轮询任务状态，也可以
通过 Webhook 接收最终结果。

## 鉴权

网站地址：

```text
https://api.opwan.ai
```

所有请求使用网站令牌鉴权：

```http
Authorization: Bearer <OPWAN_API_TOKEN>
Content-Type: application/json
```

## 深度视频

模型：`depth-video`

计费为 `$0.002/秒`（按当前网站汇率约 `¥0.0146/秒`）。系统根据上游返回的
实际帧数和 FPS 计算时长，不采用客户端申报值；不足一秒的部分向上取整。单个视频
最长 600 秒，提交时会按 600 秒预扣，任务完成后自动按实际秒数结算并退回差额。

```bash
curl https://api.opwan.ai/v1/media/jobs \
  -H "Authorization: Bearer $OPWAN_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "depth-video",
    "source_url": "https://cdn.example.com/input.mp4",
    "operation": "depth",
    "webhook_url": "https://client.example.com/webhooks/depth-media",
    "webhook_secret": "replace-with-your-secret"
  }'
```

## 图片处理

接口：

```text
POST /v1/media/jobs
```

支持的参数组合：

| 模型 | 档位 | operation | quality | scale | 单次价格 |
| --- | --- | --- | --- | --- | --- |
| `background-remove` | 快速 | `remove_background` | `fast` | 不传 | `$0.02` |
| `background-remove` | 高质量 | `remove_background` | `quality` | 不传 | `$0.03` |
| `background-remove` | 精细抠图 | `remove_background` | `matting` | 不传 | `$0.03` |
| `image-upscale` | 快速 2 倍 | `upscale` | `fast` | `2` | `$0.02` |
| `image-upscale` | 快速 4 倍 | `upscale` | `fast` | `4` | `$0.02` |
| `image-upscale` | 高保真 4 倍 | `upscale` | `fidelity` | `4` | `$0.05` |
| `image-upscale` | 锐化 4 倍 | `upscale` | `sharp` | `4` | `$0.05` |

模型广场只展示 `depth-video`、`background-remove`、`image-upscale`、
`subtitle-remove`、`video-upscale`、`video-background-remove` 六个模型。
具体处理档位和价格由参数决定，并在模型详情抽屉中展示。图片格式支持上游允许的
`png` 和 `webp`。

```bash
curl https://api.opwan.ai/v1/media/jobs \
  -H "Authorization: Bearer $OPWAN_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "image-upscale",
    "source_url": "https://cdn.example.com/input.png",
    "operation": "upscale",
    "quality": "fidelity",
    "scale": 4,
    "format": "webp",
    "webhook_url": "https://client.example.com/webhooks/depth-media",
    "webhook_secret": "replace-with-your-secret"
  }'
```

## 视频去字幕

模型：`subtitle-remove`

按源视频实际时长计费，单价为 `$0.02/秒`。系统根据上游返回的帧数和 FPS
计算时长，不采用客户端申报值；不足一秒的部分向上取整。单个视频最长 600 秒，
提交时按 600 秒预扣，任务完成后按实际秒数结算并退回差额。

`subtitle_area` 支持：

- `bottom`：只扫描画面底部字幕区域，默认值，速度更快。
- `full`：扫描完整画面，适用于字幕位置不固定的视频。

```bash
curl https://api.opwan.ai/v1/media/jobs \
  -H "Authorization: Bearer $OPWAN_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "subtitle-remove",
    "source_url": "https://cdn.example.com/captioned.mp4",
    "operation": "remove_subtitles",
    "quality": "quality",
    "format": "mp4",
    "subtitle_area": "bottom",
    "webhook_url": "https://client.example.com/webhooks/depth-media",
    "webhook_secret": "replace-with-your-secret"
  }'
```

## 视频增强

### 视频放大

模型：`video-upscale`

按源视频实际时长计费，支持 2 倍和 4 倍放大，默认输出 MP4。上游最多处理 300 帧，输出最高 4K；
模型可能为了对齐要求调整输出尺寸，客户端应读取任务结果中的实际宽高。视频音频会保留。

| 档位 | operation | quality | scale | format |
| --- | --- | --- | --- | --- |
| 2 倍 | `video_upscale` | `quality` | `2` | `mp4` |
| 4 倍 | `video_upscale` | `quality` | `4` | `mp4` |

```bash
curl https://api.opwan.ai/v1/media/jobs \
  -H "Authorization: Bearer $OPWAN_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "video-upscale",
    "source_url": "https://cdn.example.com/input.mp4",
    "operation": "video_upscale",
    "quality": "quality",
    "scale": 2,
    "format": "mp4"
  }'
```

### 视频去背景

模型：`video-background-remove`

按源视频实际时长计费，最长 60 秒。默认输出透明 WebM；如选择 MP4，前景会合成到黑色背景。视频音频会保留。

| 档位 | operation | quality | format |
| --- | --- | --- | --- |
| 快速 | `remove_video_background` | `fast` | `webm` |
| 高质量 | `remove_video_background` | `quality` | `webm` |

```bash
curl https://api.opwan.ai/v1/media/jobs \
  -H "Authorization: Bearer $OPWAN_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "video-background-remove",
    "source_url": "https://cdn.example.com/subject.mp4",
    "operation": "remove_video_background",
    "quality": "quality",
    "format": "webm"
  }'
```

## 提交响应

提交成功返回 HTTP `202`：

```json
{
  "id": "task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "status": "queued",
  "progress": 0,
  "created_at": 1785312000
}
```

对外只返回网站生成的 `task_id`，不会暴露上游任务 ID。

## 查询任务

```bash
curl https://api.opwan.ai/v1/media/jobs/task_xxx \
  -H "Authorization: Bearer $OPWAN_API_TOKEN"
```

任务状态为：

- `QUEUED`
- `IN_PROGRESS`
- `SUCCESS`
- `FAILURE`

成功后结果位于 `data.result_url`：

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "task_xxx",
    "status": "SUCCESS",
    "progress": "100%",
    "result_url": "https://cdn.example.com/result.webp"
  }
}
```

建议轮询间隔为 3 至 5 秒。

## Webhook

提交任务时可传：

```json
{
  "webhook_url": "https://client.example.com/webhooks/depth-media",
  "webhook_secret": "replace-with-your-secret"
}
```

要求：

- `webhook_url` 必须是公网 HTTPS 地址。
- 网关仅在任务进入 `SUCCESS` 或 `FAILURE` 后发送回调。
- 投递语义为至少一次，接收方应使用 Delivery ID 去重。
- 接收方应返回 HTTP `2xx`，否则网关会重试。

回调头：

```text
X-Webhook-Delivery-Id: task_xxx
X-Webhook-Timestamp: <unix timestamp>
X-Webhook-Signature: v1=<hmac sha256>
```

签名内容：

```text
v1.<timestamp>.<delivery_id>.<raw_request_body>
```

使用 `webhook_secret` 作为 HMAC-SHA256 密钥。

回调体示例：

```json
{
  "task_id": "task_xxx",
  "platform": "59",
  "status": "SUCCESS",
  "progress": "100%",
  "result_url": "https://cdn.example.com/result.webp",
  "error": "",
  "created_at": 1785312000,
  "updated_at": 1785312060
}
```

## 当前限制

- 当前生产接口仅接收公网 `source_url`。
- `/v1/media/jobs/upload` 暂未通过网关开放；当前仍使用公网 `source_url`。
- `/v1/depth/jobs` 暂时保留给深度视频兼容调用；新接入统一使用
  `/v1/media/jobs`。
- 渠道密钥和模型价格必须先在管理员后台配置，否则网站不会路由任务。
