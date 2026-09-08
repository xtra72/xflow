// 값 글자 자리의 **옛 좌표**(`value_offset_x/y`)를 지금도 읽는다.
//
// 값 글자는 한때 도형과 같은 SVG 안에 있었고, 끌어 옮긴 변위는 그 SVG 의 `viewBox`
// 단위로 저장됐다. 지금은 도형 밖 형제 오버레이라 변위가 패널 상자 대비 **백분율**
// (`value_pos_x/y`)이다. 좌표계만 바뀌고 옛 키를 읽는 코드가 남지 않아, 저장된
// 대시보드에서 값을 끌어 옮겨 두었다면 그 자리를 잃고 기본 위치로 돌아갔다.
//
// **두 좌표 사이에는 config 만 보고 알 수 있는 환산 상수가 없다.** SVG 는 기본
// `preserveAspectRatio`(=meet)로 두 축을 같은 배율 `s = min(rect.w/vb.w, rect.h/vb.h)` 로
// 줄이고 남는 쪽에 여백을 둔다. 그래서 같은 viewBox 변위가 만드는 백분율은
// `offset * s / rect.w * 100` 이고, 이 값은 **그려진 상자의 종횡비**에 달렸다 — 패널
// 크기는 config 에 없으므로 여기서 백분율로 바꾸면 레터박스가 있는 축에서 그만큼
// 어긋난다(정사각 게이지를 낮고 넓은 패널에 놓으면 세로가 절반 넘게 틀어진다).
//
// 그래서 **환산하지 않는다.** 옛 값은 옛 좌표 그대로 내주고, 그리기는 옛 코드와 같은
// viewBox 를 쓰는 SVG 안에서 한다 — 배율 `s` 는 브라우저가 건다. 근사가 아니라 어떤
// 패널 크기에서도 옛 화면과 같은 자리이며, 상자를 재는 코드가 필요 없다.

import { readPanelOffset } from '../charts/panelGeometry';
import { STAT_OFFSET_LIMIT } from '../charts/statLayout';
import { readValueOffset } from '../charts/valueScale';

/** 값 글자의 자리 — 두 좌표계의 변위를 **함께** 낸다. */
export interface GaugeValuePlacement {
  /** 패널 상자 대비 백분율(지금 좌표 `value_pos_x/y`). */
  percentX: number;
  percentY: number;
  /** viewBox 단위 변위(옛 좌표 `value_offset_x/y`). 죄기는 그리는 쪽이 한다. */
  viewBoxX: number;
  viewBoxY: number;
}

/**
 * config 에서 값 글자의 자리를 읽는다.
 *
 * 옛 변위를 신규 변위로 **덮지 않고 더한다.** 덮으면 그 config 를 처음 끌거나 정렬하는
 * 순간 옛 몫이 사라져 값이 그만큼 튄다 — 끌기도 정렬도 화면에서 잰 자리(옛 몫이 이미
 * 들어간 자리)를 기준으로 새 백분율을 계산하는데, 저장되는 것은 옛 몫이 빠진 백분율
 * 하나뿐이기 때문이다. 더하면 두 조작 모두 옛 자리를 출발점으로 삼아 그대로 맞고,
 * 상자를 실측해 옛 몫을 백분율로 환산하는 코드가 필요 없다.
 *
 * 옛 몫을 지우는 곳은 **배치 초기화 두 곳뿐**이다(설정의 값 초기화 단추, 정렬 툴바의
 * 배치 초기화). 초기화가 신규 키만 지우면 값만 제자리로 돌아오지 않아 반쪽이 된다.
 */
export function resolveValuePlacement(config: Record<string, unknown>): GaugeValuePlacement {
  return {
    // 백분율 쪽 상한은 ±50 이다 — 게이지 상자(±40)가 아니라 **통계 본값과 같은 부류**의
    // 값이기 때문이다. 값 글자는 도형 영역에 갇히지 않는 작은 글자 덩어리이고 흐름상
    // 가운데에서 시작하므로, 패널 어느 모서리에든 놓으려면 축마다 50%가 필요하다
    // (`GaugeDragLayer` 가 끌 때 쓰는 상한과 같은 값이며, 같아야 한다 — 읽기가 더
    // 좁으면 사용자가 40 너머로 끌어 놓은 자리가 다음에 열 때 40 으로 되돌아간다).
    percentX: readPanelOffset(config.value_pos_x, STAT_OFFSET_LIMIT),
    percentY: readPanelOffset(config.value_pos_y, STAT_OFFSET_LIMIT),
    // viewBox 쪽은 좌표계가 다르다 — 죄기는 그리는 쪽이 viewBox 크기를 알고 한다
    // (`clampViewBoxOffset`). 두 좌표가 더해지는 규약(머리말)은 여기서 건드리지 않는다.
    viewBoxX: readValueOffset(config.value_offset_x),
    viewBoxY: readValueOffset(config.value_offset_y),
  };
}

/** `viewBox="minX minY w h"` 를 폭·높이로 읽는다. 형식이 아니면 `null`. */
export function parseViewBox(raw: string | null | undefined): { w: number; h: number } | null {
  if (!raw) return null;
  const parts = raw.trim().split(/[\s,]+/).map(Number);
  if (parts.length !== 4 || parts.some((n) => !Number.isFinite(n))) return null;
  const [, , w, h] = parts as [number, number, number, number];
  return w > 0 && h > 0 ? { w, h } : null;
}

/**
 * 옛 변위를 캔버스 밖으로 나가지 않을 만큼 죈다.
 *
 * 상한은 viewBox 한 변의 절반 — 옛 드래그 코드가 저장할 때 걸던 상한과 같다. 손으로
 * 고친 config 가 값을 캔버스 밖으로 완전히 밀어내 다시 잡을 수 없게 되는 것을 막는다.
 */
export function clampViewBoxOffset(value: number, span: number): number {
  if (!Number.isFinite(value) || !Number.isFinite(span) || span <= 0) return 0;
  const limit = span / 2;
  return Math.min(Math.max(value, -limit), limit);
}
