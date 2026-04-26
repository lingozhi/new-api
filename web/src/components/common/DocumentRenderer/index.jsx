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

import React, { useEffect, useMemo, useState } from 'react';
import { API, showError } from '../../../helpers';
import { Empty, Card, Spin, Typography } from '@douyinfe/semi-ui';
const { Title } = Typography;
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';
import { useTranslation } from 'react-i18next';
import MarkdownRenderer from '../markdown/MarkdownRenderer';

// Check whether content is a URL.
const isUrl = (content) => {
  try {
    new URL(content.trim());
    return true;
  } catch {
    return false;
  }
};

// Check whether content contains HTML.
const isHtmlContent = (content) => {
  if (!content || typeof content !== 'string') return false;

  const htmlTagRegex = /<\/?[a-z][\s\S]*>/i;
  return htmlTagRegex.test(content);
};

// Parse HTML content and extract inline styles.
const sanitizeHtml = (html) => {
  const tempDiv = document.createElement('div');
  tempDiv.innerHTML = html;

  const styles = Array.from(tempDiv.querySelectorAll('style'))
    .map((style) => style.innerHTML)
    .join('\n');

  const bodyContent = tempDiv.querySelector('body');
  const content = bodyContent ? bodyContent.innerHTML : html;

  return { content, styles };
};

/**
 * 通用文档渲染组件
 * @param {string} apiEndpoint - API 接口地址
 * @param {string} title - 文档标题
 * @param {string} cacheKey - 本地存储缓存键
 * @param {string} emptyMessage - 空内容时的提示消息
 */
const DocumentRenderer = ({ apiEndpoint, title, cacheKey, emptyMessage }) => {
  const { t } = useTranslation();
  const [content, setContent] = useState('');
  const [loading, setLoading] = useState(true);

  const loadContent = async () => {
    const cachedContent = localStorage.getItem(cacheKey) || '';
    if (cachedContent) {
      setContent(cachedContent);
      setLoading(false);
    }

    try {
      const res = await API.get(apiEndpoint);
      const { success, message, data } = res.data;
      if (success && data) {
        setContent(data);
        localStorage.setItem(cacheKey, data);
      } else {
        if (!cachedContent) {
          showError(message || emptyMessage);
          setContent('');
        }
      }
    } catch (error) {
      if (!cachedContent) {
        showError(emptyMessage);
        setContent('');
      }
    } finally {
      setLoading(false);
    }
  };

  const htmlPayload = useMemo(() => {
    if (!isHtmlContent(content)) {
      return { content: '', styles: '' };
    }
    return sanitizeHtml(content);
  }, [content]);

  useEffect(() => {
    loadContent();
  }, []);

  // 处理HTML样式注入
  useEffect(() => {
    const styleId = `document-renderer-styles-${cacheKey}`;
    const { styles } = htmlPayload;

    if (styles) {
      let styleEl = document.getElementById(styleId);
      if (!styleEl) {
        styleEl = document.createElement('style');
        styleEl.id = styleId;
        styleEl.type = 'text/css';
        document.head.appendChild(styleEl);
      }
      styleEl.innerHTML = styles;
    } else {
      const el = document.getElementById(styleId);
      if (el) el.remove();
    }

    return () => {
      const el = document.getElementById(styleId);
      if (el) el.remove();
    };
  }, [cacheKey, htmlPayload]);

  // 显示加载状态
  if (loading) {
    return (
      <div className='playful-doc-shell'>
        <div className='playful-doc-frame flex min-h-[50vh] items-center justify-center'>
          <Spin size='large' />
        </div>
      </div>
    );
  }

  // 如果没有内容，显示空状态
  if (!content || content.trim() === '') {
    return (
      <div className='playful-doc-shell'>
        <Empty
          title={t('管理员未设置' + title + '内容')}
          image={
            <IllustrationConstruction style={{ width: 150, height: 150 }} />
          }
          darkModeImage={
            <IllustrationConstructionDark style={{ width: 150, height: 150 }} />
          }
          className='playful-doc-frame p-8 sticker-card bg-white'
        />
      </div>
    );
  }

  // 如果是 URL，显示链接卡片
  if (isUrl(content)) {
    return (
      <div className='playful-doc-shell'>
        <Card className='playful-doc-frame max-w-md w-full sticker-card bg-white relative z-10'>
          <div className='absolute -top-6 -left-6 w-16 h-16 bg-playful-accent rounded-full -z-10 animate-bounce'></div>
          <div className='absolute -bottom-6 -right-6 w-20 h-20 bg-playful-tertiary border-4 border-playful-foreground rounded-lg -z-10 rotate-12'></div>
          <div className='text-center'>
            <Title heading={4} className='mb-4 font-outfit font-black text-playful-foreground'>
              {title}
            </Title>
            <p className='text-playful-foreground font-jakarta mb-4'>
              {t('管理员设置了外部链接，点击下方按钮访问')}
            </p>
            <a
              href={content.trim()}
              target='_blank'
              rel='noopener noreferrer'
              title={content.trim()}
              aria-label={`${t('访问' + title)}: ${content.trim()}`}
              className='candy-btn bg-playful-accent !text-white inline-block px-6 py-3'
            >
              {t('访问' + title)}
            </a>
          </div>
        </Card>
      </div>
    );
  }

  // 如果是 HTML 内容，直接渲染
  if (isHtmlContent(content)) {
    return (
      <div className='playful-doc-shell'>
        <div className='playful-doc-frame max-w-4xl sticker-card bg-white p-8 md:p-12'>
          <Title heading={2} className='playful-doc-title text-center mb-8 font-outfit font-black text-playful-foreground relative'>
              <span className='relative z-10'>{title}</span>
              <div className='absolute -bottom-2 left-1/2 -translate-x-1/2 w-24 h-4 bg-playful-tertiary -z-10 -rotate-2'></div>
          </Title>
          <div
            className='typography-content max-w-none'
            dangerouslySetInnerHTML={{ __html: htmlPayload.content }}
          />
        </div>
      </div>
    );
  }

  // 其他内容统一使用 Markdown 渲染器
  return (
    <div className='playful-doc-shell'>
      <div className='playful-doc-frame max-w-4xl sticker-card bg-white p-8 md:p-12'>
        <Title heading={2} className='playful-doc-title text-center mb-8 font-outfit font-black text-playful-foreground relative'>
          <span className='relative z-10'>{title}</span>
          <div className='absolute -bottom-2 left-1/2 -translate-x-1/2 w-24 h-4 bg-playful-tertiary -z-10 -rotate-2'></div>
        </Title>
        <div className='typography-content max-w-none'>
          <MarkdownRenderer content={content} />
        </div>
      </div>
    </div>
  );
};

export default DocumentRenderer;
