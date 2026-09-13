// 특성화 시험 — `${색}NN` 이어붙이기 12자리의 **오늘의 출력**을 고정한다.
//
// @spec SPEC-COLOR-001 §결정 5 (M3) — AC-E6 · 불변식 I8 · 함정 E9
//
// 이 파일은 교체 **전에** 먼저 커밋되고, 교체 전후로 같은 결과를 내야 한다. 그것이
// 이 단계가 존재하는 이유다 — 교체 diff 가 크므로 회귀가 묻힌다.
//
// ## 재는 방법을 고른 이유 (실측으로 정정한 것)
//
// acceptance.md 의 함정 E1 은 "`element.style.borderColor` 를 직접 읽고 비어 있지
// 않음까지 단언" 하라고 적었다. 그 방법은 **선언이 버려졌는지**는 잡지만
// **바이트 동일성은 잡지 못한다.** 실측:
//
//     el.style.borderColor = '#3b82f630'  →  "rgba(59, 130, 246, 0.19)"
//     el.style.backgroundColor = '#3b82f620'  →  "rgba(59, 130, 246, 0.125)"
//
// jsdom 의 CSSOM 이 8자리 hex 를 받아서 `rgba()` 로 **정규화하며 알파를 반올림한다.**
// `#3b82f630`(0.18824) 과 `#3b82f631`(0.19216) 이 둘 다 `0.19` 가 되므로, DOM 을 읽는
// 시험은 한 눈금 어긋난 반올림을 **놓친다.** AC-E6 은 "반올림 방향 차이로 한 자리라도
// 다르면 실패" 를 요구하므로 DOM 읽기로는 그 요구를 만족시킬 수 없다.
//
// 그래서 둘로 나눈다:
//   (1) **바이트 동일성**은 문자열을 직접 비교한다 — 순수 함수는 반환값을, JSX 자리는
//       `withAlpha` 가 옛 이어붙이기와 같은 문자열을 내는지를 입력 전 영역에 걸쳐.
//   (2) **선언이 살아 있는지**(E1)는 DOM 을 읽어 확인한다.

import { render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { withAlpha } from '@/components/common/colorpicker/colorFormat';

import { readListPanelStyle } from './listPanelStyle';
import { resolveTileStyle, TILE_TINT_ALPHA } from './tileSelection';

// ---------------------------------------------------------------------------
// (1) 바이트 동일성 — 12자리가 쓰는 알파 상수 전부 × 현실적인 색 전 영역
// ---------------------------------------------------------------------------

/** 12자리가 실제로 이어붙이는 두 자리들(실측). */
const SITE_ALPHAS = ['20', '30', '40', '80', '90'] as const;

/** 저장소에 실제로 들어 있는 색들 + 경계값. */
const COLORS = [
  '#3b82f6',
  '#8b5cf6',
  '#06b6d4',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#ec4899',
  '#6b7280',
  '#000000',
  '#ffffff',
  '#010203',
  '#fefefe',
] as const;

describe('withAlpha 가 옛 이어붙이기와 바이트 단위로 같다 (I8)', () => {
  it.each(SITE_ALPHAS)('알파 %s — 모든 색에서 `${색}NN` 과 같은 문자열', (nn) => {
    const alpha = parseInt(nn, 16) / 255;
    for (const color of COLORS) {
      // 왼쪽이 교체 후, 오른쪽이 교체 전. 이 등식이 곧 I8 이다.
      expect(withAlpha(color, alpha)).toBe(`${color}${nn}`);
    }
  });

  it('대문자 색도 소문자로 내려 같은 색을 낸다', () => {
    expect(withAlpha('#3B82F6', 0x20 / 255)).toBe('#3b82f620');
  });
});

// ---------------------------------------------------------------------------
// (2) 자리 11 — tileSelection.resolveTileStyle
// ---------------------------------------------------------------------------

describe('자리 11 — resolveTileStyle 의 배경 틴트', () => {
  it('6자리 글자색이면 틴트가 8자리 문자열이다', () => {
    const { style } = resolveTileStyle(undefined, { color: '#3b82f6' });
    expect(style?.backgroundColor).toBe('#3b82f620');
  });

  it('직접 정한 배경이 틴트를 이긴다', () => {
    const { style } = resolveTileStyle(undefined, { color: '#3b82f6', bg: '#ef4444' });
    expect(style?.backgroundColor).toBe('#ef4444');
  });

  it('글자색이 없으면 틴트를 만들지 않는다', () => {
    const { style } = resolveTileStyle({ backgroundColor: '#111111' }, { size: 12 });
    expect(style?.backgroundColor).toBe('#111111');
  });

  it('틴트 상수는 0x20 이다', () => {
    // 교체 뒤 이 상수는 문자열이 아니라 수가 된다. 값이 같음을 여기서 고정한다.
    expect(parseInt(String(TILE_TINT_ALPHA), 16) || Math.round(Number(TILE_TINT_ALPHA) * 255)).toBe(
      0x20,
    );
  });
});

// ---------------------------------------------------------------------------
// (3) 자리 12 — listPanelStyle 의 배지 틴트
// ---------------------------------------------------------------------------

describe('자리 12 — 배지 배경 틴트', () => {
  it('배지 글자색이 정해지면 같은 색을 옅게 깐다', () => {
    const style = readListPanelStyle({ badge_font: { color: '#10b981' } });
    expect(style.badgeStyle?.color).toBe('#10b981');
    expect(style.badgeStyle?.backgroundColor).toBe('#10b98120');
  });

  it('악센트 폴백 색으로도 틴트가 만들어진다', () => {
    const style = readListPanelStyle({ panelColor: '#ef4444', badge_font: { size: 12 } });
    expect(style.badgeStyle?.backgroundColor).toBe('#ef444420');
  });

  it('색이 없으면 틴트도 없다 — 상태별 기본 클래스가 살아야 한다', () => {
    const style = readListPanelStyle({ badge_font: { size: 12 } });
    expect(style.badgeStyle?.backgroundColor).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// (4) 자리 1 — PropertiesGridPanel 이 실제로 선언을 그리는가 (E1)
// ---------------------------------------------------------------------------

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

vi.mock('@/hooks/useDevice', () => ({
  useDeviceRealtime: () => ({
    data: {
      id: 'dev-1',
      name: '센서',
      type: 'sensor',
      protocol: 'modbus',
      metadata: undefined,
      state: { properties: { temperature: 21 } },
    },
    isLoading: false,
  }),
}));

vi.mock('@/hooks/useDetailTargets', () => ({
  useDeviceDetailTarget: () => ({ data: undefined, isLoading: false }),
}));

describe('자리 1 — PropertiesGridPanel 카드 테두리 (E1)', () => {
  it('악센트 테두리 색이 선언으로 살아 있고 비어 있지 않다', async () => {
    const { PanelEditContext } = await import('../panelEditContext');
    const { default: PropertiesGridPanel } = await import('./PropertiesGridPanel');

    const { container } = render(
      <PanelEditContext.Provider value={false}>
        <PropertiesGridPanel
          panelId="p1"
          title="현황"
          config={{
            deviceId: 'dev-1',
            visibleProperties: ['temperature'],
            panelColor: '#3b82f6',
            accentElements: { borders: '#3b82f6' },
          }}
          onConfigChange={vi.fn()}
        />
      </PanelEditContext.Provider>,
    );

    const card = container.querySelector<HTMLElement>('[data-testid="property-card"]');
    expect(card).not.toBeNull();
    // E1: "없음" 과 "잘못됨" 을 가르기 위해 비어 있지 않음까지 본다.
    expect(card!.style.borderColor).not.toBe('');
    // jsdom 이 `rgba()` 로 정규화한다(머리말 참조). 알파 0x30/255 = 0.18824 → 0.19.
    expect(card!.style.borderColor).toBe('rgba(59, 130, 246, 0.19)');
  });
});
