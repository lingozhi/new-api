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
            'Use one public model, wan3.0. The server selects the standard, fast, reference or frame workflow from mode and speed. Your client does not need the four upstream model IDs.'
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
            'mode defaults to auto: frame fields select frames, otherwise general. general supports standard (default) or fast speed. reference requires reference lists and supports fast only. frames requires both first_frame and last_frame and supports standard only. Unsupported combinations are rejected.'
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
            'general and reference accept the same reference lists: up to 10 images, 5 videos and 5 audios. general allows image roles reference_image, first_frame and last_frame; reference allows reference_image only. Optional reference video duration is supported only in general mode. All media URLs must be publicly accessible HTTPS URLs without credentials.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Frames cannot be mixed with reference lists. Do not send media: the server builds it internally. n is optional and must be 1; prompt_extend is an optional boolean, and false disables prompt enhancement. Streaming, callbacks, uploads, seed and other undocumented parameters are not supported by this unified interface.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Wan uses USD per second at the effective resolution and group price. Failed tasks refund the website reservation. The four legacy model IDs retain their original request formats for compatibility; new integrations should use wan3.0.'
          )}
        </p>
      </section>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Wan currently charges the requested output seconds. Reference video duration is not added to the website charge, and actual output duration does not change the final charge. Failed tasks are refunded. The 3600-second reference duration bound is a gateway safety limit, not a provider capability guarantee.'
        )}
      </p>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Download after completion. Earlier requests return 409. Range and If-Range support resumable downloads with 206; an unsatisfiable range returns 416. The provider normally retains results for 48 hours, but may change this period. Download promptly; permanent storage is not guaranteed.'
        )}
      </p>
      <section className='space-y-3'>
        <h3 className='text-sm font-semibold'>{t('Example')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'These are four alternative JSON requests: standard, fast references, R2V references and first/last frames. Replace media URL placeholders with real files. Do not merge all optional fields into one request.'
          )}
        </p>
        {WAN_EXAMPLES.map((request) => (
          <CodeBlock
            key={`${request.mode}-${request.speed}`}
            language='json'
            code={JSON.stringify(request, null, 2)}
          >
            <CodeBlockCopyButton />
          </CodeBlock>
        ))}
      </section>
      <section className='space-y-3'>
        <h3 className='text-sm font-semibold'>{t('Generate and download')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Create returns HTTP 200; save the public id. Query states are queued, in_progress, completed and failed. Poll every 10–15 seconds with the same token, then download content after completed. On timeouts resume the saved ID instead of creating another paid task. Python requires requests, NEW_API_BASE_URL and NEW_API_KEY.'
          )}
        </p>
        <CodeBlock language='python' code={wanPythonExample()}>
          <CodeBlockCopyButton />
        </CodeBlock>
      </section>
    </div>
  )
}
