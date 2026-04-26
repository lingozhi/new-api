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
import { Card, Table, Empty, Typography } from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';

const { Text } = Typography;
import { getPricingTableColumns } from './PricingTableColumns';

const PricingTable = ({
  filteredModels,
  loading,
  rowSelection,
  pageSize,
  setPageSize,
  selectedGroup,
  groupRatio,
  copyText,
  setModalImageUrl,
  setIsModalOpenurl,
  currency,
  siteDisplayType,
  tokenUnit,
  displayPrice,
  searchValue,
  showRatio,
  compactMode = false,
  openModelDetail,
  t,
}) => {
  const columns = useMemo(() => {
    return getPricingTableColumns({
      t,
      selectedGroup,
      groupRatio,
      copyText,
      setModalImageUrl,
      setIsModalOpenurl,
      currency,
      siteDisplayType,
      tokenUnit,
      displayPrice,
      showRatio,
    });
  }, [
    t,
    selectedGroup,
    groupRatio,
    copyText,
    setModalImageUrl,
    setIsModalOpenurl,
    currency,
    siteDisplayType,
    tokenUnit,
    displayPrice,
    showRatio,
  ]);

  // 更新列定义中的 searchValue
  const processedColumns = useMemo(() => {
    const cols = columns.map((column) => {
      if (column.dataIndex === 'model_name') {
        return {
          ...column,
          filteredValue: searchValue ? [searchValue] : [],
        };
      }
      return column;
    });

    // Remove fixed property when in compact mode (mobile view)
    if (compactMode) {
      return cols.map(({ fixed, ...rest }) => rest);
    }
    return cols;
  }, [columns, searchValue, compactMode]);

  const ModelTable = useMemo(
    () => (
      <Card className='!rounded-xl overflow-hidden' bordered={false}>
        <Table
          columns={processedColumns}
          dataSource={filteredModels}
          loading={loading}
          rowSelection={rowSelection}
          scroll={compactMode ? undefined : { x: 'max-content' }}
          onRow={(record) => ({
            onClick: () => openModelDetail && openModelDetail(record),
            style: { cursor: 'pointer' },
          })}
          empty={
            <div className='playful-table-empty-state'>
              <div className='playful-table-empty-badge'>
                <span>{t('暂无匹配结果')}</span>
              </div>
              <Empty
                image={
                  <IllustrationNoResult style={{ width: 150, height: 150 }} />
                }
                darkModeImage={
                  <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
                }
                description={
                  <div className='space-y-2'>
                    <Text className='!block !font-outfit !text-lg !font-extrabold !text-playful-foreground'>
                      {t('搜索无结果')}
                    </Text>
                    <Text className='!block !text-sm !leading-6 !text-playful-muted-fg'>
                      {t('试试切换左侧筛选条件、恢复默认显示设置，或清空搜索词后重新查看。')}
                    </Text>
                  </div>
                }
                style={{ padding: 30 }}
              />
            </div>
          }
          pagination={{
            defaultPageSize: 20,
            pageSize: pageSize,
            showSizeChanger: true,
            pageSizeOptions: [10, 20, 50, 100],
            onPageSizeChange: (size) => setPageSize(size),
          }}
        />
      </Card>
    ),
    [
      filteredModels,
      loading,
      processedColumns,
      rowSelection,
      pageSize,
      setPageSize,
      openModelDetail,
      t,
      compactMode,
    ],
  );

  return ModelTable;
};

export default PricingTable;
