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

import VChart, { ThemeManager } from '@visactor/vchart';

export const PLAYFUL_PALETTE = [
  '#8B5CF6', // accent (violet)
  '#F472B6', // secondary (pink)
  '#FBBF24', // tertiary (amber)
  '#34D399', // quaternary (mint)
  '#1E293B', // foreground (slate)
  '#60A5FA', // supporting blue
  '#FB7185', // supporting rose
  '#14B8A6', // supporting teal
  '#F97316', // supporting orange
  '#A78BFA', // supporting light violet
];

export const PLAYFUL_FOREGROUND = '#1E293B';
export const PLAYFUL_MUTED_FG = '#64748B';
export const PLAYFUL_BORDER = '#E2E8F0';
export const PLAYFUL_CARD = '#FFFFFF';

const PLAYFUL_THEME_NAME = 'playful-geometric';

const playfulTheme = {
  name: PLAYFUL_THEME_NAME,
  fontFamily: "'Plus Jakarta Sans', system-ui, sans-serif",
  colorScheme: {
    default: {
      dataScheme: [
        {
          name: 'playful',
          scheme: PLAYFUL_PALETTE,
        },
      ],
    },
  },
  token: {
    colorAxisLabel: PLAYFUL_MUTED_FG,
    colorAxisGrid: PLAYFUL_BORDER,
    colorAxisDomain: PLAYFUL_BORDER,
    colorLegendLabel: PLAYFUL_FOREGROUND,
    colorTooltipTitle: PLAYFUL_FOREGROUND,
    colorTooltipContent: PLAYFUL_FOREGROUND,
    fontFamily: "'Plus Jakarta Sans', system-ui, sans-serif",
    fontFamilyTitle: "'Outfit', system-ui, sans-serif",
  },
  axis: {
    label: {
      style: {
        fill: PLAYFUL_MUTED_FG,
        fontFamily: "'Plus Jakarta Sans', system-ui, sans-serif",
        fontWeight: 500,
      },
    },
    domainLine: { style: { stroke: PLAYFUL_BORDER, lineWidth: 1 } },
    grid: { style: { stroke: PLAYFUL_BORDER, lineDash: [4, 4] } },
  },
  legends: {
    item: {
      label: {
        style: {
          fill: PLAYFUL_FOREGROUND,
          fontFamily: "'Plus Jakarta Sans', system-ui, sans-serif",
          fontWeight: 600,
        },
      },
    },
  },
  tooltip: {
    style: {
      panel: {
        backgroundColor: PLAYFUL_CARD,
        border: { color: PLAYFUL_FOREGROUND, width: 2, radius: 12 },
        shadow: {
          x: 4,
          y: 4,
          blur: 0,
          spread: 0,
          color: PLAYFUL_FOREGROUND,
        },
      },
      titleLabel: {
        fontFamily: "'Outfit', system-ui, sans-serif",
        fontWeight: 800,
        fontColor: PLAYFUL_FOREGROUND,
      },
      keyLabel: {
        fontFamily: "'Plus Jakarta Sans', system-ui, sans-serif",
        fontWeight: 600,
        fontColor: PLAYFUL_MUTED_FG,
      },
      valueLabel: {
        fontFamily: "'Plus Jakarta Sans', system-ui, sans-serif",
        fontWeight: 700,
        fontColor: PLAYFUL_FOREGROUND,
      },
    },
  },
};

let registered = false;

export function registerPlayfulVChartTheme() {
  if (registered) return;
  try {
    ThemeManager.registerTheme(PLAYFUL_THEME_NAME, playfulTheme);
    ThemeManager.setCurrentTheme(PLAYFUL_THEME_NAME);
    // Keep both the manager and the static VChart.ThemeManager aligned for
    // legacy call sites that reach into VChart.
    if (VChart?.ThemeManager?.setCurrentTheme) {
      VChart.ThemeManager.setCurrentTheme(PLAYFUL_THEME_NAME);
    }
    registered = true;
  } catch (err) {
    // Swallow — a failed theme registration must not break the app.
    console.warn('[playful] VChart theme registration failed', err);
  }
}

export const PLAYFUL_VCHART_THEME_NAME = PLAYFUL_THEME_NAME;
