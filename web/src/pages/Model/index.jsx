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
import ModelsTable from '../../components/table/models';

const ModelPage = () => {
  return (
    <div className='playful-model-shell'>
      <div className='playful-model-page-frame mx-auto max-w-[1600px]'>
        <div className='playful-model-page-hero mb-4'>
          <div>
            <span className='playful-kicker mb-3'>Model Plaza</span>
            <h1 className='font-outfit text-3xl font-extrabold tracking-[-0.03em] text-playful-foreground md:text-4xl'>
              模型广场
            </h1>
            <p className='mt-3 max-w-3xl text-sm leading-7 text-playful-muted-fg md:text-base'>
              统一浏览模型展示配置、供应商分布与可见性效果。左侧筛选区与主内容区将保持一致的 Playful Geometric 视觉语言。
            </p>
          </div>
        </div>
        <ModelsTable />
      </div>
    </div>
  );
};

export default ModelPage;
