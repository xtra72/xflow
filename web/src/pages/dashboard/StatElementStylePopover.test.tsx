// 통계 패널 요소의 글자 스타일 팝오버.
//
// @spec SPEC-CHART-004 AC-17 / AC-18 / AC-19 / AC-21 / AC-22 / AC-23

import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { StatElementStylePopover, type StatStyleValue } from './StatElementStylePopover';
import type { StatElementKind } from './panels/charts/statLayout';

const EMPTY: StatStyleValue = {
  fontFamily: undefined,
  fontSize: undefined,
  fontColor: undefined,
  fontWeight: undefined,
};

/** 앵커 요소를 화면의 특정 자리에 심는다. jsdom 은 레이아웃을 하지 않는다. */
function makeAnchor(rect: Partial<DOMRect> = {}): HTMLElement {
  const el = document.createElement('div');
  el.setAttribute('data-testid', 'anchor');
  document.body.appendChild(el);
  const box = { top: 10, left: 20, right: 120, bottom: 40, width: 100, height: 30, x: 20, y: 10, ...rect };
  vi.spyOn(el, 'getBoundingClientRect').mockReturnValue({
    ...box,
    toJSON: () => ({}),
  } as DOMRect);
  return el;
}

function open(
  kind: StatElementKind,
  over: {
    value?: Partial<StatStyleValue>;
    anchorRect?: Partial<DOMRect>;
    onChange?: ReturnType<typeof vi.fn>;
    onDeltaColorChange?: ReturnType<typeof vi.fn>;
    onClose?: ReturnType<typeof vi.fn>;
  } = {},
) {
  const anchor = makeAnchor(over.anchorRect);
  const onChange = over.onChange ?? vi.fn();
  const onDeltaColorChange = over.onDeltaColorChange ?? vi.fn();
  const onClose = over.onClose ?? vi.fn();
  const view = render(
    <StatElementStylePopover
      kind={kind}
      anchor={anchor}
      value={{ ...EMPTY, ...over.value }}
      onChange={onChange}
      onDeltaColorChange={onDeltaColorChange}
      onClose={onClose}
    />,
  );
  return { anchor, onChange, onDeltaColorChange, onClose, view };
}

beforeEach(() => {
  document.body.innerHTML = '';
  window.innerWidth = 1024;
});

describe('필드 편집 (AC-17)', () => {
  it('글꼴을 바꾸면 font_family 로 저장한다', () => {
    const { onChange } = open('value');
    fireEvent.change(screen.getByTestId('stat-style-value-family'), {
      target: { value: 'serif' },
    });
    expect(onChange).toHaveBeenCalledWith({ font_family: 'serif' });
  });

  it('색을 바꾸면 font_color 로 저장한다', () => {
    const { onChange } = open('value');
    fireEvent.change(screen.getByTestId('stat-style-value-color'), {
      target: { value: '#123456' },
    });
    expect(onChange).toHaveBeenCalledWith({ font_color: '#123456' });
  });

  it('굵기를 바꾸면 font_weight 로 저장한다', () => {
    const { onChange } = open('value');
    fireEvent.change(screen.getByTestId('stat-style-value-weight'), {
      target: { value: 'bold' },
    });
    expect(onChange).toHaveBeenCalledWith({ font_weight: 'bold' });
  });
});

describe('크기 칸은 핸들과 같은 필드를 쓴다 (AC-18)', () => {
  it('저장된 font_size 를 표시한다', () => {
    open('value', { value: { fontSize: 50 } });
    expect(screen.getByTestId('stat-style-value-size')).toHaveValue(50);
  });

  it('크기를 입력하면 font_size 로 저장한다', () => {
    const { onChange } = open('value', { value: { fontSize: 50 } });
    fireEvent.change(screen.getByTestId('stat-style-value-size'), { target: { value: '80' } });
    expect(onChange).toHaveBeenCalledWith({ font_size: 80 });
  });

  it('통계 본값에 필요한 큰 크기를 받는다 — 상한이 40 이 아니다', () => {
    open('value');
    expect(screen.getByTestId('stat-style-value-size')).toHaveAttribute('max', '160');
  });
});

describe('변화량은 방향별 3색을 편집한다 (AC-19 / D4)', () => {
  it('단일 글자색 입력이 없고 3색 입력이 있다', () => {
    open('delta');
    expect(screen.queryByTestId('stat-style-delta-color')).toBeNull();
    expect(screen.getByTestId('stat-style-delta-colors')).toBeInTheDocument();
    for (const slot of ['up', 'down', 'flat']) {
      expect(screen.getByTestId(`stat-style-delta-${slot}`)).toBeInTheDocument();
    }
  });

  it('증가 색을 바꾸면 delta_display 쪽으로 간다', () => {
    const { onChange, onDeltaColorChange } = open('delta');
    fireEvent.change(screen.getByTestId('stat-style-delta-up'), {
      target: { value: '#123456' },
    });
    expect(onDeltaColorChange).toHaveBeenCalledWith({ up_color: '#123456' });
    // layout 쪽으로는 가지 않는다 — 축이 하나여야 한다.
    expect(onChange).not.toHaveBeenCalled();
  });

  it('변화 없음 기본색은 CSS 변수라 색 입력에 중립 회색을 넣는다', () => {
    open('delta');
    // var(--color-text-muted) 를 그대로 주면 브라우저가 검정으로 떨어뜨린다.
    expect(screen.getByTestId('stat-style-delta-flat')).toHaveValue('#9ca3af');
  });

  it('본값·구간 통계에는 3색이 없다', () => {
    const { view } = open('value');
    expect(screen.queryByTestId('stat-style-delta-colors')).toBeNull();
    view.unmount();
    document.body.innerHTML = '';

    open('stats');
    expect(screen.queryByTestId('stat-style-delta-colors')).toBeNull();
  });
});

describe('자리잡기 (AC-21)', () => {
  it('앵커 아래에 붙는다', () => {
    open('value', { anchorRect: { left: 20, bottom: 40 } });
    const box = screen.getByTestId('stat-style-popover');
    expect(box.style.top).toBe('44px');
    expect(box.style.left).toBe('20px');
  });

  it('오른쪽 경계를 넘지 않는 자리로 옮긴다', () => {
    window.innerWidth = 400;
    open('value', { anchorRect: { left: 380, bottom: 40 } });
    const box = screen.getByTestId('stat-style-popover');
    // 폭 288 + 여백 8 → 왼쪽은 400 - 288 - 8 = 104 가 최대다.
    expect(parseInt(box.style.left, 10)).toBeLessThanOrEqual(104);
  });

  it('화면이 팝오버보다 좁으면 폭을 줄인다', () => {
    window.innerWidth = 200;
    open('value');
    expect(screen.getByTestId('stat-style-popover').style.width).toBe('184px');
  });
});

describe('닫기 (AC-22 / AC-23)', () => {
  it('바깥을 클릭하면 닫힌다', () => {
    const { onClose } = open('value');
    fireEvent.mouseDown(document.body);
    expect(onClose).toHaveBeenCalled();
  });

  it('팝오버 안을 클릭해도 닫히지 않는다', () => {
    const { onClose } = open('value');
    fireEvent.mouseDown(screen.getByTestId('stat-style-popover'));
    expect(onClose).not.toHaveBeenCalled();
  });

  it('앵커를 클릭해도 닫히지 않는다 — 더블클릭의 두 번째 눌림이 곧바로 닫으면 안 된다', () => {
    const { onClose, anchor } = open('value');
    fireEvent.mouseDown(anchor);
    expect(onClose).not.toHaveBeenCalled();
  });

  it('Esc 로 닫힌다', () => {
    const { onClose } = open('value');
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });
});

describe('접근성', () => {
  it('열리면 첫 입력으로 포커스가 간다 (AC-23)', () => {
    open('value');
    expect(document.activeElement).toBe(screen.getByTestId('stat-style-value-family'));
  });

  it('닫으면 포커스를 원래 요소로 되돌린다', () => {
    // 키보드로 연 자리를 재현한다 — 본값에 포커스가 있는 상태에서 팝오버가 뜬다.
    const anchor = makeAnchor();
    anchor.tabIndex = 0;
    anchor.focus();

    const view = render(
      <StatElementStylePopover
        kind="value"
        anchor={anchor}
        value={EMPTY}
        onChange={vi.fn()}
        onDeltaColorChange={vi.fn()}
        onClose={vi.fn()}
      />,
    );
    expect(document.activeElement).toBe(screen.getByTestId('stat-style-value-family'));

    view.unmount();
    expect(document.activeElement).toBe(anchor);
  });

  it('열었던 요소가 사라졌으면 되돌리지 않는다', () => {
    const { anchor, view } = open('value');
    anchor.remove();
    expect(() => view.unmount()).not.toThrow();
  });

  it('대화상자로 읽히고 요소 이름을 갖는다', () => {
    open('stats');
    const box = screen.getByTestId('stat-style-popover');
    expect(box).toHaveAttribute('role', 'dialog');
    expect(box).toHaveAttribute('aria-label', 'dashboard.chart.statElementStats');
  });
});
