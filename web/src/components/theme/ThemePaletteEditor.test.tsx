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

beforeEach(() => {
  useUIStore.setState({ themeOverrides: { day: {}, night: {} }, notifications: [] });
});

describe('ThemePaletteEditor', () => {
  it('활성 팔레트의 기본값을 컬러 테이블에 보여준다', () => {
    renderEditor();
    const input = screen.getByLabelText('기본 배경') as HTMLInputElement;
    expect(input.value).toBe(DAY_PRESET['--color-bg-primary']);
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
    fireEvent.change(screen.getByLabelText('기본 배경'), { target: { value: '#123456' } });

    expect(useUIStore.getState().themeOverrides).toEqual({
      day: { '--color-bg-primary': '#123456' },
      night: {},
    });
  });

  it('다른 팔레트 탭에서 편집하면 그쪽 오버라이드만 바뀐다', () => {
    renderEditor();
    fireEvent.click(screen.getByTestId('palette-tab-night'));

    const input = screen.getByLabelText('기본 배경') as HTMLInputElement;
    expect(input.value).toBe(NIGHT_PRESET['--color-bg-primary']);

    fireEvent.change(input, { target: { value: '#010203' } });
    expect(useUIStore.getState().themeOverrides.night).toEqual({
      '--color-bg-primary': '#010203',
    });
    expect(useUIStore.getState().themeOverrides.day).toEqual({});
  });

  it('기본값과 같은 색으로 되돌리면 오버라이드가 사라진다', () => {
    renderEditor();
    const input = screen.getByLabelText('기본 배경');

    fireEvent.change(input, { target: { value: '#123456' } });
    fireEvent.change(input, { target: { value: DAY_PRESET['--color-bg-primary']! } });

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
