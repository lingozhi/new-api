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
 * Playful Geometric primitives — canonical import path.
 *
 * Usage:
 *   import {
 *     StickerCard, StickerButton, PlayfulKicker, SquiggleDivider,
 *     DotGridBackdrop, ConfettiShapes, ConfettiShape, FloatingIconBadge,
 *     SectionHeader, PlayfulTable, PlayfulFormField, PlayfulModal,
 *     PlayfulTag, PlayfulEmpty, PlayfulToastContainer,
 *   } from '@/components/common/ui/playful';
 *
 * See ./README.md for the full prop reference and usage guide.
 */

export { default as StickerCard } from './StickerCard';
export { default as StickerButton } from './StickerButton';
export { default as PlayfulKicker } from './PlayfulKicker';
export { default as SquiggleDivider } from './SquiggleDivider';
export { default as DotGridBackdrop } from './DotGridBackdrop';
export { default as ConfettiShapes, ConfettiShape } from './ConfettiShapes';
export { default as FloatingIconBadge } from './FloatingIconBadge';
export { default as SectionHeader } from './SectionHeader';
export { default as PlayfulTable } from './PlayfulTable';
export { default as PlayfulFormField } from './PlayfulFormField';
export { default as PlayfulModal } from './PlayfulModal';
export { default as PlayfulTag } from './PlayfulTag';
export { default as PlayfulEmpty } from './PlayfulEmpty';
export { default as PlayfulToastContainer } from './PlayfulToast';
export { buildToastClassName, PLAYFUL_TOAST_DEFAULTS } from './toast';
