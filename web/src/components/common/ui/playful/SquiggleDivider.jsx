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

const COLOR_CLASS = {
  default: '',
  accent: 'playful-squiggle--accent',
  pink: 'playful-squiggle--pink',
  tertiary: 'playful-squiggle--tertiary',
  mint: 'playful-squiggle--mint',
};

/**
 * SquiggleDivider — hand-drawn-feeling section divider.
 * Props:
 *   color   — 'default' | 'accent' | 'pink' | 'tertiary' | 'mint'
 *   height  — number (default 18)
 *   label   — optional centered label pill (e.g. "or")
 */
const SquiggleDivider = ({
  color = 'default',
  height = 18,
  label,
  className = '',
}) => {
  const colorClass = COLOR_CLASS[color] || '';
  const id = React.useId();

  return (
    <div
      className={`relative flex items-center justify-center my-6 ${className}`.trim()}
      aria-hidden={label ? undefined : true}
      role={label ? 'separator' : 'presentation'}
    >
      <svg
        className={`playful-squiggle ${colorClass}`}
        viewBox='0 0 400 20'
        preserveAspectRatio='none'
        height={height}
      >
        <defs>
          <pattern
            id={`squiggle-${id}`}
            x='0'
            y='0'
            width='40'
            height='20'
            patternUnits='userSpaceOnUse'
          >
            <path
              d='M0 10 Q10 0, 20 10 T40 10'
              fill='none'
              stroke='currentColor'
              strokeWidth='2.5'
              strokeLinecap='round'
            />
          </pattern>
        </defs>
        <rect width='400' height='20' fill={`url(#squiggle-${id})`} />
      </svg>
      {label ? (
        <span className='absolute left-1/2 -translate-x-1/2 bg-playful-bg px-3 py-1 font-outfit text-xs font-bold uppercase tracking-[0.18em] text-playful-muted-fg'>
          {label}
        </span>
      ) : null}
    </div>
  );
};

export default SquiggleDivider;
