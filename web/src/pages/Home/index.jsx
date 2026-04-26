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

import React, { useContext, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Typography,
  Input,
  ScrollList,
  ScrollItem,
} from '@douyinfe/semi-ui';
import { API, showError, copy, showSuccess } from '../../helpers';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { API_ENDPOINTS } from '../../constants/common.constant';
import { StatusContext } from '../../context/Status';
import { marked } from 'marked';
import { useTranslation } from 'react-i18next';
import {
  IconGithubLogo,
  IconPlay,
  IconFile,
  IconCopy,
} from '@douyinfe/semi-icons';
import { ArrowRight, Sparkles, ShieldCheck, Layers3 } from 'lucide-react';
import { Link } from 'react-router-dom';
import NoticeModal from '../../components/layout/NoticeModal';
import {
  Moonshot,
  OpenAI,
  XAI,
  Zhipu,
  Volcengine,
  Cohere,
  Claude,
  Gemini,
  Suno,
  Minimax,
  Wenxin,
  Spark,
  Qingyan,
  DeepSeek,
  Qwen,
  Midjourney,
  Grok,
  AzureAI,
  Hunyuan,
  Xinference,
} from '@lobehub/icons';

const { Text } = Typography;

const providerHighlights = [OpenAI, Claude.Color, Gemini.Color, DeepSeek.Color, Qwen.Color, Midjourney];

const featureItems = [
  {
    key: 'providers',
    icon: Layers3,
    title: '多供应商统一接入',
    description: '一次接入，灵活切换 OpenAI、Claude、Gemini、DeepSeek 等主流模型。',
    tone: 'bg-playful-tertiary',
  },
  {
    key: 'reliability',
    icon: ShieldCheck,
    title: '更稳的可用性',
    description: '把鉴权、分组、配额和模型路由管理集中在一个控制台里。',
    tone: 'bg-playful-quaternary',
  },
  {
    key: 'developer',
    icon: Sparkles,
    title: '开发者友好',
    description: '复制基址即可开始调用，无需为每家上游单独维护接入流程。',
    tone: 'bg-playful-secondary text-white',
  },
];

const Home = () => {
  const { t, i18n } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const [homePageContentLoaded, setHomePageContentLoaded] = useState(false);
  const [homePageContent, setHomePageContent] = useState('');
  const [noticeVisible, setNoticeVisible] = useState(false);
  const isMobile = useIsMobile();
  const isDemoSiteMode = statusState?.status?.demo_site_enabled || false;
  const docsLink = statusState?.status?.docs_link || '';
  const serverAddress =
    statusState?.status?.server_address || `${window.location.origin}`;
  const endpointItems = API_ENDPOINTS.map((e) => ({ value: e }));
  const [endpointIndex, setEndpointIndex] = useState(0);
  const isChinese = i18n.language.startsWith('zh');
  const localizedFeatureItems = useMemo(
    () =>
      featureItems.map((item) => ({
        ...item,
        title: t(item.title),
        description: t(item.description),
      })),
    [t],
  );

  const displayHomePageContent = async () => {
    setHomePageContent(localStorage.getItem('home_page_content') || '');
    const res = await API.get('/api/home_page_content');
    const { success, message, data } = res.data;
    if (success) {
      let content = data;
      if (!data.startsWith('https://')) {
        content = marked.parse(data);
      }
      setHomePageContent(content);
      localStorage.setItem('home_page_content', content);

      // 如果内容是 URL，则发送主题模式
      if (data.startsWith('https://')) {
        const iframe = document.querySelector('iframe');
        if (iframe) {
          iframe.onload = () => {
            iframe.contentWindow.postMessage({ themeMode: 'light' }, '*');
            iframe.contentWindow.postMessage({ lang: i18n.language }, '*');
          };
        }
      }
    } else {
      showError(message);
      setHomePageContent('加载首页内容失败...');
    }
    setHomePageContentLoaded(true);
  };

  const handleCopyBaseURL = async () => {
    const ok = await copy(serverAddress);
    if (ok) {
      showSuccess(t('已复制到剪切板'));
    }
  };

  useEffect(() => {
    const checkNoticeAndShow = async () => {
      const lastCloseDate = localStorage.getItem('notice_close_date');
      const today = new Date().toDateString();
      if (lastCloseDate !== today) {
        try {
          const res = await API.get('/api/notice');
          const { success, data } = res.data;
          if (success && data && data.trim() !== '') {
            setNoticeVisible(true);
          }
        } catch (error) {
          console.error('获取公告失败:', error);
        }
      }
    };

    checkNoticeAndShow();
  }, []);

  useEffect(() => {
    displayHomePageContent().then();
  }, []);

  useEffect(() => {
    const timer = setInterval(() => {
      setEndpointIndex((prev) => (prev + 1) % endpointItems.length);
    }, 3000);
    return () => clearInterval(timer);
  }, [endpointItems.length]);

  return (
    <div className='playful-home-shell w-full overflow-x-hidden font-jakarta text-playful-foreground bg-playful-bg min-h-[calc(100vh-64px)]'>
      <NoticeModal
        visible={noticeVisible}
        onClose={() => setNoticeVisible(false)}
        isMobile={isMobile}
      />
      {homePageContentLoaded && homePageContent === '' ? (
        <div className='w-full overflow-x-hidden pb-20'>
          {/* Hero Section */}
          <div className='playful-home-hero relative w-full overflow-hidden border-b-2 border-playful-foreground bg-playful-bg pb-18 pt-24 md:pb-24 md:pt-28'>
            {/* Geometric Decoration */}
            <div className='absolute inset-0 bg-dot-grid opacity-50 z-0' />
            <div className='absolute top-20 -right-20 h-[520px] w-[520px] rounded-full border-4 border-playful-foreground bg-playful-tertiary shadow-pop z-0 hidden lg:block' />
            <div className='absolute bottom-10 left-10 h-32 w-32 rounded-tl-[100px] rounded-br-[100px] border-4 border-playful-foreground bg-playful-secondary animate-pulse z-0 hidden md:block' />
            <div className='absolute left-1/2 top-16 hidden h-20 w-20 -translate-x-1/2 rotate-12 rounded-3xl border-4 border-playful-foreground bg-playful-quaternary shadow-pop lg:block' />

            <div className='relative z-10 mx-auto grid max-w-6xl grid-cols-1 items-center gap-12 px-6 lg:grid-cols-[minmax(0,1.08fr)_minmax(340px,0.92fr)]'>
              {/* Left: Text */}
              <div className='playful-home-hero-copy relative flex flex-col items-center text-center lg:items-start lg:text-left'>
                <div className='playful-home-hero-copy-glow hidden lg:block' aria-hidden='true' />
                <span className='playful-kicker mb-5'>
                  <Sparkles size={14} strokeWidth={2.5} />
                  {t('AI Gateway · Playground Ready')}
                </span>
                <h1 className='font-outfit text-5xl md:text-6xl lg:text-7xl font-extrabold leading-[0.98] text-playful-foreground mb-6'>
                  {t('统一的')}
                  <br />
                  <span className='playful-home-hero-highlight mt-4 inline-block -ml-1 px-4 py-1 text-white'>
                    {t('大模型接口网关')}
                  </span>
                </h1>
                <p className='mt-4 max-w-2xl text-lg font-medium leading-8 md:text-xl'>
                  {t('更好的价格，更好的稳定性，只需要将模型基址替换为：')}
                </p>

                <div className='playful-feature-grid mt-8 w-full max-w-2xl'>
                  {localizedFeatureItems.map((item) => {
                    const Icon = item.icon;
                    return (
                      <div key={item.key} className='playful-feature-card'>
                        <div className={`playful-feature-icon ${item.tone}`}>
                          <Icon size={18} strokeWidth={2.5} />
                        </div>
                        <div>
                          <h3 className='font-outfit text-base font-extrabold text-playful-foreground'>
                            {item.title}
                          </h3>
                          <p className='mt-1 text-sm leading-6 text-playful-muted-fg'>
                            {item.description}
                          </p>
                        </div>
                      </div>
                    );
                  })}
                </div>

                {/* Input */}
                <div className='mt-2 flex w-full max-w-2xl flex-col items-center gap-4 lg:flex-row lg:items-stretch'>
                  <div className='playful-endpoint-shell flex-1 flex items-center px-4 py-2 w-full overflow-hidden'>
                    <div className='playful-endpoint-prefix whitespace-nowrap'>
                      {serverAddress}
                    </div>
                    <div className='playful-endpoint-scroll'>
                      <ScrollList
                        bodyHeight={32}
                        style={{ border: 'unset', boxShadow: 'unset', background: 'transparent', width: '100%' }}
                      >
                        <ScrollItem
                          mode='wheel'
                          cycled={true}
                          list={endpointItems}
                          selectedIndex={endpointIndex}
                          onSelect={({ index }) => setEndpointIndex(index)}
                        />
                      </ScrollList>
                    </div>
                  </div>
                  {/* Copy button */}
                  <button onClick={handleCopyBaseURL} className='candy-btn-secondary px-6 py-2 w-full lg:w-auto h-full min-h-[48px]'>
                    <IconCopy className='mr-2' /> {t('复制')}
                  </button>
                </div>

                {/* Action Buttons */}
                <div className='mt-2 flex flex-wrap gap-4 justify-center lg:justify-start'>
                  <Link to='/console' className='candy-btn px-8 py-3 text-lg'>
                    <span className='mr-3 bg-white text-playful-accent rounded-full p-1 border-2 border-playful-foreground flex item-center justify-center'>
                      <IconPlay size="small" />
                    </span>
                    {t('获取密钥')}
                  </Link>
                  <Link to='/console/token' className='playful-link-button px-7 py-3 text-base'>
                    <ArrowRight size={18} strokeWidth={2.5} />
                    {t('进入控制台')}
                  </Link>

                  {isDemoSiteMode && statusState?.status?.version ? (
                    <button
                      className='candy-btn-secondary px-8 py-3 text-lg'
                      onClick={() =>
                        window.open('https://github.com/QuantumNous/new-api', '_blank')
                      }
                    >
                      <IconGithubLogo className='mr-2' /> {statusState.status.version}
                    </button>
                  ) : (
                    docsLink && (
                      <button
                        className='candy-btn-secondary px-8 py-3 text-lg'
                        onClick={() => window.open(docsLink, '_blank')}
                      >
                        <IconFile className='mr-2' /> {t('文档')}
                      </button>
                    )
                  )}
                </div>

                <div className='playful-home-trust-strip mt-8 grid w-full max-w-2xl grid-cols-1 gap-3 sm:grid-cols-3'>
                  <div className='playful-home-trust-pill bg-playful-tertiary'>
                    <span>{t('统一鉴权')}</span>
                    <strong>{t('更少接线')}</strong>
                  </div>
                  <div className='playful-home-trust-pill bg-playful-secondary text-white'>
                    <span>{t('弹性路由')}</span>
                    <strong>{t('切换更快')}</strong>
                  </div>
                  <div className='playful-home-trust-pill bg-playful-quaternary'>
                    <span>{t('开发友好')}</span>
                    <strong>{t('复制即用')}</strong>
                  </div>
                </div>
              </div>

              {/* Right: Graphic / Info */}
              <div className='hidden lg:flex items-center justify-center relative'>
                <div className='playful-home-hero-card sticker-card bg-white p-8 w-full max-w-md flex flex-col items-center rotate-2 relative z-10'>
                  <div className='playful-home-hero-card-badge'>{t('Playful Stack')}</div>
                  <div className='w-full border-b-4 border-playful-border pb-4 mb-6 text-center'>
                    <h3 className='font-outfit font-bold text-2xl uppercase tracking-wide'>{t('支持众多的大模型供应商')}</h3>
                  </div>
                  <div className='grid grid-cols-3 gap-6 w-full justify-items-center'>
                    {providerHighlights.map((IconComponent, idx) => (
                      <div key={idx} className='playful-provider-orb'>
                        <IconComponent size={28} />
                      </div>
                    ))}
                  </div>
                  <div className='mt-8 candy-btn-secondary px-6 py-2 cursor-default text-sm tracking-widest'>{t('And 30+ More')}</div>
                  <div className='playful-home-metrics mt-8 w-full'>
                    <div>
                      <span>{t('统一入口')}</span>
                      <strong>API</strong>
                    </div>
                    <div>
                      <span>{t('主流模型')}</span>
                      <strong>40+</strong>
                    </div>
                    <div>
                      <span>{t('更快切换')}</span>
                      <strong>{t('一处管理')}</strong>
                    </div>
                  </div>
                </div>
                {/* Decorative floating icon */}
                <div className='absolute -bottom-8 -left-8 w-16 h-16 bg-playful-quaternary rounded-full border-2 border-playful-foreground shadow-pop flex items-center justify-center -rotate-12 animate-bounce cursor-default'>
                  <span className='font-outfit font-bold text-xl'>AI</span>
                </div>
                <div className='playful-home-floating-chip absolute right-0 top-8'>
                  <span>{t('40+ Models')}</span>
                </div>
              </div>
            </div>
          </div>

          {/* Full Provider List on Mobile / Tablet */}
          <div className='max-w-6xl mx-auto px-6 py-16 lg:hidden'>
            <div className='flex items-center justify-center mb-10'>
              <h2 className='font-outfit text-2xl md:text-3xl font-bold bg-playful-tertiary px-6 py-2 border-2 border-playful-foreground shadow-pop -rotate-2 inline-block'>
                {t('支持众多的大模型供应商')}
              </h2>
            </div>
            <div className='flex flex-wrap items-center justify-center gap-4 max-w-3xl mx-auto'>
              {[Moonshot, OpenAI, XAI, Zhipu.Color, Volcengine.Color, Cohere.Color, Claude.Color, Gemini.Color, Suno, Minimax.Color, Wenxin.Color, Spark.Color, Qingyan.Color, DeepSeek.Color, Qwen.Color, Midjourney, Grok, AzureAI.Color, Hunyuan.Color, Xinference.Color].map((IconComponent, idx) => {
                const colors = ['bg-white', 'bg-playful-muted', 'bg-playful-bg'];
                const bgColor = colors[idx % colors.length];
                return (
                  <div key={idx} className={`playful-provider-orb h-14 w-14 md:h-16 md:w-16 ${bgColor}`}>
                    <IconComponent size={32} />
                  </div>
                )
              })}
              <div className='playful-provider-orb h-14 w-14 md:h-16 md:w-16 bg-playful-secondary text-white font-bold text-xl'>
                30+
              </div>
            </div>
          </div>
        </div>
      ) : (
        <div className='overflow-x-hidden w-full'>
          {homePageContent.startsWith('https://') ? (
            <iframe
              src={homePageContent}
              className='w-full h-screen border-none'
            />
          ) : (
            <div className='playful-home-content-shell'>
              <div
                className='playful-home-content typography-content'
                dangerouslySetInnerHTML={{ __html: homePageContent }}
              />
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export default Home;
