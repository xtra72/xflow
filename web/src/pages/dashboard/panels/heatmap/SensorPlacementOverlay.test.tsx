// SensorPlacementOverlay 드래그/제거/배치-인 테스트 (SPEC-HEATMAP-PANEL-002 T6).
// fireEvent 포인터 이벤트로 드래그 → 정규화 좌표 갱신(AC-03), 도면 밖 clamp(AC-E2),
// 마커 제거(AC-04), 미배치 센서 배치-인(AC-04)을 커버한다. 컨테이너 rect 는 모킹한다.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

// i18n 은 키를 그대로 반환하도록 모킹(치환 토큰 포함 문자열은 replace 로 처리됨).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import SensorPlacementOverlay from './SensorPlacementOverlay';

// jsdom 은 PointerEvent 를 구현하지 않아 fireEvent.pointer* 가 clientX/clientY 를 실어 나르지
// 못한다. MouseEvent(백엔드가 clientX 를 지원) 기반 폴리필로 대체해 좌표를 전달한다.
if (typeof globalThis.PointerEvent === 'undefined') {
  globalThis.PointerEvent = class extends MouseEvent {} as unknown as typeof PointerEvent;
}

// 오버레이 rect: left100 / top50 / width200 / height400 → 중심(200,250)=(0.5,0.5).
const RECT = { left: 100, top: 50, width: 200, height: 400, right: 300, bottom: 450, x: 100, y: 50 };

beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
    ...RECT,
    toJSON: () => RECT,
  } as DOMRect);
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('SensorPlacementOverlay', () => {
  it('AC-03: 마커 드래그가 포인터 위치를 정규화 좌표로 갱신한다', () => {
    const onPositionChange = vi.fn();
    render(
      <SensorPlacementOverlay
        placed={[{ key: 'a', pos: { x: 0.1, y: 0.1 } }]}
        unplaced={[]}
        onPositionChange={onPositionChange}
        onRemove={vi.fn()}
      />,
    );
    const handle = screen.getByTestId('sensor-marker-handle-a');
    const overlay = screen.getByTestId('sensor-placement-overlay');
    fireEvent.pointerDown(handle, { clientX: 120, clientY: 90 });
    fireEvent.pointerMove(overlay, { clientX: 200, clientY: 250 }); // 중심 → (0.5,0.5)
    fireEvent.pointerUp(overlay, { clientX: 200, clientY: 250 });
    // 마지막 호출(드롭)이 clamp 된 정규화 좌표여야 한다.
    expect(onPositionChange).toHaveBeenCalledWith('a', { x: 0.5, y: 0.5 });
  });

  it('AC-E2: 도면 경계 밖으로 드래그하면 [0,1] 로 clamp 되어 저장된다', () => {
    const onPositionChange = vi.fn();
    render(
      <SensorPlacementOverlay
        placed={[{ key: 'a', pos: { x: 0.5, y: 0.5 } }]}
        unplaced={[]}
        onPositionChange={onPositionChange}
        onRemove={vi.fn()}
      />,
    );
    const handle = screen.getByTestId('sensor-marker-handle-a');
    const overlay = screen.getByTestId('sensor-placement-overlay');
    fireEvent.pointerDown(handle, { clientX: 200, clientY: 250 });
    fireEvent.pointerUp(overlay, { clientX: 99999, clientY: 99999 }); // 우하단 한참 밖
    expect(onPositionChange).toHaveBeenLastCalledWith('a', { x: 1, y: 1 });
  });

  it('드래그 시작 전이면(pointerdown 없이) 이동은 좌표를 갱신하지 않는다', () => {
    const onPositionChange = vi.fn();
    render(
      <SensorPlacementOverlay
        placed={[{ key: 'a', pos: { x: 0.5, y: 0.5 } }]}
        unplaced={[]}
        onPositionChange={onPositionChange}
        onRemove={vi.fn()}
      />,
    );
    const overlay = screen.getByTestId('sensor-placement-overlay');
    fireEvent.pointerMove(overlay, { clientX: 200, clientY: 250 });
    expect(onPositionChange).not.toHaveBeenCalled();
  });

  it('AC-04: 마커 제거 버튼은 해당 센서 좌표만 삭제한다', () => {
    const onRemove = vi.fn();
    render(
      <SensorPlacementOverlay
        placed={[{ key: 'a', pos: { x: 0.5, y: 0.5 } }]}
        unplaced={[]}
        onPositionChange={vi.fn()}
        onRemove={onRemove}
      />,
    );
    fireEvent.click(screen.getByTestId('sensor-remove-a'));
    expect(onRemove).toHaveBeenCalledWith('a');
  });

  it('AC-04: 미배치 센서를 도면 위로 드래그하면 좌표가 부여된다(배치-인)', () => {
    const onPositionChange = vi.fn();
    render(
      <SensorPlacementOverlay
        placed={[]}
        unplaced={['b']}
        onPositionChange={onPositionChange}
        onRemove={vi.fn()}
      />,
    );
    const chip = screen.getByTestId('sensor-unplaced-b');
    const overlay = screen.getByTestId('sensor-placement-overlay');
    fireEvent.pointerDown(chip, { clientX: 10, clientY: 10 });
    fireEvent.pointerUp(overlay, { clientX: 100, clientY: 50 }); // 좌상단 → (0,0)
    expect(onPositionChange).toHaveBeenCalledWith('b', { x: 0, y: 0 });
  });

  it('마커는 정규화 좌표를 % 로 환산해 배치한다(리사이즈 불변, AC-E4)', () => {
    render(
      <SensorPlacementOverlay
        placed={[{ key: 'a', pos: { x: 0.25, y: 0.75 } }]}
        unplaced={[]}
        onPositionChange={vi.fn()}
        onRemove={vi.fn()}
      />,
    );
    const marker = screen.getByTestId('sensor-marker-a');
    expect(marker.style.left).toBe('25%');
    expect(marker.style.top).toBe('75%');
  });
});
