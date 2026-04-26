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
import { API, showError, showSuccess } from '../../helpers';
import { Card, Form, Typography } from '@douyinfe/semi-ui';
import React, { useState } from 'react';
import {
  StickerButton,
  PlayfulFormField,
  SquiggleDivider,
} from '../common/ui/playful';

const { Title, Text, Paragraph } = Typography;

const verificationTips = [
  '验证码每30秒更新一次',
  '如果无法获取验证码，请使用备用码',
  '每个备用码只能使用一次',
];

const renderTips = () => (
  <div className='playful-auth-note p-4'>
    <Text size='small' className='!leading-6 !text-playful-foreground'>
      <strong>提示：</strong>
    </Text>
    <div className='playful-bullet-list mt-3'>
      {verificationTips.map((tip) => (
        <div key={tip} className='playful-bullet-item'>
          <span className='playful-bullet-dot' aria-hidden='true' />
          <Text size='small' className='!leading-6 !text-playful-foreground'>
            {tip}
          </Text>
        </div>
      ))}
    </div>
  </div>
);

const TwoFAVerification = ({ onSuccess, onBack, isModal = false }) => {
  const [loading, setLoading] = useState(false);
  const [useBackupCode, setUseBackupCode] = useState(false);
  const [verificationCode, setVerificationCode] = useState('');

  const handleSubmit = async () => {
    if (!verificationCode) {
      showError('请输入验证码');
      return;
    }
    // Validate code format
    if (useBackupCode && verificationCode.length !== 8) {
      showError('备用码必须是8位');
      return;
    } else if (!useBackupCode && !/^\d{6}$/.test(verificationCode)) {
      showError('验证码必须是6位数字');
      return;
    }

    setLoading(true);
    try {
      const res = await API.post('/api/user/login/2fa', {
        code: verificationCode,
      });

      if (res.data.success) {
        showSuccess('登录成功');
        // 保存用户信息到本地存储
        localStorage.setItem('user', JSON.stringify(res.data.data));
        if (onSuccess) {
          onSuccess(res.data.data);
        }
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError('验证失败，请重试');
    } finally {
      setLoading(false);
    }
  };

  const handleKeyPress = (e) => {
    if (e.key === 'Enter') {
      handleSubmit();
    }
  };

  if (isModal) {
    return (
      <div className='space-y-4 font-jakarta text-playful-foreground'>
        <Paragraph className='!text-playful-muted-fg !leading-7'>
          请输入认证器应用显示的验证码完成登录
        </Paragraph>

        <Form onSubmit={handleSubmit} className='playful-auth-form'>
          <PlayfulFormField
            field='code'
            label={useBackupCode ? '备用码' : '验证码'}
            placeholder={useBackupCode ? '请输入8位备用码' : '请输入6位验证码'}
            value={verificationCode}
            onChange={setVerificationCode}
            onKeyPress={handleKeyPress}
            size='large'
            fieldClassName='mb-4'
            autoFocus
          />

          <StickerButton
            variant='primary'
            block
            htmlType='submit'
            loading={loading}
            className='!mb-4'
          >
            验证并登录
          </StickerButton>
        </Form>

        <SquiggleDivider color='accent' />

        <div className='flex flex-wrap items-center justify-center gap-3 text-center'>
          <StickerButton
            variant='ghost'
            size='sm'
            onClick={() => {
              setUseBackupCode(!useBackupCode);
              setVerificationCode('');
            }}
          >
            {useBackupCode ? '使用认证器验证码' : '使用备用码'}
          </StickerButton>

          {onBack && (
            <StickerButton variant='ghost' size='sm' onClick={onBack}>
              返回登录
            </StickerButton>
          )}
        </div>

        {renderTips()}
      </div>
    );
  }

  return (
    <div className='playful-auth-shell'>
      <Card className='playful-auth-card'>
        <div className='relative overflow-hidden px-6 py-8 md:px-8'>
          <div className='playful-floating-shape playful-floating-shape--circle -left-8 top-6 h-16 w-16 bg-playful-tertiary' />
          <div className='playful-floating-shape playful-floating-shape--blob -right-6 top-4 h-20 w-20 bg-playful-secondary' />

          <div className='relative z-10 text-center'>
            <span className='playful-kicker mb-4'>{useBackupCode ? '备用码验证' : '安全验证'}</span>
            <Title heading={3} className='!mb-2 !font-outfit !text-3xl !font-extrabold !text-playful-foreground'>
              两步验证
            </Title>
            <Paragraph className='!mb-6 !leading-7 !text-playful-muted-fg'>
              请输入认证器应用显示的验证码完成登录
            </Paragraph>
          </div>

          <Form onSubmit={handleSubmit} className='playful-auth-form'>
            <PlayfulFormField
              field='code'
              label={useBackupCode ? '备用码' : '验证码'}
              placeholder={useBackupCode ? '请输入8位备用码' : '请输入6位验证码'}
              value={verificationCode}
              onChange={setVerificationCode}
              onKeyPress={handleKeyPress}
              size='large'
              fieldClassName='mb-4'
              autoFocus
            />

            <StickerButton
              variant='primary'
              block
              htmlType='submit'
              loading={loading}
              className='!mb-4'
            >
              验证并登录
            </StickerButton>
          </Form>

          <SquiggleDivider color='accent' />

          <div className='mb-5 flex flex-wrap items-center justify-center gap-3 text-center'>
            <StickerButton
              variant='ghost'
              size='sm'
              onClick={() => {
                setUseBackupCode(!useBackupCode);
                setVerificationCode('');
              }}
            >
              {useBackupCode ? '使用认证器验证码' : '使用备用码'}
            </StickerButton>

            {onBack && (
              <StickerButton variant='ghost' size='sm' onClick={onBack}>
                返回登录
              </StickerButton>
            )}
          </div>

          <div className='playful-auth-note p-4'>
            <Text size='small' className='!leading-6 !text-playful-foreground'>
              <strong>提示：</strong>
              <br />
              • 验证码每30秒更新一次
              <br />
              • 如果无法获取验证码，请使用备用码
              <br />• 每个备用码只能使用一次
            </Text>
          </div>
        </div>
      </Card>
    </div>
  );
};

export default TwoFAVerification;
