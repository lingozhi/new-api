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
import { Modal } from '@douyinfe/semi-ui';

/**
 * PlayfulModal — Semi's Modal wrapped with the Playful chrome.
 *
 * Props:
 *   tone — 'tertiary' (default, yellow header) | 'violet' | 'pink' | 'mint' |
 *          'neutral'
 *
 * All other props pass straight to Semi's Modal.
 */
const PlayfulModal = ({ tone = 'tertiary', className = '', children, ...rest }) => {
  const toneClass = `playful-modal--tone-${tone}`;
  return (
    <Modal className={`playful-modal ${toneClass} ${className}`.trim()} {...rest}>
      {children}
    </Modal>
  );
};

PlayfulModal.info = Modal.info;
PlayfulModal.error = Modal.error;
PlayfulModal.success = Modal.success;
PlayfulModal.warning = Modal.warning;
PlayfulModal.confirm = Modal.confirm;

export default PlayfulModal;
