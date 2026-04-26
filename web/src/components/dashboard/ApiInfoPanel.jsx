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
import { Card, Avatar } from '@douyinfe/semi-ui';
import { Server, Gauge, ExternalLink, Copy } from 'lucide-react';
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';
import ScrollableContainer from '../common/ui/ScrollableContainer';
import { PlayfulKicker, PlayfulTag, PlayfulEmpty } from '../common/ui/playful';

const ApiInfoPanel = ({
  apiInfoData,
  handleCopyUrl,
  handleSpeedTest,
  CARD_PROPS,
  FLEX_CENTER_GAP2,
  ILLUSTRATION_SIZE,
  t,
}) => {
  return (
    <Card
      {...CARD_PROPS}
      className={`${CARD_PROPS?.className || ''} lg:col-span-1`.trim()}
      title={
        <div className='flex flex-col gap-1'>
          <PlayfulKicker tone='mint' icon={<Server size={12} />}>
            {t('API')}
          </PlayfulKicker>
          <div className='font-outfit text-lg font-extrabold text-playful-foreground'>
            {t('API信息')}
          </div>
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      <ScrollableContainer maxHeight='24rem'>
        {apiInfoData.length > 0 ? (
          apiInfoData.map((api) => (
            <div key={api.id} className='playful-api-row'>
              <div className='flex-shrink-0'>
                <Avatar
                  className='playful-api-row__avatar'
                  size='extra-small'
                  color={api.color}
                >
                  {api.route.substring(0, 2)}
                </Avatar>
              </div>
              <div className='flex-1 min-w-0'>
                <div className='flex flex-wrap items-center justify-between mb-1 w-full gap-2'>
                  <span className='playful-api-row__route text-sm'>
                    {api.route}
                  </span>
                  <div className='flex items-center gap-1 mt-1 lg:mt-0'>
                    <PlayfulTag
                      tone='tertiary'
                      size='sm'
                      onClick={() => handleSpeedTest(api.url)}
                      className='cursor-pointer'
                    >
                      <Gauge size={12} />
                      <span className='ml-1'>{t('测速')}</span>
                    </PlayfulTag>
                    <PlayfulTag
                      tone='violet'
                      size='sm'
                      onClick={() =>
                        window.open(api.url, '_blank', 'noopener,noreferrer')
                      }
                      className='cursor-pointer'
                    >
                      <ExternalLink size={12} />
                      <span className='ml-1'>{t('跳转')}</span>
                    </PlayfulTag>
                  </div>
                </div>
                <div className='flex items-center gap-1 mb-1'>
                  <span
                    className='playful-api-row__url text-sm'
                    onClick={() => handleCopyUrl(api.url)}
                  >
                    {api.url}
                  </span>
                  <Copy
                    size={14}
                    className='flex-shrink-0 text-playful-muted-fg hover:text-playful-accent cursor-pointer transition-colors'
                    onClick={() => handleCopyUrl(api.url)}
                  />
                </div>
                <div className='playful-api-row__description'>
                  {api.description}
                </div>
              </div>
            </div>
          ))
        ) : (
          <div className='flex justify-center items-center min-h-[20rem] w-full p-4'>
            <PlayfulEmpty
              illustration={
                <IllustrationConstruction style={ILLUSTRATION_SIZE} />
              }
              title={t('暂无API信息')}
              description={t('请联系管理员在系统设置中配置API信息')}
            />
          </div>
        )}
      </ScrollableContainer>
    </Card>
  );
};

export default ApiInfoPanel;
