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
import { Card, Spin, Tabs, TabPane } from '@douyinfe/semi-ui';
import { Gauge, RefreshCw } from 'lucide-react';
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';
import ScrollableContainer from '../common/ui/ScrollableContainer';
import {
  PlayfulKicker,
  PlayfulTag,
  PlayfulEmpty,
  StickerButton,
} from '../common/ui/playful';

const UptimePanel = ({
  uptimeData,
  uptimeLoading,
  activeUptimeTab,
  setActiveUptimeTab,
  loadUptimeData,
  uptimeLegendData,
  renderMonitorList,
  CARD_PROPS,
  ILLUSTRATION_SIZE,
  t,
}) => {
  return (
    <Card
      {...CARD_PROPS}
      className={`${CARD_PROPS?.className || ''} lg:col-span-1`.trim()}
      title={
        <div className='flex items-center justify-between w-full gap-2'>
          <div className='flex flex-col gap-1'>
            <PlayfulKicker tone='mint' icon={<Gauge size={12} />}>
              {t('Uptime')}
            </PlayfulKicker>
            <div className='font-outfit text-lg font-extrabold text-playful-foreground'>
              {t('服务可用性')}
            </div>
          </div>
          <StickerButton
            variant='secondary'
            size='sm'
            icon={<RefreshCw size={14} />}
            onClick={loadUptimeData}
            loading={uptimeLoading}
          />
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      <div className='relative'>
        <Spin spinning={uptimeLoading}>
          {uptimeData.length > 0 ? (
            uptimeData.length === 1 ? (
              <ScrollableContainer maxHeight='24rem'>
                {renderMonitorList(uptimeData[0].monitors)}
              </ScrollableContainer>
            ) : (
              <Tabs
                type='button'
                collapsible
                activeKey={activeUptimeTab}
                onChange={setActiveUptimeTab}
                size='small'
                className='playful-charts-panel'
              >
                {uptimeData.map((group, groupIdx) => (
                  <TabPane
                    tab={
                      <span className='flex items-center gap-2'>
                        <Gauge size={14} />
                        {group.categoryName}
                        <PlayfulTag
                          tone={
                            activeUptimeTab === group.categoryName
                              ? 'pink'
                              : 'muted'
                          }
                          size='sm'
                        >
                          {group.monitors ? group.monitors.length : 0}
                        </PlayfulTag>
                      </span>
                    }
                    itemKey={group.categoryName}
                    key={groupIdx}
                  >
                    <ScrollableContainer maxHeight='21.5rem'>
                      {renderMonitorList(group.monitors)}
                    </ScrollableContainer>
                  </TabPane>
                ))}
              </Tabs>
            )
          ) : (
            <div className='flex justify-center items-center py-8 px-4'>
              <PlayfulEmpty
                illustration={
                  <IllustrationConstruction style={ILLUSTRATION_SIZE} />
                }
                title={t('暂无监控数据')}
                description={t('请联系管理员在系统设置中配置Uptime')}
              />
            </div>
          )}
        </Spin>
      </div>

      {uptimeData.length > 0 && (
        <div className='playful-dashboard-legend-bar'>
          <div className='flex flex-wrap gap-3 justify-center'>
            {uptimeLegendData.map((legend, index) => (
              <div key={index} className='flex items-center gap-1.5'>
                <span
                  className='playful-dashboard-legend-dot'
                  style={{ backgroundColor: legend.color }}
                />
                <span className='playful-dashboard-legend-label'>
                  {legend.label}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </Card>
  );
};

export default UptimePanel;
