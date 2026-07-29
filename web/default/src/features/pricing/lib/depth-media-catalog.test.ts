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
  buildDepthMediaAiIntegrationGuide,
  buildDepthMediaJobSample,
  consolidateDepthMediaModels,
} from './depth-media-catalog'
import { getFixedPriceUnit, supportsPerformanceMetrics } from './model-helpers'
import { calculateMediaVariantPrice } from './price'

const sourceModels: PricingModel[] = [
  {
    id: 73,
    model_name: 'depth-anything-v2-small-video',
    description: 'legacy depth',
    quota_type: 1,
    model_ratio: 0,
    completion_ratio: 0,
    model_price: 0.002,
    enable_groups: ['media'],
    supported_endpoint_types: ['openai', 'depth-media'],
  },
  ...[
    ['background-remove-fast', 0.02],
    ['background-remove-quality', 0.03],
    ['background-remove-matting', 0.03],
    ['image-upscale-fast-2x', 0.02],
    ['image-upscale-fast-4x', 0.02],
    ['image-upscale-fidelity-4x', 0.05],
    ['image-upscale-sharp-4x', 0.05],
  ].map(([modelName, price]) => ({
    id: 74,
    model_name: String(modelName),
    description: `legacy ${modelName}`,
    quota_type: 1 as const,
    model_ratio: 0,
    completion_ratio: 0,
    model_price: Number(price),
    enable_groups: ['media'],
    supported_endpoint_types: ['openai', 'depth-media'],
  })),
]

describe('DepthMedia model plaza catalog', () => {
  test('collapses eight implementation profiles into three public models', () => {
    const models = consolidateDepthMediaModels(sourceModels)

    assert.deepEqual(
      models.map((model) => model.model_name),
      ['depth-video', 'background-remove', 'image-upscale']
    )
    assert.ok(
      models.every(
        (model) =>
          model.supported_endpoint_types?.length === 1 &&
          model.supported_endpoint_types[0] === 'depth-media'
      )
    )
  })

  test('publishes parameter-specific prices in each drawer profile', () => {
    const models = consolidateDepthMediaModels(sourceModels)
    const background = models.find(
      (model) => model.model_name === 'background-remove'
    )
    const upscale = models.find((model) => model.model_name === 'image-upscale')

    assert.deepEqual(
      background?.api_profile?.pricing_variants?.map(
        (variant) => variant.price
      ),
      [0.02, 0.03, 0.03]
    )
    assert.deepEqual(
      upscale?.api_profile?.pricing_variants?.map((variant) => variant.price),
      [0.02, 0.02, 0.05, 0.05]
    )
    assert.equal(getFixedPriceUnit(models[0]), 'seconds')
    assert.equal(getFixedPriceUnit(models[1]), 'request')
  })

  test('applies group and recharge multipliers to parameter prices', () => {
    assert.equal(calculateMediaVariantPrice(0.03, 1.5, false, 1, 1), 0.045)
    assert.equal(calculateMediaVariantPrice(0.03, 1.5, true, 4, 8), 0.0225)
  })

  test('keeps polling in SDK samples until the task reaches a terminal state', () => {
    for (const language of ['python', 'typescript', 'javascript'] as const) {
      const sample = buildDepthMediaJobSample(language, {
        baseUrl: 'https://api.opwan.ai',
        apiKeyEnv: 'OPWAN_API_KEY',
        modelName: 'background-remove',
        endpointPath: '/v1/jobs',
      })

      assert.match(sample, /SUCCESS/)
      assert.match(sample, /FAILURE/)
      assert.match(sample, /sleep|setTimeout/)
      assert.match(sample, /raise_for_status|response\.ok|result\.ok/)
      assert.match(sample, /deadline/i)
      assert.match(sample, /timed out/i)
      if (language === 'python') {
        assert.match(sample, /timeout=/)
      } else {
        assert.match(sample, /AbortSignal\.timeout/)
      }
    }
  })

  test('does not advertise synthetic performance metrics for media aliases', () => {
    const models = consolidateDepthMediaModels(sourceModels)

    assert.equal(supportsPerformanceMetrics(models[0]), false)
    assert.equal(supportsPerformanceMetrics(sourceModels[0]), true)
  })

  test('generates the unified jobs contract instead of chat completions', () => {
    const sample = buildDepthMediaJobSample('curl', {
      baseUrl: 'https://api.opwan.ai',
      apiKeyEnv: 'OPWAN_API_KEY',
      modelName: 'image-upscale',
      endpointPath: '/v1/jobs',
    })

    assert.match(sample, /POST https:\/\/api\.opwan\.ai\/v1\/jobs/)
    assert.match(sample, /"model": "image-upscale"/)
    assert.match(sample, /"operation": "upscale"/)
    assert.match(sample, /"quality": "fast"/)
    assert.match(sample, /"scale": 2/)
    assert.match(sample, /\/v1\/jobs\/<TASK_ID>/)
    assert.doesNotMatch(sample, /chat\/completions/)
    assert.doesNotMatch(sample, /messages/)
  })

  test('generates a self-contained AI integration guide for one-click copy', () => {
    const models = consolidateDepthMediaModels(sourceModels)
    const guide = buildDepthMediaAiIntegrationGuide({
      baseUrl: 'https://api.opwan.ai',
      apiKeyEnv: 'OPWAN_API_KEY',
      selectedModel: models[2],
      publicModels: models,
    })

    assert.match(guide, /^# Opwan DepthMedia API integration guide/m)
    assert.match(guide, /POST https:\/\/api\.opwan\.ai\/v1\/jobs/)
    assert.match(guide, /GET https:\/\/api\.opwan\.ai\/v1\/jobs\/\{task_id\}/)
    assert.match(guide, /Authorization: Bearer \$OPWAN_API_KEY/)
    assert.match(guide, /image-upscale/)
    assert.match(guide, /operation=upscale, quality=fidelity, scale=4/)
    assert.match(guide, /0\.05 USD per request/)
    assert.match(guide, /background-remove/)
    assert.match(guide, /depth-video/)
    assert.match(guide, /Webhook/)
    assert.match(guide, /X-Webhook-Signature/)
    assert.match(guide, /v1=<hex>/)
    assert.match(guide, /replay window/)
    assert.match(guide, /SUCCESS/)
    assert.match(guide, /FAILURE/)
    assert.match(guide, /Do not use \/v1\/chat\/completions/)
  })
})
