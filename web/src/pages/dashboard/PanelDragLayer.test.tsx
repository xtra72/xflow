// 통계 패널의 요소를 끌어 옮기고 모서리 핸들로 크기를 바꾸는 레이어.
//
// 게이지·파이와 같은 규칙이다 — 정해진 표식 위에서만 시작하고, 프레임당 한 번만
// 반영하며, 오프셋은 백분율이다. 다른 점은 크기 조절이 붙는다는 것 하나다.
//
// @spec SPEC-CHART-004 AC-07 / AC-08 / AC-09 / AC-13

import { describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';

import { PanelDragLayer, PanelResizeHandle, type PanelDragTarget } from './PanelDragLayer';
type StatElementKind = 'value' | 'delta' | 'stats';
import type { PanelSelection } from './panels/charts/panelEditSelection';
import { PANEL_OFFSET_LIMIT } from './panels/charts/panelGeometry';
import { STAT_OFFSET_LIMIT } from './panels/charts/statLayout';

// 이 파일의 기본 대상은 **통계 패널의 글자 덩어리**다 — 상한을 밝히지 않으면 레이어가
// 그림 기준(±40)으로 보므로, 통계를 흉내 내는 자리에서는 ±50 을 명시한다.
function target(over: Partial<PanelDragTarget> = {}): PanelDragTarget {
  return {
    offsetX: 0,
    offsetY: 0,
    limit: STAT_OFFSET_LIMIT,
    fontSize: 36,
    onMove: vi.fn(),
    onResize: vi.fn(),
    ...over,
  };
}

function Fixture({
  enabled = true,
  // 이동·크기 계산을 재는 테스트에서는 격자를 끈다 — 켜 두면 스냅이 값을 한 번 더
  // 만져서 무엇을 재고 있는지 흐려진다(격자는 아래 전용 describe 가 검증한다).
  snap = false,
  // 기본은 빈 선택 — 누르면 그 요소가 골라지고 혼자 움직인다(종전 동작과 같다).
  selection = new Set<StatElementKind>(),
  onSelectionChange = () => {},
  value,
  delta,
  stats,
}: {
  enabled?: boolean;
  snap?: boolean;
  selection?: PanelSelection;
  onSelectionChange?: (next: PanelSelection) => void;
  value: PanelDragTarget;
  delta: PanelDragTarget;
  stats: PanelDragTarget;
}) {
  return (
    <PanelDragLayer
      enabled={enabled}
      snap={snap}
      selection={selection}
      onSelectionChange={onSelectionChange}
      targets={{ value, delta, stats }}
    >
      {/* 기준 상자 — 백분율 환산의 분모다. */}
      <div data-panel-bounds="" data-testid="bounds">
        <div data-panel-drag="value" data-testid="value">
          1,234
          <PanelResizeHandle kind="value" enabled={enabled} label="본값 크기" />
        </div>
        <div data-panel-drag="delta" data-testid="delta">
          ↑ +12
          <PanelResizeHandle kind="delta" enabled={enabled} label="변화량 크기" />
        </div>
        <div data-panel-drag="stats" data-testid="stats">
          평 980
          <PanelResizeHandle kind="stats" enabled={enabled} label="구간 통계 크기" />
        </div>
        <div data-testid="empty">여백</div>
      </div>
    </PanelDragLayer>
  );
}

/** jsdom 은 레이아웃을 하지 않으므로 기준 상자 크기를 직접 심는다. */
function stubBounds(width: number, height: number): void {
  vi.spyOn(screen.getByTestId('bounds'), 'getBoundingClientRect').mockReturnValue({
    width,
    height,
    top: 0,
    left: 0,
    right: width,
    bottom: height,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect);
}

/** 요소 상자를 화면에 심는다 — 스냅은 요소의 중심을 봐야 한다. */
function stubEl(testId: string, left: number, top: number, w: number, h: number): void {
  vi.spyOn(screen.getByTestId(testId), 'getBoundingClientRect').mockReturnValue({
    left,
    top,
    width: w,
    height: h,
    right: left + w,
    bottom: top + h,
    x: left,
    y: top,
    toJSON: () => ({}),
  } as DOMRect);
}

function pointer(type: string, x: number, y: number): PointerEvent {
  return new MouseEvent(type, { clientX: x, clientY: y, bubbles: true }) as unknown as PointerEvent;
}

/** 레이어는 이동을 프레임당 한 번으로 모은다 — 프레임을 기다려야 실제와 같은 시점이다. */
async function nextFrame(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  });
}

describe('이동 — 세 요소가 독립이다 (AC-07)', () => {
  it('본값을 끌면 value 대상만 갱신된다', async () => {
    const value = target();
    const delta = target();
    const stats = target();
    render(<Fixture value={value} delta={delta} stats={stats} />);
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 120, 110));
    await nextFrame();

    // 폭 200 에서 20px = 10%, 높이 100 에서 10px = 10%.
    expect(value.onMove).toHaveBeenLastCalledWith({ x: 10, y: 10 });
    expect(delta.onMove).not.toHaveBeenCalled();
    expect(stats.onMove).not.toHaveBeenCalled();
  });

  it('변화량을 끌면 delta 대상만 갱신된다', async () => {
    const value = target();
    const delta = target();
    const stats = target();
    render(<Fixture value={value} delta={delta} stats={stats} />);
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('delta'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', -20, 0));
    await nextFrame();

    expect(delta.onMove).toHaveBeenLastCalledWith({ x: -10, y: 0 });
    expect(value.onMove).not.toHaveBeenCalled();
  });

  it('구간 통계를 끌면 stats 대상만 갱신된다', async () => {
    const value = target();
    const delta = target();
    const stats = target();
    render(<Fixture value={value} delta={delta} stats={stats} />);
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('stats'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 0, 15));
    await nextFrame();

    expect(stats.onMove).toHaveBeenLastCalledWith({ x: 0, y: 15 });
    expect(delta.onMove).not.toHaveBeenCalled();
  });

  it('이미 놓인 오프셋에 이어서 더한다', async () => {
    const value = target({ offsetX: 5, offsetY: -5 });
    render(<Fixture value={value} delta={target()} stats={target()} />);
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 20, 10));
    await nextFrame();

    expect(value.onMove).toHaveBeenLastCalledWith({ x: 15, y: 5 });
  });
});

describe('이동 — 백분율 환산과 클램프 (AC-08)', () => {
  it('픽셀 이동량을 기준 상자 대비 백분율로 환산한다', async () => {
    const value = target();
    render(<Fixture value={value} delta={target()} stats={target()} />);
    stubBounds(400, 200);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 40, 40));
    await nextFrame();

    // 폭 400 에서 40px = 10%, 높이 200 에서 40px = 20%.
    expect(value.onMove).toHaveBeenLastCalledWith({ x: 10, y: 20 });
  });

  it('±50 으로 죈다 — 요소를 패널 어느 모서리에든 놓을 수 있어야 한다', async () => {
    // 게이지·파이의 ±40 과 다르다. 저 둘은 패널을 채우는 그림이라 40%면 이미 절반이
    // 잘리지만, 통계는 가운데에서 시작하는 작은 글자라 모서리까지 50% 가 필요하다.
    const value = target();
    render(<Fixture value={value} delta={target()} stats={target()} />);
    stubBounds(100, 100);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 90, -90));
    await nextFrame();

    expect(value.onMove).toHaveBeenLastCalledWith({ x: 50, y: -50 });
  });

  it('기준 상자를 잴 수 없으면 시작하지 않는다', async () => {
    const value = target();
    render(<Fixture value={value} delta={target()} stats={target()} />);
    stubBounds(0, 0);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 20, 20));
    await nextFrame();

    // 0 으로 나눈 이동량이 요소를 화면 밖으로 날리지 않게 한다.
    expect(value.onMove).not.toHaveBeenCalled();
  });

  it('한 프레임 안의 여러 이동은 한 번만 반영한다', async () => {
    const value = target();
    render(<Fixture value={value} delta={target()} stats={target()} />);
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 10, 0));
    fireEvent(document, pointer('pointermove', 20, 0));
    fireEvent(document, pointer('pointermove', 40, 0));
    await nextFrame();

    expect(value.onMove).toHaveBeenCalledTimes(1);
    expect(value.onMove).toHaveBeenLastCalledWith({ x: 20, y: 0 });
  });
});

describe('크기 조절 (AC-13)', () => {
  it('크기 핸들을 오른쪽 아래로 끌면 커진다', async () => {
    const value = target({ fontSize: 36 });
    render(<Fixture value={value} delta={target()} stats={target()} />);
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('panel-resize-value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 6, 4));
    await nextFrame();

    // 대각선 투영을 더한다 — 손잡이를 끈 거리만큼 커진다. 36 + (6+4)/2 = 41.
    expect(value.onResize).toHaveBeenLastCalledWith(41);
    // 크기 조절은 위치를 건드리지 않는다.
    expect(value.onMove).not.toHaveBeenCalled();
  });

  it('반대로 끌면 작아진다', async () => {
    const value = target({ fontSize: 36 });
    render(<Fixture value={value} delta={target()} stats={target()} />);
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('panel-resize-value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', -6, -4));
    await nextFrame();

    expect(value.onResize).toHaveBeenLastCalledWith(31);
  });

  it('소수 px 을 그대로 넘긴다 — 정수로 반올림하면 크기가 계단으로 뛴다', async () => {
    const value = target({ fontSize: 36 });
    render(<Fixture value={value} delta={target()} stats={target()} />);
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('panel-resize-value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 1, 0));
    await nextFrame();

    expect(value.onResize).toHaveBeenLastCalledWith(36.5);
  });

  it('6~160 으로 죈다', async () => {
    const value = target({ fontSize: 10 });
    const { unmount } = render(<Fixture value={value} delta={target()} stats={target()} />);
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('panel-resize-value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', -100, -100));
    await nextFrame();
    expect(value.onResize).toHaveBeenLastCalledWith(6);
    unmount();

    const big = target({ fontSize: 150 });
    render(<Fixture value={big} delta={target()} stats={target()} />);
    stubBounds(200, 100);
    fireEvent(screen.getByTestId('panel-resize-value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 100, 100));
    await nextFrame();
    expect(big.onResize).toHaveBeenLastCalledWith(160);
  });

  it('요소마다 자기 핸들이 자기 크기를 바꾼다', async () => {
    const value = target({ fontSize: 36 });
    const delta = target({ fontSize: 14 });
    render(<Fixture value={value} delta={delta} stats={target()} />);
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('panel-resize-delta'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 2, 2));
    await nextFrame();

    expect(delta.onResize).toHaveBeenLastCalledWith(16);
    expect(value.onResize).not.toHaveBeenCalled();
  });
});

describe('시작 조건 (AC-09)', () => {
  it('표식 없는 자리를 잡으면 아무 일도 없다', async () => {
    const value = target();
    const delta = target();
    const stats = target();
    render(<Fixture value={value} delta={delta} stats={stats} />);
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('empty'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 40, 40));
    await nextFrame();

    expect(value.onMove).not.toHaveBeenCalled();
    expect(delta.onMove).not.toHaveBeenCalled();
    expect(stats.onMove).not.toHaveBeenCalled();
  });

  it('편집이 꺼져 있으면 끌리지 않고 핸들도 없다', async () => {
    const value = target();
    render(<Fixture enabled={false} value={value} delta={target()} stats={target()} />);

    expect(screen.queryByTestId('panel-resize-value')).toBeNull();

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 40, 40));
    await nextFrame();

    expect(value.onMove).not.toHaveBeenCalled();
  });

  it('손을 떼면 그 뒤 이동은 무시된다', async () => {
    const value = target();
    render(<Fixture value={value} delta={target()} stats={target()} />);
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 20, 0));
    fireEvent(document, pointer('pointerup', 20, 0));
    await nextFrame();
    const calls = (value.onMove as ReturnType<typeof vi.fn>).mock.calls.length;

    fireEvent(document, pointer('pointermove', 80, 0));
    await nextFrame();
    expect(value.onMove).toHaveBeenCalledTimes(calls);
  });
});

describe('접근성', () => {
  it('크기 핸들은 이름을 갖는다', () => {
    render(<Fixture value={target()} delta={target()} stats={target()} />);
    expect(screen.getByTestId('panel-resize-value')).toHaveAttribute('aria-label', '본값 크기');
  });
});

describe('격자 스냅 (AC-33)', () => {
  it('끄는 자리를 가까운 격자선으로 붙인다', async () => {
    const value = target();
    render(<Fixture snap value={value} delta={target()} stats={target()} />);
    stubBounds(200, 100);
    // 중심이 폭 200 상자의 50%(=100px)에 있는 요소.
    stubEl('value', 75, 40, 50, 20);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    // 6px 이동 = 3%p → 격자(10%) 기준으로는 0 으로 되붙는다.
    fireEvent(document, pointer('pointermove', 6, 0));
    await nextFrame();

    expect(value.onMove).toHaveBeenLastCalledWith({ x: 0, y: 0 });
  });

  it('한 칸을 넘기면 다음 격자선으로 간다', async () => {
    const value = target();
    render(<Fixture snap value={value} delta={target()} stats={target()} />);
    stubBounds(200, 100);
    stubEl('value', 75, 40, 50, 20);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    // 24px = 12%p → 10%p 로 붙는다.
    fireEvent(document, pointer('pointermove', 24, 0));
    await nextFrame();

    expect(value.onMove).toHaveBeenLastCalledWith({ x: 10, y: 0 });
  });

  it('Alt 를 누르고 끌면 격자를 잠시 끈다 — 정밀 조정이 막히면 안 된다', async () => {
    const value = target();
    render(<Fixture snap value={value} delta={target()} stats={target()} />);
    stubBounds(200, 100);
    stubEl('value', 75, 40, 50, 20);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(
      document,
      new MouseEvent('pointermove', {
        clientX: 6,
        clientY: 0,
        altKey: true,
        bubbles: true,
      }) as unknown as PointerEvent,
    );
    await nextFrame();

    expect(value.onMove).toHaveBeenLastCalledWith({ x: 3, y: 0 });
  });

  it('snap=false 면 붙지 않는다', async () => {
    const value = target();
    render(<Fixture snap={false} value={value} delta={target()} stats={target()} />);
    stubBounds(200, 100);
    stubEl('value', 75, 40, 50, 20);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 6, 0));
    await nextFrame();

    expect(value.onMove).toHaveBeenLastCalledWith({ x: 3, y: 0 });
  });
});

describe('선택과 무리 이동 (AC-37 / AC-38)', () => {
  it('요소를 누르면 그것 하나가 골라진다', () => {
    const onSelectionChange = vi.fn();
    render(
      <Fixture
        onSelectionChange={onSelectionChange}
        value={target()}
        delta={target()}
        stats={target()}
      />,
    );
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('delta'), pointer('pointerdown', 0, 0));
    expect([...onSelectionChange.mock.calls[0]![0]]).toEqual(['delta']);
  });

  it('Shift 를 누르고 누르면 선택에 더하고, 그 조작으로는 끌리지 않는다', () => {
    const onSelectionChange = vi.fn();
    const value = target();
    render(
      <Fixture
        selection={new Set(['value'])}
        onSelectionChange={onSelectionChange}
        value={value}
        delta={target()}
        stats={target()}
      />,
    );
    stubBounds(200, 100);

    fireEvent(
      screen.getByTestId('delta'),
      new MouseEvent('pointerdown', { clientX: 0, clientY: 0, shiftKey: true, bubbles: true }),
    );
    expect([...onSelectionChange.mock.calls[0]![0]].sort()).toEqual(['delta', 'value']);

    // 고르기 전용 조작이다 — 이어지는 이동은 무시된다.
    fireEvent(document, pointer('pointermove', 20, 0));
    expect(value.onMove).not.toHaveBeenCalled();
  });

  it('빈 자리를 누르면 선택이 풀린다', () => {
    const onSelectionChange = vi.fn();
    render(
      <Fixture
        selection={new Set(['value'])}
        onSelectionChange={onSelectionChange}
        value={target()}
        delta={target()}
        stats={target()}
      />,
    );

    fireEvent(screen.getByTestId('empty'), pointer('pointerdown', 0, 0));
    expect([...onSelectionChange.mock.calls[0]![0]]).toEqual([]);
  });

  it('AC-38: 고른 요소를 끌면 선택 전체가 같은 만큼 움직인다', async () => {
    const value = target({ offsetX: 0 });
    const delta = target({ offsetX: 10 });
    const stats = target();
    render(
      <Fixture
        selection={new Set(['value', 'delta'])}
        value={value}
        delta={delta}
        stats={stats}
      />,
    );
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 20, 0));
    await nextFrame();

    // 이동량 10%p 를 둘 다 그대로 받는다 — 상대 배치가 유지된다.
    expect(value.onMove).toHaveBeenLastCalledWith({ x: 10, y: 0 });
    expect(delta.onMove).toHaveBeenLastCalledWith({ x: 20, y: 0 });
    // 고르지 않은 것은 그대로다.
    expect(stats.onMove).not.toHaveBeenCalled();
  });

  it('AC-38: 한 요소가 상한에 닿으면 무리 전체가 멈춘다', async () => {
    const value = target({ offsetX: 0 });
    const delta = target({ offsetX: 45 });
    render(
      <Fixture selection={new Set(['value', 'delta'])} value={value} delta={delta} stats={target()} />,
    );
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 60, 0)); // 30%p 를 요청
    await nextFrame();

    // delta 가 45 → 50 까지 5%p 만 갈 수 있으므로 무리 전체가 5%p 에서 멈춘다.
    expect(value.onMove).toHaveBeenLastCalledWith({ x: 5, y: 0 });
    expect(delta.onMove).toHaveBeenLastCalledWith({ x: 50, y: 0 });
  });

  it('상한이 다른 요소를 함께 끌면 먼저 닿는 쪽에서 무리가 멈춘다', async () => {
    // 바 패널은 그림(±40)과 범례(±50)를 함께 고를 수 있다. 그림이 35 에 있으므로
    // 무리는 +5 에서 멈춘다 — 무리에 한 상한을 씌우면 이 자리를 알 수 없다.
    const value = target({ offsetX: 35, limit: PANEL_OFFSET_LIMIT });
    const delta = target({ offsetX: 0, limit: STAT_OFFSET_LIMIT });
    render(
      <Fixture selection={new Set(['value', 'delta'])} value={value} delta={delta} stats={target()} />,
    );
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 60, 0)); // 30%p 를 요청
    await nextFrame();

    expect(value.onMove).toHaveBeenLastCalledWith({ x: 40, y: 0 });
    expect(delta.onMove).toHaveBeenLastCalledWith({ x: 5, y: 0 });
  });

  it('크기 핸들은 무리와 무관하게 자기 요소만 바꾼다', async () => {
    const value = target({ fontSize: 36 });
    const delta = target({ fontSize: 14 });
    render(
      <Fixture selection={new Set(['value', 'delta'])} value={value} delta={delta} stats={target()} />,
    );
    stubBounds(200, 100);

    fireEvent(screen.getByTestId('panel-resize-value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 6, 4));
    await nextFrame();

    expect(value.onResize).toHaveBeenLastCalledWith(41);
    expect(delta.onResize).not.toHaveBeenCalled();
    expect(delta.onMove).not.toHaveBeenCalled();
  });
});

describe('오프셋 상한은 요소마다 다르다', () => {
  // 이 레이어는 성질이 다른 요소를 함께 나른다 — 통계 세 줄과 범례는 가운데에서 시작하는
  // 작은 글자 덩어리라 모서리에 닿으려면 ±50 이 필요하고, 파이 그림과 바 그림 영역은
  // 영역을 가득 채우므로 ±40 만 넘어도 절반이 잘린다. 상한이 하나뿐이면 한쪽이 반드시
  // 읽는 쪽과 어긋나, 놓은 자리가 다음에 열 때 되돌아간다.

  it('상한을 밝히지 않으면 ±40 으로 죈다 — 잊었을 때 안전한 쪽이 기본값이다', async () => {
    const value = target({ limit: undefined });
    render(<Fixture value={value} delta={target()} stats={target()} />);
    stubBounds(100, 100);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 90, -90));
    await nextFrame();

    expect(value.onMove).toHaveBeenLastCalledWith({
      x: PANEL_OFFSET_LIMIT,
      y: -PANEL_OFFSET_LIMIT,
    });
  });

  it('파이 그림은 40 을 넘겨 끌 수 없다', async () => {
    const value = target({ limit: PANEL_OFFSET_LIMIT });
    render(<Fixture value={value} delta={target()} stats={target()} />);
    stubBounds(100, 100);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 90, 90));
    await nextFrame();

    expect(value.onMove).toHaveBeenLastCalledWith({ x: 40, y: 40 });
  });

  it('같은 레이어에서 글자 덩어리는 50 까지 간다', async () => {
    const value = target({ limit: PANEL_OFFSET_LIMIT });
    const delta = target({ limit: STAT_OFFSET_LIMIT });
    render(<Fixture value={value} delta={delta} stats={target()} />);
    stubBounds(100, 100);

    fireEvent(screen.getByTestId('delta'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 90, 90));
    await nextFrame();

    expect(delta.onMove).toHaveBeenLastCalledWith({ x: 50, y: 50 });
    expect(value.onMove).not.toHaveBeenCalled();
  });

  it('격자 스냅도 그 상한을 넘기지 않는다 — 스냅이 죄기를 되돌리지 못한다', async () => {
    // 스냅은 중심을 격자로 끌어당기므로 죄기 뒤에 값을 반 칸까지 도로 밀어낸다.
    // 아래 상자에서 38%p 를 끌면 중심이 86% 라 90% 로 붙어 오프셋이 42%p 가 되는데,
    // 그림(±40)이면 40 에서 멈춰야 한다.
    const value = target({ limit: PANEL_OFFSET_LIMIT });
    render(<Fixture snap value={value} delta={target()} stats={target()} />);
    stubBounds(200, 100);
    stubEl('value', 71, 40, 50, 20);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 76, 0));
    await nextFrame();

    expect(value.onMove).toHaveBeenLastCalledWith({ x: PANEL_OFFSET_LIMIT, y: 0 });
  });

  it('같은 자리라도 글자 덩어리는 격자에 붙어 42 까지 간다', async () => {
    // 위 테스트와 같은 상자·같은 이동량이다. 다른 것은 요소의 성질(상한)뿐이다.
    const delta = target({ limit: STAT_OFFSET_LIMIT });
    render(<Fixture snap value={target()} delta={delta} stats={target()} />);
    stubBounds(200, 100);
    stubEl('delta', 71, 40, 50, 20);

    fireEvent(screen.getByTestId('delta'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 76, 0));
    await nextFrame();

    expect(delta.onMove).toHaveBeenLastCalledWith({ x: 42, y: 0 });
  });
});
