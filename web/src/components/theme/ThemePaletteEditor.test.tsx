// 팔레트 편집기 동작 검증 — 편집이 스토어에 남고, 초기화가 되돌리며,
// 잘못된 가져오기 파일이 팔레트를 오염시키지 않는지 확인한다.

import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

import { ThemePaletteEditor } from './ThemePaletteEditor';
import { useUIStore } from '@/stores/uiStore';
import { I18nProvider } from '@/lib/i18n';
import {
  DAY_PRESET,
  NIGHT_PRESET,
  THEME_FILE_SCHEMA,
  TOKEN_CATEGORIES,
} from '@/lib/theme/tokens';

/** 로케일에 묶이지 않도록 조회는 testid 와 토큰 라벨(비-i18n)만 쓴다. */
function renderEditor(activePreset: 'day' | 'night' = 'day') {
  return render(
    <I18nProvider>
      <ThemePaletteEditor activePreset={activePreset} />
    </I18nProvider>,
  );
}

/**
 * 파일 선택을 흉내낸다.
 *
 * jsdom 에는 표준 `File.prototype.text()` 가 없어(브라우저에는 있다) 테스트에서만
 * 보완한다. 구현부는 브라우저 표준 API 를 그대로 쓴다.
 */
function selectFile(content: string) {
  const input = screen.getByTestId('theme-import-input') as HTMLInputElement;
  const file = new File([content], 'theme.json', { type: 'application/json' });
  Object.defineProperty(file, 'text', { value: () => Promise.resolve(content) });
  fireEvent.change(input, { target: { files: [file] } });
}

/**
 * 색 칸에 색을 넣는다.
 *
 * 색 칸은 네이티브 색 입력이 아니라 공용 고르개의 팝오버 단추다 — 값은 팝오버 안의
 * 16진 칸으로 들어간다. 고른 뒤 Esc 로 닫는 것은 팝오버가 하나만 떠 있게 하기
 * 위해서다(둘이 뜨면 `colorpicker-hex` 조회가 갈라진다).
 */
function pickColor(label: string, hex: string): void {
  fireEvent.click(screen.getByLabelText(label));
  fireEvent.change(screen.getByTestId('colorpicker-hex'), { target: { value: hex } });
  fireEvent.keyDown(document, { key: 'Escape' });
}

beforeEach(() => {
  useUIStore.setState({ themeOverrides: { day: {}, night: {} }, notifications: [] });
});

describe('ThemePaletteEditor', () => {
  it('활성 팔레트의 기본값을 컬러 테이블에 보여준다', () => {
    renderEditor();
    expect(screen.getByLabelText('기본 배경')).toHaveStyle({
      backgroundColor: DAY_PRESET['--color-bg-primary']!,
    });
  });

  it('카테고리를 나눠도 표는 하나다 — 표를 쪼개면 컬럼 폭이 어긋난다', () => {
    const { container } = renderEditor();

    // 표 1개 + 헤더 1개, 카테고리 수만큼의 tbody.
    expect(container.querySelectorAll('table')).toHaveLength(1);
    expect(container.querySelectorAll('thead')).toHaveLength(1);
    expect(container.querySelectorAll('tbody')).toHaveLength(TOKEN_CATEGORIES.length);

    // 폭 고정 격자 — colgroup 이 모든 카테고리에 같은 컬럼 폭을 준다.
    const table = container.querySelector('table')!;
    expect(table.className).toContain('table-fixed');
    expect(table.querySelectorAll('colgroup > col')).toHaveLength(4);
  });

  it('색을 바꾸면 해당 프리셋의 오버라이드로 저장된다', () => {
    renderEditor();
    pickColor('기본 배경', '#123456');

    expect(useUIStore.getState().themeOverrides).toEqual({
      day: { '--color-bg-primary': '#123456' },
      night: {},
    });
  });

  it('다른 팔레트 탭에서 편집하면 그쪽 오버라이드만 바뀐다', () => {
    renderEditor();
    fireEvent.click(screen.getByTestId('palette-tab-night'));

    expect(screen.getByLabelText('기본 배경')).toHaveStyle({
      backgroundColor: NIGHT_PRESET['--color-bg-primary']!,
    });

    pickColor('기본 배경', '#010203');
    expect(useUIStore.getState().themeOverrides.night).toEqual({
      '--color-bg-primary': '#010203',
    });
    expect(useUIStore.getState().themeOverrides.day).toEqual({});
  });

  it('기본값과 같은 색으로 되돌리면 오버라이드가 사라진다', () => {
    renderEditor();

    pickColor('기본 배경', '#123456');
    pickColor('기본 배경', DAY_PRESET['--color-bg-primary']!);

    expect(useUIStore.getState().themeOverrides.day).toEqual({});
  });

  it('초기화는 해당 팔레트만 되돌린다', () => {
    useUIStore.setState({
      themeOverrides: { day: { '--color-bg-primary': '#111111' }, night: { '--color-bg-primary': '#222222' } },
    });
    renderEditor();

    fireEvent.click(screen.getByTestId('palette-reset'));

    expect(useUIStore.getState().themeOverrides).toEqual({
      day: {},
      night: { '--color-bg-primary': '#222222' },
    });
  });

  it('유효한 파일을 가져오면 팔레트에 반영된다', async () => {
    renderEditor();
    selectFile(
      JSON.stringify({
        schema: THEME_FILE_SCHEMA,
        version: 1,
        preset: 'day',
        tokens: { '--color-bg-primary': '#0a0b0c' },
      }),
    );

    await waitFor(() => {
      expect(useUIStore.getState().themeOverrides.day).toEqual({ '--color-bg-primary': '#0a0b0c' });
    });
  });

  it('XFlow 테마 파일이 아니면 팔레트를 건드리지 않고 오류만 알린다', async () => {
    renderEditor();
    selectFile('{"hello":"world"}');

    await waitFor(() => {
      expect(useUIStore.getState().notifications.at(-1)?.type).toBe('error');
    });
    expect(useUIStore.getState().themeOverrides.day).toEqual({});
  });

  it('알 수 없는 키가 섞인 파일은 쓸 수 있는 색만 취한다', async () => {
    renderEditor();
    selectFile(
      JSON.stringify({
        schema: THEME_FILE_SCHEMA,
        version: 1,
        preset: 'day',
        tokens: { '--color-bg-primary': '#0a0b0c', '--evil': '#ffffff' },
      }),
    );

    await waitFor(() => {
      expect(useUIStore.getState().themeOverrides.day).toEqual({ '--color-bg-primary': '#0a0b0c' });
    });
    expect(useUIStore.getState().notifications.at(-1)?.type).toBe('warning');
  });
});

// --- 미리보기에서 고르면 표가 따라온다 (AC-08) ---

describe('미리보기에서 조각을 고르면 그 색 항목들이 표에서 선택된다', () => {
  /** 지금 선택 표시가 붙어 있는 토큰 행들. */
  function selectedTokens(container: HTMLElement): string[] {
    return [...container.querySelectorAll('tr[data-selected="true"]')].map(
      (el) => el.getAttribute('data-token') ?? '',
    );
  }

  it('조각이 쓰는 색이 여럿이면 행도 여럿이 선택된다', () => {
    const { container } = renderEditor();
    expect(selectedTokens(container)).toEqual([]);

    fireEvent.click(screen.getByTestId('theme-preview-tab-flow'));
    fireEvent.click(screen.getByTestId('theme-preview-node'));

    // 노드 조각은 다섯 색을 쓴다 — 첫 하나만 고르면 나머지 넷은 찾을 길이 없다.
    // 표 안의 순서는 카테고리 배열이 정하므로 집합으로 견준다.
    expect([...selectedTokens(container)].sort()).toEqual(
      [
        '--color-bg-surface',
        '--color-bg-sunken',
        '--color-border-default',
        '--color-text-primary',
        '--color-text-muted',
      ].sort(),
    );
  });

  it('표 위를 지나도 고른 조각이 지워지지 않는다', () => {
    // 둘을 한 상태로 묶으면 고른 직후 표로 커서를 옮기는 것만으로 선택이 사라진다.
    const { container } = renderEditor();
    fireEvent.click(screen.getByTestId('theme-preview-tab-flow'));
    fireEvent.click(screen.getByTestId('theme-preview-edge'));
    expect(selectedTokens(container)).toEqual(['--color-flow-edge']);

    const otherRow = container.querySelector('tr[data-token="--color-bg-primary"]');
    fireEvent.mouseEnter(otherRow!);
    fireEvent.mouseLeave(otherRow!);
    expect(selectedTokens(container)).toEqual(['--color-flow-edge']);
  });

  it('고른 조각은 미리보기에서도 표시가 남는다', () => {
    renderEditor();
    fireEvent.click(screen.getByTestId('theme-preview-tab-flow'));
    fireEvent.click(screen.getByTestId('theme-preview-edge'));
    expect(screen.getByTestId('theme-preview-edge').dataset.selected).toBe('true');
  });

  it('다른 조각을 고르면 선택이 그쪽으로 옮겨 간다', () => {
    const { container } = renderEditor();
    fireEvent.click(screen.getByTestId('theme-preview-tab-flow'));
    fireEvent.click(screen.getByTestId('theme-preview-edge'));
    fireEvent.click(screen.getByTestId('theme-preview-shell'));

    expect(screen.getByTestId('theme-preview-edge').dataset.selected).toBe('false');
    expect([...selectedTokens(container)].sort()).toEqual(
      [
        '--color-bg-primary',
        '--color-bg-secondary',
        '--color-text-primary',
        '--color-border-subtle',
      ].sort(),
    );
  });

  it('고른 조각이 없는 화면으로 옮기면 화면이 바뀌고 선택이 비워진다', () => {
    // 종전에는 선택이 화면을 못으로 박아 탭을 눌러도 화면이 바뀌지 않았다.
    const { container } = renderEditor();
    fireEvent.click(screen.getByTestId('theme-preview-tab-flow'));
    fireEvent.click(screen.getByTestId('theme-preview-node'));
    expect(selectedTokens(container).length).toBeGreaterThan(0);

    fireEvent.click(screen.getByTestId('theme-preview-tab-schedule'));

    expect(screen.getByTestId('theme-preview').dataset.area).toBe('schedule');
    expect(selectedTokens(container)).toEqual([]);
    expect(screen.queryByTestId('theme-preview-node')).toBeNull();
  });

  it('어느 화면에나 있는 조각을 골랐으면 화면을 옮겨도 선택이 남는다', () => {
    // 앱 셸은 어느 탭에서나 보이므로 비울 이유가 없다.
    const { container } = renderEditor();
    fireEvent.click(screen.getByTestId('theme-preview-shell'));
    const before = selectedTokens(container);
    expect(before.length).toBeGreaterThan(0);

    fireEvent.click(screen.getByTestId('theme-preview-tab-list'));

    expect(screen.getByTestId('theme-preview').dataset.area).toBe('list');
    expect(selectedTokens(container)).toEqual(before);
    expect(screen.getByTestId('theme-preview-shell').dataset.selected).toBe('true');
  });

  it('같은 화면 안의 조각이면 탭을 눌러도 선택이 남는다', () => {
    const { container } = renderEditor();
    fireEvent.click(screen.getByTestId('theme-preview-tab-flow'));
    fireEvent.click(screen.getByTestId('theme-preview-edge'));
    fireEvent.click(screen.getByTestId('theme-preview-tab-flow'));

    expect(selectedTokens(container)).toEqual(['--color-flow-edge']);
  });

  it('목록·스케줄·노드를 골라도 그 화면에 머문다', () => {
    // 셋 다 첫 토큰이 `bg-surface` 라, 선택이 화면 따라가기를 타면 모두 대시보드로
    // 끌려갔다. 고른 조각은 이미 지금 화면에 있으므로 옮길 이유가 없다.
    renderEditor();
    for (const [tab, partId] of [
      ['list', 'list-rows'],
      ['schedule', 'schedule-rows'],
      ['flow', 'node'],
    ] as const) {
      fireEvent.click(screen.getByTestId(`theme-preview-tab-${tab}`));
      fireEvent.click(screen.getByTestId(`theme-preview-${partId}`));
      expect(screen.getByTestId('theme-preview').dataset.area, partId).toBe(tab);
      expect(screen.getByTestId(`theme-preview-${partId}`).dataset.selected).toBe('true');
    }
  });
});

// --- 고르는 것은 누르는 일이다 (커서 따라가기는 옵션) ---

describe('표에서 색을 고르는 길', () => {
  function row(container: HTMLElement, cssVar: string): HTMLElement {
    const el = container.querySelector(`tr[data-token="${cssVar}"]`);
    expect(el, `${cssVar} 행이 없다`).not.toBeNull();
    return el as HTMLElement;
  }
  function litTokens(container: HTMLElement): string[] {
    return [...container.querySelectorAll('tr[data-lit="true"]')].map(
      (el) => el.getAttribute('data-token') ?? '',
    );
  }

  it('커서가 지나가기만 하면 아무 일도 없다 — 기본', () => {
    // 표를 훑어보는 동안 미리보기가 쉬지 않고 바뀌면 읽으려던 사람이 멀미를 한다.
    const { container } = renderEditor();
    fireEvent.mouseEnter(row(container, '--color-flow-edge'));

    expect(litTokens(container)).toEqual([]);
    expect(screen.getByTestId('theme-preview').dataset.area).toBe('dashboard');
  });

  it('행을 누르면 고른 색이 되고 미리보기가 그 화면으로 간다', () => {
    const { container } = renderEditor();
    fireEvent.click(row(container, '--color-flow-edge'));

    expect(litTokens(container)).toEqual(['--color-flow-edge']);
    expect(screen.getByTestId('theme-preview').dataset.area).toBe('flow');
    expect(screen.getByTestId('theme-preview-edge').dataset.lit).toBe('true');
  });

  it('커서 따라가기를 켜면 지나가기만 해도 따라온다', () => {
    const { container } = renderEditor();
    fireEvent.click(screen.getByTestId('palette-follow-cursor'));
    fireEvent.mouseEnter(row(container, '--color-flow-edge'));

    expect(litTokens(container)).toEqual(['--color-flow-edge']);
    expect(screen.getByTestId('theme-preview').dataset.area).toBe('flow');
  });

  it('따라가기를 켜도 커서가 떠나면 누른 것이 돌아온다', () => {
    const { container } = renderEditor();
    fireEvent.click(row(container, '--color-flow-edge'));
    fireEvent.click(screen.getByTestId('palette-follow-cursor'));

    const other = row(container, '--color-bg-sunken');
    fireEvent.mouseEnter(other);
    expect(litTokens(container)).toEqual(['--color-bg-sunken']);

    fireEvent.mouseLeave(other);
    expect(litTokens(container)).toEqual(['--color-flow-edge']);
  });

  it('행을 누르면 미리보기의 조각 선택이 지워진다 — 둘이 함께 고정되지 않는다', () => {
    const { container } = renderEditor();
    fireEvent.click(screen.getByTestId('theme-preview-tab-flow'));
    fireEvent.click(screen.getByTestId('theme-preview-node'));
    expect(screen.getByTestId('theme-preview-node').dataset.selected).toBe('true');

    fireEvent.click(row(container, '--color-flow-edge'));
    expect(screen.getByTestId('theme-preview-node').dataset.selected).toBe('false');
    expect(litTokens(container)).toEqual(['--color-flow-edge']);
  });
});
