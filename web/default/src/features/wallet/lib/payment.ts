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
import {
  PAYMENT_TYPES,
  DEFAULT_PRESET_MULTIPLIERS,
  DEFAULT_PAYMENT_TYPE,
  DEFAULT_MIN_TOPUP,
} from '../constants'
import type { PaymentMethod, PresetAmount, TopupInfo } from '../types'

// ============================================================================
// Payment Processing Functions
// ============================================================================

/**
 * Check if browser is Safari
 */
function isSafariBrowser(): boolean {
  return (
    navigator.userAgent.indexOf('Safari') > -1 &&
    navigator.userAgent.indexOf('Chrome') < 1
  )
}

/**
 * Submit payment form (for non-Stripe payments)
 */
export function submitPaymentForm(
  url: string,
  params: Record<string, unknown>
): void {
  const form = document.createElement('form')
  form.action = url
  form.method = 'POST'

  // Don't open in new tab for Safari
  if (!isSafariBrowser()) {
    form.target = '_blank'
  }

  // Add form parameters
  Object.entries(params).forEach(([key, value]) => {
    const input = document.createElement('input')
    input.type = 'hidden'
    input.name = key
    input.value = String(value)
    form.appendChild(input)
  })

  document.body.appendChild(form)
  form.submit()
  document.body.removeChild(form)
}

/**
 * Check if payment method is Stripe
 */
export function isStripePayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.STRIPE
}

/**
 * Check if payment method is Waffo Pancake
 *
 * Pancake is a metered-style payment that goes through a dedicated checkout
 * URL flow rather than the generic epay form submission, so it must be
 * special-cased in payment dispatch logic.
 */
export function isWaffoPancakePayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.WAFFO_PANCAKE
}

/**
 * Get default payment type from topup info
 */
export function getDefaultPaymentType(topupInfo: TopupInfo | null): string {
  if (!topupInfo) {
    return DEFAULT_PAYMENT_TYPE
  }

  // Return first available payment method or default
  if (topupInfo.pay_methods?.length > 0) {
    return topupInfo.pay_methods[0].type
  }

  if (topupInfo.enable_stripe_topup) {
    return PAYMENT_TYPES.STRIPE
  }

  if (topupInfo.enable_waffo_topup) {
    return PAYMENT_TYPES.WAFFO
  }

  if (topupInfo.enable_waffo_pancake_topup) {
    return PAYMENT_TYPES.WAFFO_PANCAKE
  }

  return DEFAULT_PAYMENT_TYPE
}

/**
 * Get the effective minimum for a standard payment method.
 */
export function getPaymentMethodMinTopup(
  topupInfo: TopupInfo | null,
  method: PaymentMethod
): number {
  const methodMinTopup = normalizeMinTopup(method.min_topup)

  if (method.type === PAYMENT_TYPES.STRIPE) {
    return Math.max(
      methodMinTopup,
      normalizeMinTopup(topupInfo?.stripe_min_topup)
    )
  }

  if (method.type === PAYMENT_TYPES.WAFFO_PANCAKE) {
    return Math.max(
      methodMinTopup,
      normalizeMinTopup(topupInfo?.waffo_pancake_min_topup)
    )
  }

  return Math.max(methodMinTopup, normalizeMinTopup(topupInfo?.min_topup))
}

/**
 * Get the smallest amount accepted by at least one visible payment path.
 */
export function getMinTopupAmount(topupInfo: TopupInfo | null): number {
  if (!topupInfo) {
    return DEFAULT_MIN_TOPUP
  }

  const minimums = topupInfo.pay_methods.map((method) =>
    getPaymentMethodMinTopup(topupInfo, method)
  )

  if (
    topupInfo.enable_waffo_topup &&
    Array.isArray(topupInfo.waffo_pay_methods) &&
    topupInfo.waffo_pay_methods.length > 0
  ) {
    minimums.push(normalizeMinTopup(topupInfo.waffo_min_topup))
  }

  if (minimums.length > 0) {
    return Math.min(...minimums)
  }

  if (topupInfo.enable_stripe_topup) {
    return normalizeMinTopup(topupInfo.stripe_min_topup)
  }

  if (topupInfo.enable_online_topup) {
    return normalizeMinTopup(topupInfo.min_topup)
  }

  if (topupInfo.enable_waffo_pancake_topup) {
    return normalizeMinTopup(topupInfo.waffo_pancake_min_topup)
  }

  if (topupInfo.enable_waffo_topup) {
    return normalizeMinTopup(topupInfo.waffo_min_topup)
  }

  return DEFAULT_MIN_TOPUP
}

function normalizeMinTopup(minAmount: number | undefined): number {
  return Number.isFinite(minAmount) && Number(minAmount) > 0
    ? Number(minAmount)
    : DEFAULT_MIN_TOPUP
}

/**
 * Generate preset amounts based on minimum topup
 */
export function generatePresetAmounts(minAmount: number): PresetAmount[] {
  const normalizedMinAmount = normalizeMinTopup(minAmount)

  return DEFAULT_PRESET_MULTIPLIERS.map((multiplier) => ({
    value: normalizedMinAmount * multiplier,
  }))
}

/**
 * Merge custom preset amounts with discounts
 */
export function mergePresetAmounts(
  amountOptions: number[],
  discounts: Record<number, number>
): PresetAmount[] {
  if (!amountOptions || amountOptions.length === 0) {
    return []
  }

  return amountOptions.map((amount) => ({
    value: amount,
    discount: discounts[amount] || 1.0,
  }))
}

/**
 * Resolve the visible preset tiers while ensuring each tier can be purchased.
 */
export function resolvePresetAmounts(
  amountOptions: number[],
  discounts: Record<number, number>,
  minAmount: number
): PresetAmount[] {
  const normalizedMinAmount = normalizeMinTopup(minAmount)
  const configuredPresets = mergePresetAmounts(amountOptions, discounts)

  if (configuredPresets.length === 0) {
    return generatePresetAmounts(normalizedMinAmount)
  }

  if (configuredPresets.some((preset) => preset.value >= normalizedMinAmount)) {
    return configuredPresets
  }

  return [
    ...configuredPresets,
    {
      value: normalizedMinAmount,
      discount: discounts[normalizedMinAmount] || 1,
    },
  ]
}
