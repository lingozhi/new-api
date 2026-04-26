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
import {
  Avatar,
  Card,
  Tag,
  Divider,
  Typography,
  Badge,
} from '@douyinfe/semi-ui';
import {
  isRoot,
  isAdmin,
  renderQuota,
  stringToColor,
} from '../../../../helpers';
import { Coins, BarChart2, Users } from 'lucide-react';

const UserInfoHeader = ({ t, userState }) => {
  const getUsername = () => {
    if (userState.user) {
      return userState.user.username;
    } else {
      return 'null';
    }
  };

  const getAvatarText = () => {
    const username = getUsername();
    if (username && username.length > 0) {
      return username.slice(0, 2).toUpperCase();
    }
    return 'NA';
  };

  return (
    <Card
      className='!rounded-2xl overflow-hidden'
      cover={
        <div
          className='relative h-32'
          style={{
            '--palette-primary-darkerChannel': '0 75 80',
            backgroundImage: `linear-gradient(0deg, rgba(var(--palette-primary-darkerChannel) / 80%), rgba(var(--palette-primary-darkerChannel) / 80%)), url('/cover-4.webp')`,
            backgroundSize: 'cover',
            backgroundPosition: 'center',
            backgroundRepeat: 'no-repeat',
          }}
        >
          {/* 用户信息内容 */}
          <div className='relative z-10 h-full flex flex-col justify-end p-6'>
            <div className='flex items-center'>
              <div className='flex items-stretch gap-3 sm:gap-4 flex-1 min-w-0'>
                <Avatar size='large' color={stringToColor(getUsername())}>
                  {getAvatarText()}
                </Avatar>
                <div className='flex-1 min-w-0 flex flex-col justify-between'>
                  <div
                    className='text-3xl font-bold truncate'
                    style={{ color: 'white' }}
                  >
                    {getUsername()}
                  </div>
                  <div className='flex flex-wrap items-center gap-2'>
                    {isRoot() ? (
                      <Tag
                        size='large'
                        shape='circle'
                        style={{ color: 'white' }}
                      >
                        {t('超级管理员')}
                      </Tag>
                    ) : isAdmin() ? (
                      <Tag
                        size='large'
                        shape='circle'
                        style={{ color: 'white' }}
                      >
                        {t('管理员')}
                      </Tag>
                    ) : (
                      <Tag
                        size='large'
                        shape='circle'
                        style={{ color: 'white' }}
                      >
                        {t('普通用户')}
                      </Tag>
                    )}
                    <Tag size='large' shape='circle' style={{ color: 'white' }}>
                      ID: {userState?.user?.id}
                    </Tag>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      }
    >
      {/* 当前余额和桌面版统计信息 */}
      <div className='flex items-center justify-between gap-6 px-2 py-1'>
        {/* 当前余额显示 */}
        <div className='flex flex-col items-start'>
          <div className='px-3 py-1 bg-playful-primary text-playful-foreground border-2 border-playful-foreground rounded-full text-xs font-bold shadow-sm mb-2 -rotate-2'>
            {t('当前余额')}
          </div>
          <div className='text-4xl sm:text-5xl font-black text-playful-foreground font-outfit tracking-tighter drop-shadow-[2px_2px_0_var(--playful-secondary)]'>
            {renderQuota(userState?.user?.quota)}
          </div>
        </div>

        {/* 桌面版统计信息 */}
        <div className='hidden lg:block flex-shrink-0'>
          <div className='flex items-center gap-6 bg-playful-muted border-2 border-playful-foreground px-6 py-3 rounded-xl shadow-sm'>
            <div className='flex flex-col items-center gap-1'>
              <div className='flex items-center gap-1 text-gray-500 text-xs font-bold uppercase tracking-wider'><Coins size={14}/> {t('历史消耗')}</div>
              <Typography.Text size='normal' type='primary' className='font-outfit font-bold'>
                {renderQuota(userState?.user?.used_quota)}
              </Typography.Text>
            </div>
            <div className='w-0.5 h-8 bg-playful-border'></div>
            <div className='flex flex-col items-center gap-1'>
              <div className='flex items-center gap-1 text-gray-500 text-xs font-bold uppercase tracking-wider'><BarChart2 size={14}/> {t('请求次数')}</div>
              <Typography.Text size='normal' type='primary' className='font-outfit font-bold'>
                {userState.user?.request_count || 0}
              </Typography.Text>
            </div>
            <div className='w-0.5 h-8 bg-playful-border'></div>
            <div className='flex flex-col items-center gap-1'>
              <div className='flex items-center gap-1 text-gray-500 text-xs font-bold uppercase tracking-wider'><Users size={14}/> {t('用户分组')}</div>
              <Typography.Text size='normal' type='primary' className='font-outfit font-bold'>
                {userState?.user?.group || t('默认')}
              </Typography.Text>
            </div>
          </div>
        </div>
      </div>

      {/* 移动端和中等屏幕统计信息卡片 */}
      <div className='lg:hidden mt-2'>
        <Card
          size='small'
          className='!rounded-xl'
          bodyStyle={{ padding: '12px 16px' }}
        >
          <div className='space-y-3'>
            <div className='flex items-center justify-between'>
              <div className='flex items-center gap-2'>
                <Coins size={16} />
                <Typography.Text size='small' type='tertiary'>
                  {t('历史消耗')}
                </Typography.Text>
              </div>
              <Typography.Text size='small' type='tertiary' strong>
                {renderQuota(userState?.user?.used_quota)}
              </Typography.Text>
            </div>
            <Divider margin='8px' />
            <div className='flex items-center justify-between'>
              <div className='flex items-center gap-2'>
                <BarChart2 size={16} />
                <Typography.Text size='small' type='tertiary'>
                  {t('请求次数')}
                </Typography.Text>
              </div>
              <Typography.Text size='small' type='tertiary' strong>
                {userState.user?.request_count || 0}
              </Typography.Text>
            </div>
            <Divider margin='8px' />
            <div className='flex items-center justify-between'>
              <div className='flex items-center gap-2'>
                <Users size={16} />
                <Typography.Text size='small' type='tertiary'>
                  {t('用户分组')}
                </Typography.Text>
              </div>
              <Typography.Text size='small' type='tertiary' strong>
                {userState?.user?.group || t('默认')}
              </Typography.Text>
            </div>
          </div>
        </Card>
      </div>
    </Card>
  );
};

export default UserInfoHeader;
