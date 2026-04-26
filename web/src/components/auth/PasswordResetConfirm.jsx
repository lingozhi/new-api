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
  copy,
  showError,
  showNotice,
  getLogo,
  getSystemName,
} from '../../helpers';
import { useSearchParams, Link } from 'react-router-dom';
import { Button, Card, Form, Typography, Banner } from '@douyinfe/semi-ui';
import { IconMail, IconLock, IconCopy } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { StickerButton, PlayfulFormField } from '../common/ui/playful';

const { Text, Title } = Typography;

const PasswordResetConfirm = () => {
  const { t } = useTranslation();
  const [inputs, setInputs] = useState({
    email: '',
    token: '',
  });
  const { email, token } = inputs;
  const isValidResetLink = email && token;

  const [loading, setLoading] = useState(false);
  const [disableButton, setDisableButton] = useState(false);
  const [countdown, setCountdown] = useState(30);
  const [newPassword, setNewPassword] = useState('');
  const [searchParams, setSearchParams] = useSearchParams();
  const [formApi, setFormApi] = useState(null);

  const logo = getLogo();
  const systemName = getSystemName();

  useEffect(() => {
    let token = searchParams.get('token');
    let email = searchParams.get('email');
    setInputs({
      token: token || '',
      email: email || '',
    });
    if (formApi) {
      formApi.setValues({
        email: email || '',
        newPassword: newPassword || '',
      });
    }
  }, [searchParams, newPassword, formApi]);

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

  async function handleSubmit(e) {
    if (!email || !token) {
      showError(t('无效的重置链接，请重新发起密码重置请求'));
      return;
    }
    setDisableButton(true);
    setLoading(true);
    const res = await API.post(`/api/user/reset`, {
      email,
      token,
    });
    const { success, message } = res.data;
    if (success) {
      let password = res.data.data;
      setNewPassword(password);
      await copy(password);
      showNotice(`${t('密码已重置并已复制到剪贴板：')} ${password}`);
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
        {t('Safe Reset')}
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

        <Card className='playful-auth-card sticker-card bg-white'>
          <div className='flex justify-center pb-2 pt-3'>
            <Title heading={3} className='playful-auth-title !mb-0'>
              {t('密码重置确认')}
            </Title>
          </div>

          <p className='playful-auth-subtitle'>
            {t('确认后将生成新密码，并自动复制到你的剪贴板。')}
          </p>

          {!isValidResetLink && (
            <Banner
              type='danger'
              description={t('无效的重置链接，请重新发起密码重置请求')}
              className='mb-4 !rounded-[20px]'
              closeIcon={null}
            />
          )}

          <Form
            getFormApi={(api) => setFormApi(api)}
            initValues={{
              email: email || '',
              newPassword: newPassword || '',
            }}
            className='playful-auth-form'
          >
            <PlayfulFormField
              field='email'
              label={t('邮箱')}
              name='email'
              disabled={true}
              prefix={<IconMail />}
              placeholder={email ? '' : t('等待获取邮箱信息...')}
            />

            {newPassword && (
              <PlayfulFormField
                field='newPassword'
                label={t('新密码')}
                name='newPassword'
                disabled={true}
                prefix={<IconLock />}
                suffix={
                  <Button
                    icon={<IconCopy />}
                    type='tertiary'
                    theme='borderless'
                    className='playful-inline-action'
                    onClick={async () => {
                      await copy(newPassword);
                      showNotice(`${t('密码已复制到剪贴板：')} ${newPassword}`);
                    }}
                  >
                    {t('复制')}
                  </Button>
                }
              />
            )}

            <div className='space-y-2 pt-2'>
              <StickerButton
                variant='primary'
                block
                htmlType='submit'
                onClick={handleSubmit}
                loading={loading}
                disabled={disableButton || newPassword || !isValidResetLink}
              >
                {newPassword ? t('密码重置完成') : t('确认重置密码')}
              </StickerButton>
            </div>
          </Form>

          {newPassword && (
            <div className='playful-auth-note mt-5 p-4 text-center text-sm text-playful-muted-fg'>
              {t('请妥善保存新密码，并在登录后尽快修改为你自己的安全密码。')}
            </div>
          )}

          <div className='playful-auth-footer'>
            <Text>
              <Link to='/login'>{t('返回登录')}</Link>
            </Text>
          </div>
        </Card>
      </div>
    </div>
  );
};

export default PasswordResetConfirm;
