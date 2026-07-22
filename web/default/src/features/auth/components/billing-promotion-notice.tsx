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
import { BadgePercent } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { cn } from '@/lib/utils'

type BillingPromotionNoticeProps = {
  className?: string
}

export function BillingPromotionNotice(props: BillingPromotionNoticeProps) {
  const { t } = useTranslation()

  return (
    <Alert
      role='note'
      className={cn('border-emerald-500/30 bg-emerald-500/5', props.className)}
    >
      <BadgePercent className='text-emerald-600 dark:text-emerald-400' />
      <AlertDescription className='text-xs leading-5 sm:text-sm'>
        {t(
          'Promotion offer: CNY and USD credits are billed 1:1, and your selected top-up amount is credited in full. Sign in to get started.'
        )}
      </AlertDescription>
    </Alert>
  )
}
