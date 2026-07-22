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
// @ts-expect-error The project does not load Bun's ambient test types.
import { describe, it, mock } from 'bun:test'
import assert from 'node:assert/strict'

import { createInstance } from 'i18next'
import { createElement, type PropsWithChildren } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

let confirmPayment: (() => void) | undefined
let confirmCalls = 0

function Wrapper({ children }: PropsWithChildren) {
  return createElement('div', null, children)
}

function Action({
  children,
  onClick,
}: PropsWithChildren<{ onClick?: () => void }>) {
  confirmPayment = onClick
  return createElement('button', { type: 'button' }, children)
}

mock.module('@/components/ui/alert-dialog', () => ({
  AlertDialog: Wrapper,
  AlertDialogAction: Action,
  AlertDialogCancel: Wrapper,
  AlertDialogContent: Wrapper,
  AlertDialogDescription: Wrapper,
  AlertDialogFooter: Wrapper,
  AlertDialogHeader: Wrapper,
  AlertDialogTitle: Wrapper,
}))

const { PaymentConfirmDialog } = await import('./payment-confirm-dialog')

function renderPaymentDialog(): string {
  const i18n = createInstance()
  void i18n.init({
    lng: 'en',
    resources: {
      en: {
        translation: {
          'Confirm Payment': 'Confirm Payment',
          'Review your payment details': 'Review your payment details',
          'Topup Amount': 'Topup Amount',
          'You Pay': 'You Pay',
          'Payment Method': 'Payment Method',
          Cancel: 'Cancel',
        },
      },
    },
  })

  return renderToStaticMarkup(
    createElement(
      I18nextProvider,
      { i18n },
      createElement(PaymentConfirmDialog, {
        open: true,
        onOpenChange: () => {},
        onConfirm: () => confirmCalls++,
        topupAmount: 100,
        paymentAmount: 97.47,
        paymentMethod: {
          name: 'Waffo Pancake',
          type: 'waffo_pancake',
        },
        calculating: false,
        discountRate: 1,
        usdExchangeRate: 1,
        amountCurrency: 'CNY',
        paymentCurrency: 'CNY',
        processing: false,
      })
    )
  )
}

describe('PaymentConfirmDialog', () => {
  it('keeps the selected amount and method without repeating the provider estimate', () => {
    confirmCalls = 0
    const markup = renderPaymentDialog()

    assert.match(markup, /Topup Amount/)
    assert.match(markup, /Waffo Pancake/)
    assert.doesNotMatch(markup, /You Pay/)
    assert.doesNotMatch(markup, /97\.47/)
    assert.match(markup, /Confirm Payment/)

    confirmPayment?.()
    assert.equal(confirmCalls, 1)
  })
})
