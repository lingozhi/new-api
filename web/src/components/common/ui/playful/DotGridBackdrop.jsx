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

const DENSITY_CLASS = {
  sparse: 'playful-dot-grid-backdrop--sparse',
  normal: '',
  dense: 'playful-dot-grid-backdrop--dense',
};

/**
 * DotGridBackdrop — absolute-positioned dot-grid texture for pages/sections.
 *
 * Drop inside a `position: relative` parent. Pointer-events are disabled.
 * Props:
 *   density — 'sparse' | 'normal' | 'dense'
 *   opacity — 0..1 (default 1)
 *   rotate  — degrees, e.g. '-4deg' (default '0deg')
 */
const DotGridBackdrop = ({
  density = 'normal',
  opacity = 1,
  rotate = '0deg',
  className = '',
  style,
}) => (
  <div
    aria-hidden='true'
    className={`playful-dot-grid-backdrop ${DENSITY_CLASS[density] || ''} ${className}`.trim()}
    style={{ opacity, transform: `rotate(${rotate})`, ...style }}
  />
);

export default DotGridBackdrop;
