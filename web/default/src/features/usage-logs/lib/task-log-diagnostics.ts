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
import type { TaskLog } from '../types'

const REDACTED_VALUE = '[REDACTED]'
const MAX_DIAGNOSTIC_STRING_LENGTH = 20_000
const MAX_DIAGNOSTIC_ARRAY_ITEMS = 100
const MAX_DIAGNOSTIC_OBJECT_FIELDS = 200
const MAX_DIAGNOSTIC_DEPTH = 20
const REQUEST_PARAMETER_KEYS = [
  'source_url',
  'operation',
  'quality',
  'scale',
  'format',
  'resolution',
  'aspect_ratio',
  'size',
  'duration',
  'seconds',
  'output_format',
] as const

export interface TaskLogDiagnostics {
  input: unknown
  request: Record<string, unknown>
  modelMapping: Record<string, string>
  resultUrl?: string
  error: {
    code?: string
    message?: string
    fail_reason?: string
  }
  response: unknown
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value != null && typeof value === 'object' && !Array.isArray(value)
}

function parseStructuredValue(value: unknown): unknown {
  if (typeof value !== 'string') return value
  const normalized = value.trim()
  if (normalized === '') return undefined
  try {
    return JSON.parse(normalized)
  } catch {
    return value
  }
}

function optionalString(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const normalized = value.trim()
  return normalized === '' ? undefined : normalized
}

function isSensitiveKey(key: string): boolean {
  const normalized = key.toLowerCase().replaceAll(/[^a-z0-9]/g, '')
  return (
    normalized === 'authorization' ||
    normalized.endsWith('apikey') ||
    normalized.endsWith('token') ||
    normalized.endsWith('secret') ||
    normalized.endsWith('secretkey') ||
    normalized.endsWith('password') ||
    normalized.endsWith('credential') ||
    normalized.endsWith('privatekey')
  )
}

function summarizeDiagnosticString(value: string): string {
  if (
    value.length > MAX_DIAGNOSTIC_STRING_LENGTH &&
    value.toLowerCase().startsWith('data:')
  ) {
    return `[DATA URI OMITTED: ${value.length} characters]`
  }
  if (value.length <= MAX_DIAGNOSTIC_STRING_LENGTH) return value
  const omitted = value.length - MAX_DIAGNOSTIC_STRING_LENGTH
  return `${value.slice(0, MAX_DIAGNOSTIC_STRING_LENGTH)}… [truncated ${omitted} characters]`
}

function redactSensitiveValues(value: unknown, depth = 0): unknown {
  if (depth >= MAX_DIAGNOSTIC_DEPTH) return '[MAX DEPTH REACHED]'
  if (Array.isArray(value)) {
    const visibleItems = value
      .slice(0, MAX_DIAGNOSTIC_ARRAY_ITEMS)
      .map((item) => redactSensitiveValues(item, depth + 1))
    const omitted = value.length - visibleItems.length
    if (omitted > 0) {
      visibleItems.push(`[${omitted} more items omitted]`)
    }
    return visibleItems
  }
  if (typeof value === 'string') return summarizeDiagnosticString(value)
  if (!isRecord(value)) return value

  const entries = Object.entries(value)
  const visibleEntries = entries
    .slice(0, MAX_DIAGNOSTIC_OBJECT_FIELDS)
    .map(([key, item]) => [
      key,
      isSensitiveKey(key)
        ? REDACTED_VALUE
        : redactSensitiveValues(item, depth + 1),
    ])
  const omitted = entries.length - visibleEntries.length
  if (omitted > 0) {
    visibleEntries.push(['__omitted_fields__', omitted])
  }
  return Object.fromEntries(visibleEntries)
}

export function getSafeTaskLogUrl(value: unknown): string | undefined {
  const url = optionalString(value)
  if (!url) return undefined
  try {
    const parsed = new URL(url)
    if (
      (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') ||
      parsed.username !== '' ||
      parsed.password !== ''
    ) {
      return undefined
    }
    return url
  } catch {
    return undefined
  }
}

export function buildTaskLogDiagnostics(log: TaskLog): TaskLogDiagnostics {
  const parsedData = parseStructuredValue(log.data)
  const parsedProperties = parseStructuredValue(log.properties)
  const response = redactSensitiveValues(parsedData)
  const dataRecord = isRecord(response) ? response : {}
  const sanitizedProperties = redactSensitiveValues(parsedProperties)
  const propertiesRecord = isRecord(sanitizedProperties)
    ? sanitizedProperties
    : {}

  let nestedRequest: Record<string, unknown> = {}
  if (isRecord(dataRecord.request)) {
    nestedRequest = dataRecord.request
  } else if (isRecord(dataRecord.parameters)) {
    nestedRequest = dataRecord.parameters
  }
  const request: Record<string, unknown> = {
    ...nestedRequest,
    ...Object.fromEntries(
      REQUEST_PARAMETER_KEYS.flatMap((key) => {
        const value = dataRecord[key]
        return value == null || value === '' ? [] : [[key, value]]
      })
    ),
  }
  const input = propertiesRecord.input
  if (input != null && input !== '') {
    request.input = redactSensitiveValues(parseStructuredValue(input))
  }

  const requestedModel = optionalString(propertiesRecord.origin_model_name)
  const upstreamModel = optionalString(propertiesRecord.upstream_model_name)
  const modelMapping: Record<string, string> = {}
  if (requestedModel) modelMapping.requested_model = requestedModel
  if (upstreamModel) modelMapping.upstream_model = upstreamModel

  const topLevelResultUrl = getSafeTaskLogUrl(log.result_url)
  const responseResultUrl = getSafeTaskLogUrl(dataRecord.result_url)

  const error: TaskLogDiagnostics['error'] = {}
  const nestedError = isRecord(dataRecord.error) ? dataRecord.error : {}
  const errorCode =
    optionalString(dataRecord.error_code) ??
    optionalString(nestedError.code) ??
    optionalString(nestedError.type)
  const errorMessage =
    optionalString(dataRecord.error) ?? optionalString(nestedError.message)
  const failReason = optionalString(log.fail_reason)
  if (errorCode) error.code = errorCode
  if (errorMessage) error.message = errorMessage
  const normalizedStatus = log.status.toUpperCase()
  if (
    failReason &&
    normalizedStatus !== 'SUCCESS' &&
    normalizedStatus !== 'SUCCEEDED'
  ) {
    error.fail_reason = failReason
  }

  return {
    input: redactSensitiveValues(parseStructuredValue(log.input_log)),
    request,
    modelMapping,
    resultUrl: topLevelResultUrl ?? responseResultUrl,
    error,
    response,
  }
}
