// 미리보기가 **편집 중인** 프리셋을 그리는지, 그리고 편집기를 끌고 오지 않는지 잰다.
//
// 이 파일의 시험 하나는 감싸는 것이 없다는 사실 자체가 단언이다 — `ReactFlowProvider`
// 를 감싸야 통과하는 순간 불변식 I7 은 이미 깨진 것이다.
//
// @spec SPEC-THEME-001 AC-05 · AC-06 · AC-07 · AC-08 (M5·M6)

import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { ALL_TOKEN_VARS, DAY_PRESET, NIGHT_PRESET } from '@/lib/theme/tokens';

import { ThemePreview } from './ThemePreview';
import {
  PREVIEW_PARTS,
  PREVIEW_TABS,
  areaForToken,
  partsUsingToken,
  tokensOfPart,
} from './themePreviewParts';

const HERE = dirname(fileURLToPath(import.meta.url));

describe('미리보기는 편집 중인 프리셋을 그린다 (AC-06 · I3 · I4)', () => {
  it('night 를 고르면 컨테이너 변수가 night 값이다', () => {
    render(<ThemePreview target="night" overrides={{}} />);
    const box = screen.getByTestId('theme-preview');
    expect(box.style.getPropertyValue('--color-bg-surface')).toBe(
      NIGHT_PRESET['--color-bg-surface'],
    );
  });

  it('day 를 고르면 day 값이다', () => {
    render(<ThemePreview target="day" overrides={{}} />);
    const box = screen.getByTestId('theme-preview');
    expect(box.style.getPropertyValue('--color-bg-surface')).toBe(DAY_PRESET['--color-bg-surface']);
  });

  it('night 일 때만 컨테이너에 `.dark` 가 붙는다', () => {
    // 이것이 없으면 토큰은 야간인데 `dark:` 유틸리티는 주간으로 그려져, 방금 고친
    // 결함과 같은 어긋남이 미리보기 안에 재현된다.
    const { rerender } = render(<ThemePreview target="day" overrides={{}} />);
    expect(screen.getByTestId('theme-preview').className).not.toContain('dark');

    rerender(<ThemePreview target="night" overrides={{}} />);
    expect(screen.getByTestId('theme-preview').className).toContain('dark');
  });

  it('바깥 화면을 건드리지 않는다 — `<html>` 이 그대로다', () => {
    const before = {
      theme: document.documentElement.dataset.theme,
      dark: document.documentElement.classList.contains('dark'),
    };
    render(<ThemePreview target="night" overrides={{}} />);
    expect(document.documentElement.dataset.theme).toBe(before.theme);
    expect(document.documentElement.classList.contains('dark')).toBe(before.dark);
  });

  it('오버라이드가 컨테이너 변수에 반영된다', () => {
    render(<ThemePreview target="day" overrides={{ '--color-bg-surface': '#123456' }} />);
    expect(
      screen.getByTestId('theme-preview').style.getPropertyValue('--color-bg-surface'),
    ).toBe('#123456');
  });
});

describe('미리보기는 편집기를 적재하지 않는다 (AC-07 · I7)', () => {
  it('Provider 없이 렌더된다', () => {
    // 이 시험이 아무것도 감싸지 않는다는 사실 자체가 단언이다.
    expect(() => render(<ThemePreview target="day" overrides={{}} />)).not.toThrow();
  });

  it('편집기 모듈을 import 하지 않는다', () => {
    const src = readFileSync(join(HERE, 'ThemePreview.tsx'), 'utf8');
    for (const forbidden of ['@xyflow/react', 'editorStore', 'tapStore', 'components/flow/']) {
      expect(src, forbidden).not.toContain(forbidden);
    }
  });
});

describe('조각과 토큰이 서로를 가리킨다 (AC-05 · AC-08)', () => {
  it('모든 토큰이 최소 한 조각에 나타난다 — 탭 전체의 합집합 기준', () => {
    const uncovered = ALL_TOKEN_VARS.filter((v) => partsUsingToken(v).length === 0);
    // 나타나지 않는 토큰은 미리보기로 고를 수 없다.
    expect(uncovered).toEqual([]);
  });

  it('탭을 옮기면 그 화면의 조각이 다 그려진다', () => {
    render(<ThemePreview target="day" overrides={{}} />);
    for (const tab of PREVIEW_TABS) {
      fireEvent.click(screen.getByTestId(`theme-preview-tab-${tab.id}`));
      const expected = PREVIEW_PARTS.filter((p) => p.area === tab.id || p.area === 'always');
      for (const p of expected) {
        expect(screen.getByTestId(`theme-preview-${p.id}`), `${tab.id}/${p.id}`).toBeInTheDocument();
      }
      // 다른 화면의 조각은 그 탭에서 보이지 않는다.
      const others = PREVIEW_PARTS.filter((p) => p.area !== tab.id && p.area !== 'always');
      for (const p of others) {
        expect(screen.queryByTestId(`theme-preview-${p.id}`), `${tab.id}/${p.id}`).toBeNull();
      }
    }
  });

  it('표에서 고른 토큰을 쓰는 조각만 강조된다', () => {
    render(<ThemePreview target="day" overrides={{}} activeToken="--color-flow-edge" />);
    const lit = PREVIEW_PARTS.filter((p) => {
      const el = screen.queryByTestId(`theme-preview-${p.id}`);
      return el !== null && el.dataset.lit === 'true';
    }).map((p) => p.id);
    expect(lit).toEqual(['edge']);
  });

  it('짚은 색이 다른 화면에 있으면 그 화면으로 따라간다', () => {
    // 따라가지 않으면 강조가 아무 데도 나타나지 않고, 사용자는 "이 색은 아무 데도
    // 안 쓰이나" 로 읽는다.
    const { rerender } = render(<ThemePreview target="day" overrides={{}} />);
    expect(screen.getByTestId('theme-preview').dataset.area).toBe('dashboard');

    rerender(<ThemePreview target="day" overrides={{}} activeToken="--color-flow-edge" />);
    expect(screen.getByTestId('theme-preview').dataset.area).toBe('flow');
    expect(screen.getByTestId('theme-preview-edge')).toBeInTheDocument();
  });

  it('어느 화면에나 있는 조각만 쓰는 색은 탭을 옮기지 않는다', () => {
    // `text-inverse` 는 단추 줄에만 있고 단추 줄은 항상 보인다.
    expect(areaForToken('--color-text-inverse')).toBeUndefined();
    render(<ThemePreview target="day" overrides={{}} activeToken="--color-text-inverse" />);
    expect(screen.getByTestId('theme-preview').dataset.area).toBe('dashboard');
    expect(screen.getByTestId('theme-preview-controls').dataset.lit).toBe('true');
  });

  it('조각을 누르면 그 조각의 식별자를 낸다', () => {
    const onPickPart = vi.fn();
    render(<ThemePreview target="day" overrides={{}} onPickPart={onPickPart} />);
    fireEvent.click(screen.getByTestId('theme-preview-tab-flow'));
    fireEvent.click(screen.getByTestId('theme-preview-node'));
    expect(onPickPart).toHaveBeenCalledWith('node');
  });

  it('양방향이 같은 대응표를 읽는다', () => {
    // 조각 → 토큰 → 조각 을 왕복하면 자기 자신이 다시 나와야 한다.
    for (const p of PREVIEW_PARTS) {
      for (const cssVar of tokensOfPart(p.id)) {
        expect(partsUsingToken(cssVar), `${p.id} ↔ ${cssVar}`).toContain(p.id);
      }
    }
  });

  it('대응표가 존재하지 않는 토큰을 가리키지 않는다', () => {
    const known = new Set(ALL_TOKEN_VARS);
    const unknown = PREVIEW_PARTS.flatMap((p) => tokensOfPart(p.id)).filter((v) => !known.has(v));
    expect(unknown).toEqual([]);
  });
});

describe('미리보기가 플로우 한 화면에 갇히지 않는다', () => {
  it('네 화면이 각자 조각을 갖는다', () => {
    for (const tab of PREVIEW_TABS) {
      const own = PREVIEW_PARTS.filter((p) => p.area === tab.id);
      expect(own.length, `${tab.id} 화면에 조각이 없다`).toBeGreaterThan(0);
    }
  });

  it('탭 목록이 대시보드 · 플로우 · 목록 · 스케줄 넷이다', () => {
    expect(PREVIEW_TABS.map((t) => t.id)).toEqual(['dashboard', 'flow', 'list', 'schedule']);
  });
});
