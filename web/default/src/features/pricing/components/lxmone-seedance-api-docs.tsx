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
import { useState } from 'react'
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
  buildLxmoneSeedanceAiIntegrationGuide,
  lxmoneSeedanceRequest,
  lxmoneSeedancePythonExample,
} from '../lib/lxmone-seedance-api-docs'

export function LxmoneSeedanceApiDocs(props: { modelName: string }) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [copying, setCopying] = useState(false)
  const configured = (status as Record<string, unknown> | null)?.server_address
  const baseUrl =
    typeof configured === 'string' && configured
      ? configured.replace(/\/$/, '')
      : window.location.origin
  const copyGuide = async () => {
    setCopying(true)
    try {
      const copied = await copyToClipboard(
        buildLxmoneSeedanceAiIntegrationGuide(props.modelName, baseUrl)
      )
      if (copied) {
        toast.success(t('Copied to clipboard'))
      } else {
        toast.error(t('Failed to copy'))
      }
    } catch {
      toast.error(t('Failed to copy'))
    } finally {
      setCopying(false)
    }
  }
  const fast =
    props.modelName === 'seedance-2-fast' ||
    props.modelName === 'seedance-2-mini'
  const v25 = props.modelName === 'seedance-2.5'
  let duration = '4–15'
  let resolutions = '480p | 720p | 1080p | 4k'
  if (fast) {
    duration = '5 | 10'
    resolutions = '480p | 720p'
  } else if (v25) {
    duration = '4–30'
    resolutions = '480p | 720p | 1080p'
  }
  const parameters = [
    { name: 'model *', value: props.modelName },
    { name: 'prompt *', value: 'string' },
    { name: 'seconds / duration', value: `${duration}; ${t('Default')}: 5` },
    {
      name: 'resolution / size',
      value: `${resolutions}; ${t('Default')}: 720p`,
    },
    {
      name: 'aspect_ratio / ratio',
      value: `16:9 | 9:16 | 1:1; ${t('Default')}: 16:9`,
    },
    {
      name: 'input_reference / image',
      value: '"https://example.com/first.jpg"',
    },
    {
      name: 'image_end / end_image_url',
      value: '"https://example.com/last.jpg"',
    },
    {
      name: 'reference_images',
      value: '["https://example.com/reference.jpg"]',
    },
    {
      name: 'reference_videos',
      value: '["https://example.com/reference.mp4"]',
    },
    {
      name: 'reference_audios',
      value: '["https://example.com/reference.mp3"]',
    },
  ]
  parameters.push({
    name: 'sound_effects / no_music',
    value: 'boolean; false / true',
  })
  const request = lxmoneSeedanceRequest(props.modelName)
  const rules = [
    t(
      'Use a website API key in the official group. Send JSON to POST /v1/videos; the model names below are public website names.'
    ),
    t(
      'Fields marked * are required. seconds accepts an integer or integer string; duration accepts an integer. Aliases must agree. Resolution is case-insensitive.'
    ),
    t(
      'input_reference or image supplies the first frame; image_end or end_image_url supplies the last frame and requires a first frame. Frame inputs cannot be mixed with reference lists.'
    ),
    t(
      'Reference inputs use publicly accessible media URLs. Seedance 2, Fast and Mini allow 9 images, 3 videos and 3 audios; Seedance 2.5 allows 30 images, 10 videos and 10 audios. Media validity and limits are checked by the provider.'
    ),
    t(
      'Seedance 2, Fast and Mini require a reference image or video when reference audio is supplied. Seedance 2.5 also accepts reference audio alone.'
    ),
    t(
      'Do not send stream, n, response_format or webhook_url. generate_audio, seed, negative_prompt, file_id and the old channel upload API are not documented for this channel.'
    ),
    t(
      'Fast and Mini normalize 1080p to 720p and reject 4K. Seedance 2.5 normalizes 4K to 1080p. Billing uses the effective resolution and USD per second; failed tasks are refunded. See the current model price.'
    ),
  ]
  rules.push(
    t(
      'Reference arrays accept URL strings or objects containing only url; the gateway sends URL strings upstream. Frame aliases must agree. sound_effects=false or no_music=true disables generated sound effects. If both are supplied, their boolean values must be opposite.'
    )
  )
  rules.push(
    t(
      'Use @Image1, @Video1 and @Audio1 to reference media in array order. Seedance 2/Fast/Mini reference videos are typically 2–15 seconds and up to 50 MB; audio is typically 2–15 seconds and up to 15 MB. Seedance 2.5 reference videos are typically 2–30 seconds and up to 200 MB. The provider checks actual media compatibility.'
    )
  )
  rules.push(
    t(
      'Download after completion. Earlier requests return 409. Range and If-Range support resumable downloads with 206; an unsatisfiable range returns 416. The provider normally retains results for 48 hours, but may change this period. Download promptly; permanent storage is not guaranteed.'
    )
  )
  return (
    <div className='space-y-6'>
      <section className='space-y-3'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <h3 className='text-sm font-semibold'>{t('Seedance API')}</h3>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={copying}
            aria-busy={copying}
            onClick={copyGuide}
          >
            <Copy aria-hidden='true' className='size-3.5' />
            {t('Copy AI integration guide')}
          </Button>
        </div>
        <p className='text-muted-foreground text-sm'>{rules[0]}</p>
        <CodeBlock
          language='text'
          code={`Authorization: Bearer <NEW_API_KEY>\nContent-Type: application/json\nPOST ${baseUrl}/v1/videos\nGET  ${baseUrl}/v1/videos/{id}\nGET  ${baseUrl}/v1/videos/{id}/content`}
        >
          <CodeBlockCopyButton />
        </CodeBlock>
      </section>
      <section className='space-y-3'>
        <h3 className='text-sm font-semibold'>{t('Supported parameters')}</h3>
        <StaticDataTable
          data={parameters}
          getRowKey={(row) => row.name}
          columns={[
            {
              id: 'name',
              header: t('Parameter'),
              cell: (row) => <code className='break-all'>{row.name}</code>,
            },
            {
              id: 'value',
              header: t('Default / allowed values'),
              cell: (row) => <code className='break-all'>{row.value}</code>,
            },
          ]}
        />
        {rules.slice(1).map((rule) => (
          <p key={rule} className='text-muted-foreground text-sm'>
            {rule}
          </p>
        ))}
      </section>
      <section className='space-y-3'>
        <h3 className='text-sm font-semibold'>{t('Example')}</h3>
        <CodeBlock
          language='bash'
          code={`export NEW_API_BASE_URL='${baseUrl}'\nexport NEW_API_KEY='<NEW_API_KEY>'\ncurl --fail-with-body --max-time 120 "$NEW_API_BASE_URL/v1/videos" \\
  -H "Authorization: Bearer $NEW_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '${JSON.stringify(request, null, 2)}'`}
        >
          <CodeBlockCopyButton />
        </CodeBlock>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Media examples are alternative request bodies. Replace example URLs with accessible files; do not combine frame and reference modes.'
          )}
        </p>
        <CodeBlock
          language='json'
          code={JSON.stringify(
            {
              ...request,
              input_reference: 'https://example.com/first.jpg',
              image_end: 'https://example.com/last.jpg',
            },
            null,
            2
          )}
        >
          <CodeBlockCopyButton />
        </CodeBlock>
        <CodeBlock
          language='json'
          code={JSON.stringify(
            {
              ...request,
              reference_images: ['https://example.com/reference.jpg'],
              reference_videos: ['https://example.com/reference.mp4'],
              reference_audios: ['https://example.com/reference.mp3'],
            },
            null,
            2
          )}
        >
          <CodeBlockCopyButton />
        </CodeBlock>
      </section>
      <section className='space-y-3'>
        <h3 className='text-sm font-semibold'>{t('Generate and download')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Creation returns HTTP 200 with id. Save it before polling. queued and in_progress are pending; completed allows content download; failed includes an error. A timeout is not a failure: query the saved ID instead of resubmitting.'
          )}
        </p>
        <CodeBlock
          language='json'
          code={JSON.stringify(
            {
              id: 'task_example',
              task_id: 'task_example',
              request_id: 'task_example',
              object: 'video.generation',
              model: props.modelName,
              status: 'queued',
              progress: 0,
            },
            null,
            2
          )}
        >
          <CodeBlockCopyButton />
        </CodeBlock>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Python example requires requests and the NEW_API_BASE_URL and NEW_API_KEY environment variables. It creates one paid task, saves its ID, polls every 15 seconds and downloads the completed MP4.'
          )}
        </p>
        <CodeBlock
          language='python'
          code={lxmoneSeedancePythonExample(props.modelName)}
        >
          <CodeBlockCopyButton />
        </CodeBlock>
        <CodeBlock
          language='bash'
          code={`TASK_ID='task_example'\ncurl --fail-with-body "$NEW_API_BASE_URL/v1/videos/$TASK_ID" -H "Authorization: Bearer $NEW_API_KEY"\ncurl --fail-with-body --location "$NEW_API_BASE_URL/v1/videos/$TASK_ID/content" -H "Authorization: Bearer $NEW_API_KEY" --output video.mp4`}
        >
          <CodeBlockCopyButton />
        </CodeBlock>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Fix invalid_request, invalid_duration, invalid_resolution or invalid_media before retrying. model_price_error requires administrator configuration. Authentication or quota errors require checking the API key, official group and balance. For video_generation_failed, inspect the saved task error; never retry a paid submission automatically.'
          )}
        </p>
      </section>
    </div>
  )
}
