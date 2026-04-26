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
import { Button } from '@douyinfe/semi-ui';
import PricingGroups from '../filter/PricingGroups';
import PricingQuotaTypes from '../filter/PricingQuotaTypes';
import PricingEndpointTypes from '../filter/PricingEndpointTypes';
import PricingVendors from '../filter/PricingVendors';
import PricingTags from '../filter/PricingTags';
import PricingDisplaySettings from '../filter/PricingDisplaySettings';

import { resetPricingFilters } from '../../../../helpers/utils';
import { usePricingFilterCounts } from '../../../../hooks/model-pricing/usePricingFilterCounts';

const PricingSidebar = ({
  showWithRecharge,
  setShowWithRecharge,
  currency,
  setCurrency,
  handleChange,
  setActiveKey,
  showRatio,
  setShowRatio,
  viewMode,
  setViewMode,
  filterGroup,
  setFilterGroup,
  handleGroupClick,
  filterQuotaType,
  setFilterQuotaType,
  filterEndpointType,
  setFilterEndpointType,
  filterVendor,
  setFilterVendor,
  filterTag,
  setFilterTag,
  currentPage,
  setCurrentPage,
  tokenUnit,
  setTokenUnit,
  loading,
  t,
  ...categoryProps
}) => {
  const {
    quotaTypeModels,
    endpointTypeModels,
    vendorModels,
    tagModels,
    groupCountModels,
  } = usePricingFilterCounts({
    models: categoryProps.models,
    filterGroup,
    filterQuotaType,
    filterEndpointType,
    filterVendor,
    filterTag,
    searchValue: categoryProps.searchValue,
  });

  const handleResetFilters = () =>
    resetPricingFilters({
      handleChange,
      setShowWithRecharge,
      setCurrency,
      setShowRatio,
      setViewMode,
      setFilterGroup,
      setFilterQuotaType,
      setFilterEndpointType,
      setFilterVendor,
      setFilterTag,
      setCurrentPage,
      setTokenUnit,
    });

  return (
    <div className='playful-pricing-sidebar-panel p-3 md:p-4'>
      <div className='playful-pricing-sidebar-hero mb-5'>
        <div>
          <span className='playful-kicker mb-3'>{t('Filter Lab')}</span>
          <div className='text-2xl font-outfit font-extrabold tracking-[-0.03em] text-playful-foreground'>
            {t('筛选')}
          </div>
          <p className='mt-2 text-sm leading-6 text-playful-muted-fg'>
            {t('按供应商、分组、计费类型与标签快速收敛展示范围。')}
          </p>
        </div>
      </div>

      <div className='playful-pricing-sidebar-summary mb-5'>
        <div className='playful-pricing-sidebar-summary-label'>{t('当前结果')}</div>
        <div className='playful-pricing-sidebar-summary-value'>
          {categoryProps.filteredModels?.length ?? categoryProps.models?.length ?? 0}
        </div>
        <div className='playful-pricing-sidebar-summary-caption'>
          {t('根据当前筛选条件可见的模型数量')}
        </div>
      </div>

      <div className='flex items-center justify-between mb-5 gap-3 flex-wrap'>
        <div className='text-sm font-bold uppercase tracking-[0.16em] text-playful-muted-fg'>
          {t('当前条件')}
        </div>
        <Button
          theme='borderless'
          type='primary'
          onClick={handleResetFilters}
          className='candy-btn-secondary playful-pricing-reset-btn px-4 py-2 text-sm'
        >
          {t('重置')}
        </Button>
      </div>

      <PricingDisplaySettings
        showWithRecharge={showWithRecharge}
        setShowWithRecharge={setShowWithRecharge}
        currency={currency}
        setCurrency={setCurrency}
        siteDisplayType={categoryProps.siteDisplayType}
        showRatio={showRatio}
        setShowRatio={setShowRatio}
        viewMode={viewMode}
        setViewMode={setViewMode}
        tokenUnit={tokenUnit}
        setTokenUnit={setTokenUnit}
        loading={loading}
        t={t}
      />

      <PricingVendors
        filterVendor={filterVendor}
        setFilterVendor={setFilterVendor}
        models={vendorModels}
        allModels={categoryProps.models}
        loading={loading}
        t={t}
      />

      <PricingGroups
        filterGroup={filterGroup}
        setFilterGroup={handleGroupClick}
        usableGroup={categoryProps.usableGroup}
        groupRatio={categoryProps.groupRatio}
        models={groupCountModels}
        loading={loading}
        t={t}
      />

      <PricingQuotaTypes
        filterQuotaType={filterQuotaType}
        setFilterQuotaType={setFilterQuotaType}
        models={quotaTypeModels}
        loading={loading}
        t={t}
      />

      <PricingTags
        filterTag={filterTag}
        setFilterTag={setFilterTag}
        models={tagModels}
        allModels={categoryProps.models}
        loading={loading}
        t={t}
      />

      <PricingEndpointTypes
        filterEndpointType={filterEndpointType}
        setFilterEndpointType={setFilterEndpointType}
        models={endpointTypeModels}
        allModels={categoryProps.models}
        loading={loading}
        t={t}
      />
    </div>
  );
};

export default PricingSidebar;
