/**
 * 직각 연결선의 **구간마다 하나**인 고정값 목록 (SPEC-CANVAS-021).
 *
 * 019 는 수 하나를 두었다. 논리 구간이 하나뿐일 때는 그것으로 족했지만, 사용자 점이 `N`
 * 개면 구간은 `N+1` 개이고 직각 갈래는 **구간마다 Z 를 하나씩** 그린다 — 그 Z 의 모서리
 * 둘은 한 축 위에 있으므로 자유도는 구간당 **수 하나**이고, 따라서 고정값도 구간마다
 * 하나여야 한다.
 *
 * **목록의 자리는 좌표가 아니라 구간 번호다.** 상자를 옮겨도 구간 번호는 변하지 않고,
 * 점을 더하거나 빼면 `insertPointAt` · `removePointAt` 이 이 목록을 **함께** 옮긴다
 * (K1). 019 가 "점 둘은 낡는다" 며 수 하나를 고른 그 근거가 목록에서도 그대로 산다.
 *
 * `null` 이 **자동**이다(§결정 4). `0` 을 빈 자리로 쓰면 "왼쪽 끝에 고정" 과 구분되지
 * 않는다.
 */

/** 한 구간의 고정값 — `null` 은 자동. */
export type OrthoSplit = number | null;

/**
 * 저장에서 읽어 들인 목록. 019 가 적은 **수 하나도 그대로 읽는다**(`[그 수]`).
 *
 * 손상된 자리는 그 자리만 자동으로 읽는다 — 요소를 통째로 떨어뜨리지 않는 001 이래의
 * 규율이고, REQ-07 이 요구하는 그것이다.
 */
export function parseOrthoSplits(raw: unknown): OrthoSplit[] | undefined {
  if (typeof raw === 'number') return Number.isFinite(raw) ? trimAuto([raw]) : undefined;
  if (!Array.isArray(raw)) return undefined;
  return trimAuto(raw.map((one) => (typeof one === 'number' && Number.isFinite(one) ? one : null)));
}

/**
 * 저장하기 전에 **뒤쪽의 자동을 잘라낸다.** 전부 자동이면 부재다.
 *
 * 019 가 "쓴 적 없음" 과 "다 지웠음" 을 구분하지 않기로 한 그 선택이다 — 구분하면 저장
 * 왕복에 없던 키가 생기고, 그 키를 읽는 사람이 아무도 없다.
 */
export function trimAuto(splits: readonly OrthoSplit[]): OrthoSplit[] | undefined {
  let end = splits.length;
  while (end > 0 && splits[end - 1] === null) end -= 1;
  return end === 0 ? undefined : splits.slice(0, end);
}

/** 구간 `index` 의 고정값 — 없는 자리는 자동이다(REQ-07: 던지지 않는다). */
export function splitAt(splits: readonly OrthoSplit[] | undefined, index: number): number | undefined {
  const one = splits?.[index];
  return typeof one === 'number' ? one : undefined;
}

/** 구간 `index` 의 고정값을 갈아 끼운 **새 목록**. 모자란 앞자리는 자동으로 채운다. */
export function withSplit(
  splits: readonly OrthoSplit[] | undefined,
  index: number,
  value: OrthoSplit,
): OrthoSplit[] | undefined {
  if (index < 0) return splits === undefined ? undefined : [...splits];
  const next: OrthoSplit[] = [...(splits ?? [])];
  while (next.length <= index) next.push(null);
  next[index] = value;
  return trimAuto(next);
}

/**
 * 구간 `index` 를 **둘로 가른다** — 점 하나를 그 구간에 끼워 넣었을 때.
 *
 * 새로 난 두 구간은 **둘 다 자동**이다. 고정값은 "이 구간의 Z 를 여기에 세워라" 이고,
 * 구간이 갈리면 그 Z 는 더 이상 없다 — 옛 값을 한쪽에 물려주면 사용자가 찍지 않은 자리에
 * 선이 꺾인다.
 */
export function splitInserted(
  splits: readonly OrthoSplit[] | undefined,
  index: number,
): OrthoSplit[] | undefined {
  if (splits === undefined) return undefined;
  const next = [...splits];
  // 목록이 그 자리에 닿지 못하면 이미 자동이다 — 늘려 둘 까닭이 없다.
  if (index >= next.length) return trimAuto(next);
  next.splice(index, 1, null, null);
  return trimAuto(next);
}

/**
 * 구간 `index` 와 `index + 1` 을 **하나로 합친다** — 그 사이의 점을 뺐을 때.
 *
 * 합쳐진 구간도 **자동**이다. 두 옛 값 중 하나를 고를 근거가 없고, 고르면 점을 빼는 일이
 * 남은 선의 모양까지 바꾼다.
 */
export function splitRemoved(
  splits: readonly OrthoSplit[] | undefined,
  index: number,
): OrthoSplit[] | undefined {
  if (splits === undefined) return undefined;
  const next = [...splits];
  if (index >= next.length) return trimAuto(next);
  next.splice(index, Math.min(2, next.length - index), null);
  return trimAuto(next);
}
