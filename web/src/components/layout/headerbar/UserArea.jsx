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

import React, { useRef } from 'react';
import { Link } from 'react-router-dom';
import { Avatar, Button, Dropdown, Typography } from '@douyinfe/semi-ui';
import { ChevronDown } from 'lucide-react';
import {
  IconExit,
  IconUserSetting,
  IconCreditCard,
  IconKey,
} from '@douyinfe/semi-icons';
import { stringToColor } from '../../../helpers';
import SkeletonWrapper from '../components/SkeletonWrapper';

const UserArea = ({
  userState,
  isLoading,
  isMobile,
  isSelfUseMode,
  logout,
  navigate,
  t,
}) => {
  const dropdownRef = useRef(null);
  if (isLoading) {
    return (
      <SkeletonWrapper
        loading={true}
        type='userArea'
        width={50}
        isMobile={isMobile}
      />
    );
  }

  if (userState.user) {
    return (
      <div className='relative' ref={dropdownRef}>
        <Dropdown
          position='bottomRight'
          getPopupContainer={() => dropdownRef.current}
          render={
            <Dropdown.Menu className='playful-dropdown-menu'>
              <Dropdown.Item
                onClick={() => {
                  navigate('/console/personal');
                }}
                className='playful-dropdown-item !px-3 !py-2 !text-sm !text-semi-color-text-0'
              >
                <div className='flex items-center gap-2'>
                  <IconUserSetting
                    size='small'
                    className='text-gray-500'
                  />
                  <span>{t('个人设置')}</span>
                </div>
              </Dropdown.Item>
              <Dropdown.Item
                onClick={() => {
                  navigate('/console/token');
                }}
                className='playful-dropdown-item !px-3 !py-2 !text-sm !text-semi-color-text-0'
              >
                <div className='flex items-center gap-2'>
                  <IconKey
                    size='small'
                    className='text-gray-500'
                  />
                  <span>{t('令牌管理')}</span>
                </div>
              </Dropdown.Item>
              <Dropdown.Item
                onClick={() => {
                  navigate('/console/topup');
                }}
                className='playful-dropdown-item !px-3 !py-2 !text-sm !text-semi-color-text-0'
              >
                <div className='flex items-center gap-2'>
                  <IconCreditCard
                    size='small'
                    className='text-gray-500'
                  />
                  <span>{t('钱包管理')}</span>
                </div>
              </Dropdown.Item>
              <Dropdown.Item
                onClick={logout}
                className='playful-dropdown-item !px-3 !py-2 !text-sm !text-semi-color-text-0'
              >
                <div className='flex items-center gap-2'>
                  <IconExit
                    size='small'
                    className='text-gray-500'
                  />
                  <span>{t('退出')}</span>
                </div>
              </Dropdown.Item>
            </Dropdown.Menu>
          }
        >
          <span className='inline-flex'>
            <Button
              theme='outline'
              type='tertiary'
              className='playful-user-trigger !px-2.5 !py-1.5'
            >
              <Avatar
                size='extra-small'
                color={stringToColor(userState.user.username)}
                className='playful-avatar mr-1'
              >
                {userState.user.username[0].toUpperCase()}
              </Avatar>
              <span className='hidden md:inline'>
                <Typography.Text className='!text-sm !font-jakarta !font-bold !text-playful-foreground mr-1'>
                  {userState.user.username}
                </Typography.Text>
              </span>
              <ChevronDown
                size={16}
                strokeWidth={3}
                className='text-playful-foreground'
              />
            </Button>
          </span>
        </Dropdown>
      </div>
    );
  }

  const showRegisterButton = !isSelfUseMode;

  return (
    <div className='flex items-center gap-2'>
        <Link to='/login' className='flex'>
          <Button className='playful-pill-ghost !bg-playful-card !text-playful-foreground'>
            <span className='font-jakarta font-bold px-1'>{t('登录')}</span>
          </Button>
        </Link>
        {showRegisterButton && (
          <div className='hidden md:block'>
            <Link to='/register' className='flex'>
              <Button className='candy-btn !bg-playful-accent !text-white'>
                <span className='font-jakarta font-bold px-1'>{t('注册')}</span>
              </Button>
            </Link>
          </div>
        )}
      </div>
    );
};

export default UserArea;
