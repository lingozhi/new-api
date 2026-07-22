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

import { createInstance } from 'i18next'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

import { BillingPromotionNotice } from './billing-promotion-notice'

const promotionCopy =
  'Simple, transparent billing: CNY and USD credits are calculated 1:1. Sign in to view pricing and get started.'

describe('BillingPromotionNotice', () => {
  it('markets the 1:1 billing offer without changing payment details', () => {
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

    const markup = renderToStaticMarkup(
      createElement(
        I18nextProvider,
        { i18n },
        createElement(BillingPromotionNotice)
      )
    )

    assert.match(markup, /role="note"/)
    assert.doesNotMatch(markup, /role="alert"/)
    assert.match(markup, /Simple, transparent billing/)
    assert.match(markup, /CNY and USD credits are calculated 1:1/)
    assert.match(markup, /Sign in to view pricing and get started/)
  })
})
