/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

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

import { calculatePresetPricing, getTopUpDisplayValue } from './format'

describe('calculatePresetPricing', () => {
  it('uses CNY presets as both the displayed and payable amount', () => {
    const pricing = calculatePresetPricing(10, 7.3, 1, 7.3, 'CNY')

    assert.equal(pricing.displayValue, 10)
    assert.equal(pricing.actualPrice, 10)
  })

  it('preserves the existing USD preset conversion', () => {
    const pricing = calculatePresetPricing(10, 7.3, 1, 7.3, 'USD')

    assert.equal(pricing.displayValue, 73)
    assert.equal(pricing.actualPrice, 73)
  })

  it('does not convert a CNY amount again in payment confirmation', () => {
    assert.equal(getTopUpDisplayValue(10, 7.3, 'CNY'), 10)
    assert.equal(getTopUpDisplayValue(10, 7.3, 'USD'), 73)
  })
})
