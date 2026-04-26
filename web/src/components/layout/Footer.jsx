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

import React, { useEffect, useState, useMemo, useContext } from 'react';
import { useTranslation } from 'react-i18next';
import { Typography } from '@douyinfe/semi-ui';
import { getFooterHTML, getLogo, getSystemName } from '../../helpers';
import { StatusContext } from '../../context/Status';

const FooterBar = () => {
  const { t } = useTranslation();
  const [footer, setFooter] = useState(getFooterHTML());
  const systemName = getSystemName();
  const logo = getLogo();
  const [statusState] = useContext(StatusContext);
  const isDemoSiteMode = statusState?.status?.demo_site_enabled || false;

  const loadFooter = () => {
    let footer_html = localStorage.getItem('footer_html');
    if (footer_html) {
      setFooter(footer_html);
    }
  };

  const currentYear = new Date().getFullYear();

  const customFooter = useMemo(
    () => (
      <footer className='playful-footer-shell relative w-full overflow-hidden border-t-4 border-playful-foreground bg-playful-bg px-6 py-16 font-jakarta md:px-10 lg:px-16'>
        <div className='playful-footer-orb-left' aria-hidden='true' />
        <div className='playful-footer-orb-right' aria-hidden='true' />
        <div className='playful-footer-dots bg-dot-grid' aria-hidden='true' />

        <div className='playful-footer-frame mx-auto flex w-full max-w-[1160px] flex-col gap-10 px-6 py-8 md:px-8 md:py-10'>
          {isDemoSiteMode && (
            <div className='grid gap-8 lg:grid-cols-[minmax(0,320px)_minmax(0,1fr)] lg:items-start'>
              <div className='playful-footer-brand-card'>
                <div className='flex items-center gap-4'>
                  <div className='playful-footer-logo-wrap'>
                    <img
                      src={logo}
                      alt={systemName}
                      className='h-14 w-14 rounded-full bg-slate-900 p-1.5 object-contain'
                    />
                  </div>
                  <div>
                    <div className='playful-kicker mb-3'>{t('AI Gateway')}</div>
                    <h2 className='font-outfit text-3xl font-extrabold tracking-tight text-playful-foreground'>
                      {systemName}
                    </h2>
                  </div>
                </div>
                <p className='mt-5 text-sm leading-7 text-playful-muted-fg'>
                  {t('把多模型接入、文档访问与控制台体验统一到一个更清晰、更有趣的入口。')}
                </p>
                <div className='mt-6 flex flex-wrap gap-3'>
                  <a
                    href='https://docs.newapi.pro/getting-started/'
                    target='_blank'
                    rel='noopener noreferrer'
                    className='candy-btn-secondary px-5 py-2 text-sm'
                  >
                    {t('快速开始')}
                  </a>
                  <a
                    href='https://github.com/QuantumNous/new-api'
                    target='_blank'
                    rel='noopener noreferrer'
                    className='candy-btn px-5 py-2 text-sm'
                  >
                    {t('项目仓库')}
                  </a>
                </div>
              </div>

              <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4'>
                <div className='playful-footer-link-card'>
                  <p className='playful-footer-title'>{t('关于我们')}</p>
                  <div className='playful-footer-links'>
                    <a href='https://docs.newapi.pro/wiki/project-introduction/' target='_blank' rel='noopener noreferrer'>
                      {t('关于项目')}
                    </a>
                    <a href='https://docs.newapi.pro/support/community-interaction/' target='_blank' rel='noopener noreferrer'>
                      {t('联系我们')}
                    </a>
                    <a href='https://docs.newapi.pro/wiki/features-introduction/' target='_blank' rel='noopener noreferrer'>
                      {t('功能特性')}
                    </a>
                  </div>
                </div>

                <div className='playful-footer-link-card'>
                  <p className='playful-footer-title'>{t('文档')}</p>
                  <div className='playful-footer-links'>
                    <a href='https://docs.newapi.pro/getting-started/' target='_blank' rel='noopener noreferrer'>
                      {t('快速开始')}
                    </a>
                    <a href='https://docs.newapi.pro/installation/' target='_blank' rel='noopener noreferrer'>
                      {t('安装指南')}
                    </a>
                    <a href='https://docs.newapi.pro/api/' target='_blank' rel='noopener noreferrer'>
                      {t('API 文档')}
                    </a>
                  </div>
                </div>

                <div className='playful-footer-link-card'>
                  <p className='playful-footer-title'>{t('相关项目')}</p>
                  <div className='playful-footer-links'>
                    <a href='https://github.com/songquanpeng/one-api' target='_blank' rel='noopener noreferrer'>
                      One API
                    </a>
                    <a href='https://github.com/novicezk/midjourney-proxy' target='_blank' rel='noopener noreferrer'>
                      Midjourney-Proxy
                    </a>
                    <a href='https://github.com/Calcium-Ion/neko-api-key-tool' target='_blank' rel='noopener noreferrer'>
                      neko-api-key-tool
                    </a>
                  </div>
                </div>

                <div className='playful-footer-link-card'>
                  <p className='playful-footer-title'>{t('友情链接')}</p>
                  <div className='playful-footer-links'>
                    <a href='https://github.com/Calcium-Ion/new-api-horizon' target='_blank' rel='noopener noreferrer'>
                      new-api-horizon
                    </a>
                    <a href='https://github.com/coaidev/coai' target='_blank' rel='noopener noreferrer'>
                      CoAI
                    </a>
                    <a href='https://www.gpt-load.com/' target='_blank' rel='noopener noreferrer'>
                      GPT-Load
                    </a>
                  </div>
                </div>
              </div>
            </div>
          )}

          <div className='playful-footer-bottom flex flex-col gap-4 border-t-2 border-dashed border-playful-foreground/30 pt-6 md:flex-row md:items-center md:justify-between'>
            <div className='flex flex-wrap items-center gap-2'>
              <Typography.Text className='text-sm !text-playful-muted-fg'>
                © {currentYear} {systemName}. {t('版权所有')}
              </Typography.Text>
            </div>

            <div className='text-sm text-playful-muted-fg'>
              <span>{t('设计与开发由')} </span>
              <a
                href='https://github.com/QuantumNous/new-api'
                target='_blank'
                rel='noopener noreferrer'
                className='font-bold !text-playful-accent'
              >
                New API
              </a>
            </div>
          </div>
        </div>
      </footer>
    ),
    [logo, systemName, t, currentYear, isDemoSiteMode],
  );

  useEffect(() => {
    loadFooter();
  }, []);

  return (
    <div className='w-full'>
      {footer ? (
        <footer className='playful-footer-shell relative w-full overflow-hidden border-t-4 border-playful-foreground bg-playful-bg px-6 py-8 font-jakarta md:px-10'>
          <div className='playful-footer-orb-left' aria-hidden='true' />
          <div className='playful-footer-orb-right' aria-hidden='true' />
          <div className='playful-footer-frame mx-auto flex w-full max-w-[1160px] flex-col gap-4 px-6 py-6 md:flex-row md:items-center md:justify-between md:px-8'>
            <div
              className='custom-footer na-cb6feafeb3990c78 text-sm !text-playful-muted-fg'
              dangerouslySetInnerHTML={{ __html: footer }}
            ></div>
            <div className='text-sm flex-shrink-0 text-playful-muted-fg'>
              <span>
                {t('设计与开发由')}{' '}
              </span>
              <a
                href='https://github.com/QuantumNous/new-api'
                target='_blank'
                rel='noopener noreferrer'
                className='font-bold !text-playful-accent'
              >
                New API
              </a>
            </div>
          </div>
        </footer>
      ) : (
        customFooter
      )}
    </div>
  );
};

export default FooterBar;
