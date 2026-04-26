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
import { RefreshCw, Search, Sparkles } from 'lucide-react';
import { PlayfulKicker, StickerButton } from '../common/ui/playful';

const DashboardHeader = ({
  getGreeting,
  greetingVisible,
  showSearchModal,
  refresh,
  loading,
  t,
}) => {
  return (
    <div className='playful-dashboard-header'>
      <div className='flex flex-col gap-2'>
        <PlayfulKicker tone='tertiary' icon={<Sparkles size={12} />}>
          {t('Dashboard')}
        </PlayfulKicker>
        <h2
          className='playful-dashboard-greeting text-2xl md:text-3xl !mb-0'
          style={{ opacity: greetingVisible ? 1 : 0 }}
        >
          {getGreeting}
        </h2>
      </div>
      <div className='flex items-center gap-2'>
        <StickerButton
          variant='secondary'
          size='sm'
          icon={<Search size={16} />}
          onClick={showSearchModal}
        >
          {t('筛选')}
        </StickerButton>
        <StickerButton
          variant='primary'
          size='sm'
          icon={<RefreshCw size={16} />}
          onClick={refresh}
          loading={loading}
        >
          {t('刷新')}
        </StickerButton>
      </div>
    </div>
  );
};

export default DashboardHeader;
