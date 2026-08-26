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

import type { PricingModel } from '../types'
import {
  buildMiniMaxVideoAiIntegrationGuide,
  buildMiniMaxVideoParameters,
  buildMiniMaxVideoSample,
  MINIMAX_VIDEO_V2_ENDPOINT_TYPE,
  supportsMiniMaxVideoV2Endpoint,
} from './minimax-video-api-docs'
import { getFixedPriceUnit, supportsPerformanceMetrics } from './model-helpers'

const context = {
  baseUrl: 'https://api.example.com',
  apiKeyEnv: 'OPWAN_API_KEY',
  modelName: 'MiniMax-H3',
  endpointPath: '/v2/video_generation',
}

const model = {
  model_name: 'MiniMax-H3',
  quota_type: 1,
  model_ratio: 0,
  completion_ratio: 0,
  enable_groups: ['media'],
  supported_endpoint_types: [MINIMAX_VIDEO_V2_ENDPOINT_TYPE],
} as PricingModel

describe('MiniMax-H3 video V2 API documentation', () => {
  test('detects the dedicated endpoint without guessing from the model name', () => {
    assert.equal(supportsMiniMaxVideoV2Endpoint(model), true)
    assert.equal(
      supportsMiniMaxVideoV2Endpoint({
        ...model,
        model_name: 'MiniMax-H3',
        supported_endpoint_types: ['openai-video'],
      }),
      false
    )
  })

  test('documents only public top-level request parameters', () => {
    const parameters = buildMiniMaxVideoParameters('MiniMax-H3')
    const byName = new Map(
      parameters.map((parameter) => [parameter.name, parameter])
    )

    assert.deepEqual(
      [...byName.keys()],
      [
        'model',
        'content',
        'resolution',
        'duration',
        'ratio',
        'audio_sync',
        'callback_url',
      ]
    )
    assert.equal(byName.get('model')?.defaultValue, 'MiniMax-H3')
    assert.deepEqual(byName.get('resolution')?.enumValues, ['768P'])
    assert.equal(byName.get('duration')?.range, '4 ~ 15')
    assert.deepEqual(byName.get('ratio')?.enumValues, ['16:9', '9:16', '1:1'])
    assert.equal(byName.get('callback_url')?.range, '≤ 2048 characters')
    assert.match(
      byName.get('callback_url')?.descriptionKey ?? '',
      /public HTTPS URL.*challenge.*3 seconds.*terminal task results/
    )
    assert.equal(byName.has('seed'), false)
    assert.equal(byName.has('aigc_watermark'), false)
  })

  test('all samples submit, poll to a terminal state, and handle errors', () => {
    for (const language of [
      'curl',
      'python',
      'typescript',
      'javascript',
    ] as const) {
      const sample = buildMiniMaxVideoSample(language, context)

      assert.match(sample, /POST|method: 'POST'|requests\.post/)
      assert.match(sample, /Authorization.*Bearer|Bearer \$\{apiKey\}/)
      assert.doesNotMatch(sample, /Idempotency-Key/)
      assert.doesNotMatch(sample, /MINIMAX_IDEMPOTENCY_KEY/)
      assert.doesNotMatch(sample, /randomUUID|uuid\.uuid4|date \+%s/)
      assert.match(sample, /MiniMax-H3/)
      assert.match(sample, /\/v2\/video_generation/)
      assert.match(sample, /\/v2\/query\/video_generation/)
      assert.match(sample, /queued/)
      assert.match(sample, /running/)
      assert.match(sample, /succeeded/)
      assert.match(sample, /failed/)
      assert.match(sample, /cancelled/)
      assert.match(sample, /Retry-After|retry-after/i)
      assert.match(sample, /429/)
      assert.match(sample, /15 minutes|900|900_000/)
      assert.match(sample, /content.*url|content"\]\["url/i)
      assert.match(sample, /response\.ok|raise_for_status|http_code|HTTP/)
      assert.doesNotMatch(sample, /callback_url/)
      assert.doesNotMatch(sample, /seed|aigc_watermark/)
      assert.doesNotMatch(sample, /chat\/completions|messages/)
    }
  })

  test('JavaScript submits once and polls the returned task', async () => {
    const sample = buildMiniMaxVideoSample('javascript', context)
    const calls: Array<{ url: string; init?: RequestInit }> = []
    const logged: string[] = []
    let attempt = 0

    const fetchMock = async (
      input: string | URL | Request,
      init?: RequestInit
    ): Promise<Response> => {
      calls.push({ url: String(input), init })
      attempt += 1
      if (attempt === 1) {
        return new Response('{"task_id":"task_simple"}', { status: 200 })
      }
      if (attempt === 2) {
        return new Response('{"task":{"status":"queued"}}', {
          status: 200,
          headers: { 'Retry-After': '0' },
        })
      }
      if (attempt === 3) {
        return new Response(
          '{"task":{"status":"succeeded","content":{"url":"https://media.example.com/result.mp4"}}}',
          { status: 200 }
        )
      }
      throw new Error('unexpected fetch call')
    }

    const AsyncFunction = Object.getPrototypeOf(async () => {}).constructor
    const execute = new AsyncFunction(
      'process',
      'fetch',
      'AbortSignal',
      'setTimeout',
      'console',
      sample
    )
    await execute(
      {
        env: {
          OPWAN_API_KEY: 'test-api-key',
        },
      },
      fetchMock,
      { timeout: () => undefined },
      (callback: () => void) => {
        callback()
        return 0
      },
      { log: (value: unknown) => logged.push(String(value)) }
    )

    const submitCalls = calls.filter(
      (call) => call.url === 'https://api.example.com/v2/video_generation'
    )
    assert.equal(submitCalls.length, 1)
    assert.equal(
      ((submitCalls[0]?.init?.headers ?? {}) as Record<string, string>)[
        'Idempotency-Key'
      ],
      undefined
    )
    assert.equal(
      calls.filter((call) =>
        call.url.startsWith(
          'https://api.example.com/v2/query/video_generation/'
        )
      ).length,
      2
    )
    assert.deepEqual(logged, ['https://media.example.com/result.mp4'])
  })

  test('cURL keeps task responses in a private temporary directory and cleans them', () => {
    const sample = buildMiniMaxVideoSample('curl', context)

    assert.match(sample, /umask 077/)
    assert.match(sample, /mktemp -d/)
    assert.match(sample, /trap cleanup EXIT/)
    assert.match(sample, /rm -f -- "\$HEADERS_FILE" "\$BODY_FILE"/)
    assert.doesNotMatch(sample, /HEADERS_FILE="minimax-video\.headers"/)
    assert.doesNotMatch(sample, /BODY_FILE="minimax-video\.json"/)
  })

  test('AI guide identifies the implemented subset without exposing implementation providers', () => {
    const guide = buildMiniMaxVideoAiIntegrationGuide(context)

    assert.match(
      guide,
      /POST https:\/\/api\.example\.com\/v2\/video_generation/
    )
    assert.match(
      guide,
      /GET https:\/\/api\.example\.com\/v2\/query\/video_generation\/\{task_id\}/
    )
    assert.match(guide, /MiniMax V2-compatible subset/)
    assert.match(guide, /resolution=768P/)
    assert.match(guide, /duration is an integer from 4 to 15/)
    assert.match(guide, /at most 9 reference_image/)
    assert.match(guide, /at most 3 reference_audio/)
    assert.match(guide, /role=first_frame.*role=last_frame/)
    assert.match(guide, /multimodal reference generation by default/)
    assert.match(guide, /audio_sync=true/)
    assert.doesNotMatch(guide, /seed|aigc_watermark/)
    assert.doesNotMatch(guide, /Idempotency-Key/)
    assert.match(guide, /seven days/)
    assert.match(guide, /requested duration in seconds/)
    assert.match(guide, /2K/)
    assert.match(guide, /adaptive/)
    assert.match(guide, /reference_video/)
    assert.match(guide, /callback_url/)
    assert.match(guide, /publicly reachable HTTPS URL of at most 2048/)
    assert.match(guide, /identical challenge value within 3 seconds/)
    assert.match(guide, /only for terminal succeeded, failed, or cancelled/)
    assert.match(guide, /exactly the same \{"task":\{\.\.\.\}\}/)
    assert.match(guide, /up to five total attempts/)
    assert.match(guide, /30, 60, 120, and 240 second delays/)
    assert.match(guide, /X-Webhook-Delivery-Id/)
    assert.match(guide, /X-Webhook-Timestamp/)
    assert.match(guide, /at least once/)
    assert.match(guide, /No callback signature is sent/)
    assert.match(guide, /does not change task status or billing/)
    assert.doesNotMatch(guide, /AutoDL/i)
    assert.doesNotMatch(
      buildMiniMaxVideoParameters('MiniMax-H3')
        .map((parameter) => parameter.descriptionKey)
        .join('\n'),
      /AutoDL/i
    )
  })

  test('uses per-second pricing and hides synthetic performance data', () => {
    assert.equal(getFixedPriceUnit(model), 'seconds')
    assert.equal(supportsPerformanceMetrics(model), false)
  })
})
