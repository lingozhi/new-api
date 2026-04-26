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
import { Avatar, Skeleton } from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { StickerCard, StickerButton } from '../common/ui/playful';

const TONE_BY_GROUP_COLOR = {
  'bg-blue-50': 'violet',
  'bg-green-50': 'mint',
  'bg-yellow-50': 'tertiary',
  'bg-indigo-50': 'pink',
};

const SHADOW_BY_TONE = {
  violet: 'pop-violet',
  mint: 'pop-mint',
  tertiary: 'pop-tertiary',
  pink: 'pop-pink',
  neutral: 'pop-soft',
};

const StatsCards = ({
  groupedStatsData,
  loading,
  getTrendSpec,
  CARD_PROPS,
  CHART_CONFIG,
}) => {
  const navigate = useNavigate();
  const { t } = useTranslation();

  return (
    <div className='playful-bento-grid playful-bento-stats mb-6'>
      {groupedStatsData.map((group, idx) => {
        const tone = TONE_BY_GROUP_COLOR[group.color] || 'neutral';
        const shadow = SHADOW_BY_TONE[tone];
        return (
          <StickerCard
            key={idx}
            tone={tone}
            shadow={shadow}
            title={group.title}
            className='playful-bento-stat-card'
            bodyClassName='!pt-2'
          >
            {group.items.map((item, itemIdx) => (
              <div
                key={itemIdx}
                className='playful-bento-row'
                onClick={item.onClick}
                role='button'
                tabIndex={0}
              >
                <div className='flex items-center gap-3 min-w-0'>
                  <Avatar
                    className='playful-bento-row__avatar'
                    size='small'
                    color={item.avatarColor}
                  >
                    {item.icon}
                  </Avatar>
                  <div className='min-w-0'>
                    <div className='playful-bento-row__label truncate'>
                      {item.title}
                    </div>
                    <div className='playful-bento-row__value truncate'>
                      <Skeleton
                        loading={loading}
                        active
                        placeholder={
                          <Skeleton.Paragraph
                            active
                            rows={1}
                            style={{
                              width: '65px',
                              height: '22px',
                              marginTop: '4px',
                            }}
                          />
                        }
                      >
                        {item.value}
                      </Skeleton>
                    </div>
                  </div>
                </div>
                {item.title === t('当前余额') ? (
                  <StickerButton
                    variant='primary'
                    size='sm'
                    onClick={(e) => {
                      e.stopPropagation();
                      navigate('/console/topup');
                    }}
                  >
                    {t('充值')}
                  </StickerButton>
                ) : (
                  (loading ||
                    (item.trendData && item.trendData.length > 0)) && (
                    <div className='playful-bento-row__sparkline'>
                      <VChart
                        spec={getTrendSpec(item.trendData, item.trendColor)}
                        option={CHART_CONFIG}
                      />
                    </div>
                  )
                )}
              </div>
            ))}
          </StickerCard>
        );
      })}
    </div>
  );
};

export default StatsCards;
