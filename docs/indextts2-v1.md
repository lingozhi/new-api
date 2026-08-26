# IndexTTS2 v1 文本转语音接口

`indextts2-v1` 通过 `POST /v1/audio/speech` 接入，并开放模型的完整文本、音色参考、情绪参考和情绪数值参数。请求必须是 JSON，并必须带 `Idempotency-Key`。

IndexTTS2 使用参考音频克隆音色：`prompt_simple` 必须是 WAV/MP3 音频来源，不能传 `alloy`、`nova` 等命名音色。旧版的 `input`、`voice` 和 `metadata` 参数仍兼容，但新接入应优先使用本文列出的完整模型参数。

## 请求

| 项目 | 值 |
| --- | --- |
| Method | `POST` |
| Path | `/v1/audio/speech` |
| Auth | `Authorization: Bearer <API_KEY>` |
| Content-Type | `application/json` |
| 幂等头 | `Idempotency-Key: <UNIQUE_KEY>`，必填，最多 256 字符 |

最小请求：

```json
{
  "model": "indextts2-v1",
  "prompt_text": "你好，欢迎使用 IndexTTS2。",
  "prompt_simple": "https://media.example.com/reference.wav",
  "emo_control_method": "与音色参考音频相同"
}
```

完整示例：

```json
{
  "model": "indextts2-v1",
  "emo_sad": 0,
  "emo_calm": 0.3,
  "emo_angry": 0,
  "emo_happy": 0.5,
  "emo_afraid": 0,
  "emo_random": false,
  "prompt_text": "你好，这是一段测试文本",
  "emo_disgusted": 0,
  "emo_ref_audio": "https://media.example.com/emotion.mp3",
  "emo_surprised": 0,
  "prompt_simple": "https://media.example.com/reference.wav",
  "emo_melancholic": 0,
  "emo_control_method": "使用情感参考音频"
}
```

调用示例：

```bash
export BASE_URL="https://api.opwan.ai"
export API_KEY="sk-your-api-key"

curl --request POST "${BASE_URL}/v1/audio/speech" \
  --header "Authorization: Bearer ${API_KEY}" \
  --header "Content-Type: application/json" \
  --header "Idempotency-Key: tts-order-20260826-001" \
  --data '{
    "model": "indextts2-v1",
    "prompt_text": "你好，欢迎使用 IndexTTS2。",
    "prompt_simple": "https://media.example.com/reference.wav",
    "emo_control_method": "与音色参考音频相同"
  }' \
  --dump-header response.headers \
  --output response.body
```

`response.body` 可能是 WAV，也可能是 HTTP 202 的 JSON。客户端应先读取状态码或 `Content-Type` 再处理文件。

### 请求字段

| 字段 | 类型 | 必填 | 约束 |
| --- | --- | --- | --- |
| `model` | string | 是 | 固定为 `indextts2-v1` |
| `emo_afraid` | number | 否 | 恐惧情绪强度，`0`–`1.4`，默认 `0` |
| `emo_angry` | number | 否 | 愤怒情绪强度，`0`–`1.4`，默认 `0` |
| `emo_calm` | number | 否 | 平静情绪强度，`0`–`1.4`，默认 `0` |
| `emo_control_method` | string | 是 | `与音色参考音频相同`、`使用情感参考音频`、`使用情感向量控制` |
| `emo_disgusted` | number | 否 | 厌恶情绪强度，`0`–`1.4`，默认 `0` |
| `emo_happy` | number | 否 | 开心情绪强度，`0`–`1.4`，默认 `0` |
| `emo_melancholic` | number | 否 | 忧郁情绪强度，`0`–`1.4`，默认 `0` |
| `emo_random` | boolean | 否 | 是否使用随机情绪，默认 `false` |
| `emo_ref_audio` | string | 否 | 情绪参考音频；来源和安全限制与 `prompt_simple` 相同 |
| `emo_sad` | number | 否 | 悲伤情绪强度，`0`–`1.4`，默认 `0` |
| `emo_surprised` | number 或 string | 否 | 当前只能为 `0` 或 `"0"` |
| `prompt_simple` | string | 是 | 音色参考音频；公网 HTTPS WAV/MP3 URL 或允许的 base64 data URI |
| `prompt_text` | string | 是 | 1–2048 个 Unicode 字符，不能只有空白 |

`instructions` 不受支持，`stream_format: "sse"` 不受支持。输出固定为 WAV。

### 兼容参数

已有客户端可以继续使用：

| 兼容参数 | 映射/约束 |
| --- | --- |
| `input` | `prompt_text` 的别名；两者同时提供时内容必须一致 |
| `voice` | `prompt_simple` 的别名；两者同时提供时内容必须一致 |
| `response_format` | 只能为 `wav` |
| `speed` | 只能为 `1` |
| `metadata.emotion_audio` | 映射为 `emo_ref_audio`，并选择情绪参考音频模式 |
| `metadata.emotion_vector` | 8 项数组，依次映射 `happy`、`angry`、`sad`、`afraid`、`disgusted`、`melancholic`、`surprised`、`calm` |
| `metadata.emotion_random` | 映射为 `emo_random` |

`metadata` 情绪别名不能与顶层 `emo_*` 参数混用。

### 参考音频

`prompt_simple` 和 `emo_ref_audio` 接受两种来源：

1. 公网 `https://` URL。只能使用默认 HTTPS 端口 443，不允许 URL userinfo；目标必须返回可验证的 WAV 或 MP3。
2. 严格 base64 data URI。允许的 MIME 为 `audio/wav`、`audio/x-wav`、`audio/mpeg`、`audio/mp3`，格式必须为 `data:<MIME>;base64,<DATA>`。

限制：

- 每段音频解码后最大 15 MiB。
- `prompt_simple` 与 `emo_ref_audio` 解码后合计最大 30 MiB。
- 每段音频最长 10 分钟。
- 整个 JSON 请求体最大 64 MiB。
- 输出必须是有效 WAV，最大 64 MiB。

### 情绪控制

`emo_control_method` 决定实际情绪来源：

- `与音色参考音频相同`：沿用 `prompt_simple` 中的情绪。
- `使用情感参考音频`：使用 `emo_ref_audio`；其他情绪数值仍可随请求发送。
- `使用情感向量控制`：使用 `emo_happy`、`emo_angry`、`emo_sad`、`emo_afraid`、`emo_disgusted`、`emo_melancholic`、`emo_calm` 和 `emo_random`。

七个可调情绪数值的范围均为 `0`–`1.4`。`emo_surprised` 是当前保留参数，只能为 `0`。所有可选情绪数值省略时按 `0` 处理，`emo_random` 省略时按 `false` 处理。

## 响应

### 200：生成完成

```http
HTTP/1.1 200 OK
Content-Type: audio/wav
X-New-Api-Task-ID: task_xxx
Location: /v1/audio/speech/task_xxx
```

响应体是 WAV 二进制。客户端不得按 JSON 解析。

### 202：任务仍在运行

如果同步等待时间内未完成，接口返回：

```http
HTTP/1.1 202 Accepted
Content-Type: application/json
X-New-Api-Task-ID: task_xxx
Location: /v1/audio/speech/task_xxx
Retry-After: 2
```

```json
{
  "task_id": "task_xxx",
  "status": "in_progress"
}
```

按 `Retry-After` 等待后，用创建任务时的同一个 API 令牌请求 `Location`：

```bash
curl --request GET "${BASE_URL}/v1/audio/speech/task_xxx" \
  --header "Authorization: Bearer ${API_KEY}" \
  --dump-header recovery.headers \
  --output recovery.body
```

恢复接口仍会返回 200 WAV 或 202 JSON。任务查询窗口为创建后 7 天；任务不存在、超出窗口或 API 令牌不匹配时返回 404。
如果恢复请求返回 429，请按 `Retry-After` 等待后重试同一个恢复地址。

POST 在任务已经受理、但生成结果下载触发限流时，也可能直接返回 `429 + Location + Retry-After`。此时不要重新提交任务；应按 `Retry-After` 等待，再使用同一个 API 令牌 GET `Location`。POST 429 未包含 `Location` 时，才按普通提交限流处理。

## 幂等重试

- 初次请求为每个业务操作生成唯一的 `Idempotency-Key`。
- 网络超时或连接中断时，使用同一个 API 令牌、同一个键和同一份请求重试。
- 命中已有任务时响应包含 `Idempotency-Replayed: true`，不会重复创建任务或重复扣费。
- 同一个键对应不同的规范化请求时返回 HTTP 409，错误码为 `idempotency_conflict`。

不要在重试时修改文本、参考音频、情绪参数或其他请求字段。若业务输入变化，应生成新键。

## 错误格式

错误使用 OpenAI 风格结构：

```json
{
  "error": {
    "message": "Idempotency-Key is required for indextts2-v1",
    "type": "invalid_request_error",
    "code": "idempotency_key_required"
  }
}
```

常见状态码：

| HTTP | 含义 |
| --- | --- |
| `400` | 参数无效、缺少幂等键或幂等键超过 256 字符 |
| `401` | API 令牌缺失或无效 |
| `404` | 恢复任务不存在、已过期或令牌无权读取 |
| `409` | 幂等键已用于不同请求 |
| `413` | JSON 请求体超过 64 MiB 上限 |
| `415` | IndexTTS2 请求不是 `application/json` |
| `429` | 触发提交、任务读取、下载或模型请求限流；若带 `Location`，按已受理任务恢复 |
| `500` | 幂等处理、任务存储或查询等内部服务失败 |
| `502` | 生成任务失败，或结果不是可安全返回的有效 WAV |

## 计费

`indextts2-v1` 按成功提交的任务次数计费，当前单价以模型广场展示为准。幂等重放不会再次计费。请求校验失败不会提交任务；服务已经受理后的任务失败不会退回按次费用。

完整机器可读定义见 [`docs/openapi/relay.json`](openapi/relay.json) 中的 `/v1/audio/speech` 与 `/v1/audio/speech/{task_id}`。
