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

import React, { useMemo } from 'react';
import { Empty, Typography } from '@douyinfe/semi-ui';
import CardTable from '../../common/ui/CardTable';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { SearchX } from 'lucide-react';
import { getTokensColumns } from './TokensColumnDefs';

const { Text } = Typography;

const TokensTable = (tokensData) => {
  const {
    tokens,
    loading,
    activePage,
    pageSize,
    tokenCount,
    compactMode,
    handlePageChange,
    handlePageSizeChange,
    rowSelection,
    handleRow,
    showKeys,
    resolvedTokenKeys,
    loadingTokenKeys,
    toggleTokenVisibility,
    copyTokenKey,
    copyTokenConnectionString,
    manageToken,
    onOpenLink,
    setEditingToken,
    setShowEdit,
    refresh,
    groupRatios,
    t,
  } = tokensData;

  // Get all columns
  const columns = useMemo(() => {
    return getTokensColumns({
      t,
      showKeys,
      resolvedTokenKeys,
      loadingTokenKeys,
      toggleTokenVisibility,
      copyTokenKey,
      copyTokenConnectionString,
      manageToken,
      onOpenLink,
      setEditingToken,
      setShowEdit,
      refresh,
      groupRatios,
    });
  }, [
    t,
    showKeys,
    resolvedTokenKeys,
    loadingTokenKeys,
    toggleTokenVisibility,
    copyTokenKey,
    copyTokenConnectionString,
    manageToken,
    onOpenLink,
    setEditingToken,
    setShowEdit,
    refresh,
    groupRatios,
  ]);

  // Handle compact mode by removing fixed positioning
  const tableColumns = useMemo(() => {
    return compactMode
      ? columns.map((col) => {
          if (col.dataIndex === 'operate') {
            const { fixed, ...rest } = col;
            return rest;
          }
          return col;
        })
      : columns;
  }, [compactMode, columns]);

  return (
    <CardTable
      columns={tableColumns}
      dataSource={tokens}
      scroll={compactMode ? undefined : { x: 'max-content' }}
      pagination={{
        currentPage: activePage,
        pageSize: pageSize,
        total: tokenCount,
        showSizeChanger: true,
        pageSizeOptions: [10, 20, 50, 100],
        onPageSizeChange: handlePageSizeChange,
        onPageChange: handlePageChange,
      }}
      hidePagination={true}
      loading={loading}
      rowSelection={rowSelection}
      onRow={handleRow}
      empty={
        <div className='playful-table-empty-state'>
          <div className='playful-table-empty-badge'>
            <SearchX size={20} strokeWidth={2.5} />
            <span>{t('暂无匹配结果')}</span>
          </div>
          <Empty
            image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
            darkModeImage={
              <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
            }
            description={
              <div className='space-y-2'>
                <Text className='!block !font-outfit !text-lg !font-extrabold !text-playful-foreground'>
                  {t('搜索无结果')}
                </Text>
                <Text className='!block !text-sm !leading-6 !text-playful-muted-fg'>
                  {t('试试更换关键字、清空筛选条件，或者先创建一个新的令牌。')}
                </Text>
              </div>
            }
            style={{ padding: 30 }}
          />
        </div>
      }
      className='playful-data-table rounded-xl overflow-hidden'
      size='middle'
    />
  );
};

export default TokensTable;
