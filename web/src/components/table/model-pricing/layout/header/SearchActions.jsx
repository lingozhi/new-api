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

import React, { memo, useCallback } from 'react';
import { Input, Button, Tag } from '@douyinfe/semi-ui';
import { IconSearch, IconCopy, IconFilter } from '@douyinfe/semi-icons';
import { Copy, SlidersHorizontal, TableProperties, Scaling } from 'lucide-react';

const SearchActions = memo(
  ({
    selectedRowKeys = [],
    copyText,
    handleChange,
    handleCompositionStart,
    handleCompositionEnd,
    isMobile = false,
    searchValue = '',
    setShowFilterModal,
    showWithRecharge,
    setShowWithRecharge,
    currency,
    setCurrency,
    siteDisplayType,
    showRatio,
    setShowRatio,
    viewMode,
    setViewMode,
    tokenUnit,
    setTokenUnit,
    t,
  }) => {
    const supportsCurrencyDisplay = siteDisplayType !== 'TOKENS';

    const handleCopyClick = useCallback(() => {
      if (copyText && selectedRowKeys.length > 0) {
        copyText(selectedRowKeys);
      }
    }, [copyText, selectedRowKeys]);

    const handleFilterClick = useCallback(() => {
      setShowFilterModal?.(true);
    }, [setShowFilterModal]);

    const handleViewModeToggle = useCallback(() => {
      setViewMode?.(viewMode === 'table' ? 'card' : 'table');
    }, [viewMode, setViewMode]);

    const handleTokenUnitToggle = useCallback(() => {
      setTokenUnit?.(tokenUnit === 'K' ? 'M' : 'K');
    }, [tokenUnit, setTokenUnit]);

    return (
      <div className='playful-pricing-actions-stack w-full'>
        <div className='playful-pricing-primary-toolbar'>
          <div className='playful-pricing-searchbox playful-filter-input'>
            <Input
              prefix={<IconSearch />}
              placeholder={t('模糊搜索模型名称')}
              value={searchValue}
              onCompositionStart={handleCompositionStart}
              onCompositionEnd={handleCompositionEnd}
              onChange={handleChange}
              showClear
            />
          </div>

          <div className='playful-pricing-primary-actions'>
            <Button
              theme='borderless'
              type='primary'
              icon={<IconCopy />}
              onClick={handleCopyClick}
              disabled={selectedRowKeys.length === 0}
              className='candy-btn playful-pricing-action-btn disabled:!border-playful-foreground disabled:!bg-[#D1D5DB] disabled:!text-gray-500 disabled:!shadow-[2px_2px_0px_0px_#94A3B8]'
            >
              <Copy size={16} strokeWidth={2.4} />
              <span>{t('复制')}</span>
            </Button>

            {isMobile && (
              <Button
                theme='outline'
                type='tertiary'
                icon={<IconFilter />}
                onClick={handleFilterClick}
                className='candy-btn-secondary playful-pricing-action-btn'
              >
                <SlidersHorizontal size={16} strokeWidth={2.4} />
                <span>{t('筛选')}</span>
              </Button>
            )}
          </div>
        </div>

        {!isMobile && (
          <div className='playful-pricing-secondary-toolbar'>
            <button
              type='button'
              onClick={handleViewModeToggle}
              className={`playful-toolbar-toggle-pill playful-toolbar-toggle-pill--wide ${
                viewMode === 'table' ? 'is-active' : ''
              }`}
            >
              <span className='playful-toolbar-meta'>
                <span className='playful-toolbar-label'>{t('布局')}</span>
                <span className='playful-toolbar-value'>
                  {viewMode === 'table' ? t('表格视图') : t('卡片视图')}
                </span>
              </span>
              <span className='playful-toolbar-icon-badge'>
                <TableProperties size={18} strokeWidth={2.6} />
              </span>
            </button>

            <button
              type='button'
              onClick={handleTokenUnitToggle}
              className={`playful-toolbar-toggle-pill playful-toolbar-toggle-pill--select ${
                tokenUnit === 'K' ? 'is-active' : ''
              }`}
            >
              <span className='playful-toolbar-meta'>
                <span className='playful-toolbar-label'>{t('单位')}</span>
                <span className='playful-toolbar-value'>
                  {tokenUnit === 'K' ? t('按千显示') : t('按百万显示')}
                </span>
              </span>
              <span className='playful-toolbar-icon-group'>
                <span className='playful-toolbar-icon-badge'>
                  <Scaling size={18} strokeWidth={2.6} />
                </span>
                <Tag className='playful-toolbar-token-tag' size='small' shape='circle'>
                  {tokenUnit}
                </Tag>
              </span>
            </button>
          </div>
        )}
      </div>
    );
  },
);

SearchActions.displayName = 'SearchActions';

export default SearchActions;
