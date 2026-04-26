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
import { Empty } from '@douyinfe/semi-ui';
import {
  IllustrationNotFound,
  IllustrationNotFoundDark,
} from '@douyinfe/semi-illustrations';
import { useTranslation } from 'react-i18next';

const NotFound = () => {
  const { t } = useTranslation();
  return (
    <div className='playful-empty-state'>
      <div className='playful-floating-shape playful-floating-shape--circle -top-10 -left-10 h-32 w-32 bg-playful-accent animate-pulse' />
      <div className='playful-floating-shape playful-floating-shape--square top-24 right-6 h-24 w-24 bg-playful-tertiary -rotate-12 shadow-pop' />
      <div className='playful-floating-shape playful-floating-shape--blob bottom-6 left-4 h-20 w-20 bg-playful-secondary shadow-pop-sm' />

      <div className='sticker-card playful-empty-card flex flex-col items-center bg-white text-center'>
        <h1 className='playful-empty-code font-outfit text-6xl font-black rotate-3'>404</h1>
        <Empty
          image={<IllustrationNotFound style={{ width: 250, height: 250 }} />}
          darkModeImage={
            <IllustrationNotFoundDark style={{ width: 250, height: 250 }} />
          }
          description={<span className='font-jakarta font-bold text-playful-foreground text-lg'>{t('页面未找到，请检查您的浏览器地址是否正确')}</span>}
        />
      </div>
    </div>
  );
};

export default NotFound;
