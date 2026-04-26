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
import { Layers3, RadioTower, Sparkles } from 'lucide-react';
import ChannelsTable from '../../components/table/channels';

const File = () => {
  return (
    <div className='playful-console-shell playful-console-shell--dense'>
      <div className='playful-console-frame mx-auto max-w-[1800px]'>
        <section className='playful-console-hero mb-4'>
          <div className='playful-console-hero-copy'>
            <span className='playful-kicker mb-3'>Channel Studio</span>
            <h1 className='playful-console-title'>渠道管理</h1>
            <p className='playful-console-subtitle'>
              管理上游渠道、路由能力与模型映射，让控制台里的高频调度操作更清晰。
            </p>
          </div>
          <div className='playful-console-badges'>
            <div className='playful-console-badge bg-playful-tertiary'>
              <Layers3 size={16} strokeWidth={2.4} />
              <span>统一路由</span>
            </div>
            <div className='playful-console-badge bg-playful-secondary text-white'>
              <RadioTower size={16} strokeWidth={2.4} />
              <span>实时调度</span>
            </div>
            <div className='playful-console-badge bg-playful-quaternary'>
              <Sparkles size={16} strokeWidth={2.4} />
              <span>批量运维</span>
            </div>
          </div>
        </section>
        <ChannelsTable />
      </div>
    </div>
  );
};

export default File;
