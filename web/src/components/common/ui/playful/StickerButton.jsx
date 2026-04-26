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
import { Button } from '@douyinfe/semi-ui';

/**
 * StickerButton — the canonical Playful Geometric button.
 *
 * Props:
 *   variant — 'primary' | 'secondary' | 'ghost' | 'danger' | 'icon'
 *   size    — 'sm' | 'md' | 'lg'   (md = 48px tap target, the spec default)
 *   block   — fills parent width
 *
 * Any other prop passes through to Semi's Button. Pair this with
 * <StickerButton icon={<Plus />}> for icon-first buttons.
 */
const VARIANT_CLASS = {
  primary: 'candy-btn !text-white',
  secondary: 'candy-btn-secondary',
  ghost: 'playful-pill-ghost !bg-playful-card !text-playful-foreground',
  danger: 'candy-btn !bg-[#ef4444] !text-white !border-playful-foreground',
  icon: 'playful-icon-button',
};

const SIZE_CLASS = {
  sm: '!min-h-[36px] !px-3 !text-sm',
  md: '!min-h-[48px] !px-5 !text-base',
  lg: '!min-h-[56px] !px-7 !text-lg',
};

const StickerButton = React.forwardRef(function StickerButton(
  {
    variant = 'primary',
    size = 'md',
    block = false,
    className = '',
    children,
    icon,
    ...rest
  },
  ref,
) {
  const variantClass = VARIANT_CLASS[variant] || VARIANT_CLASS.primary;
  const sizeClass = variant === 'icon' ? '' : SIZE_CLASS[size] || SIZE_CLASS.md;
  const blockClass = block ? 'w-full' : '';

  // Semi's `theme='solid'` is the best match for our painted primary/secondary
  // backgrounds — it gives us a plain <button> with no extra skin chrome.
  return (
    <Button
      ref={ref}
      theme={variant === 'ghost' || variant === 'icon' ? 'borderless' : 'solid'}
      type={variant === 'danger' ? 'danger' : 'primary'}
      icon={icon}
      className={`${variantClass} ${sizeClass} ${blockClass} ${className}`.trim()}
      {...rest}
    >
      {children}
    </Button>
  );
});

export default StickerButton;
