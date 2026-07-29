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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { TaskLog } from '../types'
import { buildTaskLogDiagnostics } from './task-log-diagnostics'

const failedDepthMediaTask: TaskLog = {
  id: 131,
  user_id: 1,
  platform: 'depth_media',
  task_id: 'task_example',
  action: 'remove_background',
  channel_id: 120,
  submit_time: 1_785_343_764,
  finish_time: 1_785_343_783,
  progress: '100%',
  status: 'FAILURE',
  fail_reason: 'Media processing failed',
  properties: {
    input: '',
    origin_model_name: 'background-remove',
    upstream_model_name: 'background-remove-fast',
  },
  data: {
    error: 'Media processing failed',
    error_code: 'processing_failed',
    format: 'webp',
    operation: 'remove_background',
    progress: 5,
    quality: 'fast',
    source_url: 'https://cdn.example.com/source.png',
    status: 'failed',
  },
}

describe('task log diagnostics', () => {
  test('separates request parameters, output, and error details', () => {
    const diagnostics = buildTaskLogDiagnostics(failedDepthMediaTask)

    assert.deepEqual(diagnostics.request, {
      source_url: 'https://cdn.example.com/source.png',
      operation: 'remove_background',
      quality: 'fast',
      format: 'webp',
    })
    assert.deepEqual(diagnostics.modelMapping, {
      requested_model: 'background-remove',
      upstream_model: 'background-remove-fast',
    })
    assert.equal(diagnostics.resultUrl, undefined)
    assert.deepEqual(diagnostics.error, {
      code: 'processing_failed',
      message: 'Media processing failed',
      fail_reason: 'Media processing failed',
    })
  })

  test('finds output URLs and accepts JSON strings from legacy responses', () => {
    const diagnostics = buildTaskLogDiagnostics({
      ...failedDepthMediaTask,
      status: 'SUCCESS',
      fail_reason: undefined,
      result_url: 'https://cdn.example.com/result.webp',
      data: JSON.stringify({
        result_url: 'https://upstream.example.com/result.webp',
        status: 'succeeded',
      }),
    })

    assert.equal(diagnostics.resultUrl, 'https://cdn.example.com/result.webp')
    assert.deepEqual(diagnostics.error, {})
    assert.deepEqual(diagnostics.response, {
      result_url: 'https://upstream.example.com/result.webp',
      status: 'succeeded',
    })
  })

  test('redacts nested credentials before rendering or copying diagnostics', () => {
    const diagnostics = buildTaskLogDiagnostics({
      ...failedDepthMediaTask,
      data: {
        api_key: 'sk-secret',
        authorization: 'Bearer secret',
        'x-api-key': 'sk-header-secret',
        secret_key: 'secret-key-value',
        nested: {
          access_token: 'secret-token',
          accessToken: 'camel-token',
          client_secret: 'client-secret',
          webhook_secret: 'webhook-secret',
          safe_value: 'visible',
        },
      },
      properties: {
        input: JSON.stringify({
          prompt: 'visible',
          apiKey: 'sk-input-secret',
        }),
      },
    })

    assert.deepEqual(diagnostics.response, {
      api_key: '[REDACTED]',
      authorization: '[REDACTED]',
      'x-api-key': '[REDACTED]',
      secret_key: '[REDACTED]',
      nested: {
        access_token: '[REDACTED]',
        accessToken: '[REDACTED]',
        client_secret: '[REDACTED]',
        webhook_secret: '[REDACTED]',
        safe_value: 'visible',
      },
    })
    assert.deepEqual(diagnostics.request.input, {
      prompt: 'visible',
      apiKey: '[REDACTED]',
    })
  })

  test('does not report a legacy success result URL as an error', () => {
    const resultUrl = 'https://cdn.example.com/video.mp4'
    const diagnostics = buildTaskLogDiagnostics({
      ...failedDepthMediaTask,
      status: 'SUCCESS',
      fail_reason: resultUrl,
      result_url: resultUrl,
      data: { result_url: resultUrl, status: 'succeeded' },
    })

    assert.equal(diagnostics.resultUrl, resultUrl)
    assert.deepEqual(diagnostics.error, {})
  })

  test('extracts structured upstream errors into the error summary', () => {
    const diagnostics = buildTaskLogDiagnostics({
      ...failedDepthMediaTask,
      fail_reason: 'Upstream request failed',
      data: {
        error: {
          code: 'source_not_found',
          message: 'Source image returned HTTP 404',
        },
        status: 'failed',
      },
    })

    assert.deepEqual(diagnostics.error, {
      code: 'source_not_found',
      message: 'Source image returned HTTP 404',
      fail_reason: 'Upstream request failed',
    })
  })

  test('bounds large response values before they reach the diagnostics UI', () => {
    const largeText = 'x'.repeat(25_000)
    const dataUri = `data:image/png;base64,${'a'.repeat(25_000)}`
    const diagnostics = buildTaskLogDiagnostics({
      ...failedDepthMediaTask,
      data: {
        large_text: largeText,
        image: dataUri,
        items: Array.from({ length: 105 }, (_, index) => index),
      },
    })

    const response = diagnostics.response as Record<string, unknown>
    assert.match(String(response.large_text), /truncated 5000 characters/)
    assert.equal(
      response.image,
      `[DATA URI OMITTED: ${dataUri.length} characters]`
    )
    assert.deepEqual((response.items as unknown[]).slice(-1), [
      '[5 more items omitted]',
    ])
  })

  test('stops traversing deeply nested upstream responses', () => {
    let nested: Record<string, unknown> = { value: 'visible' }
    for (let depth = 0; depth < 1_000; depth++) {
      nested = { nested }
    }

    const diagnostics = buildTaskLogDiagnostics({
      ...failedDepthMediaTask,
      data: nested,
    })

    let cursor = diagnostics.response as Record<string, unknown>
    for (let depth = 0; depth < 20; depth++) {
      cursor = cursor.nested as Record<string, unknown>
    }
    assert.equal(cursor.nested, '[MAX DEPTH REACHED]')
  })

  test('keeps malformed legacy data readable without throwing', () => {
    const diagnostics = buildTaskLogDiagnostics({
      ...failedDepthMediaTask,
      data: '{not-json',
      properties: '{also-not-json',
    })

    assert.equal(diagnostics.response, '{not-json')
    assert.deepEqual(diagnostics.request, {})
    assert.deepEqual(diagnostics.modelMapping, {})
  })
})
