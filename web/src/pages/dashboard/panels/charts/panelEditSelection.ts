// 패널 편집의 **선택 모델**과 무리 이동 계산.
//
// 통계·바·파이가 함께 쓴다. 요소 종류는 문자열 제네릭이다(패널마다 다르다).
//
// 선택은 런타임 상태다 — config 에 저장하지 않는다. 저장하면 다음에 열 때도 무엇인가
// 골라져 있고, 그것을 푸는 방법이 화면에 없다(편집 모드 자체를 저장하지 않는 것과 같은
// 이유 — spec.md §2.4 U4-4).
//
// @spec SPEC-CHART-004 §2.9 [U9] · SPEC-CHART-005 §2.1 [U1]

import { STAT_OFFSET_LIMIT } from './statLayout';

/** 고른 요소들. 순서는 뜻이 없다. */
export type StatSelection<K extends string = string> = ReadonlySet<K>;

/** 빈 선택 — 매 렌더 새 Set 을 만들지 않는다. */
export const EMPTY_SELECTION: StatSelection<never> = new Set();

/**
 * 요소를 눌렀을 때의 다음 선택.
 *
 * `additive`(Shift·Ctrl·Cmd)면 고른 것을 더하거나 뺀다. 아니면 그것 하나만 남긴다 —
 * 이미 골라져 있던 것을 다시 누르면 선택이 유지된다(끌기를 시작하려는 것이지 고르기를
 * 무르려는 것이 아니다).
 */
export function nextSelection<K extends string>(
  current: StatSelection<K>,
  kind: K,
  additive: boolean,
): StatSelection<K> {
  if (!additive) return current.has(kind) ? current : new Set([kind]);
  const next = new Set(current);
  if (next.has(kind)) next.delete(kind);
  else next.add(kind);
  return next;
}

/** 무리 이동에 참여하는 요소 하나의 현재 오프셋. */
export interface GroupMember {
  offsetX: number;
  offsetY: number;
}

/**
 * 무리 전체가 함께 움직일 수 있는 만큼으로 이동량을 죈다. @spec SPEC-CHART-004 §2.9 [U9-4]
 *
 * 요소마다 따로 죄면 상한에 먼저 닿은 것만 멈추고 나머지는 계속 가서 **대형이 무너진다**.
 * 함께 옮긴다는 말은 상대 배치가 유지된다는 뜻이므로, 한 요소라도 더 못 가면 무리 전체가
 * 거기서 멈춰야 한다.
 *
 * 이동량(`dx`/`dy`)은 오프셋과 같은 단위(백분율 포인트)다.
 */
export function clampGroupDelta(
  members: readonly GroupMember[],
  dx: number,
  dy: number,
  limit = STAT_OFFSET_LIMIT,
): { dx: number; dy: number } {
  if (members.length === 0) return { dx, dy };
  let minDx = -Infinity;
  let maxDx = Infinity;
  let minDy = -Infinity;
  let maxDy = Infinity;
  for (const m of members) {
    minDx = Math.max(minDx, -limit - m.offsetX);
    maxDx = Math.min(maxDx, limit - m.offsetX);
    minDy = Math.max(minDy, -limit - m.offsetY);
    maxDy = Math.min(maxDy, limit - m.offsetY);
  }
  return {
    dx: Math.min(Math.max(dx, minDx), maxDx),
    dy: Math.min(Math.max(dy, minDy), maxDy),
  };
}
