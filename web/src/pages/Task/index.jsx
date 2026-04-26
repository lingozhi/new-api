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
import { Clock3, ListTodo, Workflow } from 'lucide-react';
import TaskLogsTable from '../../components/table/task-logs';

const Task = () => (
  <div className='playful-console-shell playful-console-shell--dense'>
    <div className='playful-console-frame mx-auto max-w-[1800px]'>
      <section className='playful-console-hero mb-4'>
        <div className='playful-console-hero-copy'>
          <span className='playful-kicker mb-3'>Task Stream</span>
          <h1 className='playful-console-title'>任务日志</h1>
          <p className='playful-console-subtitle'>
            聚合异步任务、媒体生成与执行状态，把任务追踪页做成真正可读、可筛、可回看的操作面板。
          </p>
        </div>
        <div className='playful-console-badges'>
          <div className='playful-console-badge bg-playful-tertiary'>
            <ListTodo size={16} strokeWidth={2.4} />
            <span>任务追踪</span>
          </div>
          <div className='playful-console-badge bg-playful-secondary text-white'>
            <Workflow size={16} strokeWidth={2.4} />
            <span>状态链路</span>
          </div>
          <div className='playful-console-badge bg-playful-quaternary'>
            <Clock3 size={16} strokeWidth={2.4} />
            <span>执行时间线</span>
          </div>
        </div>
      </section>
      <TaskLogsTable />
    </div>
  </div>
);

export default Task;
