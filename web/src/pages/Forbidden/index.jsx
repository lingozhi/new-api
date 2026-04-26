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
  IllustrationNoAccess,
  IllustrationNoAccessDark,
} from '@douyinfe/semi-illustrations';
import { useTranslation } from 'react-i18next';

const Forbidden = () => {
  const { t } = useTranslation();
  return (
    <div className='playful-empty-state'>
      <div className='playful-floating-shape playful-floating-shape--circle -top-10 -left-10 h-32 w-32 bg-playful-secondary animate-pulse' />
      <div className='playful-floating-shape playful-floating-shape--square top-24 right-6 h-24 w-24 bg-playful-accent -rotate-12 shadow-pop' />
      <div className='playful-floating-shape playful-floating-shape--blob bottom-6 left-4 h-20 w-20 bg-playful-tertiary shadow-pop-sm' />

      <div className='sticker-card playful-empty-card flex flex-col items-center bg-white text-center'>
        <h1 className='playful-empty-code font-outfit text-6xl font-black rotate-3'>403</h1>
        <Empty
          image={<IllustrationNoAccess style={{ width: 250, height: 250 }} />}
          darkModeImage={
            <IllustrationNoAccessDark style={{ width: 250, height: 250 }} />
          }
          description={<span className='font-jakarta font-bold text-playful-foreground text-lg'>{t('您无权访问此页面，请联系管理员')}</span>}
        />
      </div>
    </div>
  );
};

export default Forbidden;
