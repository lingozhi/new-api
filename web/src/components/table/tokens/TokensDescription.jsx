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
import { Typography } from '@douyinfe/semi-ui';
import { Key, Sparkles } from 'lucide-react';
import CompactModeToggle from '../../common/ui/CompactModeToggle';

const { Text } = Typography;

const TokensDescription = ({ compactMode, setCompactMode, t }) => {
  return (
    <div className='playful-token-page-header flex w-full flex-col gap-4 md:flex-row md:items-start md:justify-between'>
      <div className='flex items-start gap-3'>
        <div className='flex h-11 w-11 shrink-0 items-center justify-center rounded-full border-2 border-playful-foreground bg-playful-tertiary shadow-pop-sm'>
          <Key size={18} color='var(--playful-foreground)' />
        </div>
        <div className='space-y-2'>
          <span className='playful-kicker'>
            <Sparkles size={14} strokeWidth={2.5} />
            {t('访问控制')}
          </span>
          <div>
            <Text className='!block !font-outfit !text-xl !font-extrabold !text-playful-foreground'>
              {t('令牌管理')}
            </Text>
            <Text className='!mt-1 !block !text-sm !leading-6 !text-playful-muted-fg'>
              {t('在这里创建、筛选并维护 API 令牌，快速完成权限分发与密钥轮换。')}
            </Text>
          </div>
        </div>
      </div>

      <CompactModeToggle
        compactMode={compactMode}
        setCompactMode={setCompactMode}
        t={t}
        activeLabel={t('显示完整布局')}
        inactiveLabel={t('切换到紧凑布局')}
      />
    </div>
  );
};

export default TokensDescription;
