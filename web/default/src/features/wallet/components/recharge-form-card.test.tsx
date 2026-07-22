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
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'

import type { TopupInfo } from '../types'
import { RechargeFormCard } from './recharge-form-card'

const promotionCopy =
  "Promotion offer: CNY and USD credits are billed 1:1. Your selected amount is credited in full; Waffo checkout totals may vary with the payment provider's exchange rate."

function renderRechargeForm(enableWaffoPancakeTopup: boolean): string {
  const i18n = createInstance()
  void i18n.init({
    lng: 'en',
    resources: {
      en: {
        translation: {
          [promotionCopy]: promotionCopy,
        },
      },
    },
  })

  const topupInfo: TopupInfo = {
    enable_online_topup: true,
    enable_stripe_topup: false,
    pay_methods: [{ name: 'Waffo Pancake', type: 'waffo_pancake' }],
    min_topup: 1,
    stripe_min_topup: 1,
    amount_options: [500],
    amount_currency: 'CNY',
    discount: {},
    enable_waffo_pancake_topup: enableWaffoPancakeTopup,
    enable_redemption: false,
  }

  return renderToStaticMarkup(
    createElement(
      I18nextProvider,
      { i18n },
      createElement(RechargeFormCard, {
        topupInfo,
        presetAmounts: [{ value: 500 }],
        selectedPreset: 500,
        onSelectPreset: () => undefined,
        topupAmount: 500,
        onPaymentMethodSelect: () => undefined,
        paymentLoading: null,
        redemptionCode: '',
        onRedemptionCodeChange: () => undefined,
        onRedeem: () => undefined,
        redeeming: false,
        enableWaffoPancakeTopup,
      })
    )
  )
}

describe('RechargeFormCard promotion messaging', () => {
  it('explains 1:1 credit billing and provider exchange-rate variance for Waffo', () => {
    const enabledMarkup = renderRechargeForm(true)
    const disabledMarkup = renderRechargeForm(false)

    assert.match(enabledMarkup, /CNY and USD credits are billed 1:1/)
    assert.match(enabledMarkup, /credited in full/)
    assert.match(enabledMarkup, /payment provider&#x27;s exchange rate/)
    assert.doesNotMatch(disabledMarkup, /CNY and USD credits are billed 1:1/)
  })
})
