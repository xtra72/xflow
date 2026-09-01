// 미리보기에서 현재값과 게이지를 끌어 옮기는 레이어.
//
// 미리보기에는 패널 크기 조절·휠 확대 같은 다른 조작이 이미 있다. 아무 데나 잡아도
// 끌리면 그것들과 부딪히므로, **정해진 대상 위에서만** 드래그가 시작되어야 한다.
// 잡은 대상에 따라 좌표계도 갈린다 — 값 글자는 viewBox, 게이지 상자는 백분율.

import { describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';

import { GaugeDragLayer } from './GaugeDragLayer';

/** viewBox 200×200 짜리 SVG 를 400×400 px 로 그린 상황(배율 2). */
function Fixture({
  onChange,
  onBodyChange = () => {},
}: {
  onChange: (n: { x: number; y: number }) => void;
  onBodyChange?: (n: { x: number; y: number }) => void;
}) {
  return (
    <GaugeDragLayer
      value={{ offsetX: 0, offsetY: 0, onChange }}
      body={{ offsetX: 0, offsetY: 0, onChange: onBodyChange }}
    >
      <div data-gauge-body="" data-testid="body">
        <svg viewBox="0 0 200 200" data-testid="svg">
          <text data-gauge-value-text="" data-testid="value">
            50
          </text>
          <circle data-testid="ring" cx={100} cy={100} r={80} />
        </svg>
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

const stubSvgSize = (w: number, h: number): void => stubSize('svg', w, h);

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
    stubSvgSize(400, 400);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 120));
    await nextFrame();

    // 배율 2 → 40px·20px 은 viewBox 20·10 이다.
    expect(onChange).toHaveBeenLastCalledWith({ x: 20, y: 10 });
  });

  it('한 프레임 안의 여러 이동은 한 번만 반영한다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubSvgSize(400, 400);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 110, 100));
    fireEvent(document, pointer('pointermove', 120, 100));
    fireEvent(document, pointer('pointermove', 140, 100));
    await nextFrame();

    // 화면에 보이지도 않을 렌더를 쌓지 않는다 — 마지막 위치 하나만 반영된다.
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenLastCalledWith({ x: 20, y: 0 });
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

  it('값 글자는 게이지 상자 안에 있어도 값이 움직인다 — 안쪽이 이긴다', async () => {
    const onChange = vi.fn();
    const onBodyChange = vi.fn();
    render(<Fixture onChange={onChange} onBodyChange={onBodyChange} />);
    stubSvgSize(400, 400);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 120));
    await nextFrame();

    expect(onChange).toHaveBeenLastCalledWith({ x: 20, y: 10 });
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
    stubSvgSize(400, 400);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 100, 100));
    // 프레임을 기다리지 않고 곧바로 손을 뗀다 — 그 이동이 사라지면 안 된다.
    fireEvent(document, pointer('pointermove', 140, 100));
    fireEvent(document, pointer('pointerup', 140, 100));

    expect(onChange).toHaveBeenLastCalledWith({ x: 20, y: 0 });
  });

  it('포인터를 뗀 뒤의 이동은 무시한다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubSvgSize(400, 400);

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
    stubSvgSize(0, 0);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 100, 100));
    fireEvent(document, pointer('pointermove', 140, 120));
    await nextFrame();

    expect(onChange).not.toHaveBeenCalled();
  });

  it('캔버스 절반을 넘겨 끌어도 죄인다 — 다시 잡을 수 있어야 한다', async () => {
    const onChange = vi.fn();
    render(<Fixture onChange={onChange} />);
    stubSvgSize(400, 400);

    fireEvent(screen.getByTestId('value'), pointer('pointerdown', 0, 0));
    fireEvent(document, pointer('pointermove', 5000, 5000));
    await nextFrame();

    expect(onChange).toHaveBeenLastCalledWith({ x: 100, y: 100 });
  });
});
