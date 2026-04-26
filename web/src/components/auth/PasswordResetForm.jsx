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

import React, { useEffect, useState } from 'react';
import {
  API,
  getLogo,
  showError,
  showInfo,
  showSuccess,
  getSystemName,
} from '../../helpers';
import Turnstile from 'react-turnstile';
import { Card, Form, Typography } from '@douyinfe/semi-ui';
import { IconMail } from '@douyinfe/semi-icons';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { StickerButton, PlayfulFormField } from '../common/ui/playful';

const { Text, Title } = Typography;

const PasswordResetForm = () => {
  const { t } = useTranslation();
  const [inputs, setInputs] = useState({
    email: '',
  });
  const { email } = inputs;

  const [loading, setLoading] = useState(false);
  const [turnstileEnabled, setTurnstileEnabled] = useState(false);
  const [turnstileSiteKey, setTurnstileSiteKey] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');
  const [disableButton, setDisableButton] = useState(false);
  const [countdown, setCountdown] = useState(30);

  const logo = getLogo();
  const systemName = getSystemName();

  useEffect(() => {
    let status = localStorage.getItem('status');
    if (status) {
      status = JSON.parse(status);
      if (status.turnstile_check) {
        setTurnstileEnabled(true);
        setTurnstileSiteKey(status.turnstile_site_key);
      }
    }
  }, []);

  useEffect(() => {
    let countdownInterval = null;
    if (disableButton && countdown > 0) {
      countdownInterval = setInterval(() => {
        setCountdown(countdown - 1);
      }, 1000);
    } else if (countdown === 0) {
      setDisableButton(false);
      setCountdown(30);
    }
    return () => clearInterval(countdownInterval);
  }, [disableButton, countdown]);

  function handleChange(value) {
    setInputs((inputs) => ({ ...inputs, email: value }));
  }

  async function handleSubmit(e) {
    if (!email) {
      showError(t('请输入邮箱地址'));
      return;
    }
    if (turnstileEnabled && turnstileToken === '') {
      showInfo(t('请稍后几秒重试，Turnstile 正在检查用户环境！'));
      return;
    }
    setDisableButton(true);
    setLoading(true);
    const res = await API.get(
      `/api/reset_password?email=${email}&turnstile=${turnstileToken}`,
    );
    const { success, message } = res.data;
    if (success) {
      showSuccess(t('重置邮件发送成功，请检查邮箱！'));
      setInputs({ ...inputs, email: '' });
    } else {
      showError(message);
    }
    setLoading(false);
  }

  return (
    <div className='playful-auth-page'>
      <div className='playful-auth-decor' aria-hidden='true' />
      <div className='playful-auth-decor-square' aria-hidden='true' />
      <div className='playful-auth-decor-pill' aria-hidden='true'>
        {t('Reset Flow')}
      </div>

      <div className='playful-auth-panel'>
        <div className='playful-auth-brand'>
          <div className='playful-auth-brand-mark'>
            <img src={logo} alt='Logo' />
          </div>
          <Title heading={3} className='playful-auth-brand-name !mb-0'>
            {systemName}
          </Title>
        </div>

        <Card className='playful-auth-card sticker-card bg-white relative z-10'>
          <div className='flex justify-center pb-2 pt-3'>
            <Title heading={3} className='playful-auth-title !mb-0'>
              {t('密码重置')}
            </Title>
          </div>

          <p className='playful-auth-subtitle'>
            {t('输入你的邮箱，我们会向你发送密码重置链接。')}
          </p>

          <Form className='playful-auth-form'>
            <PlayfulFormField
              field='email'
              label={t('邮箱')}
              placeholder={t('请输入您的邮箱地址')}
              name='email'
              value={email}
              onChange={handleChange}
              prefix={<IconMail />}
            />

            <div className='space-y-2 pt-2'>
              <StickerButton
                variant='primary'
                block
                htmlType='submit'
                onClick={handleSubmit}
                loading={loading}
                disabled={disableButton}
              >
                {disableButton ? `${t('重试')} (${countdown})` : t('发送重置邮件')}
              </StickerButton>
            </div>
          </Form>

          <div className='playful-auth-note mt-5 p-4 text-center text-sm text-playful-muted-fg'>
            {t('邮件通常会在几分钟内送达，请同时检查垃圾邮箱。')}
          </div>

          <div className='playful-auth-footer'>
            <Text>
              {t('想起来了？')}{' '}
              <Link to='/login'>{t('登录')}</Link>
            </Text>
          </div>
        </Card>

        {turnstileEnabled && (
          <div className='playful-auth-turnstile'>
            <Turnstile
              sitekey={turnstileSiteKey}
              onVerify={(token) => {
                setTurnstileToken(token);
              }}
            />
          </div>
        )}
      </div>
    </div>
  );
};

export default PasswordResetForm;
