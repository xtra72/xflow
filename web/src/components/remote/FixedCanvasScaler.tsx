// 고정 캔버스 스케일러 (SPEC-REMOTE-001 M11, 그룹 M, REQ-M07/M08, OQ-M4).
//
// 노드 장비 화면을 충실히 재현하기 위해, 자식(원격 대시보드)을 노드 보고
// 해상도(`width`×`height` px)의 **고정 픽셀 캔버스**에 렌더한 뒤 가용 콘텐츠
// 영역에 **종횡비 보존 스케일-투-핏(레터박스)** 으로 맞춘다(stretch/crop 금지).
//
//   scale = min(availW / width, availH / height)
//
// 캔버스 내부 그리드/패널은 리플로우 없이(고정 컬럼·고정 레이아웃) 노드 장비
// 배치를 그대로 유지하며, 스케일은 바깥 래퍼의 CSS transform 으로만 적용된다 →
// 패널 컴포넌트 코드는 분기하지 않는다(A21/A16 일관 — 데이터/게이팅 불변).
//
// 가용 영역은 ResizeObserver 로 추적해 컨테이너 리사이즈 시 재스케일한다.
// 해상도 미보고(0) 폴백은 호출자(NodeDashboard)가 1920×1080 으로 결정한다.

import { useEffect, useRef, useState } from 'react';

/** 측정 전(또는 ResizeObserver 미지원 환경) 폴백 가용 영역. */
const FALLBACK_AVAIL_WIDTH = 1280;
const FALLBACK_AVAIL_HEIGHT = 720;

/**
 * 종횡비 보존 스케일-투-핏 배율을 계산한다(REQ-M08, OQ-M4).
 *
 * `scale = min(availW/canvasW, availH/canvasH)` — 가용 영역에 캔버스를 종횡비
 * 보존하며 맞춘 배율이다. 가용 영역과 캔버스의 종횡비가 다르면 남는 축에
 * 레터박스/필러박스 여백이 생긴다(stretch/crop 없음).
 *
 * 방어:
 *   - 캔버스 폭/높이가 0 이하이면 1 을 반환한다(폴백은 호출자 책임).
 *   - 가용 영역이 0 이하이면 1 을 반환한다(측정 전 상태 — 후속 측정으로 교체).
 *
 * @returns 양수 배율(0 < scale). 1:1 이상으로 확대될 수도, 축소될 수도 있다.
 */
export function computeFitScale(
  availW: number,
  availH: number,
  canvasW: number,
  canvasH: number,
): number {
  if (canvasW <= 0 || canvasH <= 0) return 1;
  if (availW <= 0 || availH <= 0) return 1;
  return Math.min(availW / canvasW, availH / canvasH);
}

interface FixedCanvasScalerProps {
  /** 고정 캔버스 가로 px(노드 해상도 또는 폴백). 양수여야 한다. */
  width: number;
  /** 고정 캔버스 세로 px(노드 해상도 또는 폴백). 양수여야 한다. */
  height: number;
  /** 캔버스 안에 고정 크기로 렌더할 자식(원격 대시보드). */
  children: React.ReactNode;
}

/**
 * 자식을 `width`×`height` 고정 캔버스에 렌더하고 가용 영역에 레터박스 스케일한다.
 *
 * 구조:
 *   - 바깥(outer): 가용 영역을 채우는 flex 중앙 정렬 + overflow-hidden(레터박스 여백).
 *   - 안(canvas): 정확히 width×height px 의 고정 박스. CSS transform: scale()
 *     (transform-origin: center)로 비례 축소/확대된다. transform 은 레이아웃
 *     박스를 바꾸지 않으므로 flex 중앙 정렬이 시각적 레터박스를 만든다.
 */
export function FixedCanvasScaler({
  width,
  height,
  children,
}: FixedCanvasScalerProps): React.JSX.Element {
  const outerRef = useRef<HTMLDivElement>(null);
  const [avail, setAvail] = useState<{ w: number; h: number }>({
    w: FALLBACK_AVAIL_WIDTH,
    h: FALLBACK_AVAIL_HEIGHT,
  });

  // 가용 영역(바깥 컨테이너 clientWidth/Height)을 ResizeObserver 로 추적한다.
  // 측정값이 0(레이아웃 전/jsdom)인 경우 폴백을 유지해 빈 화면을 방지한다.
  useEffect(() => {
    const node = outerRef.current;
    if (!node) return;

    const measure = (): void => {
      const w = node.clientWidth;
      const h = node.clientHeight;
      setAvail({
        w: w > 0 ? w : FALLBACK_AVAIL_WIDTH,
        h: h > 0 ? h : FALLBACK_AVAIL_HEIGHT,
      });
    };

    measure();

    // ResizeObserver 미지원 환경(일부 테스트 런타임)에서는 1회 측정으로 끝낸다.
    if (typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(() => measure());
    ro.observe(node);
    return () => ro.disconnect();
  }, []);

  const scale = computeFitScale(avail.w, avail.h, width, height);

  return (
    <div
      ref={outerRef}
      data-testid="fixed-canvas-scaler"
      className="flex h-full w-full items-center justify-center overflow-hidden"
    >
      <div
        data-testid="fixed-canvas"
        data-canvas-width={width}
        data-canvas-height={height}
        data-scale={scale}
        style={{
          width: `${width}px`,
          height: `${height}px`,
          transform: `scale(${scale})`,
          transformOrigin: 'center',
          // 고정 캔버스: 내부 패널은 리플로우 없이 노드 해상도 레이아웃을 유지한다.
          flex: 'none',
          overflow: 'hidden',
        }}
      >
        {children}
      </div>
    </div>
  );
}
