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
import { useTranslation } from 'react-i18next'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'

import {
  VIDEO_WEBHOOK_EXAMPLE,
  VIDEO_WEBHOOK_RESPONSE,
} from '../lib/video-webhook-docs'

export function VideoWebhookDocs() {
  const { t } = useTranslation()
  return (
    <section className='space-y-3'>
      <h3 className='text-sm font-semibold'>Webhook</h3>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Optionally add webhook_url and webhook_secret to a valid creation request. Use a public HTTPS endpoint on port 443. URL limit: 2048 bytes; secret limit: 512 bytes. The secret requires a URL. Omit unused fields; null is rejected. These fields stay on the website and are not forwarded upstream.'
        )}
      </p>
      <CodeBlock
        language='json'
        code={JSON.stringify(VIDEO_WEBHOOK_EXAMPLE, null, 2)}
      >
        <CodeBlockCopyButton />
      </CodeBlock>
      <p className='text-muted-foreground text-sm'>
        {t(
          'The website sends a JSON POST after completion or failure. The callback contains your public model and task ID. On failure, status is failed and error.message replaces content_url. Download the relative content_url from this website with your API key. Polling remains available.'
        )}
      </p>
      <CodeBlock
        language='json'
        code={JSON.stringify(VIDEO_WEBHOOK_RESPONSE, null, 2)}
      >
        <CodeBlockCopyButton />
      </CodeBlock>
      <CodeBlock
        language='text'
        code={
          'X-Webhook-Delivery-Id: task_example\nX-Webhook-Timestamp: <unix_seconds>\nX-Webhook-Signature: v1=<HMAC-SHA256 hex>\n\nHMAC-SHA256(webhook_secret, "v1." + timestamp + "." + delivery_id + "." + raw_body)'
        }
      >
        <CodeBlockCopyButton />
      </CodeBlock>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Use a separate webhook secret to verify the signature against the raw body, check the timestamp, and deduplicate Delivery ID. Without a secret there is no signature. Return 2xx promptly; redirects fail. Delivery times out after 15 seconds and allows 5 attempts with 30/60/120/240-second retry delays. Callback failure does not change the video result; query the saved ID as fallback.'
        )}
      </p>
    </section>
  )
}
