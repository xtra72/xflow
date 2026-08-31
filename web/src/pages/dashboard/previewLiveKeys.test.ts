// 미리보기에서 디바운스를 **건너뛰는** config 키.
//
// 미리보기는 200ms 디바운스된 config 로 그린다 — 잦은 편집이 재조회를 부르는 것을 막기
// 위해서다. 그런데 조회에 전혀 관여하지 않고 그리기만 바꾸는 값까지 늦추면, 값 글자를
// 끌 때 미리보기가 200ms 계단으로 따라와 드래그가 뚝뚝 끊긴다.
//
// 이 파일은 그 "건너뛰기" 규칙을 잠근다. 규칙 자체는 다이얼로그 안에 있으므로 여기서는
// 같은 병합 규칙을 재현해 의미를 고정한다.

import { describe, expect, it } from 'vitest';

/** 다이얼로그의 `PREVIEW_LIVE_KEYS` 와 같은 목록. */
const PREVIEW_LIVE_KEYS = ['value_scale', 'value_offset_x', 'value_offset_y'] as const;

/** 다이얼로그의 `livePreviewConfig` 병합 규칙. */
function mergeLive(
  debounced: Record<string, unknown>,
  draft: Record<string, unknown>,
): Record<string, unknown> {
  const live: Record<string, unknown> = { ...debounced };
  for (const key of PREVIEW_LIVE_KEYS) {
    if (key in draft) live[key] = draft[key];
    else delete live[key];
  }
  return live;
}

describe('디바운스 건너뛰기', () => {
  it('크기·위치는 최신 draft 값을 쓴다', () => {
    const merged = mergeLive(
      { value_scale: 1, value_offset_x: 0, min: 0 },
      { value_scale: 2, value_offset_x: 30, min: 0 },
    );
    expect(merged.value_scale).toBe(2);
    expect(merged.value_offset_x).toBe(30);
  });

  it('조회 축은 디바운스된 값을 그대로 둔다 — 재조회 억제가 목적이다', () => {
    const merged = mergeLive(
      { interval_ms: 60_000, store_source: { agent_name: 'a' } },
      { interval_ms: 1_000, store_source: { agent_name: 'b' } },
    );
    expect(merged.interval_ms).toBe(60_000);
    expect(merged.store_source).toEqual({ agent_name: 'a' });
  });

  it('draft 에서 지워진 키는 미리보기에서도 지워진다 — 초기화가 즉시 보인다', () => {
    // `초기화` 버튼은 세 키를 undefined 로 지운다. 디바운스된 옛 값이 남으면
    // 초기화했는데 200ms 동안 그대로 보인다.
    const merged = mergeLive({ value_scale: 2, value_offset_y: 40 }, { min: 0 });
    expect('value_scale' in merged).toBe(false);
    expect('value_offset_y' in merged).toBe(false);
  });

  it('draft 에 undefined 로 들어 있으면 그 값을 쓴다', () => {
    const merged = mergeLive({ value_scale: 2 }, { value_scale: undefined });
    expect(merged.value_scale).toBeUndefined();
  });
});
