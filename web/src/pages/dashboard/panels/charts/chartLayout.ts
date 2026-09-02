// 라인 차트의 **배치** — 그림 상자의 크기·자리와 범례의 자리.
//
// 이 값들은 설정 칸이 아니라 **미리보기에서 끌어** 고친다. 그래서 되돌리는 일도 한 번에
// 되어야 한다: 그림을 옮기고 범례도 옮긴 뒤 "원래대로" 를 하려면 네 개의 키를 각각 찾아
// 지워야 하는데, 그 키들은 화면 어디에도 이름으로 나오지 않는다.
//
// 판정(되돌릴 것이 있는가)과 동작(무엇을 지우는가)을 한 파일에 둔다. 둘이 갈리면 판정이
// 빠뜨린 키는 버튼이 안 뜬 채로 남고, 동작이 빠뜨린 키는 눌러도 지워지지 않는다.

/** 배치를 이루는 패널 config 키 — 그림 상자 쪽. */
const PLOT_KEYS = ['plot_size', 'plot_offset_x', 'plot_offset_y'] as const;
/** 범례 쪽은 표시 옵션과 한 오브젝트에 산다 — 통째로 지우지 않고 이 키만 뺀다. */
const LEGEND_OFFSET_KEYS = ['offset_x', 'offset_y'] as const;

/** 기본값에서 벗어난 배치가 있는가. 없으면 되돌릴 것이 없다. */
export function isChartLayoutDirty(config: Record<string, unknown> | undefined): boolean {
  if (!config) return false;
  // 크기는 `undefined` 가 기본이고 자리는 0 이 기본이다 — 두 기준이 다르므로 함께 적는다.
  if (config.plot_size !== undefined) return true;
  if (config.plot_offset_x || config.plot_offset_y) return true;
  const legend = config.legend as Record<string, unknown> | undefined;
  return !!legend?.offset_x || !!legend?.offset_y;
}

/**
 * 배치를 기본값으로 되돌리는 config 패치.
 *
 * 범례 오브젝트는 자리 키만 빼고 나머지(이름·선·마지막 값·글자 모양)를 남긴다. 통째로
 * 지우면 "자리를 되돌렸더니 글꼴도 초기화됐다" 가 된다. 남는 것이 없으면 오브젝트 자체를
 * 지운다 — 빈 오브젝트가 남으면 config 에 뜻 없는 흔적이 쌓인다.
 */
export function chartLayoutResetPatch(
  config: Record<string, unknown> | undefined,
): Record<string, unknown> {
  const patch: Record<string, unknown> = {};
  for (const key of PLOT_KEYS) patch[key] = undefined;

  const legend = { ...((config?.legend as Record<string, unknown> | undefined) ?? {}) };
  for (const key of LEGEND_OFFSET_KEYS) delete legend[key];
  patch.legend = Object.keys(legend).length > 0 ? legend : undefined;
  return patch;
}
