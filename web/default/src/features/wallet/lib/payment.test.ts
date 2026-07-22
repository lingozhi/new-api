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
import { describe, it } from 'node:test'

import type { TopupInfo } from '../types'
import {
  getMinTopupAmount,
  getPaymentMethodMinTopup,
  resolvePresetAmounts,
} from './payment'

function createTopupInfo(overrides: Partial<TopupInfo> = {}): TopupInfo {
  return {
    enable_online_topup: true,
    enable_stripe_topup: false,
    pay_methods: [],
    min_topup: 1,
    stripe_min_topup: 1,
    amount_options: [],
    discount: {},
    ...overrides,
  }
}

describe('resolvePresetAmounts', () => {
  it('keeps configured tiers and preserves discounts when one is purchasable', () => {
    const presets = resolvePresetAmounts([5, 10, 20], { 10: 0.9 }, 10)

    assert.deepEqual(presets, [
      { value: 5, discount: 1 },
      { value: 10, discount: 0.9 },
      { value: 20, discount: 1 },
    ])
  })

  it('adds a purchasable tier when configured options are all too small', () => {
    const presets = resolvePresetAmounts([1, 5], {}, 10)

    assert.deepEqual(presets, [
      { value: 1, discount: 1 },
      { value: 5, discount: 1 },
      { value: 10, discount: 1 },
    ])
  })

  it('uses the default positive minimum for an invalid minimum', () => {
    const presets = resolvePresetAmounts([], {}, 0)

    assert.equal(presets[0].value, 1)
    assert.ok(presets.every((preset) => preset.value > 0))
  })
})

describe('payment method minimums', () => {
  it('uses the smallest minimum accepted by a visible payment method', () => {
    const topupInfo = createTopupInfo({
      min_topup: 10,
      stripe_min_topup: 1,
      pay_methods: [
        { name: 'Alipay', type: 'alipay' },
        { name: 'Stripe', type: 'stripe' },
      ],
    })

    assert.equal(getMinTopupAmount(topupInfo), 1)
  })

  it('honors a method-specific minimum over the gateway default', () => {
    const topupInfo = createTopupInfo({
      pay_methods: [{ name: 'Alipay', type: 'alipay', min_topup: 50 }],
    })

    assert.equal(getMinTopupAmount(topupInfo), 50)
    assert.equal(
      getPaymentMethodMinTopup(topupInfo, topupInfo.pay_methods[0]),
      50
    )
  })

  it('never allows a method minimum below its gateway minimum', () => {
    const topupInfo = createTopupInfo({
      min_topup: 10,
      stripe_min_topup: 20,
      waffo_pancake_min_topup: 30,
    })

    assert.equal(
      getPaymentMethodMinTopup(topupInfo, {
        name: 'Alipay',
        type: 'alipay',
        min_topup: 1,
      }),
      10
    )
    assert.equal(
      getPaymentMethodMinTopup(topupInfo, {
        name: 'Stripe',
        type: 'stripe',
        min_topup: 1,
      }),
      20
    )
    assert.equal(
      getPaymentMethodMinTopup(topupInfo, {
        name: 'Waffo Pancake',
        type: 'waffo_pancake',
        min_topup: 1,
      }),
      30
    )
  })
})
