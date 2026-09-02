// FloorPlanTransformOverlay — 도면 드래그 이동 / 모서리 크기조절 인터랙션.

import { beforeAll, describe, it, expect, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

// jsdom 은 PointerEvent 를 구현하지 않아 clientX/clientY 가 이벤트에 실리지 않는다.
// MouseEvent 로 대체해 좌표를 전달한다(테스트 전용 폴리필).
class PointerEventPolyfill extends MouseEvent {
  pointerId: number;
  constructor(type: string, props: MouseEventInit & { pointerId?: number } = {}) {
    super(type, props);
    this.pointerId = props.pointerId ?? 1;
  }
}
// setPointerCapture 도 jsdom 에 없다 — 호출만 되고 무시되면 된다.
beforeAll(() => {
  (globalThis as unknown as { PointerEvent: unknown }).PointerEvent = PointerEventPolyfill;
  Element.prototype.setPointerCapture = () => {};
});

import FloorPlanTransformOverlay from './FloorPlanTransformOverlay';

const STAGE = { left: 0, top: 50, width: 400, height: 200 };
const CONTAINER = { width: 400, height: 300 };

function setup(transform?: { offset_x?: number; offset_y?: number; scale?: number }) {
  const onChange = vi.fn();
  const onReset = vi.fn();
  render(
    <FloorPlanTransformOverlay
      stage={STAGE}
      container={CONTAINER}
      transform={transform}
      onChange={onChange}
      onReset={onReset}
    />,
  );
  return { onChange, onReset };
}

/** 포인터 드래그 시퀀스(누르고 → 옮기고). */
function drag(el: Element, dx: number, dy: number) {
  fireEvent.pointerDown(el, { clientX: 100, clientY: 100, pointerId: 1 });
  fireEvent.pointerMove(screen.getByTestId('floorplan-transform-overlay'), {
    clientX: 100 + dx,
    clientY: 100 + dy,
    pointerId: 1,
  });
}

describe('아웃라인 배치', () => {
  it('현재 스테이지 박스 위에 아웃라인을 그린다', () => {
    setup();
    const el = screen.getByTestId('floorplan-transform-overlay') as HTMLElement;
    expect(el.style.left).toBe('0px');
    expect(el.style.top).toBe('50px');
    expect(el.style.width).toBe('400px');
  });

  it('네 모서리에 크기조절 핸들을 노출한다', () => {
    setup();
    for (const h of ['nw', 'ne', 'sw', 'se']) {
      expect(screen.getByTestId(`floorplan-transform-handle-${h}`)).toBeInTheDocument();
    }
  });
});

describe('드래그 이동', () => {
  it('이동량을 컨테이너 크기 대비 비율로 누적한다', () => {
    const { onChange } = setup();
    drag(screen.getByTestId('floorplan-transform-body'), 40, -30);
    // 40/400 = 0.1, -30/300 = -0.1
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ offset_x: 0.1, offset_y: -0.1 }),
    );
  });

  it('기존 변형 위에 누적한다(덮어쓰지 않음)', () => {
    const { onChange } = setup({ offset_x: 0.25, scale: 2 });
    drag(screen.getByTestId('floorplan-transform-body'), 40, 0);
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ offset_x: 0.35, scale: 2 }),
    );
  });
});

describe('모서리 크기조절', () => {
  it('se 핸들을 바깥으로 끌면 배율이 커진다', () => {
    const { onChange } = setup();
    drag(screen.getByTestId('floorplan-transform-handle-se'), 40, 40);
    const arg = onChange.mock.calls.at(-1)![0] as { scale: number };
    expect(arg.scale).toBeGreaterThan(1);
  });

  it('nw 핸들을 바깥(좌상단)으로 끌어도 배율이 커진다', () => {
    const { onChange } = setup();
    drag(screen.getByTestId('floorplan-transform-handle-nw'), -40, -40);
    const arg = onChange.mock.calls.at(-1)![0] as { scale: number };
    expect(arg.scale).toBeGreaterThan(1);
  });

  it('안쪽으로 끌면 작아진다', () => {
    const { onChange } = setup();
    drag(screen.getByTestId('floorplan-transform-handle-se'), -40, -40);
    const arg = onChange.mock.calls.at(-1)![0] as { scale: number };
    expect(arg.scale).toBeLessThan(1);
  });

  it('배율은 하한 아래로 내려가지 않는다(스테이지 붕괴 방지)', () => {
    const { onChange } = setup({ scale: 0.2 });
    drag(screen.getByTestId('floorplan-transform-handle-se'), -2000, -2000);
    const arg = onChange.mock.calls.at(-1)![0] as { scale: number };
    expect(arg.scale).toBeGreaterThanOrEqual(0.1);
  });

  it('크기조절은 이동값을 건드리지 않는다', () => {
    const { onChange } = setup({ offset_x: 0.3, offset_y: -0.2 });
    drag(screen.getByTestId('floorplan-transform-handle-se'), 40, 40);
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ offset_x: 0.3, offset_y: -0.2 }),
    );
  });
});

describe('초기화', () => {
  it('초기화 버튼은 변형을 지운다', () => {
    const { onReset } = setup({ scale: 2 });
    fireEvent.click(screen.getByTestId('floorplan-transform-reset'));
    expect(onReset).toHaveBeenCalled();
  });
});
