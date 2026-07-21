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

import { DEFAULT_CURRENCY_CONFIG } from '@/stores/system-config-store'

import { formatBillingCurrencyFromUSD } from './currency'

describe('billing currency formatting', () => {
  test('formats against the subscribed currency snapshot supplied by the caller', () => {
    const cnyConfig = {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CNY' as const,
      usdExchangeRate: 7,
    }

    assert.equal(
      formatBillingCurrencyFromUSD(
        10,
        { abbreviate: false, locale: 'en-US' },
        cnyConfig
      ),
      '¥70'
    )
  })

  test('keeps token display mode monetary for billing surfaces', () => {
    const tokenConfig = {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'TOKENS' as const,
    }

    assert.equal(
      formatBillingCurrencyFromUSD(
        10,
        { abbreviate: false, locale: 'en-US' },
        tokenConfig
      ),
      '$10'
    )
  })
})
