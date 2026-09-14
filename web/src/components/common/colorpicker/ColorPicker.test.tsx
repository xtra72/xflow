// 통합 색 고르개 — 조립된 상태에서 본다.
//
// **꺼져 있어서 통과하는 초록은 초록이 아니다.** 이 부품의 절반이 조건부 렌더
// (스포이트 · 불투명도 슬라이더 · 비우기 · 최근색)라, 조건을 켜지 않은 채 "없다" 만
// 확인하면 아무것도 재지 못한다. 아래는 조건부 요소마다 **켠 갈래와 끈 갈래를 둘 다**
// 보고, 켠 갈래에서는 실제로 동작하는 데까지 간다.
//
// jsdom 이 인라인 스타일의 색을 `rgb()`/`rgba()` 로 정규화한다(실측). 그래서 8자리
// 문자열을 스와치에 넣었을 때의 단언은 `rgba(...)` 꼴이며, **무효한 색이면 빈 문자열이
// 되므로** "비어 있지 않다" 가 곧 "브라우저가 그 색을 받아들였다" 는 뜻이다.

import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { act, useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider, type Locale } from '@/lib/i18n';

import ColorPicker, { type ColorPickerProps } from './ColorPicker';
import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';
import { UNIFIED_PALETTE } from './palette';
import {
  RECENT_COLORS_STORAGE_KEY,
  readRecentColors,
  resetRecentColorsForTest,
} from './recentColors';

const BLUE = '#3b82f6';
const BLUE_A50 = '#3b82f680';
const RED = '#ef4444';
const LABEL = '테두리';

/** 판의 상자 — jsdom 에는 레이아웃이 없어 직접 꽂는다. */
const FIELD_RECT = {
  left: 100,
  top: 50,
  right: 300,
  bottom: 150,
  width: 200,
  height: 100,
  x: 100,
  y: 50,
  toJSON: () => ({}),
} as DOMRect;

type PickerProps = Omit<ColorPickerProps, 'value' | 'onChange' | 'ariaLabel'>;

/**
 * 값을 실제로 들고 있는 부모. 고르개는 제어 부품이므로, 값이 돌아오지 않으면
 * 바깥에서 값이 바뀔 때의 경로가 한 번도 지나지 않는다.
 */
function Controlled({
  initial,
  onChange,
  ...rest
}: PickerProps & { initial?: string; onChange: (next: string | undefined) => void }) {
  const [v, setV] = useState<string | undefined>(initial);
  return (
    <ColorPicker
      {...rest}
      value={v}
      ariaLabel={LABEL}
      onChange={(next) => {
        setV(next);
        onChange(next);
      }}
    />
  );
}

function renderPicker(props: PickerProps & { initial?: string } = {}, locale: Locale = 'ko') {
  localStorage.setItem('xflow-locale', locale);
  const onChange = vi.fn();
  const utils = render(
    <I18nProvider>
      <Controlled {...props} onChange={onChange} />
    </I18nProvider>,
  );
  return { onChange, ...utils };
}

/** 값이 바깥에서 절대 바뀌지 않는 자리 — "열었다는 이유로 덮이지 않는다" 를 재려면 필요하다. */
function renderFixed(value: string | undefined, props: PickerProps = {}) {
  localStorage.setItem('xflow-locale', 'ko');
  const onChange = vi.fn();
  render(
    <I18nProvider>
      <ColorPicker {...props} value={value} ariaLabel={LABEL} onChange={onChange} />
    </I18nProvider>,
  );
  return { onChange };
}

const trigger = (): HTMLElement => screen.getByRole('button', { name: LABEL });
const popover = (): HTMLElement => screen.getByTestId('colorpicker-popover');
const hex = (): HTMLInputElement => screen.getByTestId('colorpicker-hex');
const hue = (): HTMLInputElement => screen.getByTestId('colorpicker-hue');
const alphaSlider = (): HTMLInputElement => screen.getByTestId('colorpicker-alpha');
const preview = (): HTMLElement => screen.getByTestId('colorpicker-preview');
const open = (): void => {
  fireEvent.click(trigger());
};
/** 같은 색이 프리셋과 최근색 양쪽에 있을 수 있으므로 격자를 집어 찾는다. */
const swatch = (grid: string, color: string): HTMLElement =>
  within(screen.getByTestId(grid)).getByLabelText(color);

/** 스포이트 가짜. `delete` 로 원복하지 않으면 다음 시험이 오염된다. */
function stubEyeDropper(open: () => Promise<{ sRGBHex: string }>): { calls: number } {
  const state = { calls: 0 };
  class Fake {
    open(): Promise<{ sRGBHex: string }> {
      state.calls += 1;
      return open();
    }
  }
  (window as unknown as { EyeDropper?: unknown }).EyeDropper = Fake;
  return state;
}

beforeEach(() => {
  localStorage.clear();
  resetRecentColorsForTest();
});

afterEach(() => {
  delete (window as unknown as { EyeDropper?: unknown }).EyeDropper;
  vi.restoreAllMocks();
});

describe('고르개 — 여닫기', () => {
  it('닫혀 있을 때는 팝오버가 문서에 없다', () => {
    renderPicker({ initial: BLUE });

    expect(screen.queryByTestId('colorpicker-popover')).not.toBeInTheDocument();
    expect(trigger()).toHaveAttribute('aria-expanded', 'false');
    expect(trigger()).toHaveAttribute('aria-haspopup', 'dialog');
  });

  it('열면 대화상자 역할과 이름을 갖는다', () => {
    renderPicker({ initial: BLUE });

    open();

    expect(popover()).toHaveAttribute('role', 'dialog');
    expect(popover()).toHaveAttribute('aria-label', `${LABEL} 색 고르개`);
    expect(trigger()).toHaveAttribute('aria-expanded', 'true');
  });

  it('여는 것만으로는 아무 값도 나가지 않는다', () => {
    const { onChange } = renderPicker({ initial: BLUE });

    open();

    expect(onChange).not.toHaveBeenCalled();
  });

  it('트리거를 다시 누르면 닫힌다', () => {
    renderPicker({ initial: BLUE });

    open();
    fireEvent.click(trigger());

    expect(screen.queryByTestId('colorpicker-popover')).not.toBeInTheDocument();
  });
});

describe('고르개 — 키보드만으로 임의의 색에 닿는다 (AC-07)', () => {
  it('트리거가 탭 정지이고, 열면 포커스가 첫 조작 요소로 간다', () => {
    renderPicker({ initial: BLUE });

    expect(trigger().tabIndex).toBeGreaterThanOrEqual(0);
    open();

    expect(document.activeElement).toBe(hex());
  });

  it('탭 정지 차례가 16진 → 색상 → 불투명도 → 스포이트 → 프리셋 → 최근색 → 비우기 다', () => {
    localStorage.setItem(RECENT_COLORS_STORAGE_KEY, JSON.stringify([RED]));
    stubEyeDropper(() => Promise.resolve({ sRGBHex: RED }));
    renderPicker({ initial: BLUE, alpha: true, clearable: true });

    open();

    const stops = Array.from(popover().querySelectorAll<HTMLElement>('*'))
      .filter((el) => el.tabIndex >= 0)
      .map(
        (el) =>
          el.dataset['testid'] ??
          `option:${el.closest('[role="listbox"]')?.getAttribute('data-testid')}`,
      );
    expect(stops).toEqual([
      'colorpicker-hex',
      'colorpicker-hue',
      'colorpicker-alpha',
      'colorpicker-eyedropper',
      'option:colorpicker-presets',
      'option:colorpicker-recent',
      'colorpicker-clear',
    ]);
  });

  it('2D 판은 탭 정지가 아니고 `aria-hidden` 이다', () => {
    renderPicker({ initial: BLUE });

    open();

    const field = screen.getByTestId('colorpicker-field');
    expect(field).toHaveAttribute('aria-hidden', 'true');
    expect(field.hasAttribute('tabindex')).toBe(false);
  });

  it('16진 칸으로 임의의 색을 지정한다 — 판 없이 닿는다', () => {
    const { onChange } = renderPicker({ initial: BLUE, alpha: true });

    open();
    fireEvent.change(hex(), { target: { value: 'abcd' } });
    fireEvent.blur(hex());

    expect(onChange).toHaveBeenCalledExactlyOnceWith('#aabbccdd');
    // 떠난 뒤에도 글자가 되돌아가지 않는다 — 이번엔 형식이 맞았으므로.
    expect(hex().value).toBe('#aabbccdd');
  });

  it('프리셋 격자에서 방향키로 옮기고 Enter 로 고른다', () => {
    const { onChange } = renderPicker({ initial: '#ffffff' });

    open();
    const grid = screen.getByTestId('colorpicker-presets');
    expect(screen.getByLabelText('#ffffff')).toHaveAttribute('aria-selected', 'true');

    fireEvent.keyDown(grid, { key: 'ArrowRight' });
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    fireEvent.keyDown(grid, { key: 'Enter' });

    // 1행 2번째(`#f1f5f9`)에서 아래로 → 2행 2번째(`#f97316`).
    expect(onChange).toHaveBeenCalledExactlyOnceWith('#f97316');
    expect(screen.queryByTestId('colorpicker-popover')).not.toBeInTheDocument();
  });

  it('Esc 로 닫으면 포커스가 연 단추로 돌아온다', () => {
    renderPicker({ initial: BLUE });

    open();
    expect(document.activeElement).toBe(hex());
    fireEvent.keyDown(document, { key: 'Escape' });

    expect(screen.queryByTestId('colorpicker-popover')).not.toBeInTheDocument();
    expect(document.activeElement).toBe(trigger());
  });
});

describe('고르개 — 알파를 켠 자리 (AC-03 · AC-04)', () => {
  it('불투명도 50% 가 8자리 문자열 그대로 나간다', () => {
    const { onChange } = renderPicker({ initial: BLUE, alpha: true });

    open();
    fireEvent.change(alphaSlider(), { target: { value: '50' } });

    expect(onChange).toHaveBeenCalledExactlyOnceWith(BLUE_A50);
  });

  it('8자리가 스와치에 실제로 먹는다 — 선언이 버려지지 않는다', () => {
    renderPicker({ initial: BLUE, alpha: true });

    open();
    fireEvent.change(alphaSlider(), { target: { value: '50' } });

    // 무효한 색이면 빈 문자열이 된다. 비어 있지 않다는 것이 곧 유효하다는 뜻이다.
    expect(preview().style.backgroundColor).not.toBe('');
    expect(preview().style.backgroundColor).toBe('rgba(59, 130, 246, 0.5)');
  });

  it('불투명도 100% 는 6자리를 낸다 — `#3b82f6ff` 가 아니다', () => {
    const { onChange } = renderPicker({ initial: BLUE, alpha: true });

    open();
    fireEvent.change(alphaSlider(), { target: { value: '50' } });
    fireEvent.change(alphaSlider(), { target: { value: '100' } });

    expect(onChange).toHaveBeenLastCalledWith(BLUE);
    // 이것이 완전 불투명의 표현을 하나로 두는 이유다 — 프리셋 선택 표시가 문자열 비교다.
    expect(swatch('colorpicker-presets', BLUE)).toHaveAttribute('aria-selected', 'true');
  });

  it('색상 슬라이더가 색상만 바꾼다 — 알파는 그대로다', () => {
    const { onChange } = renderPicker({ initial: BLUE_A50, alpha: true });

    open();
    fireEvent.change(hue(), { target: { value: '0' } });

    expect(onChange).toHaveBeenCalledExactlyOnceWith('#f63b3b80');
  });
});

describe('고르개 — 알파를 끈 자리의 다섯 누출 경로 (AC-E3)', () => {
  it('1. 텍스트: 8자리를 치면 나가지 않고, 떠나면 글자가 되돌아온다', () => {
    const { onChange } = renderPicker({ initial: BLUE });

    open();
    fireEvent.change(hex(), { target: { value: '#aabbccdd' } });
    expect(onChange).not.toHaveBeenCalled();

    fireEvent.blur(hex());
    expect(hex().value).toBe(BLUE);
  });

  it('0. 이미 8자리가 저장돼 있어도 나가는 값은 6자리다', () => {
    // B급 필드에 예전 값이나 손으로 고친 값으로 8자리가 들어 있을 수 있다. 그 값을
    // 읽어 HSV 로 들면 알파가 0.5 인 채로 시작하므로, 내보낼 때 접지 않으면 알파를 켠
    // 적 없는 자리에서 8자리가 흘러나간다(불변식 I2).
    const { onChange } = renderPicker({ initial: BLUE_A50 });

    open();
    fireEvent.change(hue(), { target: { value: '0' } });

    expect(onChange).toHaveBeenCalledExactlyOnceWith('#f63b3b');
  });

  it('2. 슬라이더: 불투명도 슬라이더가 문서에 없다', () => {
    renderPicker({ initial: BLUE });

    open();

    expect(screen.queryByTestId('colorpicker-alpha')).not.toBeInTheDocument();
  });

  it('3. 프리셋: 격자에 그려진 색이 전부 6자리다', () => {
    renderPicker({ initial: BLUE });

    open();

    const labels = screen
      .getAllByRole('option')
      .map((el) => el.getAttribute('aria-label') ?? '');
    expect(labels).toHaveLength(UNIFIED_PALETTE.length);
    expect(labels.filter((l) => !/^#[0-9a-f]{6}$/.test(l))).toEqual([]);
  });

  it('4. 최근색: 8자리 항목이 격자에 없다 — 알파를 켜면 있다', () => {
    localStorage.setItem(RECENT_COLORS_STORAGE_KEY, JSON.stringify([BLUE_A50, RED]));
    const { unmount } = renderPicker({ initial: BLUE });

    open();
    expect(screen.getByTestId('colorpicker-recent')).toBeInTheDocument();
    expect(screen.queryByLabelText(BLUE_A50)).not.toBeInTheDocument();
    unmount();

    renderPicker({ initial: BLUE, alpha: true });
    open();
    expect(screen.getByLabelText(BLUE_A50)).toBeInTheDocument();
  });

  it('5. 스포이트: 6자리로 나간다', async () => {
    stubEyeDropper(() => Promise.resolve({ sRGBHex: RED }));
    const { onChange } = renderPicker({ initial: BLUE });

    open();
    fireEvent.click(screen.getByTestId('colorpicker-eyedropper'));

    await waitFor(() => expect(onChange).toHaveBeenCalledExactlyOnceWith(RED));
  });
});

describe('고르개 — 상속 색과 비우기 (AC-06)', () => {
  it('값이 없으면 상속 색을 미리보기로 쓰되 저장하지 않는다', () => {
    const { onChange } = renderPicker({ inheritedColor: BLUE });

    open();

    expect(preview().style.backgroundColor).toBe('rgb(59, 130, 246)');
    // 판·슬라이더도 상속 색에서 시작한다.
    expect(hue().value).toBe('217');
    expect(onChange).not.toHaveBeenCalled();
  });

  it('제 값이 있으면 상속 색을 쓰지 않는다', () => {
    renderPicker({ initial: BLUE, inheritedColor: RED });

    open();

    expect(preview().style.backgroundColor).toBe('rgb(59, 130, 246)');
    expect(hue().value).toBe('217');
  });

  it('상속 색도 hex 가 아니면 중립 회색에서 시작한다', () => {
    renderPicker({ inheritedColor: 'var(--color-text-muted)' });

    open();

    expect(hue().value).toBe('218');
  });

  it('`clearable` 이면 ✕ 가 `onChange(undefined)` 를 낸다', () => {
    const { onChange } = renderPicker({ initial: BLUE, clearable: true });

    open();
    fireEvent.click(screen.getByTestId('colorpicker-clear'));

    expect(onChange).toHaveBeenCalledExactlyOnceWith(undefined);
    expect(screen.queryByTestId('colorpicker-popover')).not.toBeInTheDocument();
  });

  it('`clearable` 이 꺼지면 ✕ 가 없고 `undefined` 가 나갈 길도 없다', () => {
    const { onChange } = renderPicker({ initial: BLUE });

    open();
    expect(screen.queryByTestId('colorpicker-clear')).not.toBeInTheDocument();

    fireEvent.change(hex(), { target: { value: '#ffffff' } });
    fireEvent.change(hue(), { target: { value: '10' } });
    fireEvent.click(swatch('colorpicker-presets', RED));

    expect(onChange.mock.calls.filter(([c]) => c === undefined)).toEqual([]);
  });

  it('hex 가 아닌 저장값은 그대로 보이고, 열었다는 이유로 덮이지 않는다', () => {
    const { onChange } = renderFixed('var(--color-text-muted)');

    open();

    expect(preview().style.backgroundColor).toBe('var(--color-text-muted)');
    // 2D 판·슬라이더는 중립 회색(`#9ca3af`)에서 시작한다.
    expect(hue().value).toBe('218');

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onChange).not.toHaveBeenCalled();
  });
});

describe('고르개 — 스포이트 (AC-08)', () => {
  it('`EyeDropper` 가 없으면 단추를 그리지 않는다 — 비활성 단추도 없다', () => {
    renderPicker({ initial: BLUE });

    open();

    expect(screen.queryByTestId('colorpicker-eyedropper')).not.toBeInTheDocument();
  });

  it('있으면 단추가 있고, 누르면 실제로 연다', async () => {
    const state = stubEyeDropper(() => Promise.resolve({ sRGBHex: RED }));
    const { onChange } = renderPicker({ initial: BLUE });

    open();
    fireEvent.click(screen.getByTestId('colorpicker-eyedropper'));

    await waitFor(() => expect(onChange).toHaveBeenCalled());
    expect(state.calls).toBe(1);
  });

  it('집은 색이 RGB 만 갈아 끼우고 현재 알파를 유지한다', async () => {
    stubEyeDropper(() => Promise.resolve({ sRGBHex: RED }));
    const { onChange } = renderPicker({ initial: BLUE_A50, alpha: true });

    open();
    fireEvent.click(screen.getByTestId('colorpicker-eyedropper'));

    await waitFor(() => expect(onChange).toHaveBeenCalledExactlyOnceWith('#ef444480'));
  });

  it('취소는 오류가 아니다 — 값도 안 바뀌고 오류도 안 뜬다', async () => {
    stubEyeDropper(() => Promise.reject(new Error('AbortError')));
    const { onChange } = renderPicker({ initial: BLUE });

    open();
    fireEvent.click(screen.getByTestId('colorpicker-eyedropper'));
    await Promise.resolve();

    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByTestId('colorpicker-popover')).toBeInTheDocument();
  });

  it('hex 가 아닌 결과는 무시한다 — 우리가 만드는 값이 아니다', async () => {
    stubEyeDropper(() => Promise.resolve({ sRGBHex: 'rgb(1,2,3)' }));
    const { onChange } = renderPicker({ initial: BLUE });

    open();
    fireEvent.click(screen.getByTestId('colorpicker-eyedropper'));
    await Promise.resolve();

    expect(onChange).not.toHaveBeenCalled();
  });
});

describe('고르개 — 최근 사용 색 (AC-09)', () => {
  it('중복을 만들지 않고 맨 앞으로 올린다', () => {
    renderPicker({ initial: undefined });

    for (const c of [BLUE, RED, BLUE]) {
      open();
      fireEvent.click(swatch('colorpicker-presets', c));
    }

    expect(readRecentColors()).toEqual([BLUE, RED]);
  });

  it('열세 색을 고르면 열두 칸으로 잘린다', () => {
    renderPicker({ initial: undefined });

    const picked = UNIFIED_PALETTE.slice(0, 13);
    for (const c of picked) {
      open();
      fireEvent.click(swatch('colorpicker-presets', c));
    }

    const list = readRecentColors();
    expect(list).toHaveLength(12);
    expect(list).not.toContain(picked[0]);
  });

  it('비우기는 목록을 바꾸지 않는다 — 비움은 색이 아니다', () => {
    renderPicker({ initial: undefined, clearable: true });

    open();
    fireEvent.click(swatch('colorpicker-presets', BLUE));
    open();
    fireEvent.click(screen.getByTestId('colorpicker-clear'));

    expect(readRecentColors()).toEqual([BLUE]);
  });

  it('끄는 도중의 중간 색이 목록을 채우지 않는다', () => {
    renderPicker({ initial: BLUE });

    open();
    vi.spyOn(screen.getByTestId('colorpicker-field'), 'getBoundingClientRect').mockReturnValue(
      FIELD_RECT,
    );
    fireEvent.mouseDown(screen.getByTestId('colorpicker-field'), { clientX: 100, clientY: 50 });
    fireEvent.mouseMove(window, { clientX: 150, clientY: 80 });
    fireEvent.mouseMove(window, { clientX: 100, clientY: 150 });
    fireEvent.mouseUp(window);
    fireEvent.keyDown(document, { key: 'Escape' });

    // 한 번 끄는 동안 색은 여러 번 나갔지만, 목록에 남는 것은 **마지막 하나**다.
    expect(readRecentColors()).toEqual(['#000000']);
  });

  it('`localStorage` 가 던져도 같은 세션 안에서는 계속 동작한다', () => {
    renderPicker({ initial: undefined });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota');
    });

    open();
    fireEvent.click(swatch('colorpicker-presets', BLUE));
    open();

    // 저장소가 막혀도 메모리 목록으로 떨어져 같은 세션 안에서는 그대로 보인다.
    expect(swatch('colorpicker-recent', BLUE)).toBeInTheDocument();
  });

  it('세션을 넘긴다 — 언마운트하고 다시 마운트해도 남는다', () => {
    const first = renderPicker({ initial: undefined });
    open();
    fireEvent.click(swatch('colorpicker-presets', RED));
    first.unmount();

    renderPicker({ initial: undefined });
    open();

    expect(screen.getByTestId('colorpicker-recent')).toBeInTheDocument();
    expect(
      screen
        .getByTestId('colorpicker-recent')
        .querySelectorAll<HTMLElement>('[role="option"]')[0]
        ?.getAttribute('aria-label'),
    ).toBe(RED);
  });

  it('고르개끼리 목록을 나눠 쓰고, 그냥 열었다 닫는 것은 순서를 바꾸지 않는다', () => {
    // 최근색은 **브라우저 전역 한 벌**이다. 자리별로 나누면 "방금 쓴 색" 이 옆 칸에서
    // 안 보여 기능의 목적이 사라진다.
    localStorage.setItem('xflow-locale', 'ko');
    render(
      <I18nProvider>
        <ColorPicker value={undefined} ariaLabel="테두리" onChange={vi.fn()} />
        <ColorPicker value={undefined} ariaLabel="글자" onChange={vi.fn()} />
      </I18nProvider>,
    );
    const border = screen.getByRole('button', { name: '테두리' });
    const text = screen.getByRole('button', { name: '글자' });

    fireEvent.click(border);
    fireEvent.click(swatch('colorpicker-presets', BLUE));
    fireEvent.click(text);
    expect(swatch('colorpicker-recent', BLUE)).toBeInTheDocument();
    fireEvent.click(swatch('colorpicker-presets', RED));
    expect(readRecentColors()).toEqual([RED, BLUE]);

    // 아무것도 고르지 않고 열었다 닫으면 목록이 그대로다 — 연 것은 쓴 것이 아니다.
    fireEvent.click(border);
    fireEvent.keyDown(document, { key: 'Escape' });

    expect(readRecentColors()).toEqual([RED, BLUE]);
  });

  it('최근색이 없으면 격자를 아예 그리지 않는다', () => {
    renderPicker({ initial: BLUE });

    open();

    expect(screen.queryByTestId('colorpicker-recent')).not.toBeInTheDocument();
  });
});

describe('고르개 — 속성으로 갈라지는 두 자리', () => {
  it('`presets` 를 주면 그 목록만 그리고, 열 칸씩 끊어 담는다', () => {
    // 통합 팔레트에 없는 색으로만 짠다 — 목록이 진짜로 대체되는지 보려면 겹치면 안 된다.
    const custom = Array.from({ length: 13 }, (_, i) => `#0${i.toString(16)}0${i.toString(16)}0${i.toString(16)}`);
    renderPicker({ initial: BLUE, presets: custom });

    open();

    const grid = screen.getByTestId('colorpicker-presets');
    expect(within(grid).getAllByRole('option')).toHaveLength(13);
    // 통합 팔레트에만 있는 색은 그려지지 않는다 — 목록이 진짜로 대체된다.
    expect(within(grid).queryByLabelText(RED)).not.toBeInTheDocument();
    // 열 칸이 차면 다음 행으로 넘어간다.
    expect(grid.querySelectorAll('[role="presentation"]')).toHaveLength(2);
  });

  it('`testId` 를 주면 트리거를 그것으로 집을 수 있다', () => {
    renderPicker({ initial: BLUE, testId: 'row-3-color' });

    expect(screen.getByTestId('row-3-color')).toBe(trigger());
  });

  it('`testId` 가 없으면 트리거에 그 속성이 붙지 않는다', () => {
    renderPicker({ initial: BLUE });

    expect(trigger().hasAttribute('data-testid')).toBe(false);
  });
});

describe('고르개 — 어긋난 자리에 떠 있느니 닫는다 (AC-E11)', () => {
  it('스크롤로 닫는다', () => {
    renderPicker({ initial: BLUE });

    open();
    fireEvent.scroll(document);

    expect(screen.queryByTestId('colorpicker-popover')).not.toBeInTheDocument();
  });

  it('리사이즈로 닫는다', () => {
    renderPicker({ initial: BLUE });

    open();
    fireEvent.resize(window);

    expect(screen.queryByTestId('colorpicker-popover')).not.toBeInTheDocument();
  });

  it('바깥을 누르면 닫히고, 포커스를 빼앗지 않는다', () => {
    renderPicker({ initial: BLUE });
    const outside = document.createElement('button');
    document.body.appendChild(outside);

    open();
    // `focus()` 는 16진 칸에 blur 를 일으키고(= React 상태 갱신), 바깥 mousedown 은
    // 문서에 직접 단 리스너를 거쳐 상태를 바꾼다. 둘 다 React 바깥에서 나므로 감싼다.
    act(() => {
      outside.focus();
      fireEvent.mouseDown(outside);
    });

    expect(screen.queryByTestId('colorpicker-popover')).not.toBeInTheDocument();
    expect(document.activeElement).toBe(outside);
    outside.remove();
  });

  it('닫으면 문서·창에 단 리스너가 전부 떨어진다', () => {
    const addDoc = vi.spyOn(document, 'addEventListener');
    const removeDoc = vi.spyOn(document, 'removeEventListener');
    const addWin = vi.spyOn(window, 'addEventListener');
    const removeWin = vi.spyOn(window, 'removeEventListener');
    renderPicker({ initial: BLUE });

    open();
    fireEvent.keyDown(document, { key: 'Escape' });

    const handlers = (spy: typeof addDoc, type: string) =>
      spy.mock.calls.filter((c) => c[0] === type).map((c) => c[1]);
    for (const type of ['mousedown', 'keydown', 'scroll']) {
      expect(handlers(removeDoc, type)).toEqual(handlers(addDoc, type));
    }
    expect(handlers(removeWin, 'resize')).toEqual(handlers(addWin, 'resize'));
  });

  it('열린 채 언마운트해도 리스너가 남지 않는다', () => {
    const addDoc = vi.spyOn(document, 'addEventListener');
    const removeDoc = vi.spyOn(document, 'removeEventListener');
    const { unmount } = renderPicker({ initial: BLUE });

    open();
    unmount();

    const added = addDoc.mock.calls.filter((c) => c[0] === 'keydown').map((c) => c[1]);
    const removed = removeDoc.mock.calls.filter((c) => c[0] === 'keydown').map((c) => c[1]);
    expect(removed).toEqual(added);
  });
});

describe('고르개 — 판에서 온 좌표가 저장되는 색이 된다 (이음매)', () => {
  it('좌상단은 흰색, 좌하단은 검정이 된다', () => {
    const { onChange } = renderPicker({ initial: BLUE });

    open();
    const field = screen.getByTestId('colorpicker-field');
    vi.spyOn(field, 'getBoundingClientRect').mockReturnValue(FIELD_RECT);

    // 채도 0 · 명도 1 → 색상과 무관하게 흰색. 판과 산술을 잇는 자리를 색 이름으로 잰다.
    fireEvent.mouseDown(field, { clientX: 100, clientY: 50 });
    expect(onChange).toHaveBeenLastCalledWith('#ffffff');

    // 명도 0 → 검정.
    fireEvent.mouseDown(field, { clientX: 100, clientY: 150 });
    expect(onChange).toHaveBeenLastCalledWith('#000000');
  });

  it('채도 0 에서도 색상이 소실되지 않는다 — 슬라이더가 튀지 않는다', () => {
    renderPicker({ initial: BLUE });

    open();
    const field = screen.getByTestId('colorpicker-field');
    vi.spyOn(field, 'getBoundingClientRect').mockReturnValue(FIELD_RECT);
    const before = hue().value;

    fireEvent.mouseDown(field, { clientX: 100, clientY: 50 }); // 채도 0

    // hex 를 상태로 들면 여기서 색상이 0 으로 떨어진다. HSV 를 상태로 드는 이유다.
    expect(hue().value).toBe(before);
  });
});

describe('고르개 — i18n 두 로케일 (AC-E8)', () => {
  const KEYS = [
    'ariaDialog',
    'custom',
    'hex',
    'hue',
    'opacity',
    'eyedropper',
    'presets',
    'recent',
    'clear',
  ];

  it('아홉 키가 ko · en 양쪽에 있다', () => {
    // 한쪽에만 있으면 타입 검사가 먼저 잡는다(JSON 이 그대로 타입이다). 여기서는
    // **키 집합이 정확히 아홉**인 것까지 본다 — 오타로 하나 더 생긴 키는 아무도 안 쓴다.
    expect(Object.keys(ko.colorPicker).sort()).toEqual([...KEYS].sort());
    expect(Object.keys(en.colorPicker).sort()).toEqual([...KEYS].sort());
  });

  it('en 으로 강제한 렌더에 치환되지 않은 키 문자열이 없다', () => {
    localStorage.setItem(RECENT_COLORS_STORAGE_KEY, JSON.stringify([RED]));
    stubEyeDropper(() => Promise.resolve({ sRGBHex: RED }));
    renderPicker({ initial: BLUE, alpha: true, clearable: true }, 'en');

    open();

    // 기본 로케일이 `ko` 라 이 확인이 없으면 en 누락이 초록으로 통과한다.
    const names = [
      popover().getAttribute('aria-label') ?? '',
      ...Array.from(popover().querySelectorAll<HTMLElement>('[aria-label]')).map(
        (el) => el.getAttribute('aria-label') ?? '',
      ),
    ];
    expect(names.filter((n) => n.includes('colorPicker.'))).toEqual([]);
    expect(names).toContain(`${LABEL} color picker`);
    expect(names).toContain('Opacity');
  });
});
