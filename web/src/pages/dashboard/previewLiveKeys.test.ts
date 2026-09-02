// 미리보기에서 디바운스를 **건너뛰는** config 키.
//
// 미리보기는 200ms 디바운스된 config 로 그린다 — 잦은 편집이 재조회를 부르는 것을 막기
// 위해서다. 그런데 조회에 전혀 관여하지 않고 그리기만 바꾸는 값까지 늦추면, 값 글자를
// 끌 때 미리보기가 200ms 계단으로 따라와 드래그가 뚝뚝 끊긴다.
//
// 이 파일은 그 "건너뛰기" 규칙을 잠근다. 종전에는 목록과 병합 규칙이 다이얼로그 안에
// 있고 여기에 **사본**이 있었다 — 그래서 새 드래그 대상을 만들 때 목록에 넣는 것을 잊어도
// 테스트는 초록이었다(게이지 값 · 범례 · 그림 상자가 차례로 같은 증상으로 보고됐다).
// 이제 실물을 그대로 가져다 쓴다.

import { describe, expect, it } from 'vitest';

import { PREVIEW_LIVE_KEYS, mergeLivePreviewConfig as mergeLive } from './previewLiveKeys';

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

describe('끌어서 고치는 값은 모두 목록에 있다', () => {
  // 드래그가 200ms 계단으로 끊기는 증상은 언제나 "이 목록에 키가 빠졌다" 였다.
  // 대상을 새로 만들 때 함께 실패하도록 대상별로 한 줄씩 못박는다.
  it.each([
    ['게이지 현재값', ['value_scale', 'value_offset_x', 'value_offset_y']],
    ['게이지 그림 상자', ['gauge_size', 'gauge_offset_x', 'gauge_offset_y']],
    ['게이지 임계값 범례', ['threshold_legend_offset_x', 'threshold_legend_offset_y']],
    ['라인 차트 그림 상자', ['plot_size', 'plot_offset_x', 'plot_offset_y']],
    ['라인 차트 범례', ['legend']],
    ['패널 타이틀', ['title_font']],
  ])('%s', (_name, keys) => {
    for (const key of keys) {
      expect(PREVIEW_LIVE_KEYS as readonly string[]).toContain(key);
    }
  });
});
