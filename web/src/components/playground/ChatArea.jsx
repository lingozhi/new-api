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
import { Card, Chat, Typography, Button } from '@douyinfe/semi-ui';
import { MessageSquare, Eye, EyeOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import CustomInputRender from './CustomInputRender';

const UPLOAD_ACTION_PLACEHOLDER = 'about:blank';

const ChatArea = ({
  chatRef,
  message,
  inputs,
  styleState,
  showDebugPanel,
  roleInfo,
  onMessageSend,
  onMessageCopy,
  onMessageReset,
  onMessageDelete,
  onStopGenerator,
  onClearMessages,
  onToggleDebugPanel,
  renderCustomChatContent,
  renderChatBoxAction,
}) => {
  const { t } = useTranslation();

  const renderInputArea = React.useCallback((props) => {
    return <CustomInputRender {...props} />;
  }, []);

  return (
    <Card
      className='playful-chat-card h-full overflow-hidden'
      bordered={false}
      bodyStyle={{
        padding: 0,
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      }}
    >
      {/* 聊天头部 */}
      {styleState.isMobile ? (
        <div className='playful-mobile-chat-banner'>
          <div>
            <p className='playful-mobile-chat-label'>{t('Prompt Lab')}</p>
            <Typography.Title heading={6} className='!mb-0 !text-playful-foreground'>
              {t('AI 对话')}
            </Typography.Title>
          </div>
          <div className='playful-mobile-chat-model'>{inputs.model || t('选择模型开始对话')}</div>
        </div>
      ) : (
        <div className='playful-chat-header'>
          <div className='flex items-center justify-between'>
            <div className='flex items-center gap-3'>
              <div className='playful-chat-icon-wrap'>
                <MessageSquare size={20} className='text-playful-foreground' />
              </div>
              <div>
                <p className='playful-chat-eyebrow'>{t('Prompt Lab')}</p>
                <Typography.Title heading={5} className='!text-playful-foreground mb-0'>
                  {t('AI 对话')}
                </Typography.Title>
                <Typography.Text className='!text-playful-muted-fg text-sm hidden sm:inline'>
                  {inputs.model || t('选择模型开始对话')}
                </Typography.Text>
              </div>
            </div>
            <div className='flex items-center gap-2'>
              <Button
                icon={showDebugPanel ? <EyeOff size={14} /> : <Eye size={14} />}
                onClick={onToggleDebugPanel}
                theme='borderless'
                type='primary'
                size='small'
                className='playful-chat-debug-toggle'
              >
                {showDebugPanel ? t('隐藏调试') : t('显示调试')}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* 聊天内容区域 */}
      <div className='playful-chat-body flex-1 overflow-hidden'>
        <Chat
          ref={chatRef}
          uploadProps={{
            action: UPLOAD_ACTION_PLACEHOLDER,
          }}
          chatBoxRenderConfig={{
            renderChatBoxContent: renderCustomChatContent,
            renderChatBoxAction: renderChatBoxAction,
            renderChatBoxTitle: () => null,
          }}
          renderInputArea={renderInputArea}
          roleConfig={roleInfo}
          style={{
            height: '100%',
            maxWidth: '100%',
            overflow: 'hidden',
          }}
          chats={message}
          onMessageSend={onMessageSend}
          onMessageCopy={onMessageCopy}
          onMessageReset={onMessageReset}
          onMessageDelete={onMessageDelete}
          showClearContext
          showStopGenerate
          onStopGenerator={onStopGenerator}
          onClear={onClearMessages}
          className='playful-chat-instance h-full'
          placeholder={t('请输入您的问题...')}
        />
      </div>
    </Card>
  );
};

export default ChatArea;
