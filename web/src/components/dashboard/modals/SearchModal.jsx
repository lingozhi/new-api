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

import React, { useRef } from 'react';
import { Form } from '@douyinfe/semi-ui';
import {
  PlayfulModal,
  PlayfulFormField,
  StickerButton,
} from '../../common/ui/playful';

const SearchModal = ({
  searchModalVisible,
  handleSearchConfirm,
  handleCloseModal,
  isMobile,
  isAdminUser,
  inputs,
  dataExportDefaultTime,
  timeOptions,
  handleInputChange,
  t,
}) => {
  const formRef = useRef();

  const { start_timestamp, end_timestamp, username } = inputs;

  return (
    <PlayfulModal
      tone='tertiary'
      title={t('搜索条件')}
      visible={searchModalVisible}
      onCancel={handleCloseModal}
      closeOnEsc={true}
      size={isMobile ? 'full-width' : 'small'}
      centered
      footer={
        <div className='flex justify-end gap-2'>
          <StickerButton
            variant='secondary'
            size='sm'
            onClick={handleCloseModal}
          >
            {t('取消')}
          </StickerButton>
          <StickerButton
            variant='primary'
            size='sm'
            onClick={handleSearchConfirm}
          >
            {t('确认')}
          </StickerButton>
        </div>
      }
    >
      <Form ref={formRef} layout='vertical' className='playful-auth-form w-full'>
        <PlayfulFormField
          as={Form.DatePicker}
          field='start_timestamp'
          label={t('起始时间')}
          initValue={start_timestamp}
          value={start_timestamp}
          type='dateTime'
          name='start_timestamp'
          className='w-full'
          onChange={(value) => handleInputChange(value, 'start_timestamp')}
        />

        <PlayfulFormField
          as={Form.DatePicker}
          field='end_timestamp'
          label={t('结束时间')}
          initValue={end_timestamp}
          value={end_timestamp}
          type='dateTime'
          name='end_timestamp'
          className='w-full'
          onChange={(value) => handleInputChange(value, 'end_timestamp')}
        />

        <PlayfulFormField
          as={Form.Select}
          field='data_export_default_time'
          label={t('时间粒度')}
          initValue={dataExportDefaultTime}
          placeholder={t('时间粒度')}
          name='data_export_default_time'
          optionList={timeOptions}
          className='w-full'
          onChange={(value) =>
            handleInputChange(value, 'data_export_default_time')
          }
        />

        {isAdminUser && (
          <PlayfulFormField
            field='username'
            label={t('用户名称')}
            value={username}
            placeholder={t('可选值')}
            name='username'
            className='w-full'
            onChange={(value) => handleInputChange(value, 'username')}
          />
        )}
      </Form>
    </PlayfulModal>
  );
};

export default SearchModal;
