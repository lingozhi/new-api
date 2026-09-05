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
import { StaticDataTable } from '@/components/data-table'
import { useStatus } from '@/hooks/use-status'

import {
  seedanceGenerationExample,
  seedanceUploadExample,
} from '../lib/seedance-api-docs'

export function SeedanceApiDocs(props: { modelName: string }) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const configured = (status as Record<string, unknown> | null)?.server_address
  const baseUrl =
    typeof configured === 'string' && configured
      ? configured.replace(/\/$/, '')
      : window.location.origin
  const variants = [
    {
      model: 'seedance-2.0',
      duration: '4–15 s',
      resolution: '480p / 720p / 1080p*',
      multiplier: '×2',
    },
    {
      model: 'seedance-2.0-fast',
      duration: '4–15 s',
      resolution: '480p / 720p',
      multiplier: '×2',
    },
    {
      model: 'seedance-2.5',
      duration: '4–30 s',
      resolution: '480p / 720p / 1080p',
      multiplier: '×1.6',
    },
  ]
  const parameters = [
    { name: 'model', value: props.modelName },
    { name: 'prompt', value: 'string' },
    {
      name: 'duration',
      value:
        props.modelName === 'seedance-2.5'
          ? `4–30; ${t('Default')}: 5`
          : `4–15; ${t('Default')}: 5`,
    },
    {
      name: 'resolution',
      value:
        props.modelName === 'seedance-2.0-fast'
          ? `480p | 720p; ${t('Default')}: 720p`
          : `480p | 720p | 1080p; ${t('Default')}: 720p`,
    },
    {
      name: 'aspect_ratio',
      value: '16:9 | 9:16 | 1:1 | 4:3 | 3:4 | 21:9 | adaptive',
    },
    { name: 'start_image / end_image', value: '{"url":"https://…"}' },
    {
      name: 'reference_images / reference_videos / reference_audios',
      value: '[{"url":"https://…"}]',
    },
    { name: 'generate_audio', value: `boolean; ${t('Default')}: false` },
    { name: 'n', value: '1' },
  ]
  return (
    <div className='space-y-6'>
      <section className='space-y-3'>
        <h3 className='text-sm font-semibold'>{t('Seedance API')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Submit once, save request_id, poll until done, then download with the same API token.'
          )}
        </p>
        <CodeBlock
          language='text'
          code={
            'Authorization: Bearer <NEW_API_KEY>\nPOST /v1/videos/generations\nPOST /v1/media/uploads\nGET  /v1/videos/{request_id}\nGET  /v1/videos/{request_id}/content'
          }
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
        <p className='text-muted-foreground text-sm'>
          {t(
            'Prompt is required for text-to-video. Media inputs use public HTTPS URLs; base64, multipart generation requests, and file_id are unsupported.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Start and end frames cannot be combined with reference lists. Seedance 2.5 requires adaptive for first-frame, first-and-last-frame, and video-only input.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Seedance 2.0 and Fast allow 9 images, 3 videos, and 3 audios. Seedance 2.5 allows 30 images, 10 videos, and 10 audios, up to 50 assets; video and audio references each total at most 30 seconds.'
          )}
        </p>
      </section>
      <section className='space-y-3'>
        <h3 className='text-sm font-semibold'>{t('Generate and download')}</h3>
        <CodeBlock
          language='bash'
          code={seedanceGenerationExample(props.modelName, baseUrl)}
        >
          <CodeBlockCopyButton />
        </CodeBlock>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Submit returns HTTP 202 with request_id. Poll every 15 seconds: pending, done, or failed. A polling timeout does not mean generation failed; query the saved ID instead of submitting again.'
          )}
        </p>
        <CodeBlock
          language='json'
          code={JSON.stringify(
            {
              model: props.modelName,
              request_id: 'task_example',
              status: 'done',
              video: { duration: 4, url: '/v1/videos/task_example/content' },
            },
            null,
            2
          )}
        >
          <CodeBlockCopyButton />
        </CodeBlock>
      </section>
      <section className='space-y-3'>
        <h3 className='text-sm font-semibold'>
          {t('Upload a reference image')}
        </h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Request an upload ticket, PUT the file to upload_url, then pass media_url to generation. Do not send your API token to the storage URL. Match content_type and size_bytes to the file.'
          )}
        </p>
        <CodeBlock
          language='bash'
          code={seedanceUploadExample(props.modelName, baseUrl)}
        >
          <CodeBlockCopyButton />
        </CodeBlock>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Upload types: image (JPEG, PNG, WebP; 20 MiB), video (MP4, MOV, WebM; 500 MiB), audio (MP3, WAV, M4A, AAC; 20 MiB). Upload tickets expire after 15 minutes; media URLs last 7 days.'
          )}
        </p>
      </section>
      <section className='space-y-3'>
        <h3 className='text-sm font-semibold'>
          {t('Model differences and billing')}
        </h3>
        <StaticDataTable
          data={variants}
          getRowKey={(row) => row.model}
          columns={[
            {
              id: 'model',
              header: t('Model'),
              cell: (row) => <code>{row.model}</code>,
            },
            {
              id: 'duration',
              header: t('Duration'),
              cell: (row) => row.duration,
            },
            {
              id: 'resolution',
              header: t('Resolution'),
              cell: (row) => row.resolution,
            },
            {
              id: 'multiplier',
              header: t('Reference video multiplier'),
              cell: (row) => row.multiplier,
            },
          ]}
        />
        <p className='text-muted-foreground text-sm'>
          {t(
            'Seedance 2.0 supports 1080p only with media input. Fast does not support 1080p. None of these models supports 4K.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Billing uses delivered seconds, resolution, your group price, and the reference video multiplier. Failed tasks are refunded. Audio generation and image references do not add a multiplier. Use the current model price rather than a fixed example price.'
          )}
        </p>
      </section>
    </div>
  )
}
