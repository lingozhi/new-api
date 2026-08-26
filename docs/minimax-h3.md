# MiniMax-H3 视频生成接口

`MiniMax-H3` 通过 MiniMax V2 风格的异步接口接入：调用 `POST /v2/video_generation` 创建任务后，可通过 `GET /v2/query/video_generation/{task_id}` 查询状态，也可选择通过 `callback_url` 接收终态结果。

本文描述的是当前兼容接口实际开放的能力子集，不是 MiniMax 官方接口的完整能力。当前只支持 768P、4～15 秒，以及文生视频、参考图、参考图加参考音频三种工作流；不支持 2K、自适应画幅、首尾帧、参考视频或 AIGC 水印。回调仅在任务进入 `succeeded`、`failed` 或 `cancelled` 终态时发送，不会像 MiniMax 官方完整回调那样逐次推送每个状态变化。

## 接口概览

| 操作 | Method | Path |
| --- | --- | --- |
| 创建任务 | `POST` | `/v2/video_generation` |
| 查询任务 | `GET` | `/v2/query/video_generation/{task_id}` |

两个接口都使用 Bearer Token：

```http
Authorization: Bearer sk-your-api-key
```

创建请求必须使用 `Content-Type: application/json`，整个 JSON 请求体最大 64 MiB。

## 创建任务

### 请求字段

| 字段 | 类型 | 必填 | 当前接口约束 |
| --- | --- | --- | --- |
| `model` | string | 是 | 固定为 `MiniMax-H3` |
| `content` | array | 是 | 1～16 项，必须至少有一条非空 `text` |
| `resolution` | string | 是 | 固定为 `768P`；不支持 `2K` |
| `duration` | integer | 是 | 4～15 秒 |
| `ratio` | string | 是 | 必须显式填写；支持情况见下表 |
| `callback_url` | string | 否 | 公网可访问的 HTTPS URL，最长 2048 个字符；创建时先进行 challenge 校验 |
| `aigc_watermark` | boolean | 否 | 只能省略或传 `false`；`true` 不受支持 |

画幅比例支持情况：

| `ratio` | 文生视频 | 参考图 | 参考图加音频 |
| --- | --- | --- | --- |
| `16:9` | 支持 | 支持 | 支持 |
| `9:16` | 支持 | 支持 | 支持 |
| `1:1` | 支持 | 支持 | 不支持 |
| `adaptive`、`21:9`、`4:3`、`3:4` | 不支持 | 不支持 | 不支持 |

### `content` 项

#### 提示词

```json
{
  "type": "text",
  "text": "电影感航拍，一艘帆船穿过晨雾"
}
```

- 每条 `text` 最多 7000 个 Unicode 字符。
- 多条文本会以换行连接。
- 参考图加音频工作流中，合并后的提示词最多 10000 个 Unicode 字符。

#### 参考图

```json
{
  "type": "image_url",
  "image_url": {
    "url": "https://media.example.com/portrait.png"
  },
  "role": "reference_image"
}
```

- 最多 9 张。
- `role` 必须显式为 `reference_image`。
- 当前不支持 `first_frame` 和 `last_frame`；省略 `role` 也不会被当作可用的参考图。

#### 参考音频

```json
{
  "type": "audio_url",
  "audio_url": {
    "url": "https://media.example.com/voice.mp3"
  },
  "role": "reference_audio"
}
```

- 最多 3 条。
- `role` 必须为 `reference_audio`。
- 必须同时提供至少一张 `reference_image`，不能只用文本加音频生成。

当前不支持 `type: "video_url"` 的参考视频。

### 媒体来源

参考图片和音频可使用：

1. 公网可访问的绝对 HTTPS URL。只允许默认 HTTPS 端口 443，不允许 URL userinfo、localhost、私网字面 IP 或含糊主机名。
2. 严格 base64 data URI，格式为 `data:<MIME>;base64,<DATA>`。

data URI 限制：

- 图片允许 JPEG、PNG、WebP，单项解码后最大 30 MiB。
- 音频允许 MP3、WAV，单项解码后最大 15 MiB。
- 一个请求内全部 data URI 解码后合计最大 64 MiB。
- `mm_file:` 不受支持。

公网 URL 的媒体内容由生成服务读取；网关不会在本地下载它来验证尺寸或时长。请确保 URL 在任务执行期间保持可访问。

### `callback_url` 回调

`callback_url` 可选。它必须是公网可访问的 HTTPS URL，最长 2048 个字符。建议在创建任务前先部署好回调接收器；本文的快速开始示例默认不传该字段，避免复制后向无法校验的示例地址提交。

创建请求中带有 `callback_url` 时，网关会先向该地址发送 challenge：

```http
POST /your-callback-path HTTP/1.1
Content-Type: application/json

{"challenge":"gateway-generated-value"}
```

接收器必须在 3 秒内返回 2xx，并以 JSON 原样回显同一个 `challenge` 值：

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"challenge":"gateway-generated-value"}
```

只有 challenge 校验通过后，网关才会接受任务，并在后台维护任务状态。本兼容层只在任务进入 `succeeded`、`failed` 或 `cancelled` 时发送终态回调；不会推送 `queued` 或 `running` 状态变化。终态回调的 JSON 请求体与查询接口当时返回的 `{"task": {...}}` 结构完全相同。

challenge 请求受用户和 API Key 双重限流，并受全局并发保护。请求过于频繁或当前校验容量已满时，创建接口返回 `429 callback_rate_limit_exceeded`，同时通过 `Retry-After` 响应头给出建议重试秒数；失败的 challenge 尝试也会计入限流。

终态回调包含：

- `X-Webhook-Delivery-Id`：同一任务重试时保持不变，接收方应用它去重。
- `X-Webhook-Timestamp`：本次发送的 Unix 时间戳（秒）。

接收器返回任意 2xx 即表示已接收。重定向、非 2xx 响应、网络错误或超时都视为发送失败；包含首次发送在内最多尝试 5 次，后续四次重试分别延迟 30、60、120、240 秒。回调是“至少一次”投递：如果接收器已处理请求但确认响应丢失，可能收到重复请求。

当前接口没有 `callback_secret` 字段，网关也不发送回调签名。应使用不可猜测的回调路径，核心状态变更还应通过带 Bearer Token 的查询接口复核。回调最终投递失败不会改变任务状态或计费；查询接口在任务创建后 7 天内始终可作为回退方案。

### 文生视频示例

```bash
export BASE_URL="https://api.opwan.ai"
export API_KEY="sk-your-api-key"
export MINIMAX_IDEMPOTENCY_KEY="video-order-20260826-001"

curl --request POST "${BASE_URL}/v2/video_generation" \
  --header "Authorization: Bearer ${API_KEY}" \
  --header "Content-Type: application/json" \
  --header "Idempotency-Key: ${MINIMAX_IDEMPOTENCY_KEY}" \
  --data '{
    "model": "MiniMax-H3",
    "content": [
      {
        "type": "text",
        "text": "电影感航拍，一艘帆船穿过晨雾，镜头缓慢向前推进"
      }
    ],
    "resolution": "768P",
    "duration": 6,
    "ratio": "16:9"
  }'
```

### 参考图示例

```bash
export MINIMAX_IDEMPOTENCY_KEY="video-order-20260826-002"

curl --request POST "${BASE_URL}/v2/video_generation" \
  --header "Authorization: Bearer ${API_KEY}" \
  --header "Content-Type: application/json" \
  --header "Idempotency-Key: ${MINIMAX_IDEMPOTENCY_KEY}" \
  --data '{
    "model": "MiniMax-H3",
    "content": [
      {
        "type": "text",
        "text": "保持人物外观，微风吹动头发，镜头轻微环绕"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://media.example.com/portrait.png"
        },
        "role": "reference_image"
      }
    ],
    "resolution": "768P",
    "duration": 8,
    "ratio": "9:16"
  }'
```

### 参考图加音频示例

```bash
export MINIMAX_IDEMPOTENCY_KEY="video-order-20260826-003"

curl --request POST "${BASE_URL}/v2/video_generation" \
  --header "Authorization: Bearer ${API_KEY}" \
  --header "Content-Type: application/json" \
  --header "Idempotency-Key: ${MINIMAX_IDEMPOTENCY_KEY}" \
  --data '{
    "model": "MiniMax-H3",
    "content": [
      {
        "type": "text",
        "text": "人物自然说话，保持参考图中的外观和构图"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://media.example.com/portrait.png"
        },
        "role": "reference_image"
      },
      {
        "type": "audio_url",
        "audio_url": {
          "url": "https://media.example.com/voice.mp3"
        },
        "role": "reference_audio"
      }
    ],
    "resolution": "768P",
    "duration": 8,
    "ratio": "16:9"
  }'
```

### 创建响应

创建成功返回 HTTP 200：

```json
{
  "task_id": "task_xxx"
}
```

保存 `task_id`，随后调用查询接口。响应在幂等重放或提交状态不明确时可能包含：

- `Location: /v2/query/video_generation/task_xxx`
- `Retry-After: 2`
- `Idempotency-Replayed: true`

`Retry-After` 存在时应优先按该秒数等待。

## 查询与轮询

```bash
curl --request GET \
  "${BASE_URL}/v2/query/video_generation/task_xxx" \
  --header "Authorization: Bearer ${API_KEY}"
```

任务只能由同一用户的令牌读取，不要求必须是创建任务时的那一个令牌。查询窗口为任务创建后的 7 天。任务不存在、属于其他用户或已超出查询窗口时，当前接口返回 HTTP 400 的 MiniMax 错误结构。

任务状态：

| `status` | 含义 | 是否终态 |
| --- | --- | --- |
| `queued` | 已排队 | 否 |
| `running` | 正在生成 | 否 |
| `succeeded` | 生成成功 | 是 |
| `failed` | 生成失败 | 是 |
| `cancelled` | 任务已取消 | 是 |

成功响应示例：

```json
{
  "task": {
    "id": "task_xxx",
    "model": "MiniMax-H3",
    "status": "succeeded",
    "created_at": 1787688000,
    "updated_at": 1787688060,
    "content": {
      "url": "https://api.opwan.ai/v1/videos/task_xxx/content"
    },
    "resolution": "768P",
    "duration": 8,
    "usage": {
      "total_seconds": 8,
      "input_seconds": 0,
      "output_seconds": 8,
      "input_image_count": 1
    },
    "ratio": "16:9",
    "task_type": "generation",
    "modality": "video"
  }
}
```

`task.content.url` 是经过网关代理的视频地址，不包含生成服务的真实地址。下载时必须携带同一用户的 Bearer Token；请在 7 天查询窗口内保存结果。

失败任务会包含：

```json
{
  "task": {
    "id": "task_xxx",
    "model": "MiniMax-H3",
    "status": "failed",
    "created_at": 1787688000,
    "updated_at": 1787688060,
    "error": {
      "code": "generation_failed",
      "message": "Video generation failed"
    },
    "task_type": "generation",
    "modality": "video"
  }
}
```

Python 创建并轮询示例：

```python
import os
import time

import requests

base_url = os.environ.get("BASE_URL", "https://api.opwan.ai")
api_key = os.environ["API_KEY"]
idempotency_key = os.environ["MINIMAX_IDEMPOTENCY_KEY"]
if not idempotency_key or len(idempotency_key) > 256:
    raise RuntimeError(
        "MINIMAX_IDEMPOTENCY_KEY must be a stable value of at most 256 characters"
    )
headers = {
    "Authorization": f"Bearer {api_key}",
    "Content-Type": "application/json",
    "Idempotency-Key": idempotency_key,
}
payload = {
    "model": "MiniMax-H3",
    "content": [
        {
            "type": "text",
            "text": "电影感航拍，一艘帆船穿过晨雾",
        }
    ],
    "resolution": "768P",
    "duration": 6,
    "ratio": "16:9",
}

submit_deadline = time.monotonic() + 5 * 60
while True:
    try:
        created = requests.post(
            f"{base_url}/v2/video_generation",
            headers=headers,
            json=payload,
            timeout=120,
        )
    except (requests.ConnectionError, requests.Timeout) as error:
        if time.monotonic() >= submit_deadline:
            raise TimeoutError(
                "Submission remained uncertain; rerun with the same idempotency key"
            ) from error
        time.sleep(5)
        continue

    if created.status_code == 429:
        if time.monotonic() >= submit_deadline:
            raise TimeoutError(
                "Submission remained rate-limited; rerun with the same idempotency key"
            )
        time.sleep(max(int(created.headers.get("Retry-After", "15")), 1))
        continue

    created.raise_for_status()
    break

task_id = created.json()["task_id"]
delay = int(created.headers.get("Retry-After", "15"))

deadline = time.monotonic() + 15 * 60
while time.monotonic() < deadline:
    time.sleep(delay)
    response = requests.get(
        f"{base_url}/v2/query/video_generation/{task_id}",
        headers={"Authorization": f"Bearer {api_key}"},
        timeout=30,
    )
    if response.status_code == 429:
        delay = max(int(response.headers.get("Retry-After", "15")), 1)
        continue
    response.raise_for_status()
    task = response.json()["task"]

    if task["status"] == "succeeded":
        print(task["content"]["url"])
        break
    if task["status"] in {"failed", "cancelled"}:
        raise RuntimeError(task.get("error", task["status"]))
else:
    raise TimeoutError(f"task {task_id} did not finish in 15 minutes")
```

## 幂等重试

`Idempotency-Key` 是可选请求头，最长 256 个字符。生产客户端应为每个业务操作生成唯一键：

- 网络超时或连接中断时，使用同一用户、同一个键和完全相同的请求重试。
- 命中已有任务时返回原 `task_id`，并可能带 `Idempotency-Replayed: true`，不会再次创建任务。
- 同一个键对应不同的规范化请求时返回 HTTP 409。
- 不传该头时，不提供重复提交保护。

若创建响应是 HTTP 200 且带 `Retry-After`，任务已经被系统记录，不要换新键重复提交；应保存 `task_id` 并查询状态。

## 错误格式

创建和查询接口的请求级错误使用 MiniMax 风格结构：

```json
{
  "type": "error",
  "error": {
    "type": "bad_request_error",
    "message": "Invalid request",
    "http_code": "400"
  },
  "request_id": "req_xxx"
}
```

`error.http_code` 是字符串。常见状态码：

| HTTP | 含义 |
| --- | --- |
| `400` | 参数无效，或任务不存在、无权访问、超过 7 天查询窗口 |
| `401` / `403` | API 令牌缺失、无效或认证失败 |
| `402` | 余额不足 |
| `409` | 幂等键已用于不同请求 |
| `413` | JSON 请求体超过 64 MiB |
| `422` | 生成服务明确拒绝任务提交 |
| `429` | 请求过于频繁 |
| `500` | 内部服务错误 |
| `502` | 生成服务响应异常 |
| `503` | 服务暂时不可用 |
| `529` | 服务过载 |

请求级错误与任务失败不同：任务已经创建后若生成失败，查询接口仍返回 HTTP 200，并在 `task.status: "failed"` 和 `task.error` 中说明失败。

## 计费

`MiniMax-H3` 按创建请求的 `duration` 秒数计费，单价以模型广场实时展示为准。例如请求 `duration: 8`，计费乘数就是 8 秒。幂等重放返回原任务，不重复创建任务。

完整机器可读定义见 [`docs/openapi/relay.json`](openapi/relay.json) 中的 `/v2/video_generation` 和 `/v2/query/video_generation/{task_id}`。
