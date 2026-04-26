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
import { Activity, ReceiptText, Search } from 'lucide-react';
import UsageLogsTable from '../../components/table/usage-logs';

const Token = () => (
  <div className='playful-console-shell playful-console-shell--dense'>
    <div className='playful-console-frame mx-auto max-w-[1800px]'>
      <section className='playful-console-hero mb-4'>
        <div className='playful-console-hero-copy'>
          <span className='playful-kicker mb-3'>Usage Ledger</span>
          <h1 className='playful-console-title'>使用日志</h1>
          <p className='playful-console-subtitle'>
            查看请求明细、参数覆盖与计费足迹，把高频排障和复盘操作放到更清晰的工作台里。
          </p>
        </div>
        <div className='playful-console-badges'>
          <div className='playful-console-badge bg-playful-tertiary'>
            <ReceiptText size={16} strokeWidth={2.4} />
            <span>请求明细</span>
          </div>
          <div className='playful-console-badge bg-playful-secondary text-white'>
            <Activity size={16} strokeWidth={2.4} />
            <span>快速复盘</span>
          </div>
          <div className='playful-console-badge bg-playful-quaternary'>
            <Search size={16} strokeWidth={2.4} />
            <span>精细筛选</span>
          </div>
        </div>
      </section>
      <UsageLogsTable />
    </div>
  </div>
);

export default Token;
