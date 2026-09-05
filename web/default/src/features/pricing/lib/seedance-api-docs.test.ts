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
import { spawnSync } from 'node:child_process'
import { test } from 'node:test'

import type { PricingModel } from '../types'
import { getVideoStartingPrice } from './model-helpers'
import { formatRequestPrice } from './price'
import {
  SEEDANCE_MODELS,
  seedanceRequest,
  seedanceGenerationExample,
  seedanceUploadExample,
  isSeedanceModel,
} from './seedance-api-docs'

for (const model of SEEDANCE_MODELS) {
  test(`${model} examples use supported asynchronous routes`, () => {
    assert.equal(isSeedanceModel(model), true)
    assert.deepEqual(seedanceRequest(model), {
      model,
      prompt:
        'A peaceful green forest in soft morning sunlight. The camera slowly moves forward.',
      duration: 4,
      resolution: '480p',
      aspect_ratio: '16:9',
      generate_audio: false,
    })
    for (const code of [
      seedanceGenerationExample(model, 'https://gateway.example'),
      seedanceUploadExample(model, 'https://gateway.example'),
    ]) {
      assert.equal(
        spawnSync('bash', ['-n'], { input: code, encoding: 'utf8' }).status,
        0
      )
      assert.ok(code.includes('/v1/videos/generations'))
      assert.ok(code.includes(model))
      assert.ok(!code.includes('/v1/chat/completions'))
    }
    const code = seedanceGenerationExample(model, 'https://gateway.example')
    assert.ok(code.includes("jq -er '.request_id'"))
    assert.ok(code.includes('failed|expired'))
    assert.ok(code.includes('/v1/videos/$REQUEST_ID/content'))
    const upload = seedanceUploadExample(model, 'https://gateway.example')
    assert.ok(upload.includes('start_image'))
    assert.ok(upload.includes('aspect_ratio:"adaptive"'))
    assert.ok(upload.includes('/v1/media/uploads'))
  })
}

test('video cards use the lowest server-provided tier and apply the selected group', () => {
  const model: PricingModel = {
    id: 1,
    model_name: 'seedance-2.5',
    quota_type: 1,
    model_ratio: 0,
    completion_ratio: 0,
    model_price: 1.241,
    enable_groups: ['official'],
    group_ratio: { official: 2 },
    video_resolution_prices: { '1080p': 3.139, '720p': 1.241, '480p': 0.5621 },
  }
  assert.equal(getVideoStartingPrice(model), 0.5621)
  assert.equal(formatRequestPrice(model, false, 1, 1, 'official'), '$1.1242')
  assert.equal(
    getVideoStartingPrice({ ...model, video_resolution_prices: undefined }),
    null
  )
  assert.equal(
    formatRequestPrice(
      { ...model, video_resolution_prices: undefined },
      false,
      1,
      1,
      'official'
    ),
    '$2.482'
  )
})
