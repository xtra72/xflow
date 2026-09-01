// 미리보기에서 파이와 범례를 끌어 옮기는 레이어.
//
// 게이지 값 드래그와 같은 규칙이다 — **정해진 대상 위에서만** 시작되어야 패널 크기
// 조절·휠 확대 같은 다른 조작과 부딪히지 않는다. 잡은 대상에 따라 무엇이 움직이는지도
// 갈린다(범례는 픽셀, 파이 중심은 백분율).

import { describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';

import { PieDragLayer } from './PieDragLayer';

/** 패널 본문 400×300 안에 차트 영역과 범례가 놓인 상황. */
function Fixture({
  onChange,
  onChartChange = () => {},
}: {
  onChange: (n: { x: number; y: number }) => void;
  onChartChange?: (n: { x: number; y: number }) => void;
}) {
  return (
    <PieDragLayer
      legend={{ offsetX: 0, offsetY: 0, onChange }}
      chart={{ offsetX: 0, offsetY: 0, onChange: onChartChange }}
    >
      <div data-testid="body">
        <div data-pie-chart-area="" data-testid="chart">
          chart
        </div>
        <div data-pie-legend="" data-testid="legend">
          legend
        </div>
      </div>
    </PieDragLayer>
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

/** 범례의 죄기 기준은 부모(본문)다. */
const stubBodySize = (w: number, h: number): void => stubSize('body', w, h);

function pointer(type: string, x: number, y: number): PointerEvent {
  return new MouseEvent(type, { clientX: x, clientY: y, bubbles: true }) as unknown as PointerEvent;
}

/** 레이어는 이동을 프레임당 한 번으로 모으므로 프레임을 한 번 흘려야 콜백이 온다. */
async function nextFrame(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  });
}

describe('정해진 대상에서만 시작한다', () => {
  it('범례를 잡고 끌면 픽셀 이동량이 그대로 오프셋이 된다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubBodySize(400, 300);

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 120));
    await nextFrame();

    // HTML 요소라 환산이 없다(게이지는 viewBox 배율을 되돌린다).
    expect(onChange).toHaveBeenLastCalledWith({ x: 40, y: 20 });
  });

  it('차트 영역을 잡으면 범례가 아니라 파이가 움직인다', async () => {
    const onChange = vi.fn();
    const onChartChange = vi.fn();
    render(<Fixture onChange={onChange} onChartChange={onChartChange} />);
    stubSize('chart', 400, 300);

    fireEvent(screen.getByTestId('chart'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 130));
    await nextFrame();

    // 파이 중심은 백분율이다 — 40/400 = 10%, 30/300 = 10%.
    expect(onChartChange).toHaveBeenLastCalledWith({ x: 10, y: 10 });
    expect(onChange).not.toHaveBeenCalled();
  });

  it('둘 다 아닌 곳을 잡으면 아무것도 끌리지 않는다', async () => {
    const onChange = vi.fn();
    const onChartChange = vi.fn();
    render(<Fixture onChange={onChange} onChartChange={onChartChange} />);
    stubBodySize(400, 300);

    fireEvent(screen.getByTestId('body'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 120));
    await nextFrame();

    expect(onChange).not.toHaveBeenCalled();
    expect(onChartChange).not.toHaveBeenCalled();
  });

  it('파이 중심은 상한(±40%)을 넘지 않는다', async () => {
    const onChartChange = vi.fn();
    render(<Fixture onChange={vi.fn()} onChartChange={onChartChange} />);
    stubSize('chart', 400, 300);

    fireEvent(screen.getByTestId('chart'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 5000, 5000));
    await nextFrame();

    expect(onChartChange).toHaveBeenLastCalledWith({ x: 40, y: 40 });
  });

  it('한 프레임 안의 여러 이동은 한 번만 반영한다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubBodySize(400, 300);

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 110, 100));
    fireEvent(document, pointer('pointermove', 130, 100));
    await nextFrame();

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenLastCalledWith({ x: 30, y: 0 });
  });

  it('손을 뗄 때 대기 중인 마지막 이동을 흘리지 않는다', () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubBodySize(400, 300);

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 130, 100));
    fireEvent(document, pointer('pointerup', 130, 100));

    expect(onChange).toHaveBeenLastCalledWith({ x: 30, y: 0 });
  });

  it('본문 절반을 넘겨 끌어도 죄인다 — 다시 잡을 수 있어야 한다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubBodySize(400, 300);

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 5000, 5000));
    await nextFrame();

    expect(onChange).toHaveBeenLastCalledWith({ x: 200, y: 150 });
  });

  it('크기를 잴 수 없으면 시작하지 않는다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubBodySize(0, 0);

    fireEvent(screen.getByTestId('legend'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 120));
    await nextFrame();

    expect(onChange).not.toHaveBeenCalled();
  });
});
