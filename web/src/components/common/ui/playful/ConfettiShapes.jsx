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

const TONE_COLOR = {
  violet: 'var(--playful-accent)',
  pink: 'var(--playful-secondary)',
  tertiary: 'var(--playful-tertiary)',
  mint: 'var(--playful-quaternary)',
  neutral: 'var(--playful-card)',
};

/**
 * ConfettiShape — absolute-positioned decorative shape.
 *
 * Props:
 *   kind      — 'circle' | 'square' | 'pill' | 'blob' | 'triangle'
 *   tone      — 'violet' | 'pink' | 'tertiary' | 'mint' | 'neutral'
 *   size      — number (px), default 48
 *   rotate    — number (deg), default 0
 *   top, left, right, bottom — CSS offsets (string or number)
 *   className — extra classes
 *
 * Render inside a `position: relative` container. For clusters, use
 * <ConfettiShapes> to group them with a common `aria-hidden` wrapper.
 */
export const ConfettiShape = ({
  kind = 'circle',
  tone = 'tertiary',
  size = 48,
  rotate = 0,
  top,
  left,
  right,
  bottom,
  className = '',
  style,
}) => {
  const baseStyle = {
    top,
    left,
    right,
    bottom,
    transform: `rotate(${rotate}deg)`,
    ...style,
  };

  if (kind === 'triangle') {
    return (
      <span
        aria-hidden='true'
        className={`playful-confetti playful-confetti--triangle ${className}`.trim()}
        style={{
          ...baseStyle,
          '--tri-size': `${size / 2}px`,
          '--tri-color': TONE_COLOR[tone] || TONE_COLOR.tertiary,
        }}
      />
    );
  }

  const kindClass = `playful-confetti--${kind}`;
  const toneClass = `playful-confetti--tone-${tone}`;

  return (
    <span
      aria-hidden='true'
      className={`playful-confetti ${kindClass} ${toneClass} ${className}`.trim()}
      style={{
        width: size,
        height: kind === 'pill' ? size / 2 : size,
        backgroundColor: TONE_COLOR[tone] || TONE_COLOR.tertiary,
        ...baseStyle,
      }}
    />
  );
};

/**
 * ConfettiShapes — wrapper that renders its children at z-index 0 inside a
 * decorative layer that ignores pointer events.
 */
const ConfettiShapes = ({ className = '', children }) => (
  <div
    aria-hidden='true'
    className={`pointer-events-none absolute inset-0 z-0 overflow-hidden ${className}`.trim()}
  >
    {children}
  </div>
);

export default ConfettiShapes;
