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
  AUDIO_SPEECH_ENDPOINT_TYPE,
  buildIndexTTSAudioSpeechAiIntegrationGuide,
  buildIndexTTSAudioSpeechParameters,
  buildIndexTTSAudioSpeechSample,
  supportsAudioSpeechEndpoint,
} from './audio-speech-api-docs'

const context = {
  baseUrl: 'https://api.example.com',
  apiKeyEnv: 'OPWAN_API_KEY',
  modelName: 'indextts2-v1',
  endpointPath: '/v1/audio/speech',
}

describe('IndexTTS2 audio speech API documentation', () => {
  test('detects the dedicated endpoint without guessing from the model name', () => {
    const model = {
      model_name: 'custom-name',
      supported_endpoint_types: [AUDIO_SPEECH_ENDPOINT_TYPE],
    } as PricingModel

    assert.equal(supportsAudioSpeechEndpoint(model), true)
    assert.equal(
      supportsAudioSpeechEndpoint({
        ...model,
        supported_endpoint_types: ['openai'],
      }),
      false
    )
  })

  test('documents every accepted top-level request parameter', () => {
    const parameters = buildIndexTTSAudioSpeechParameters('public-index-alias')
    const byName = new Map(
      parameters.map((parameter) => [parameter.name, parameter])
    )

    assert.deepEqual(
      [...byName.keys()],
      ['model', 'input', 'voice', 'response_format', 'speed', 'metadata']
    )
    assert.equal(byName.get('model')?.defaultValue, 'public-index-alias')
    assert.equal(byName.get('input')?.range, '1 ~ 2048')
    assert.deepEqual(byName.get('response_format')?.enumValues, ['wav'])
    assert.equal(byName.get('speed')?.defaultValue, 1)
  })

  test('cURL saves the binary response and shows authenticated 202 recovery', () => {
    const sample = buildIndexTTSAudioSpeechSample('curl', context)

    assert.match(sample, /--request POST/)
    assert.match(sample, /Authorization: Bearer \$OPWAN_API_KEY/)
    assert.match(sample, /Content-Type: application\/json/)
    assert.match(sample, /Idempotency-Key: indextts2-<UNIQUE_ID>/)
    assert.match(sample, /"model": "indextts2-v1"/)
    assert.match(sample, /"voice": "<REFERENCE_AUDIO_URL>"/)
    assert.match(sample, /"response_format": "wav"/)
    assert.match(sample, /"speed": 1/)
    assert.match(sample, /BODY_FILE="speech\.response"/)
    assert.ok(
      sample.indexOf('location=$(awk') < sample.indexOf('while [ "$status"')
    )
    assert.match(sample, /while \[ "\$status" = "202" \]/)
    assert.match(sample, /"\$status" = "429"/)
    assert.match(sample, /retry-after:/)
    assert.match(sample, /mv "\$BODY_FILE" speech\.wav/)
  })

  test('Python follows Location and writes WAV bytes without JSON decoding', () => {
    const sample = buildIndexTTSAudioSpeechSample('python', context)

    assert.match(
      sample,
      /response\.status_code == 202 or \(response\.status_code == 429 and recovery_url\)/
    )
    assert.match(sample, /location = response\.headers\.get\("Location"\)/)
    assert.match(
      sample,
      /recovery_url = urljoin\(response\.url, location\) if location else None/
    )
    assert.match(sample, /timeout=420/)
    assert.match(sample, /time\.monotonic\(\) \+ 900/)
    assert.match(sample, /content_type != "audio\/wav"/)
    assert.match(sample, /write_bytes\(response\.content\)/)
    assert.doesNotMatch(sample, /response\.json\(/)
  })

  test('JavaScript and TypeScript follow Location and persist an ArrayBuffer', () => {
    for (const language of ['javascript', 'typescript'] as const) {
      const sample = buildIndexTTSAudioSpeechSample(language, context)

      assert.match(sample, /randomUUID\(\)/)
      assert.match(
        sample,
        /response\.status === 202 \|\| \(response\.status === 429 && recoveryUrl\)/
      )
      assert.match(
        sample,
        /initialLocation = response\.headers\.get\('Location'\)/
      )
      assert.match(
        sample,
        /recoveryUrl = new URL\(initialLocation, response\.url\)/
      )
      assert.match(sample, /AbortSignal\.timeout\(420_000\)/)
      assert.match(sample, /Date\.now\(\) \+ 900_000/)
      assert.match(sample, /contentType !== 'audio\/wav'/)
      assert.match(sample, /await response\.arrayBuffer\(\)/)
      assert.doesNotMatch(sample, /response\.json\(/)
      if (language === 'javascript') {
        assert.doesNotMatch(sample, /recoveryUrl:/)
        assert.doesNotMatch(sample, /recoveryUrl!/)
      } else {
        assert.match(sample, /let recoveryUrl: URL \| undefined/)
        assert.match(sample, /fetch\(recoveryUrl!/)
      }
    }
  })

  test('AI guide captures limits, emotion rules, replay, and unsupported options', () => {
    const guide = buildIndexTTSAudioSpeechAiIntegrationGuide(context)

    assert.match(guide, /POST https:\/\/api\.example\.com\/v1\/audio\/speech/)
    assert.match(guide, /HTTP 200 returns binary audio\/wav/)
    assert.match(guide, /HTTP 202 returns task_id and status/)
    assert.match(
      guide,
      /initial POST returns HTTP 429 with Location, the task was accepted/
    )
    assert.match(
      guide,
      /GET https:\/\/api\.example\.com\/v1\/audio\/speech\/\{task_id\}/
    )
    assert.match(guide, /HTTP 409 with code idempotency_conflict/)
    assert.match(guide, /1 to 2048 UTF-8 characters/)
    assert.match(guide, /15 MiB/)
    assert.match(guide, /30 MiB/)
    assert.match(guide, /64 MiB/)
    assert.match(
      guide,
      /happy, angry, sad, afraid, disgusted, melancholic, surprised, calm/
    )
    assert.match(guide, /surprised value currently must be 0/)
    assert.match(guide, /mutually exclusive/)
    assert.match(guide, /stream_format=sse is unsupported/)
  })
})
