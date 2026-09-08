// 미리보기에서 디바운스를 **건너뛰는** config 키와 그 병합 규칙.
//
// 미리보기는 200ms 디바운스된 config 로 그린다 — 잦은 편집이 재조회를 부르는 것을 막기
// 위해서다. 그런데 조회에 전혀 관여하지 않고 그리기만 바꾸는 값까지 늦추면, 끌어서 옮길 때
// 미리보기가 200ms 계단으로 따라와 드래그가 뚝뚝 끊긴다.
//
// 다이얼로그 안에 목록을 두었더니 **끌어 옮기는 값을 새로 만들 때마다 여기에 넣는 것을
// 잊었다** — 게이지 값, 범례, 그림 상자가 차례로 같은 증상("드래그가 부드럽지 않다")으로
// 보고됐다. 목록과 병합 규칙을 밖으로 내어, 새 드래그 대상을 만들 때 함께 보이는 자리에 둔다.
//
// 넣을지 판정하는 기준은 하나다 — **데이터 조회에 관여하지 않고 그리기만 바꾸는가.**
// 끌어서 고치는 값은 예외 없이 그렇다.

/** 디바운스를 건너뛰는 키. 끌어서 고치는 값은 모두 여기 있어야 한다. */
export const PREVIEW_LIVE_KEYS = [
  // 게이지 — 현재값 글자, 그림 상자, 임계값 범례.
  'value_scale',
  // 값 글자 자리 — 도형 밖 오버레이가 되면서 패널 대비 백분율로 바뀌었다.
  'value_pos_x',
  'value_pos_y',
  'gauge_size',
  // 세로바 게이지의 도형 치수 — 손잡이가 축을 나눠 바꾼다(상자 배율이 아니다).
  'gauge_bar_width',
  'gauge_bar_height',
  'gauge_offset_x',
  'gauge_offset_y',
  'threshold_legend_offset_x',
  'threshold_legend_offset_y',
  // 임계값 범례 글자 크기 — 크기 손잡이가 이 값을 바꾼다. 게이지 작업은 SPEC 에 묶여
  // 있지 않다(SPEC-CHART-005 는 §6 에서 게이지를 범위 밖으로 적었다).
  'threshold_legend_font_size',
  // 라인 차트 — 그림 상자.
  'plot_size',
  // 바 차트 — 손잡이가 축을 나눈다(가로는 막대 굵기 px, 세로는 그림 영역 배율 %).
  'plot_size_y',
  'bar_size',
  'plot_offset_x',
  'plot_offset_y',
  // 라인 차트 범례 — 구성·위치·글자 모양이 한 오브젝트에 있고 전부 그리기 전용이다.
  'legend',
  // 통계 — 세 요소(본값·변화량·구간 통계)의 배치·크기·글자 모양이 각각 한 오브젝트에
  // 있고 전부 그리기 전용이다(SPEC-CHART-004).
  'value_layout',
  'delta_layout',
  'stats_layout',
  // 파이·바 — 그림 상자와 범례(SPEC-CHART-005). 범례 키는 두 패널이 같은 이름을 쓴다.
  'pie_size',
  'pie_offset_x',
  'pie_offset_y',
  'legend_offset_x',
  'legend_offset_y',
  'legend_font_size',
  'legend_font_family',
  'legend_font_color',
  'legend_position',
  'show_legend',
  // 타이틀 글자 모양(모든 패널 공통).
  'title_font',
] as const;

/**
 * 디바운스된 config 위에 **즉시 반영할 키만** 최신 draft 값을 덮는다.
 *
 * draft 에 없는 키는 지운다 — 초기화 버튼이 키를 지웠는데 디바운스된 옛 값이 남으면
 * "초기화했는데 200ms 동안 그대로" 가 된다.
 */
export function mergeLivePreviewConfig(
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
