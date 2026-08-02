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
import { Check, Copy, ExternalLink } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

import { taskStatusMapper } from '../../lib/mappers'
import {
  buildTaskLogDiagnostics,
  getSafeTaskLogUrl,
} from '../../lib/task-log-diagnostics'
import type { TaskLog } from '../../types'

interface TaskLogDetailsDialogProps {
  log: TaskLog
  isAdmin: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}

interface CopyableJsonProps {
  copyLabel: string
  emptyLabel: string
  value: unknown
}

const REQUEST_LABEL_KEYS: Record<string, string> = {
  source_url: 'Source URL',
  operation: 'Operation',
  quality: 'Quality',
  scale: 'Scale',
  format: 'Format',
  resolution: 'Resolution',
  aspect_ratio: 'Aspect Ratio',
  size: 'Size',
  duration: 'Duration',
  seconds: 'Duration',
  output_format: 'Output Format',
  input: 'Input',
}

function formatDiagnosticValue(value: unknown): string {
  if (typeof value === 'string') return value
  return JSON.stringify(value, null, 2)
}

function CopyableJson(props: CopyableJsonProps) {
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const formatted = useMemo(() => {
    if (props.value == null) return ''
    if (typeof props.value === 'string') return props.value
    return JSON.stringify(props.value, null, 2)
  }, [props.value])

  if (!formatted) {
    return (
      <p className='text-muted-foreground py-3 text-sm'>{props.emptyLabel}</p>
    )
  }

  return (
    <div className='bg-muted/40 relative overflow-hidden rounded-lg border'>
      <Button
        variant='ghost'
        size='sm'
        className='absolute top-2 right-2 z-10 size-8 p-0'
        onClick={() => copyToClipboard(formatted)}
        title={props.copyLabel}
        aria-label={props.copyLabel}
      >
        {copiedText === formatted ? (
          <Check className='size-4 text-green-600' aria-hidden='true' />
        ) : (
          <Copy className='size-4' aria-hidden='true' />
        )}
      </Button>
      <pre className='max-h-72 overflow-auto p-3 pr-12 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap'>
        {formatted}
      </pre>
    </div>
  )
}

function DetailValue(props: { value: unknown }) {
  const formatted = formatDiagnosticValue(props.value)
  const safeUrl = getSafeTaskLogUrl(props.value)

  if (safeUrl) {
    return (
      <a
        href={safeUrl}
        target='_blank'
        rel='noopener noreferrer'
        className='text-primary inline-flex min-w-0 items-center gap-1 break-all hover:underline'
      >
        {formatted}
        <ExternalLink className='size-3 shrink-0' aria-hidden='true' />
      </a>
    )
  }

  return <span className='break-words whitespace-pre-wrap'>{formatted}</span>
}

function getErrorLabelKey(key: string): string {
  if (key === 'code') return 'Error code'
  if (key === 'message') return 'Upstream error'
  return 'Fail Reason'
}

export function TaskLogDetailsDialog(props: TaskLogDetailsDialogProps) {
  const { t } = useTranslation()
  const diagnostics = useMemo(
    () => buildTaskLogDiagnostics(props.log),
    [props.log]
  )
  const requestEntries = Object.entries(diagnostics.request)
  const modelEntries = Object.entries(diagnostics.modelMapping)
  const errorEntries = Object.entries(diagnostics.error)

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Task diagnostics')}
      description={t('Review request, result, and error details for this task')}
      contentClassName='sm:max-w-3xl'
      contentHeight='min(760px, calc(100vh - 10rem))'
      bodyClassName='space-y-4'
    >
      <section className='bg-muted/20 grid gap-3 rounded-lg border p-3 sm:grid-cols-2'>
        <div className='min-w-0'>
          <p className='text-muted-foreground text-xs'>{t('Task ID')}</p>
          <p className='truncate font-mono text-sm' title={props.log.task_id}>
            {props.log.task_id || '-'}
          </p>
        </div>
        <div>
          <p className='text-muted-foreground text-xs'>{t('Status')}</p>
          <StatusBadge
            label={t(
              taskStatusMapper.getLabel(
                props.log.status,
                props.log.status || 'Submitting'
              )
            )}
            variant={taskStatusMapper.getVariant(props.log.status)}
            size='sm'
            copyable={false}
          />
        </div>
        {props.isAdmin && props.log.channel_id > 0 ? (
          <div>
            <p className='text-muted-foreground text-xs'>{t('Channel')}</p>
            <p className='font-mono text-sm'>#{props.log.channel_id}</p>
          </div>
        ) : null}
        <div>
          <p className='text-muted-foreground text-xs'>{t('Operation')}</p>
          <p className='text-sm'>{props.log.action || '-'}</p>
        </div>
      </section>

      {errorEntries.length > 0 ? (
        <section className='space-y-2'>
          <h3 className='text-sm font-semibold text-red-600 dark:text-red-400'>
            {t('Error details')}
          </h3>
          <dl className='divide-border overflow-hidden rounded-lg border border-red-200/80 bg-red-50/50 text-sm dark:border-red-900/70 dark:bg-red-950/20'>
            {errorEntries.map(([key, value]) => (
              <div
                key={key}
                className='grid gap-1 border-b p-3 last:border-b-0 sm:grid-cols-[9rem_1fr]'
              >
                <dt className='text-muted-foreground'>
                  {t(getErrorLabelKey(key))}
                </dt>
                <dd className='min-w-0 text-red-700 dark:text-red-300'>
                  <DetailValue value={value} />
                </dd>
              </div>
            ))}
          </dl>
        </section>
      ) : null}

      <section className='space-y-2'>
        <h3 className='text-sm font-semibold'>{t('Request parameters')}</h3>
        {requestEntries.length === 0 && modelEntries.length === 0 ? (
          <p className='text-muted-foreground rounded-lg border p-3 text-sm'>
            {t('No request parameters recorded')}
          </p>
        ) : (
          <dl className='divide-border overflow-hidden rounded-lg border text-sm'>
            {modelEntries.map(([key, value]) => (
              <div
                key={key}
                className='grid gap-1 border-b p-3 last:border-b-0 sm:grid-cols-[9rem_1fr]'
              >
                <dt className='text-muted-foreground'>
                  {t(
                    key === 'requested_model'
                      ? 'Requested model'
                      : 'Upstream model'
                  )}
                </dt>
                <dd className='min-w-0 font-mono'>
                  <DetailValue value={value} />
                </dd>
              </div>
            ))}
            {requestEntries.map(([key, value]) => (
              <div
                key={key}
                className='grid gap-1 border-b p-3 last:border-b-0 sm:grid-cols-[9rem_1fr]'
              >
                <dt className='text-muted-foreground'>
                  {t(REQUEST_LABEL_KEYS[key] ?? key)}
                </dt>
                <dd className='min-w-0 font-mono'>
                  <DetailValue value={value} />
                </dd>
              </div>
            ))}
          </dl>
        )}
      </section>

      <section className='space-y-2'>
        <h3 className='text-sm font-semibold'>{t('Output result')}</h3>
        {diagnostics.resultUrl ? (
          <div className='rounded-lg border p-3 font-mono text-sm'>
            <DetailValue value={diagnostics.resultUrl} />
          </div>
        ) : (
          <p className='text-muted-foreground rounded-lg border p-3 text-sm'>
            {t('No output result recorded')}
          </p>
        )}
      </section>

      <section className='space-y-2'>
        <h3 className='text-sm font-semibold'>{t('Upstream response')}</h3>
        <CopyableJson
          value={diagnostics.response}
          copyLabel={t('Copy response')}
          emptyLabel={t('No upstream response recorded')}
        />
      </section>
    </Dialog>
  )
}
