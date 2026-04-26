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

/**
 * Playful toast helpers.
 *
 * Keeps toastify class-name / container-style decisions in one place so
 * future theme tweaks don't require hunting through the tree.
 */

export const PLAYFUL_TOAST_DEFAULTS = {
  containerStyle: {
    top: 'calc(var(--playful-header-offset, 96px) + 20px)',
    right: '20px',
  },
};

const TYPE_CLASS = {
  success: 'playful-toast playful-toast--success',
  error: 'playful-toast playful-toast--error',
  info: 'playful-toast playful-toast--info',
  warning: 'playful-toast playful-toast--warning',
  default: 'playful-toast',
};

export function buildToastClassName(type) {
  return TYPE_CLASS[type] || TYPE_CLASS.default;
}
