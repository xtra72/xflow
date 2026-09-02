// 라인 차트 드래그 레이어 — 범례와 그림 상자.
//
// 파이 레이어와 같은 규칙이다 — **정해진 대상 위에서만** 시작되어야 패널 크기 조절·휠
// 확대 같은 다른 조작과 부딪히지 않는다. 다른 점은 죄기 기준이 대상마다 갈린다는 것이다:
// 범례는 흐름 안에 남아 "지금 자리에서 상자 밖으로 나가지 않는 만큼", 그림 상자는 영역을
// 꽉 채우므로 파이 중심과 같은 ±상한.

import { describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';

import { ChartDragLayer } from './ChartDragLayer';

function Fixture({
  onChange,
  onPlotChange = () => {},
  offsetX = 0,
  offsetY = 0,
  enabled = true,
}: {
  onChange: (n: { x: number; y: number }) => void;
  onPlotChange?: (n: { x: number; y: number }) => void;
  offsetX?: number;
  offsetY?: number;
  enabled?: boolean;
}) {
  return (
    <ChartDragLayer
      enabled={enabled}
      legend={{ offsetX, offsetY, onChange }}
      plot={{ offsetX: 0, offsetY: 0, onChange: onPlotChange }}
    >
      <div data-testid="body" data-chart-legend-bounds="">
        <div data-chart-plot-area="" data-testid="plot">
          plot
        </div>
        {/* 실제 구조와 같다 — 잡히고 움직이는 바깥 상자 안에, 크기를 재는 내용 상자가 있다. */}
        <div data-chart-legend="" data-testid="legend">
          <div data-chart-legend-content="" data-testid="legend-content">
            legend
          </div>
        </div>
      </div>
    </ChartDragLayer>
  );
}

/** jsdom 은 레이아웃을 하지 않으므로 상자를 직접 심는다. */
function stubRect(testId: string, left: number, top: number, width: number, height: number): void {
  vi.spyOn(screen.getByTestId(testId), 'getBoundingClientRect').mockReturnValue({
    left,
    top,
    width,
    height,
    right: left + width,
    bottom: top + height,
    x: left,
    y: top,
    toJSON: () => ({}),
  } as DOMRect);
}

/**
 * 본문 400×300, 그 아래쪽에 40×30 범례가 가로 가운데로 놓인 상황.
 *
 * 바깥 상자는 flex 형제라 **가로로 본문만큼 늘어나 있다**(0~400). 죄기는 늘어난 바깥이
 * 아니라 내용 상자(180~220)를 기준으로 해야 한다 — 바깥을 재면 가로 여백이 0이 되어
 * 좌우로 한 픽셀도 못 움직인다.
 */
function stubLayout(): void {
  stubRect('body', 0, 0, 400, 300);
  stubRect('legend', 0, 255, 400, 45);
  stubRect('legend-content', 180, 270, 40, 30);
}

function pointer(type: string, x: number, y: number): PointerEvent {
  return new MouseEvent(type, { clientX: x, clientY: y, bubbles: true }) as unknown as PointerEvent;
}

/** 레이어는 이동을 프레임당 한 번으로 모으므로 프레임을 한 번 흘려야 콜백이 온다. */
async function nextFrame(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  });
}

describe('ChartLegendDragLayer', () => {
  it('범례를 끌면 담는 상자 대비 백분율이 된다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubLayout();

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 200, 280));
    fireEvent(document, pointer('pointermove', 240, 250));
    await nextFrame();

    // 40/400 = 10%, -30/300 = -10%.
    expect(onChange).toHaveBeenLastCalledWith({ x: 10, y: -10 });
  });

  it('그림 상자를 잡으면 범례가 아니라 그림이 움직인다', async () => {
    const onChange = vi.fn();
    const onPlotChange = vi.fn();
    render(<Fixture onChange={onChange} onPlotChange={onPlotChange} />);
    stubLayout();

    fireEvent(screen.getByTestId('plot'), pointer('pointerdown', 200, 100));
    fireEvent(document, pointer('pointermove', 240, 130));
    await nextFrame();

    // 40/400 = 10%, 30/300 = 10%.
    expect(onPlotChange).toHaveBeenLastCalledWith({ x: 10, y: 10 });
    expect(onChange).not.toHaveBeenCalled();
  });

  it('그림 상자는 ±상한을 넘지 않는다 — 영역을 꽉 채워 "가장자리" 라는 기준이 없다', async () => {
    const onPlotChange = vi.fn();
    render(<Fixture onChange={vi.fn()} onPlotChange={onPlotChange} />);
    stubLayout();

    fireEvent(screen.getByTestId('plot'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 5000, 5000));
    await nextFrame();

    expect(onPlotChange).toHaveBeenLastCalledWith({ x: 40, y: 40 });
  });

  it('둘 다 아닌 곳을 잡으면 아무것도 끌리지 않는다 — 다른 조작과 부딪히지 않는다', async () => {
    const onChange = vi.fn();
    const onPlotChange = vi.fn();
    render(<Fixture onChange={onChange} onPlotChange={onPlotChange} />);
    stubLayout();

    fireEvent(screen.getByTestId('body'), pointer('pointerdown', 200, 100));
    fireEvent(document, pointer('pointermove', 240, 120));
    await nextFrame();

    expect(onChange).not.toHaveBeenCalled();
    expect(onPlotChange).not.toHaveBeenCalled();
  });

  it('꺼져 있으면 끌리지 않는다 — 대시보드는 편집 모드에서만 켠다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} enabled={false} />);
    stubLayout();

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 200, 280));
    fireEvent(document, pointer('pointermove', 240, 250));
    await nextFrame();

    expect(onChange).not.toHaveBeenCalled();
  });

  it('상자 밖으로 나가지 않는다 — 아래로는 이미 붙어 있어 더 못 간다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubLayout();

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 200, 280));
    fireEvent(document, pointer('pointermove', 5000, 5000));
    await nextFrame();

    // 오른쪽 여백 400-220 = 180px → 45%. 아래 여백 300-300 = 0.
    expect(onChange).toHaveBeenLastCalledWith({ x: 45, y: 0 });
  });

  it('이미 밀려 있으면 그 자리에서 남은 만큼만 더 간다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} offsetX={20} />);
    stubLayout();

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 200, 280));
    fireEvent(document, pointer('pointermove', 5000, 280));
    await nextFrame();

    expect(onChange).toHaveBeenLastCalledWith({ x: 65, y: 0 });
  });

  it('한 프레임 안의 여러 이동은 한 번만 반영한다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubLayout();

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 200, 280));
    fireEvent(document, pointer('pointermove', 210, 280));
    fireEvent(document, pointer('pointermove', 240, 280));
    await nextFrame();

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenLastCalledWith({ x: 10, y: 0 });
  });

  it('손을 뗄 때 대기 중인 마지막 이동을 흘리지 않는다', () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubLayout();

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 200, 280));
    fireEvent(document, pointer('pointermove', 240, 280));
    fireEvent(document, pointer('pointerup', 240, 280));

    expect(onChange).toHaveBeenLastCalledWith({ x: 10, y: 0 });
  });

  it('상자를 잴 수 없으면 시작하지 않는다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubRect('body', 0, 0, 0, 0);
    stubRect('legend', 0, 0, 40, 30);
    stubRect('legend-content', 0, 0, 40, 30);

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 200, 280));
    fireEvent(document, pointer('pointermove', 240, 250));
    await nextFrame();

    expect(onChange).not.toHaveBeenCalled();
  });
});

describe('잡는 상자와 재는 상자가 다르다', () => {
  // 보고된 결함: 좌·우 배치에서 범례가 위아래로 움직이지 않았다. 범례를 감싼 자리 상자는
  // flex 교차축으로 늘어나므로, 그것을 기준으로 죄면 늘어난 축의 여백이 0이 된다.
  it('자리 상자가 본문 높이만큼 늘어나 있어도 세로로 움직인다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubRect('body', 0, 0, 400, 300);
    // 오른쪽에 붙은 범례 — 바깥 상자는 본문 높이 전체(0~300)를 차지한다.
    stubRect('legend', 340, 0, 60, 300);
    // 내용 상자는 항목만큼만: 세로 가운데 40×30.
    stubRect('legend-content', 350, 135, 40, 30);

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 370, 150));
    fireEvent(document, pointer('pointermove', 370, 120));
    await nextFrame();

    // -30/300 = -10%. 자리 상자를 기준으로 삼았다면 0 으로 죄여 움직이지 않았다.
    expect(onChange).toHaveBeenLastCalledWith({ x: 0, y: -10 });
  });

  it('바깥 상자가 본문 폭만큼 늘어나 있어도 가로로 끝까지 간다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    // 아래에 붙은 범례 — 바깥 상자가 가로로 본문 전체를 차지한다(stubLayout).
    stubLayout();

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 200, 280));
    fireEvent(document, pointer('pointermove', 5000, 280));
    await nextFrame();

    // 내용(180~220) 기준 오른쪽 여백 180px / 400px = 45%.
    // 바깥 상자(0~400)를 쟀다면 여백이 0이라 제자리에 얼어붙는다.
    expect(onChange).toHaveBeenLastCalledWith({ x: 45, y: 0 });
  });

  it('본문 위아래 끝을 넘지 않는다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubRect('body', 0, 0, 400, 300);
    stubRect('legend', 340, 0, 60, 300);
    stubRect('legend-content', 350, 135, 40, 30);

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 370, 150));
    fireEvent(document, pointer('pointermove', 370, -5000));
    await nextFrame();

    // 위쪽 여백 135px / 300px = 45%.
    expect(onChange).toHaveBeenLastCalledWith({ x: 0, y: -45 });
  });
});
