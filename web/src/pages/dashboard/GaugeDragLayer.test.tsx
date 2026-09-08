// 미리보기에서 현재값과 게이지를 끌어 옮기는 레이어.
//
// 미리보기에는 패널 크기 조절·휠 확대 같은 다른 조작이 이미 있다. 아무 데나 잡아도
// 끌리면 그것들과 부딪히므로, **정해진 대상 위에서만** 드래그가 시작되어야 한다.
// 세 대상이 같은 좌표계(담는 상자 대비 백분율)를 쓴다. 값 글자는 한때 viewBox 좌표였으나
// 도형 밖 오버레이가 되면서 합쳐졌다.

import { describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';

import { GaugeDragLayer } from './GaugeDragLayer';

/** 값·범례는 담는 상자(`area`)를, 게이지는 자기 상자를 기준으로 움직인다. */
function Fixture({
  onChange,
  onBodyChange = () => {},
  onLegendChange = () => {},
  // 아래 기존 테스트들은 **끌기 계산 자체**를 서술한다 — 격자 붙임을 켜 두면 계산 결과가
  // 10% 배수로 반올림되어 무엇을 재는 테스트인지 흐려진다. 붙임은 전용 테스트에서 켠다.
  snap = false,
}: {
  onChange: (n: { x: number; y: number }) => void;
  onBodyChange?: (n: { x: number; y: number }) => void;
  onLegendChange?: (n: { x: number; y: number }) => void;
  snap?: boolean;
}) {
  return (
    <GaugeDragLayer
      snap={snap}
      value={{ offsetX: 0, offsetY: 0, onChange }}
      legend={{ offsetX: 0, offsetY: 0, onChange: onLegendChange }}
      body={{ offsetX: 0, offsetY: 0, onChange: onBodyChange }}
    >
      <div data-testid="area">
        <div data-gauge-body="" data-testid="body">
          <svg viewBox="0 0 200 200" data-testid="svg">
            <circle data-testid="ring" cx={100} cy={100} r={80} />
          </svg>
        </div>
        {/* 값은 도형 **밖**의 형제다 — 실제 패널과 같은 구조다. */}
        <div data-gauge-value-text="" data-testid="value">
          50
        </div>
        <div data-gauge-threshold-legend="" data-testid="legend">
          legend
        </div>
      </div>
    </GaugeDragLayer>
  );
}

/** jsdom 은 레이아웃을 하지 않으므로 크기를 직접 심는다. */
function stubSize(testId: string, width: number, height: number): void {
  vi.spyOn(screen.getByTestId(testId), 'getBoundingClientRect').mockReturnValue({
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

/** 값 글자의 기준 상자는 담는 상자다(도형이 아니다). */
const stubValueBounds = (w: number, h: number): void => stubSize('area', w, h);

function pointer(type: string, x: number, y: number): PointerEvent {
  return new MouseEvent(type, { clientX: x, clientY: y, bubbles: true }) as unknown as PointerEvent;
}

/**
 * 애니메이션 프레임 한 번을 흘린다.
 *
 * 레이어는 포인터 이동을 프레임당 한 번으로 모으므로(중복 렌더 억제), 이동 직후에는
 * 아직 콜백이 오지 않았다. 프레임을 기다려야 실제 동작과 같은 시점에서 본다.
 */
async function nextFrame(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  });
}

describe('정해진 대상에서만 시작한다', () => {
  it('값 글자를 잡고 끌면 오프셋이 바뀐다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubValueBounds(400, 400);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 120));
    await nextFrame();

    // 담는 상자 대비 백분율이다 — 40/400 = 10%, 20/400 = 5%.
    expect(onChange).toHaveBeenLastCalledWith({ x: 10, y: 5 });
  });

  it('한 프레임 안의 여러 이동은 한 번만 반영한다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubValueBounds(400, 400);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 110, 100));
    fireEvent(document, pointer('pointermove', 120, 100));
    fireEvent(document, pointer('pointermove', 140, 100));
    await nextFrame();

    // 화면에 보이지도 않을 렌더를 쌓지 않는다 — 마지막 위치 하나만 반영된다.
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenLastCalledWith({ x: 10, y: 0 });
  });

  it('게이지 상자를 잡으면 값이 아니라 게이지 전체가 움직인다', async () => {
    const onChange = vi.fn();
    const onBodyChange = vi.fn();
    render(<Fixture onChange={onChange} onBodyChange={onBodyChange} />);
    stubSize('body', 400, 400);

    fireEvent(screen.getByTestId('ring'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 120));
    await nextFrame();

    // 게이지 상자는 백분율이다 — 40/400 = 10%, 20/400 = 5%.
    expect(onBodyChange).toHaveBeenLastCalledWith({ x: 10, y: 5 });
    expect(onChange).not.toHaveBeenCalled();
  });

  it('값 글자를 잡으면 도형은 움직이지 않는다 — 둘은 형제다', async () => {
    const onChange = vi.fn();
    const onBodyChange = vi.fn();
    render(<Fixture onChange={onChange} onBodyChange={onBodyChange} />);
    stubValueBounds(400, 400);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 120));
    await nextFrame();

    expect(onChange).toHaveBeenLastCalledWith({ x: 10, y: 5 });
    expect(onBodyChange).not.toHaveBeenCalled();
  });

  it('게이지 상자 오프셋도 상한(±40%)으로 죈다', async () => {
    const onBodyChange = vi.fn();
    render(<Fixture onChange={vi.fn()} onBodyChange={onBodyChange} />);
    stubSize('body', 400, 400);

    fireEvent(screen.getByTestId('ring'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 5000, 5000));
    await nextFrame();

    expect(onBodyChange).toHaveBeenLastCalledWith({ x: 40, y: 40 });
  });

  it('손을 뗄 때 대기 중인 마지막 이동을 흘리지 않는다', () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubValueBounds(400, 400);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 100, 100));
    // 프레임을 기다리지 않고 곧바로 손을 뗀다 — 그 이동이 사라지면 안 된다.
    fireEvent(document, pointer('pointermove', 140, 100));
    fireEvent(document, pointer('pointerup', 140, 100));

    expect(onChange).toHaveBeenLastCalledWith({ x: 10, y: 0 });
  });

  it('포인터를 뗀 뒤의 이동은 무시한다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubValueBounds(400, 400);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 100));
    fireEvent(document, pointer('pointerup', 140, 100));
    const calls = onChange.mock.calls.length;

    fireEvent(document, pointer('pointermove', 300, 300));
    await nextFrame();

    expect(onChange.mock.calls.length).toBe(calls);
  });

  it('크기를 잴 수 없으면 시작하지 않는다 — 0 으로 나눈 이동량이 값을 날린다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubValueBounds(0, 0);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 120));
    await nextFrame();

    expect(onChange).not.toHaveBeenCalled();
  });

  // 값은 도형이 아니라 **패널**을 기준으로 죈다 — 도형 영역에 갇히지 않는 것이 이
  // 요소의 요구다. 상한은 ±50%(제 중심이 어느 모서리에든 닿는 값)이며, 게이지 상자의
  // ±40% 보다 넓다.
  it('아무리 끌어도 ±50% 로 죄인다 — 다시 잡을 수 있어야 한다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubValueBounds(400, 400);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 5000, 5000));
    await nextFrame();

    expect(onChange).toHaveBeenLastCalledWith({ x: 50, y: 50 });
  });

  it('임계값 범례를 잡으면 범례만 백분율만큼 움직인다', async () => {
    const onChange = vi.fn();
    const onBodyChange = vi.fn();
    const onLegendChange = vi.fn();
    render(
      <Fixture onChange={onChange} onBodyChange={onBodyChange} onLegendChange={onLegendChange} />,
    );
    // 범례의 죄기 기준은 담긴 상자(게이지 영역)와 **범례 자신의 크기**다 — 모서리가
    // 가장자리에 닿는 곳이 한계이기 때문이다.
    stubSize('area', 400, 300);
    stubSize('legend', 40, 30);

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 70));
    await nextFrame();

    // 40/400 = 10%, -30/300 = -10%. 픽셀이면 패널 크기에 따라 자리가 갈린다.
    expect(onLegendChange).toHaveBeenLastCalledWith({ x: 10, y: -10 });
    expect(onChange).not.toHaveBeenCalled();
    expect(onBodyChange).not.toHaveBeenCalled();
  });
});

// SPEC-CHART-005 — 라인·게이지에 격자 붙임 / 무리 이동 / 크기 손잡이를 더한다.
describe('격자 붙임 · 무리 이동 · 크기 손잡이', () => {
  /** 백분율 대상 둘(게이지·범례)과 손잡이를 갖춘 판. */
  function Rig({
    onBodyChange = () => {},
    onLegendChange = () => {},
    onBodyResize = () => {},
    onSelectionChange = () => {},
    selection = new Set<'body' | 'legend' | 'value'>(),
    snap = true,
    bodyOffset = { x: 0, y: 0 },
    legendOffset = { x: 0, y: 0 },
  }: {
    onBodyChange?: (n: { x: number; y: number }) => void;
    onLegendChange?: (n: { x: number; y: number }) => void;
    onBodyResize?: (n: number) => void;
    onSelectionChange?: (n: ReadonlySet<'body' | 'legend' | 'value'>) => void;
    selection?: ReadonlySet<'body' | 'legend' | 'value'>;
    snap?: boolean;
    bodyOffset?: { x: number; y: number };
    legendOffset?: { x: number; y: number };
  }) {
    return (
      <GaugeDragLayer
        snap={snap}
        selection={selection}
        onSelectionChange={onSelectionChange}
        value={{ offsetX: 0, offsetY: 0, onChange: () => {} }}
        legend={{ offsetX: legendOffset.x, offsetY: legendOffset.y, onChange: onLegendChange }}
        body={{
          offsetX: bodyOffset.x,
          offsetY: bodyOffset.y,
          onChange: onBodyChange,
          size: 100,
          sizeRange: { min: 20, max: 100 },
          onResize: onBodyResize,
        }}
      >
        <div data-testid="area">
          <div data-gauge-body="" data-testid="body">
            <span data-panel-resize="body" data-testid="body-handle" />
            <svg viewBox="0 0 200 200" data-testid="svg">
              <text data-gauge-value-text="" data-testid="value">
                50
              </text>
            </svg>
          </div>
          <div data-gauge-threshold-legend="" data-testid="legend">
            legend
          </div>
        </div>
      </GaugeDragLayer>
    );
  }

  it('격자를 켜면 게이지 오프셋이 10% 눈금에 붙는다', async () => {
    const onBodyChange = vi.fn();
    render(<Rig onBodyChange={onBodyChange} selection={new Set(['body'])} />);
    stubSize('body', 400, 400);

    // 26px = 6.5% → 그대로면 6.5, 붙이면 10.
    fireEvent(screen.getByTestId('body'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 126, 100));
    await nextFrame();

    expect(onBodyChange).toHaveBeenLastCalledWith({ x: 10, y: 0 });
  });

  it('끄는 동안 Alt 를 누르면 격자를 잠시 무시한다', async () => {
    const onBodyChange = vi.fn();
    render(<Rig onBodyChange={onBodyChange} selection={new Set(['body'])} />);
    stubSize('body', 400, 400);

    fireEvent(screen.getByTestId('body'), pointer('pointerdown', 100, 100));
    fireEvent(
      document,
      new MouseEvent('pointermove', {
        clientX: 126,
        clientY: 100,
        altKey: true,
        bubbles: true,
      }) as unknown as PointerEvent,
    );
    await nextFrame();

    expect(onBodyChange).toHaveBeenLastCalledWith({ x: 6.5, y: 0 });
  });

  it('둘을 함께 고르면 같은 이동량으로 함께 움직인다', async () => {
    const onBodyChange = vi.fn();
    const onLegendChange = vi.fn();
    render(
      <Rig
        snap={false}
        selection={new Set(['body', 'legend'])}
        onBodyChange={onBodyChange}
        onLegendChange={onLegendChange}
        legendOffset={{ x: 20, y: 0 }}
      />,
    );
    stubSize('body', 400, 400);

    fireEvent(screen.getByTestId('body'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 100));
    await nextFrame();

    // 상대 배치가 유지된다 — 범례는 원래 20 이었으므로 30 이 된다.
    expect(onBodyChange).toHaveBeenLastCalledWith({ x: 10, y: 0 });
    expect(onLegendChange).toHaveBeenLastCalledWith({ x: 30, y: 0 });
  });

  it('Shift 로 잡으면 고르기만 하고 움직이지 않는다', async () => {
    const onBodyChange = vi.fn();
    const onSelectionChange = vi.fn();
    render(
      <Rig
        onBodyChange={onBodyChange}
        onSelectionChange={onSelectionChange}
        selection={new Set(['legend'])}
      />,
    );
    stubSize('body', 400, 400);

    fireEvent(
      screen.getByTestId('body'),
      new MouseEvent('pointerdown', {
        clientX: 100,
        clientY: 100,
        shiftKey: true,
        bubbles: true,
      }) as unknown as PointerEvent,
    );
    fireEvent(document, pointer('pointermove', 200, 100));
    await nextFrame();

    expect([...onSelectionChange.mock.calls[0]![0]].sort()).toEqual(['body', 'legend']);
    expect(onBodyChange).not.toHaveBeenCalled();
  });

  it('손잡이를 끌면 자리가 아니라 크기가 바뀐다', async () => {
    const onBodyResize = vi.fn();
    const onBodyChange = vi.fn();
    render(
      <Rig
        onBodyResize={onBodyResize}
        onBodyChange={onBodyChange}
        selection={new Set(['body'])}
      />,
    );
    stubSize('body', 400, 400);

    // 대각선 투영 — (-40 + -40) / 2 = -40 → 100 - 40 = 60.
    fireEvent(screen.getByTestId('body-handle'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 60, 60));
    await nextFrame();

    expect(onBodyResize).toHaveBeenLastCalledWith(60);
    expect(onBodyChange).not.toHaveBeenCalled();
  });

  it('크기는 지정된 범위 밖으로 나가지 않는다', async () => {
    const onBodyResize = vi.fn();
    render(<Rig onBodyResize={onBodyResize} selection={new Set(['body'])} />);
    stubSize('body', 400, 400);

    fireEvent(screen.getByTestId('body-handle'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', -400, -400));
    await nextFrame();

    expect(onBodyResize).toHaveBeenLastCalledWith(20);
  });

  it('값 글자도 격자와 무리에 함께 든다 — 좌표계가 하나로 합쳐졌다', async () => {
    const onSelectionChange = vi.fn();
    const onBodyChange = vi.fn();
    const onValueChange = vi.fn();
    render(
      <GaugeDragLayer
        snap
        selection={new Set<'body' | 'legend' | 'value'>(['body', 'value'])}
        onSelectionChange={onSelectionChange}
        value={{ offsetX: 0, offsetY: 0, onChange: onValueChange }}
        legend={{ offsetX: 0, offsetY: 0, onChange: () => {} }}
        body={{ offsetX: 0, offsetY: 0, onChange: onBodyChange }}
      >
        <div data-testid="area">
          <div data-gauge-body="" data-testid="body" />
          <div data-gauge-value-text="" data-testid="value">
            50
          </div>
        </div>
      </GaugeDragLayer>,
    );
    stubSize('area', 400, 400);
    stubSize('body', 400, 400);

    // 26px = 6.5% → 격자에 붙어 10%.
    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 126, 100));
    await nextFrame();

    expect(onValueChange).toHaveBeenLastCalledWith({ x: 10, y: 0 });
    // 함께 고른 도형도 같은 이동량으로 따라온다.
    expect(onBodyChange).toHaveBeenLastCalledWith({ x: 10, y: 0 });
  });
});

// 세로바 게이지는 사각형이라 축을 나눈다. 원형·반원은 종전대로 한 값이다.
describe('사각형 게이지는 손잡이가 축을 나눈다', () => {
  // `sizeY` 를 prop 기본값으로 두면 `undefined` 를 넘겨도 기본값이 되살아나
  // "세로 크기를 주지 않은" 경우를 재현할 수 없다. 불리언으로 갈라 준다.
  function Rig({
    onResize = () => {},
    onResizeY = () => {},
    splitAxes = true,
  }: {
    onResize?: (n: number) => void;
    onResizeY?: (n: number) => void;
    splitAxes?: boolean;
  }) {
    return (
      <GaugeDragLayer
        snap={false}
        selection={new Set<'body' | 'legend' | 'value'>(['body'])}
        value={{ offsetX: 0, offsetY: 0, onChange: () => {} }}
        legend={{ offsetX: 0, offsetY: 0, onChange: () => {} }}
        body={{
          offsetX: 0,
          offsetY: 0,
          onChange: () => {},
          size: 100,
          sizeRange: { min: 20, max: 200 },
          onResize,
          ...(splitAxes ? { sizeY: 100, sizeYRange: { min: 20, max: 200 } } : {}),
          onResizeY,
        }}
      >
        <div data-testid="area">
          <div data-gauge-body="" data-testid="body">
            <span data-panel-resize="body" data-testid="body-handle" />
          </div>
        </div>
      </GaugeDragLayer>
    );
  }

  it('가로로 끌면 폭만, 세로로 끌면 높이만 바뀐다', async () => {
    const onResize = vi.fn();
    const onResizeY = vi.fn();
    render(<Rig onResize={onResize} onResizeY={onResizeY} />);
    stubSize('body', 400, 400);

    fireEvent(screen.getByTestId('body-handle'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 130, 60));
    await nextFrame();

    expect(onResize).toHaveBeenLastCalledWith(130);
    expect(onResizeY).toHaveBeenLastCalledWith(60);
  });

  it('세로 크기를 주지 않으면 종전대로 대각선 투영이다 — 원형 게이지가 그렇다', async () => {
    const onResize = vi.fn();
    const onResizeY = vi.fn();
    render(<Rig onResize={onResize} onResizeY={onResizeY} splitAxes={false} />);
    stubSize('body', 400, 400);

    fireEvent(screen.getByTestId('body-handle'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 60, 60));
    await nextFrame();

    expect(onResize).toHaveBeenLastCalledWith(60);
    expect(onResizeY).not.toHaveBeenCalled();
  });
});
