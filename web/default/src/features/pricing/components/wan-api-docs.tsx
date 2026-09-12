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
  wanExamples,
  wanPythonExample,
  buildWanAiIntegrationGuide,
} from '../lib/wan-api-docs'
import { VideoWebhookDocs } from './video-webhook-docs'

export function WanApiDocs(props: { modelName: string }) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const configured = (status as Record<string, unknown> | null)?.server_address
  const baseUrl =
    typeof configured === 'string' && configured
      ? configured.replace(/\/$/, '')
      : window.location.origin
  const copyGuide = async () => {
    const copied = await copyToClipboard(
      buildWanAiIntegrationGuide(baseUrl, props.modelName)
    )
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
            'Use wan3.0-video-prime for the high-speed version, or wan3.0-video (wan3.0 alias) for standard. Both use the official input, parameters and media structure. Use a website key in the official group and a channel authorized for the selected model.'
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
            'Choose the workflow through input.media[].type and prompt: text, first frame, first/last frames, references, document, webpage, video editing or extension. No mode or speed field.'
          )}
        </p>
      </section>
      <p className='text-muted-foreground text-sm'>
        <a
          className='underline underline-offset-4'
          href='https://docs.bailian.console.aliyun.com/zh/model-studio/wan3-video-generation-api-reference'
          target='_blank'
          rel='noopener noreferrer'
        >
          {t('Wan 3.0 API Reference')}
        </a>
        {' · '}
        <a
          className='underline underline-offset-4'
          href='https://docs.bailian.console.aliyun.com/zh/model-studio/wan3-video-generation-guide'
          target='_blank'
          rel='noopener noreferrer'
        >
          {t('Wan 3.0 Generation Guide')}
        </a>
      </p>
      <section className='space-y-3'>
        <h3 className='text-sm font-semibold'>{t('Supported parameters')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'model and input are required; provide input.prompt or input.media. Defaults: 1080P, adaptive ratio, 5 seconds. For a low-cost test, explicitly set 480P and 2 seconds. Omit unused fields; null and unknown fields are rejected.'
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
            'Up to 20 media items: 10 reference images, 5 videos and 5 audios; video and audio totals are each limited to 15 seconds. One first_frame may include one last_frame; frames cannot mix with other media. One file or one link may accompany references.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Use public HTTP(S) URLs; images also accept Base64 data URLs up to 20 MB. This channel does not accept DashScope OSS URLs. The provider checks file formats, dimensions, page counts and actual durations. See the copied guide for all media limits.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'parameters supports audio, seed, prompt_extend and watermark. false and seed=0 are preserved. Input video seconds plus output seconds must not exceed 30; duration=-1 selects automatic length.'
          )}
        </p>
      </section>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Fixed duration charges requested output seconds. Automatic duration reserves 30 seconds and refunds the difference using delivered duration; missing duration settles at the 2-second minimum and is logged. Failed tasks refund the reservation. See current resolution and group prices.'
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
            'Eight separate examples: text, first frame, first/last frames, mixed references, document, webpage, editing and extension. Each uses 2-second 480P output. Replace media URLs; each POST creates a paid task.'
          )}
        </p>
        {wanExamples(props.modelName).map((request) => (
          <CodeBlock
            key={request.input.prompt}
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
        <CodeBlock language='python' code={wanPythonExample(props.modelName)}>
          <CodeBlockCopyButton />
        </CodeBlock>
      </section>
    </div>
  )
}
