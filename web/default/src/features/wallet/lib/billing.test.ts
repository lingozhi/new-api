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

import {
  formatCheckoutPaymentAmount,
  formatProviderPaymentAmount,
  formatTopUpRecordAmount,
} from './billing'

describe('provider payment amount formatting', () => {
  test('labels new USD and CNY payments explicitly', () => {
    assert.equal(formatProviderPaymentAmount(12.5, 'USD'), 'USD 12.5')
    assert.equal(formatProviderPaymentAmount(88, 'cny'), 'CNY 88')
  })

  test('preserves the legacy raw-number display without provider currency', () => {
    assert.equal(formatProviderPaymentAmount(12.5, ''), '12.5')
    assert.equal(formatProviderPaymentAmount(12.5, undefined), '12.5')
  })
})

describe('top-up record amount formatting', () => {
  test('uses the original CNY amount snapshot instead of the current exchange rate', () => {
    assert.equal(formatTopUpRecordAmount(10, 684932, 'CNY'), 'CNY 10')
  })

  test('falls back to provider currency for CNY orders created before amount snapshots', () => {
    assert.equal(
      formatTopUpRecordAmount(10, 684932, undefined, 'CNY'),
      'CNY 10'
    )
  })
})

describe('checkout payment amount formatting', () => {
  test('uses the provider currency when the gateway charges in USD', () => {
    assert.equal(formatCheckoutPaymentAmount(1.37, 'USD'), 'USD 1.37')
  })
})
