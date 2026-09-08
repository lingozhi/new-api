# Wan and Lxmone Seedance webhooks

Supported public models: wan3.0, seedance-2, seedance-2.5, seedance-2-fast, seedance-2-mini.

The legacy Seedance `POST /v1/videos/generations` interface rejects `webhook_url` and `webhook_secret` with HTTP 400 (`unsupported_webhook`) before billing or upstream submission, including null or empty fields. Omit both fields for legacy requests. Website callbacks require the supported models and channels through `POST /v1/videos`.

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
