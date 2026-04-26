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
import { ToastContainer } from 'react-toastify';
import { buildToastClassName, PLAYFUL_TOAST_DEFAULTS } from './toast';

/**
 * PlayfulToastContainer — a react-toastify <ToastContainer> pre-wired with
 * Playful Geometric chrome. Drop this into the app shell; the rest of the
 * codebase's `showSuccess`/`showError` etc. will pick up the styling.
 */
const PlayfulToastContainer = (props) => (
  <ToastContainer
    position='top-right'
    autoClose={4000}
    hideProgressBar={false}
    newestOnTop
    closeOnClick
    pauseOnFocusLoss
    pauseOnHover
    draggable
    theme='light'
    icon={false}
    toastClassName={(ctx) => buildToastClassName(ctx?.type)}
    bodyClassName='font-jakarta text-sm font-semibold'
    style={PLAYFUL_TOAST_DEFAULTS.containerStyle}
    {...props}
  />
);

export default PlayfulToastContainer;
