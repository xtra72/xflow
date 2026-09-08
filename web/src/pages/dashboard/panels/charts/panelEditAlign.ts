// 패널 요소의 **그리드 스냅**과 **요소끼리 정렬** 순수 로직.
//
// 통계·바·파이가 함께 쓴다. 요소 종류는 패널마다 다르므로(통계 3 · 파이 2 · 바 2)
// 문자열 제네릭으로 두고 이 모듈은 **개수**만 안다 — 목록을 여기서 알면 패널이 늘
// 때마다 이 파일을 고쳐야 한다.
//
// 둘 다 "요소를 어디에 놓을까" 를 푸는 계산이라 한 모듈에 둔다. 저장 형태는 배치와
// 같은 축(패널 상자 대비 백분율 오프셋)이므로 결과도 오프셋으로 돌려준다 —
// 여기서 DOM 을 만지지 않는다.
//
// **스냅은 오프셋이 아니라 요소의 절대 자리에 건다.** 오프셋을 10% 단위로 죄면 흐름상
// 시작 자리가 격자에 놓여 있지 않은 요소는 영영 격자에 맞지 않는다 — 보조 줄 두 개가
// 그렇다. 절대 자리를 격자에 맞춘 뒤 오프셋을 거꾸로 구한다.
//
// @spec SPEC-CHART-004 §2.7 [U7] / §2.8 [U8] · SPEC-CHART-005 §2.1 [U1]

import { PANEL_OFFSET_LIMIT, clampPercentOffset, pixelsToPercent } from './panelGeometry';

/**
 * 격자 간격(패널 상자 대비 백분율).
 *
 * 10%면 한 축에 칸 10개다. 더 촘촘하면 작은 패널에서 선이 뭉개져 참조선 구실을 못하고,
 * 더 성기면 맞출 수 있는 자리가 너무 적다.
 */
export const GRID_STEP_PERCENT = 10;

/** 화면에서 잰 상자 하나. `DOMRect` 의 필요한 부분만 받는다(테스트에서 만들기 쉽다). */
export interface Box {
  left: number;
  top: number;
  width: number;
  height: number;
}

/** 정렬·스냅 계산에 필요한 요소 하나의 상태. */
export interface PanelElementBox<K extends string = string> {
  kind: K;
  /** 화면에서 잰 현재 상자. */
  rect: Box;
  /** 지금 적용된 오프셋(백분율 포인트). */
  offsetX: number;
  offsetY: number;
  /**
   * 이 요소의 오프셋 상한(백분율 포인트). 미지정이면 ±40(`PANEL_OFFSET_LIMIT`).
   *
   * **상한은 요소의 성질이 정한다.** 영역을 가득 채우는 그림(파이·바 플롯·게이지 몸통)은
   * 40 이면 이미 절반이 잘리지만, 가운데에서 시작하는 작은 글자 덩어리(통계 세 줄·범례)는
   * 모서리에 닿으려면 50 이 필요하다. 한 값으로 묶으면 한쪽이 반드시 어긋난다.
   *
   * 정렬로 만든 값도 **읽는 쪽과 같은 상한**으로 죄어야 한다 — 더 느슨하게 죄면 저장은
   * 되지만 다음에 읽을 때 되돌아가, 화면이 소리 없이 제자리로 튄다.
   */
  limit?: number;
}

/** 정렬 축. */
export type AlignAxis = 'horizontal' | 'vertical';

/**
 * 정렬 방식.
 *
 * `start` / `end` 는 축에 따라 뜻이 갈린다 — 가로면 왼쪽/오른쪽, 세로면 위/아래다.
 * 축마다 다른 이름을 두면(`left`/`top`) 같은 계산을 두 벌 써야 한다.
 */
export type AlignMode = 'start' | 'center' | 'end';

/** 한 요소에 적용할 오프셋 변경. 축 하나만 담는다 — 정렬은 한 축씩 한다. */
export interface AlignPatch<K extends string = string> {
  kind: K;
  offsetX?: number;
  offsetY?: number;
}

/**
 * 요소의 **흐름상 시작 자리**를 되짚는다.
 *
 * 화면에서 잰 자리에는 이미 오프셋이 반영되어 있다. 격자에 맞추려면 오프셋을 뺀
 * 원래 자리를 알아야, 새 오프셋을 다시 구할 수 있다.
 */
function flowStart(rectStart: number, offsetPercent: number, boundsLen: number): number {
  return rectStart - (offsetPercent / 100) * boundsLen;
}

/** 백분율 자리를 격자에 맞춘다. */
function snapPercent(v: number, step: number): number {
  if (!(step > 0)) return v;
  return Math.round(v / step) * step;
}

/**
 * 끄는 중인 요소의 오프셋을 격자에 맞춘다. @spec SPEC-CHART-004 §2.7 [U7]
 *
 * 맞추는 지점은 요소의 **중심**이다 — 패널 가운데의 `+` 표식과 같은 기준이라, 오프셋 0
 * 이 곧 격자 교점이 되어 "가운데로 되돌린다" 가 눈에 보인다. 모서리를 맞추면 요소마다
 * 다른 변이 기준이 되어 무엇에 붙었는지 읽히지 않는다.
 *
 * 격자를 벗어난 자리를 강제로 끌어오지 않는다 — 죄는 것은 오프셋이 아니라 중심의
 * 절대 자리이며, 그 결과를 오프셋으로 되돌린 뒤 상한만 다시 적용한다.
 *
 * **그 상한은 호출부가 정한다.** 스냅은 중심을 격자로 끌어당기므로 죄기 뒤에 값을 다시
 * 최대 반 칸(±5%p) 밀어낼 수 있다 — 여기서 더 느슨한 상한을 쓰면 바로 위에서 죈 값이
 * 도로 풀려, 요소 종류별 상한이 스냅 한 줄에 무너진다.
 */
export function snapOffsetToGrid(
  /** 끄는 중에 계산된 **새** 오프셋. */
  proposedOffset: number,
  /**
   * 잡는 순간의 상태.
   *
   * `rectStart` 는 그때 화면에서 잰 값이라 **그때의 오프셋(`offset`)이 이미 반영되어**
   * 있다. 둘을 함께 받아야 흐름상 시작 자리를 되짚을 수 있다 — 새 오프셋으로 되짚으면
   * 이동량이 두 번 반영되어 스냅이 엉뚱한 자리에 붙는다.
   */
  base: { offset: number; rectStart: number; rectLen: number },
  /** 기준 상자의 시작 좌표와 길이. */
  bounds: { start: number; len: number },
  /** 이 요소의 오프셋 상한. 미지정이면 ±40 — 영역을 채우는 그림이 안전한 기본값이다. */
  limit = PANEL_OFFSET_LIMIT,
  step = GRID_STEP_PERCENT,
): number {
  if (!(bounds.len > 0)) return proposedOffset;
  const flow = flowStart(base.rectStart, base.offset, bounds.len);
  // 새 오프셋을 적용했을 때의 중심(기준 상자 대비 백분율).
  const centerPercent =
    ((flow + (proposedOffset / 100) * bounds.len + base.rectLen / 2 - bounds.start) / bounds.len) *
    100;
  const snapped = snapPercent(centerPercent, step);
  // 중심이 그만큼 움직이면 오프셋도 같은 만큼 움직인다. 부동소수 찌꺼기는 여기서 턴다 —
  // 붙인 값이 `10.000000000000007` 로 저장되면 눈금에 붙었는지 눈으로 알 수 없고,
  // 다음에 열었을 때 같은 자리에서 다시 붙지 않는다. 4자리면 화면 해상도보다 훨씬 곱다.
  const next = proposedOffset + (snapped - centerPercent);
  return clampPercentOffset(Math.round(next * 1e4) / 1e4, limit);
}

/**
 * 요소들을 **서로 맞춘다**. @spec SPEC-CHART-004 §2.8 [U8]
 *
 * 기준은 고른 요소들이 이루는 **바깥 상자**다(디자인 도구의 정렬과 같다) — `start` 는
 * 가장 앞선 변, `end` 는 가장 뒤선 변, `center` 는 그 둘의 가운데다. "가장 왼쪽 요소에
 * 맞춘다" 가 아니라 "바깥 상자에 맞춘다" 로 두면 세 방식이 한 계산으로 풀린다.
 *
 * 요소가 2개 미만이면 맞출 상대가 없으므로 빈 배열이다 — 보조 줄이 꺼져 있으면 그렇다.
 *
 * 세로 정렬은 세 요소를 **겹치게 만든다.** 세로로 쌓인 것을 한 줄에 맞추라는 뜻이
 * 그것이므로 막지 않는다. 되돌릴 수단(배치 초기화)이 함께 있어야 한다.
 */
export function computeAlignPatches<K extends string>(
  elements: readonly PanelElementBox<K>[],
  bounds: Box,
  axis: AlignAxis,
  mode: AlignMode,
): AlignPatch<K>[] {
  if (elements.length < 2) return [];
  const horizontal = axis === 'horizontal';
  const boundsLen = horizontal ? bounds.width : bounds.height;
  if (!(boundsLen > 0)) return [];

  const startOf = (e: PanelElementBox<K>) => (horizontal ? e.rect.left : e.rect.top);
  const lenOf = (e: PanelElementBox<K>) => (horizontal ? e.rect.width : e.rect.height);

  const minStart = Math.min(...elements.map(startOf));
  const maxEnd = Math.max(...elements.map((e) => startOf(e) + lenOf(e)));

  return elements.map((e) => {
    const len = lenOf(e);
    // 바깥 상자의 어디에 붙일지에 따라 이 요소가 놓여야 할 시작 좌표가 정해진다.
    const desiredStart =
      mode === 'start'
        ? minStart
        : mode === 'end'
          ? maxEnd - len
          : (minStart + maxEnd) / 2 - len / 2;
    const current = horizontal ? e.offsetX : e.offsetY;
    // 상한은 요소마다 다르다 — 읽는 쪽과 어긋나면 정렬한 자리가 다음 렌더에 되돌아간다.
    const next = clampPercentOffset(
      current + pixelsToPercent(desiredStart - startOf(e), boundsLen),
      e.limit ?? PANEL_OFFSET_LIMIT,
    );
    return horizontal ? { kind: e.kind, offsetX: next } : { kind: e.kind, offsetY: next };
  });
}
