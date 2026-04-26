/*
Copyright (C) 2025 QuantumNous

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

import React from 'react';

const SIZE_CLASS = {
  sm: 'playful-floating-badge--sm',
  md: 'playful-floating-badge--md',
  lg: 'playful-floating-badge--lg',
};

const TONE_CLASS = {
  neutral: 'playful-floating-badge--tone-neutral',
  violet: 'playful-floating-badge--tone-violet',
  pink: 'playful-floating-badge--tone-pink',
  tertiary: 'playful-floating-badge--tone-tertiary',
  mint: 'playful-floating-badge--tone-mint',
};

/**
 * FloatingIconBadge — circular icon badge with 2px border and mini shadow.
 *
 * Use as StickerCard.floatingIcon, or inline as an inline decoration.
 *
 * Props:
 *   icon — node (e.g. a Lucide icon)
 *   tone — 'neutral' (default) | 'violet' | 'pink' | 'tertiary' | 'mint'
 *   size — 'sm' | 'md' | 'lg'
 */
const FloatingIconBadge = ({
  icon,
  tone = 'neutral',
  size = 'md',
  className = '',
  ...rest
}) => (
  <span
    className={`playful-floating-badge ${SIZE_CLASS[size] || SIZE_CLASS.md} ${TONE_CLASS[tone] || TONE_CLASS.neutral} ${className}`.trim()}
    {...rest}
  >
    {icon}
  </span>
);

export default FloatingIconBadge;
