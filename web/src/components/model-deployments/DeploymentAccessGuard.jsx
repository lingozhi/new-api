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
import { Card, Button, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Settings, Server, AlertCircle, WifiOff } from 'lucide-react';

const { Title, Text } = Typography;

const bulletItems = [
  '启用 io.net 部署开关',
  '配置有效的 io.net API Key',
];

const panelShellStyle = {
  minHeight: 'calc(100vh - 60px)',
};

const centeredPanelShellClassName =
  'playful-console-shell playful-console-shell--dense flex items-center justify-center';

const DeploymentAccessGuard = ({
  children,
  loading,
  isEnabled,
  connectionLoading,
  connectionOk,
  connectionError,
  onRetry,
}) => {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const handleGoToSettings = () => {
    navigate('/console/setting?tab=model-deployment');
  };

  if (loading) {
    return (
      <div className={centeredPanelShellClassName} style={panelShellStyle}>
        <div className='playful-console-frame mx-auto w-full max-w-[1800px]'>
          <Card loading={true} style={{ minHeight: '400px' }}>
            <div style={{ textAlign: 'center', padding: '50px 0' }}>
              <Text type='secondary'>{t('加载设置中...')}</Text>
            </div>
          </Card>
        </div>
      </div>
    );
  }

  if (!isEnabled) {
    return (
      <div className={centeredPanelShellClassName} style={panelShellStyle}>
        <div className='playful-console-frame mx-auto w-full max-w-[1800px]'>
          <div className='mx-auto w-full max-w-3xl'>
            <Card className='playful-panel'>
              <div className='playful-panel-content px-6 py-10 md:px-10 md:py-14 text-center'>
                <div className='mb-8 flex justify-center'>
                  <div className='flex h-28 w-28 items-center justify-center rounded-full border-[3px] border-playful-foreground bg-playful-tertiary shadow-pop'>
                    <AlertCircle size={56} color='var(--playful-foreground)' />
                  </div>
                </div>

                <div className='mb-6'>
                  <span className='playful-kicker mb-4'>{t('部署功能未开启')}</span>
                  <Title
                    heading={2}
                    className='!mb-3 !font-outfit !text-[2rem] !font-extrabold !text-playful-foreground'
                  >
                    {t('模型部署服务未启用')}
                  </Title>
                  <Text className='!block !text-lg !leading-8 !text-playful-muted-fg'>
                    {t('访问模型部署功能需要先启用 io.net 部署服务')}
                  </Text>
                </div>

                <div className='playful-section-card mx-auto my-8 max-w-xl px-5 py-6 text-left md:px-6'>
                  <div className='mb-4 flex items-center gap-3'>
                    <div className='flex h-10 w-10 items-center justify-center rounded-full border-2 border-playful-foreground bg-playful-quaternary shadow-pop-sm'>
                      <Server size={20} color='var(--playful-foreground)' />
                    </div>
                    <Text className='!font-outfit !text-base !font-bold !text-playful-foreground'>
                      {t('需要配置的项目')}
                    </Text>
                  </div>

                  <div className='playful-bullet-list'>
                    {bulletItems.map((item) => (
                      <div key={item} className='playful-bullet-item'>
                        <span className='playful-bullet-dot' aria-hidden='true' />
                        <Text className='!text-[15px] !leading-7 !text-playful-foreground'>
                          {t(item)}
                        </Text>
                      </div>
                    ))}
                  </div>
                </div>

                <button
                  type='button'
                  className='playful-link-button'
                  onClick={handleGoToSettings}
                >
                  <Settings size={18} strokeWidth={2.5} />
                  <span>{t('前往部署设置')}</span>
                </button>

                <Text className='!mt-4 !block !text-sm !leading-6 !text-playful-muted-fg'>
                  {t('你可以在系统设置中启用部署服务并填写 io.net 访问凭证。')}
                </Text>
              </div>
            </Card>
          </div>
        </div>
      </div>
    );
  }

  if (connectionLoading || (connectionOk === null && !connectionError)) {
    return (
      <div className={centeredPanelShellClassName} style={panelShellStyle}>
        <div className='playful-console-frame mx-auto w-full max-w-[1800px]'>
          <Card loading={true} style={{ minHeight: '400px' }}>
            <div style={{ textAlign: 'center', padding: '50px 0' }}>
              <Text type='secondary'>{t('正在检查 io.net 连接...')}</Text>
            </div>
          </Card>
        </div>
      </div>
    );
  }

  if (connectionOk === false) {
    const isExpired = connectionError?.type === 'expired';
    const title = isExpired ? t('接口密钥已过期') : t('无法连接 io.net');
    const description = isExpired
      ? t('当前 API 密钥已过期，请在设置中更新。')
      : t('当前配置无法连接到 io.net。');
    const detail = connectionError?.message || '';

    return (
      <div className={centeredPanelShellClassName} style={panelShellStyle}>
        <div className='playful-console-frame mx-auto w-full max-w-[1800px]'>
          <div className='mx-auto w-full max-w-3xl'>
            <Card className='playful-panel'>
              <div className='playful-panel-content px-6 py-10 text-center md:px-10 md:py-14'>
                <div className='mb-8 flex justify-center'>
                  <div className='flex h-28 w-28 items-center justify-center rounded-full border-[3px] border-playful-foreground bg-playful-secondary shadow-pop'>
                    <WifiOff size={56} color='white' />
                  </div>
                </div>

                <div className='mb-6'>
                  <span className='playful-kicker mb-4 bg-playful-secondary text-white'>
                    {isExpired ? t('凭证需要更新') : t('连接失败')}
                  </span>
                  <Title
                    heading={2}
                    className='!mb-3 !font-outfit !text-[2rem] !font-extrabold !text-playful-foreground'
                  >
                    {title}
                  </Title>
                  <Text className='!block !text-lg !leading-8 !text-playful-muted-fg'>
                    {description}
                  </Text>
                  {detail ? (
                    <Text className='!mt-3 !block !text-sm !leading-6 !text-playful-muted-fg'>
                      {detail}
                    </Text>
                  ) : null}
                </div>

                <div className='flex flex-col justify-center gap-3 sm:flex-row'>
                  <Button
                    type='primary'
                    icon={<Settings size={18} strokeWidth={2.5} />}
                    onClick={handleGoToSettings}
                    className='candy-btn'
                  >
                    {t('前往设置')}
                  </Button>
                  {onRetry ? (
                    <Button
                      type='tertiary'
                      onClick={onRetry}
                      className='candy-btn-secondary'
                    >
                      {t('重试连接')}
                    </Button>
                  ) : null}
                </div>
              </div>
            </Card>
          </div>
        </div>
      </div>
    );
  }

  return children;
};

export default DeploymentAccessGuard;
