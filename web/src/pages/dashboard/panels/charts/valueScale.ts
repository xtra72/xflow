// 현재값 표기의 **크기 배율**과 **위치 변위** — 게이지와 통계가 공유하는 단일 정본.
//
// 패널마다 기본 글자 크기가 다르다(게이지 도넛 28 · 바늘 10, 통계 본값 36 · 타일 24).
// 그 값을 통째로 설정에 노출하면 패널 유형을 바꿀 때마다 다시 잡아야 하므로, 기본값에
// 대한 **배율**로 둔다 — 유형을 바꿔도 "조금 크게" 라는 뜻이 유지된다.
//
// 게이지 쪽에 두지 않고 여기(차트 공통)에 두는 이유: 통계 패널이 쓰려면 차트에서 게이지를
// import 해야 하는데, 게이지는 이미 차트의 단위 모듈을 import 하고 있어 방향이 뒤엉킨다.

/** 배율 하한 — 이보다 작으면 글자를 읽을 수 없다. */
export const VALUE_SCALE_MIN = 0.3;

/** 배율 상한 — 이보다 크면 어느 패널에서도 글자가 상자를 넘는다. */
export const VALUE_SCALE_MAX = 3;

/**
 * 배율을 허용 범위 안으로 죈다. 수가 아니면 1(기본 크기).
 *
 * 손으로 편집한 config 나 구버전 config 가 글자를 화면 밖으로 날리지 않게 한다.
 */
export function readValueScale(raw: unknown): number {
  if (typeof raw !== 'number' || !Number.isFinite(raw)) return 1;
  return Math.min(Math.max(raw, VALUE_SCALE_MIN), VALUE_SCALE_MAX);
}

/** 위치 변위를 읽는다. 수가 아니면 0(기본 자리). */
export function readValueOffset(raw: unknown): number {
  return typeof raw === 'number' && Number.isFinite(raw) ? raw : 0;
}
