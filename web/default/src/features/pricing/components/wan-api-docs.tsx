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
import { Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { StaticDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import {
  WAN_PARAMETERS,
  WAN_EXAMPLES,
  wanPythonExample,
  buildWanAiIntegrationGuide,
} from '../lib/wan-api-docs'
import { VideoWebhookDocs } from './video-webhook-docs'

export function WanApiDocs() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const configured = (status as Record<string, unknown> | null)?.server_address
  const baseUrl =
    typeof configured === 'string' && configured
      ? configured.replace(/\/$/, '')
      : window.location.origin
  const copyGuide = async () => {
    const copied = await copyToClipboard(buildWanAiIntegrationGuide(baseUrl))
    if (copied) toast.success(t('Copied to clipboard'))
    else toast.error(t('Failed to copy'))
  }
  return (
    <div className='space-y-6'>
      <section className='space-y-3'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <h3 className='text-sm font-semibold'>Wan 3.0 API</h3>
          <Button type='button' variant='outline' size='sm' onClick={copyGuide}>
            <Copy aria-hidden='true' className='size-3.5' />
            {t('Copy AI integration guide')}
          </Button>
        </div>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Use model wan3.0 with a website key in the official group. Supports text, reference media and first/last frames.'
          )}
        </p>
        <CodeBlock
          language='text'
          code={`Authorization: Bearer <NEW_API_KEY>\nContent-Type: application/json\nPOST ${baseUrl}/v1/videos\nGET ${baseUrl}/v1/videos/{id}\nGET ${baseUrl}/v1/videos/{id}/content`}
        >
          <CodeBlockCopyButton />
        </CodeBlock>
        <p className='text-muted-foreground text-sm'>
          {t(
            'mode defaults to auto: frame fields select frames, otherwise general. reference requires at least one reference image, video or audio; frames requires both first_frame and last_frame. speed is not supported; omit it.'
          )}
        </p>
      </section>
      <section className='space-y-3'>
        <h3 className='text-sm font-semibold'>{t('Supported parameters')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Use a website key in the official group. model and non-empty prompt are required. seconds is an integer string or use the integer duration alias: 2–30 seconds, default 5. Resolution defaults to 720P; aspect ratio defaults to 16:9. Aliases must agree. Omit unused fields; null and unknown fields are rejected.'
          )}
        </p>
        <StaticDataTable
          data={WAN_PARAMETERS}
          getRowKey={(row) => row.name}
          columns={[
            {
              id: 'name',
              header: t('Parameter'),
              cell: (row) => <code className='break-all'>{row.name}</code>,
            },
            {
              id: 'type',
              header: t('Type'),
              cell: (row) => <code>{row.type}</code>,
            },
            {
              id: 'value',
              header: t('Default / allowed values'),
              cell: (row) => (
                <code className='break-all'>
                  {row.value.replace('default:', `${t('Default')}:`)}
                </code>
              ),
            },
          ]}
        />
        <p className='text-muted-foreground text-sm'>
          {t(
            'general and reference accept up to 10 images, 5 videos and 5 audios. general allows image roles reference_image, first_frame and last_frame; reference allows reference_image only. reference_videos entries accept url only; duration metadata is rejected. Use public HTTPS URLs without credentials that remain accessible until completion. File formats and actual media limits are checked by the provider.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Frames cannot be mixed with reference lists. Do not send media: the server builds it internally. n is optional and must be 1; prompt_extend is an optional boolean, and false disables prompt enhancement. Streaming, uploads, seed and other undocumented parameters are not supported by this unified interface.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Wan uses USD per second at the effective resolution and group price. Use wan3.0 for all modes.'
          )}
        </p>
      </section>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Wan charges the requested output seconds at the current website price. Input video length and actual output length do not change the final charge. Failed tasks refund the website reservation.'
        )}
      </p>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Download after completion; earlier requests return 409. Range and If-Range support partial downloads with 206; an unsatisfiable range returns 416, and an If-Range mismatch may return the full file with 200. Download promptly: the current channel has no verified retention period or permanent storage guarantee.'
        )}
      </p>
      <section className='space-y-3'>
        <h3 className='text-sm font-semibold'>{t('Example')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'These are four alternative JSON requests: text, general with a reference image, reference media, and first/last frames. Replace media URL placeholders with real files. Do not merge all optional fields into one request.'
          )}
        </p>
        {WAN_EXAMPLES.map((request) => (
          <CodeBlock
            key={String(request.mode)}
            language='json'
            code={JSON.stringify(request, null, 2)}
          >
            <CodeBlockCopyButton />
          </CodeBlock>
        ))}
      </section>
      <VideoWebhookDocs />
      <section className='space-y-3'>
        <h3 className='text-sm font-semibold'>{t('Generate and download')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Create returns HTTP 200; save id. Creation status may be pending. GET query states are queued, in_progress, completed and failed. Progress and error details may be absent; timestamps may be strings. Poll every 10–15 seconds with the same token and download after completed. If creation times out without an ID, check task logs before retrying; otherwise resume the saved ID. Python requires requests, NEW_API_BASE_URL (origin without /v1) and NEW_API_KEY.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Check HTTP status first. Invalid parameters return 400 with code and message; authentication and download errors may use error.message. Correct the input, key, group or balance before retrying. The copied AI guide includes error handling and commands to resume a saved task.'
          )}
        </p>
        <CodeBlock language='python' code={wanPythonExample()}>
          <CodeBlockCopyButton />
        </CodeBlock>
      </section>
    </div>
  )
}
