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
import { Link } from 'react-router-dom';
import { Typography, Tag } from '@douyinfe/semi-ui';
import SkeletonWrapper from '../components/SkeletonWrapper';

const HeaderLogo = ({
  isMobile,
  isConsoleRoute,
  logo,
  logoLoaded,
  isLoading,
  systemName,
  isSelfUseMode,
  isDemoSiteMode,
  t,
}) => {
  if (isMobile && isConsoleRoute) {
    return null;
  }

  return (
    <Link to='/' className='group flex min-w-0 items-center gap-2'>
      <div className='playful-logo-mark relative flex h-9 w-9 items-center justify-center overflow-hidden rounded-[14px] bg-playful-secondary md:h-10 md:w-10 md:rounded-[16px]'>
        <SkeletonWrapper loading={isLoading || !logoLoaded} type='image' />
        <img
          src={logo}
          alt='logo'
          className={`absolute inset-0 w-full h-full object-contain p-0.5 transition-all duration-200 ${!isLoading && logoLoaded ? 'opacity-100' : 'opacity-0'}`}
        />
      </div>
      <div className='hidden min-w-0 md:flex md:items-center md:gap-3'>
        <div className='flex min-w-0 items-center gap-3'>
          <SkeletonWrapper
            loading={isLoading}
            type='title'
            width={120}
            height={24}
          >
            <Typography.Title
              heading={4}
              className='!mb-0 !truncate !font-outfit !text-[1.35rem] md:!text-[1.5rem] !font-black !tracking-[-0.03em] !text-playful-foreground'
            >
              {systemName}
            </Typography.Title>
          </SkeletonWrapper>
          {(isSelfUseMode || isDemoSiteMode) && !isLoading && (
            <span
              className={`playful-header-status-tag ${isSelfUseMode ? 'bg-playful-accent text-white' : 'bg-playful-tertiary text-playful-foreground'}`}
            >
              {isSelfUseMode ? t('自用模式') : t('演示站点')}
            </span>
          )}
        </div>
      </div>
    </Link>
  );
};

export default HeaderLogo;
