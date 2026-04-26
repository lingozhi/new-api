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
import { Card, Timeline } from '@douyinfe/semi-ui';
import { Bell } from 'lucide-react';
import { marked } from 'marked';
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';
import ScrollableContainer from '../common/ui/ScrollableContainer';
import { PlayfulKicker, PlayfulTag, PlayfulEmpty } from '../common/ui/playful';

const LEGEND_COLOR_MAP = {
  grey: '#8b9aa7',
  blue: '#3b82f6',
  green: '#10b981',
  orange: '#f59e0b',
  red: '#ef4444',
};

const AnnouncementsPanel = ({
  announcementData,
  announcementLegendData,
  CARD_PROPS,
  ILLUSTRATION_SIZE,
  t,
}) => {
  return (
    <Card
      {...CARD_PROPS}
      className={`${CARD_PROPS?.className || ''} lg:col-span-2`.trim()}
      title={
        <div className='flex flex-col lg:flex-row lg:items-center lg:justify-between gap-2 w-full'>
          <div className='flex flex-col gap-1'>
            <PlayfulKicker tone='pink' icon={<Bell size={12} />}>
              {t('Announcements')}
            </PlayfulKicker>
            <div className='flex items-center gap-2'>
              <div className='font-outfit text-lg font-extrabold text-playful-foreground'>
                {t('系统公告')}
              </div>
              <PlayfulTag tone='neutral' size='sm'>
                {t('显示最新20条')}
              </PlayfulTag>
            </div>
          </div>
          <div className='flex flex-wrap gap-3'>
            {announcementLegendData.map((legend, index) => (
              <div key={index} className='flex items-center gap-1.5'>
                <span
                  className='playful-dashboard-legend-dot'
                  style={{
                    backgroundColor:
                      LEGEND_COLOR_MAP[legend.color] || LEGEND_COLOR_MAP.grey,
                  }}
                />
                <span className='playful-dashboard-legend-label'>
                  {legend.label}
                </span>
              </div>
            ))}
          </div>
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      <ScrollableContainer maxHeight='24rem'>
        {announcementData.length > 0 ? (
          <div className='p-3'>
            <Timeline mode='left'>
              {announcementData.map((item, idx) => {
                const htmlExtra = item.extra ? marked.parse(item.extra) : '';
                return (
                  <Timeline.Item
                    key={idx}
                    type={item.type || 'default'}
                    time={`${item.relative ? item.relative + ' ' : ''}${item.time}`}
                    extra={
                      item.extra ? (
                        <div
                          className='text-xs text-playful-muted-fg'
                          dangerouslySetInnerHTML={{ __html: htmlExtra }}
                        />
                      ) : null
                    }
                  >
                    <div
                      className='typography-content'
                      dangerouslySetInnerHTML={{
                        __html: marked.parse(item.content || ''),
                      }}
                    />
                  </Timeline.Item>
                );
              })}
            </Timeline>
          </div>
        ) : (
          <div className='flex justify-center items-center py-8 px-4'>
            <PlayfulEmpty
              illustration={
                <IllustrationConstruction style={ILLUSTRATION_SIZE} />
              }
              title={t('暂无系统公告')}
              description={t('请联系管理员在系统设置中配置公告信息')}
            />
          </div>
        )}
      </ScrollableContainer>
    </Card>
  );
};

export default AnnouncementsPanel;
