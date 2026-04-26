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
import { Form, Table } from '@douyinfe/semi-ui';
import {
  BarChart3,
  Bell,
  Bookmark,
  Check,
  Inbox,
  PieChart,
  Sparkles,
  Zap,
} from 'lucide-react';
import {
  ConfettiShape,
  ConfettiShapes,
  DotGridBackdrop,
  FloatingIconBadge,
  PlayfulEmpty,
  PlayfulFormField,
  PlayfulKicker,
  PlayfulModal,
  PlayfulTable,
  PlayfulTag,
  SectionHeader,
  SquiggleDivider,
  StickerButton,
  StickerCard,
} from '../../components/common/ui/playful';

const sampleColumns = [
  { title: 'Name', dataIndex: 'name' },
  { title: 'Role', dataIndex: 'role' },
  {
    title: 'Status',
    dataIndex: 'status',
    render: (value) => (
      <PlayfulTag size='sm' tone={value === 'Active' ? 'mint' : 'muted'}>
        {value}
      </PlayfulTag>
    ),
  },
  { title: 'Value', dataIndex: 'value' },
];

const sampleData = [
  { key: 1, name: 'Ada Lovelace', role: 'Analyst', status: 'Active', value: '42' },
  { key: 2, name: 'Grace Hopper', role: 'Engineer', status: 'Active', value: '89' },
  { key: 3, name: 'Alan Turing', role: 'Researcher', status: 'Away', value: '17' },
];

const Section = ({ title, description, children }) => (
  <section className='playful-gallery__section'>
    <SectionHeader
      kicker={title}
      title={
        <span className='playful-section-header__title capitalize'>{title}</span>
      }
      subtitle={description}
    />
    <div className='playful-gallery__grid'>{children}</div>
  </section>
);

const PlayfulGallery = () => {
  const [modalOpen, setModalOpen] = React.useState(false);

  return (
    <div className='relative bg-playful-bg min-h-screen'>
      <DotGridBackdrop density='sparse' opacity={0.6} />
      <div className='playful-gallery relative z-10 mx-auto max-w-6xl'>
        <header className='mb-12 flex flex-col gap-4'>
          <PlayfulKicker tone='tertiary' icon={<Sparkles size={14} strokeWidth={2.5} />}>
            Playful Geometric · Gallery
          </PlayfulKicker>
          <h1 className='font-outfit text-4xl font-extrabold text-playful-foreground md:text-5xl'>
            Phase 1 primitives
          </h1>
          <p className='max-w-2xl font-jakarta text-base text-playful-muted-fg'>
            Every primitive the design system ships, in every meaningful
            variant. Use this page to verify visual regressions as we migrate
            pages in subsequent phases.
          </p>
        </header>

        <Section title='StickerCard' description='Neutral, violet, pink, tertiary, mint tones with hard shadows.'>
          <StickerCard
            tone='neutral'
            shadow='pop-soft'
            kicker='TODAY'
            title='Neutral card'
          >
            <p className='text-sm text-playful-muted-fg'>
              Default tone with soft-border shadow. Safe default for most
              content.
            </p>
          </StickerCard>
          <StickerCard
            tone='violet'
            shadow='pop-violet'
            kicker='FEATURED'
            kickerTone='violet'
            title='Violet w/ floating icon'
            floatingIcon={
              <FloatingIconBadge
                tone='violet'
                icon={<Sparkles size={20} strokeWidth={2.5} color='white' />}
              />
            }
            wiggleOnHover
          >
            <p className='text-sm text-playful-muted-fg'>
              Hover me — the card wiggles slightly, the floating icon rotates.
            </p>
          </StickerCard>
          <StickerCard
            tone='pink'
            shadow='pop-pink'
            kicker='NEW'
            kickerTone='pink'
            title='Pink card'
            floatingIcon={
              <FloatingIconBadge
                tone='pink'
                icon={<Bell size={20} strokeWidth={2.5} color='white' />}
              />
            }
          >
            <p className='text-sm text-playful-muted-fg'>Pink tone + pink hard shadow.</p>
          </StickerCard>
          <StickerCard
            tone='tertiary'
            shadow='pop-tertiary'
            kicker='WARM'
            title='Tertiary card'
            floatingIcon={
              <FloatingIconBadge
                tone='tertiary'
                icon={<Zap size={20} strokeWidth={2.5} />}
              />
            }
          >
            <p className='text-sm text-playful-muted-fg'>Yellow — optimism and energy.</p>
          </StickerCard>
          <StickerCard
            tone='mint'
            shadow='pop-mint'
            kicker='FRESH'
            kickerTone='mint'
            title='Mint card'
            floatingIcon={
              <FloatingIconBadge
                tone='mint'
                icon={<Check size={20} strokeWidth={2.5} />}
              />
            }
          >
            <p className='text-sm text-playful-muted-fg'>Mint tone + mint shadow.</p>
          </StickerCard>
          <StickerCard title='Shadow: none' shadow='none'>
            <p className='text-sm text-playful-muted-fg'>
              Flat card for interior panels. Still bordered.
            </p>
          </StickerCard>
        </Section>

        <Section title='StickerButton' description='Primary / secondary / ghost / danger / icon. Sizes sm/md/lg.'>
          <StickerCard hideHeader>
            <div className='flex flex-wrap items-center gap-3'>
              <StickerButton>Primary</StickerButton>
              <StickerButton variant='secondary'>Secondary</StickerButton>
              <StickerButton variant='ghost'>Ghost</StickerButton>
              <StickerButton variant='danger'>Danger</StickerButton>
              <StickerButton variant='icon' icon={<Bell size={18} strokeWidth={2.5} />} aria-label='bell' />
            </div>
          </StickerCard>
          <StickerCard hideHeader>
            <div className='flex flex-wrap items-center gap-3'>
              <StickerButton size='sm'>Small</StickerButton>
              <StickerButton size='md'>Medium</StickerButton>
              <StickerButton size='lg'>Large</StickerButton>
            </div>
          </StickerCard>
          <StickerCard hideHeader>
            <StickerButton block>Full-width block</StickerButton>
          </StickerCard>
        </Section>

        <Section title='PlayfulKicker + Tag' description='Eyebrow chips and inline status tags.'>
          <StickerCard hideHeader>
            <div className='flex flex-wrap gap-2'>
              <PlayfulKicker>Today</PlayfulKicker>
              <PlayfulKicker tone='pink'>Featured</PlayfulKicker>
              <PlayfulKicker tone='violet'>Beta</PlayfulKicker>
              <PlayfulKicker tone='mint'>Live</PlayfulKicker>
              <PlayfulKicker tone='neutral'>Draft</PlayfulKicker>
            </div>
          </StickerCard>
          <StickerCard hideHeader>
            <div className='flex flex-wrap gap-2'>
              <PlayfulTag>Default</PlayfulTag>
              <PlayfulTag tone='violet'>Violet</PlayfulTag>
              <PlayfulTag tone='pink'>Pink</PlayfulTag>
              <PlayfulTag tone='tertiary'>Tertiary</PlayfulTag>
              <PlayfulTag tone='mint'>Mint</PlayfulTag>
              <PlayfulTag tone='muted'>Muted</PlayfulTag>
              <PlayfulTag tone='danger'>Danger</PlayfulTag>
              <PlayfulTag size='sm' tone='mint'>
                sm mint
              </PlayfulTag>
            </div>
          </StickerCard>
        </Section>

        <Section title='FloatingIconBadge' description='Icon circles sized sm/md/lg across all tones.'>
          <StickerCard hideHeader>
            <div className='flex flex-wrap items-end gap-4'>
              {['sm', 'md', 'lg'].map((size) => (
                <FloatingIconBadge
                  key={size}
                  size={size}
                  tone='neutral'
                  icon={<Bookmark size={size === 'lg' ? 26 : size === 'md' ? 20 : 16} strokeWidth={2.5} />}
                />
              ))}
            </div>
          </StickerCard>
          <StickerCard hideHeader>
            <div className='flex flex-wrap gap-4'>
              {['violet', 'pink', 'tertiary', 'mint', 'neutral'].map((tone) => (
                <FloatingIconBadge
                  key={tone}
                  tone={tone}
                  icon={<PieChart size={20} strokeWidth={2.5} color={tone === 'violet' || tone === 'pink' ? 'white' : '#1E293B'} />}
                />
              ))}
            </div>
          </StickerCard>
        </Section>

        <Section title='SquiggleDivider' description='Color variants, optionally with a centered label.'>
          <StickerCard hideHeader>
            <SquiggleDivider />
            <SquiggleDivider color='accent' />
            <SquiggleDivider color='pink' />
            <SquiggleDivider color='tertiary' />
            <SquiggleDivider color='mint' label='or' />
          </StickerCard>
        </Section>

        <Section title='ConfettiShapes' description='Absolute-positioned decoration layer.'>
          <StickerCard hideHeader className='relative h-48 overflow-hidden'>
            <ConfettiShapes>
              <ConfettiShape kind='circle' tone='violet' size={48} top='10%' left='8%' />
              <ConfettiShape kind='square' tone='tertiary' size={40} top='15%' right='12%' rotate={14} />
              <ConfettiShape kind='triangle' tone='pink' size={54} bottom='20%' left='30%' />
              <ConfettiShape kind='pill' tone='mint' size={64} bottom='18%' right='10%' rotate={-12} />
              <ConfettiShape kind='blob' tone='violet' size={70} bottom='-10%' left='55%' rotate={8} />
            </ConfettiShapes>
            <p className='relative z-10 font-outfit text-lg font-bold text-playful-foreground'>
              Content sits on top, confetti lives behind.
            </p>
          </StickerCard>
        </Section>

        <Section title='SectionHeader' description='Kicker, title, subtitle, trailing action.'>
          <StickerCard hideHeader className='col-span-full'>
            <SectionHeader
              kicker='SECTION'
              title='A standard page heading'
              subtitle='Use this above major sections. It enforces the right font stack, spacing, and kicker positioning.'
              action={<StickerButton size='sm'>Action</StickerButton>}
            />
          </StickerCard>
        </Section>

        <Section title='PlayfulTable' description='Semi Table wrapped with the sticker chrome.'>
          <StickerCard hideHeader className='col-span-full'>
            <PlayfulTable
              columns={sampleColumns}
              dataSource={sampleData}
              pagination={false}
              bordered={false}
            />
          </StickerCard>
        </Section>

        <Section title='PlayfulFormField' description='Uppercase bold label, accent hard-shadow focus.'>
          <StickerCard hideHeader>
            <Form layout='vertical'>
              <PlayfulFormField
                field='name'
                label='Display name'
                placeholder='Ada Lovelace'
              />
              <PlayfulFormField
                as={Form.TextArea}
                field='bio'
                label='Short bio'
                placeholder='A sentence or two...'
                rows={3}
              />
              <PlayfulFormField
                as={Form.Select}
                field='role'
                label='Role'
                placeholder='Choose a role'
                optionList={[
                  { value: 'analyst', label: 'Analyst' },
                  { value: 'engineer', label: 'Engineer' },
                  { value: 'researcher', label: 'Researcher' },
                ]}
                style={{ width: '100%' }}
              />
            </Form>
          </StickerCard>
        </Section>

        <Section title='PlayfulModal' description='Bordered modal, Outfit title, tone-tinted header.'>
          <StickerCard hideHeader>
            <StickerButton onClick={() => setModalOpen(true)}>Open modal</StickerButton>
            <PlayfulModal
              title='Confirm action'
              visible={modalOpen}
              onCancel={() => setModalOpen(false)}
              onOk={() => setModalOpen(false)}
              okText='Yes, do it'
              cancelText='Cancel'
            >
              <p className='font-jakarta text-sm text-playful-foreground'>
                This modal uses the canonical Playful chrome — 2px border,
                hard-shadow popover, yellow header tint, Outfit title.
              </p>
            </PlayfulModal>
          </StickerCard>
        </Section>

        <Section title='PlayfulEmpty' description='Empty-state wrapper with confetti decoration.'>
          <StickerCard hideHeader className='col-span-full'>
            <PlayfulEmpty
              kicker='NOTHING YET'
              title='No results found'
              description='Try adjusting your filters or clear them to see every row.'
              illustration={
                <FloatingIconBadge
                  tone='tertiary'
                  size='lg'
                  icon={<Inbox size={28} strokeWidth={2.5} />}
                />
              }
              action={<StickerButton size='sm'>Clear filters</StickerButton>}
            />
          </StickerCard>
        </Section>

        <Section title='DotGridBackdrop + chart palette' description='Background texture + VChart playful palette preview.'>
          <StickerCard hideHeader className='col-span-full'>
            <div className='flex flex-wrap gap-3'>
              {['#8B5CF6', '#F472B6', '#FBBF24', '#34D399', '#1E293B', '#60A5FA', '#FB7185', '#14B8A6', '#F97316', '#A78BFA'].map((hex) => (
                <div
                  key={hex}
                  className='flex w-28 flex-col items-center gap-1'
                >
                  <div
                    className='h-12 w-full rounded-lg border-2 border-playful-foreground shadow-pop-sm'
                    style={{ backgroundColor: hex }}
                  />
                  <span className='font-jakarta text-xs font-semibold text-playful-muted-fg'>{hex}</span>
                </div>
              ))}
            </div>
          </StickerCard>
        </Section>
      </div>
    </div>
  );
};

export default PlayfulGallery;
