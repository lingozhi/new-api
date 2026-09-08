/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export const VIDEO_WEBHOOK_EXAMPLE = {
  webhook_url: 'https://your-app.example/webhooks/video',
  webhook_secret: '<YOUR_WEBHOOK_SECRET>',
}

export const VIDEO_WEBHOOK_RESPONSE = {
  id: 'task_example',
  task_id: 'task_example',
  model: '<REQUESTED_MODEL>',
  status: 'completed',
  progress: 100,
  content_url: '/v1/videos/task_example/content',
}

export function videoWebhookGuide(): string {
  return [
    '## Optional website webhooks (polling remains available)',
    '- webhook_url: optional string, maximum 2048 bytes, public HTTPS endpoint on port 443 without embedded credentials. Private destinations are rejected. Omit the field when unused; null/empty is rejected.',
    '- webhook_secret: optional string, maximum 512 bytes; requires webhook_url. Use a strong independent secret, not the website API key. Omitted/empty means no signature. Null is rejected.',
    '- These are WEBSITE fields. They are persisted in the gateway outbox and removed before forwarding to the provider. No provider callback support is required.',
    '- Merge these two optional fields into ONE of the valid creation requests:',
    '```json',
    JSON.stringify(VIDEO_WEBHOOK_EXAMPLE, null, 2),
    '```',
    '- The website sends POST application/json after completed or failed. No intermediate progress events. It continues polling the provider internally. You can still query the original task ID.',
    '- Success body:',
    '```json',
    JSON.stringify(VIDEO_WEBHOOK_RESPONSE, null, 2),
    '```',
    '- model is your public requested model, not the placeholder above. On failure status=failed, error={"message":"..."}, and content_url is absent. id and task_id are the same public ID; progress=100 means terminal, not necessarily successful.',
    '- content_url is a relative path on the WEBSITE API origin. Download it with the website API key after success; never send that key to the webhook endpoint.',
    '- Headers: X-Webhook-Delivery-Id (stable public task ID), X-Webhook-Timestamp (Unix seconds), X-Webhook-Signature (only when secret is non-empty).',
    '- Signature: v1=<lowercase hexadecimal HMAC-SHA256>. Sign the exact bytes of "v1." + timestamp + "." + delivery_id + "." + RAW_HTTP_BODY with webhook_secret. Verify before parsing JSON using a constant-time comparison, reject timestamps outside a short tolerance (e.g. 5 minutes), and deduplicate the Delivery ID atomically.',
    '- Reply with HTTP 2xx promptly after durably recording the event. Redirects are rejected. Timeout is 15 seconds per delivery; failures get at most 5 total attempts with 30/60/120/240-second retry delays, subject to scheduler timing. Delivery is at least once and retries reuse Delivery ID.',
    '- Callback delivery failure does not change generation status or create another paid task. Keep polling as fallback. Do not retry creation just because a callback is missing.',
  ].join('\n')
}
