// 캔버스 편집 도구 — **여섯** (SPEC-CANVAS-011 M3' · M8).
//
// ## 왜 모듈 하나를 세우는가
//
// 011 이전의 오버레이에는 **도구라는 개념이 없었다.** `useState` 일곱이 전부 그리는 것과
// 진행 중인 몸짓을 들었고(드롭 존 · 거절 · 드롭 강조 · 격자 붙임 · 격자 간격 · 배율 ·
// 마키), 포인터 경로는 언제나 "고르기" 한 가지 뜻으로만 돌았다. M3' 가 세우는 앵커 도구가
// **이 표면의 첫 도구 상태**다.
//
// 그 상태를 133KB 짜리 오버레이 안에 불리언 하나로 심을 수도 있었다. 심지 않는 까닭은
// M8 이 곧 **넷을 더하기 때문**이다(직선 · 꺾은 선 · 곡선 · 자유선 — plan §M8). 불리언
// 둘이 되는 순간 "앵커 도구와 곡선 도구가 함께 켜져 있다" 가 형상으로 가능해지고, 그때
// 더블클릭의 뜻은 둘 중 어느 쪽인지 말하지 못한다. 갈래 **하나**를 드는 값이면 그 상태가
// 표현 불가능하다(`CanvasGroupTools` 의 `confirming` 이 불리언 둘을 마다한 그 근거와 같다).
//
// ## 표가 **셋**인 것이 이 파일의 전부다
//
// 세 표 모두 `Record<CanvasTool, …>` 다. 갈래가 하나 늘면 컴파일러가 **세 자리를 함께**
// 가리킨다 — `if/else` 로 적으면 새 도구가 조용히 기본값으로 떨어지고, 그때 화면은
// "도구를 켰는데 아무 일도 없다" 만 말한다. `CanvasGroupTools` 의 `REFUSAL_KEY` 와
// 오버레이의 `HANDLE_ARIA_KEYS` 가 이미 세운 관용구다.
//
// ## 표가 **넷**이고, 둘은 M8 에서 **실제로 갈라졌다**
//
// M3'b 는 `TOOL_SHOWS_ANCHORS` 와 `TOOL_ANCHOR_GESTURE` 가 같은 값을 갖는 것이 **우연**
// 이라 적고, M8 에서 갈라진다고 예고했다. 갈라졌다: 연결선 도구 넷은 앵커를 **보여야**
// 하고(REQ-02-b — 앵커에서 눌러 앵커에서 놓는 것이 그 도구의 몸짓이다), 그 넷의
// 더블클릭은 앵커를 더하는 것이 **아니다**(REQ-05 — 선 위의 더블클릭은 중간점이다 · M10).
// 두 물음을 한 표로 접었다면 오늘 그 표를 쪼개야 했고, 쪼개는 사람은 두 뜻이 언제
// 갈렸는지 모른 채 한쪽을 골랐을 것이다.
//
// M8 이 더하는 넷째 표(`TOOL_CONNECTOR_ROUTE`)는 앞의 셋과 같은 규율이다 — 이 도구가
// 연결선을 긋는가, 긋는다면 어떤 `route` 로 긋는가를 **한 자리**에서 답한다. 오버레이가
// `tool === 'straight' || tool === 'elbow' || …` 를 적으면 다섯째 도구가 늘 때 그 자리가
// 조용히 빠지고, 화면은 "도구는 켜졌는데 그어지지 않는다" 만 말한다.
//
// **이 모듈은 DOM 도 React 도 모른다.** 유니온 하나 · 표 넷 · 순수 함수 하나뿐이다.
// 상태 칸(`useState`)은 오버레이가 든다 — 값을 드는 자리와 값의 뜻을 적는 자리는 다르다.
//
// @spec SPEC-CANVAS-011 REQ-02 · REQ-02' · REQ-03

import type { ConnectorRoute } from './connectorTypes';

/**
 * 도구 목록. **배열이 먼저이고 유니온이 그 파생이다** — 손으로 적은 유니온과 따로 적은
 * 목록은 언젠가 갈라지고, 그 갈라짐은 "도구는 켜지는데 단추가 없다"(또는 그 반대)로만
 * 보인다. `FIXED_ANCHOR_IDS` 가 `BOX_HANDLE_IDS` 에서 파생한 그 규율이다.
 *
 * 뒤의 넷은 **`ConnectorRoute` 와 같은 이름**이다(M8). 베낀 것이 아니라 아래
 * `TOOL_CONNECTOR_ROUTE` 가 그 둘을 이어 붙이며, 그 표가 `Record<CanvasTool, …>` 이자
 * 값이 `ConnectorRoute | null` 이므로 이름이 어긋나면 컴파일러가 먼저 운다.
 */
export const CANVAS_TOOLS = ['select', 'anchor', 'straight', 'elbow', 'curve', 'free'] as const;

/** 지금 손에 쥔 도구. 언제나 **정확히 하나**다. */
export type CanvasTool = (typeof CANVAS_TOOLS)[number];

/**
 * 아무 도구도 고르지 않은 상태 — 즉 **고르기**다.
 *
 * "도구 없음" 을 `null` 로 두지 않는 것에 뜻이 있다: 고르기도 몸짓을 가진 도구이고
 * (누름 · 마키 · 끌기), `null` 이면 그 몸짓들이 "도구가 없을 때의 특별한 경우" 로 읽힌다.
 * 갈래 하나로 두면 표 셋이 그 몸짓에 대해서도 답을 갖는다.
 */
export const DEFAULT_CANVAS_TOOL: CanvasTool = 'select';

/** 도구 이름 → i18n 키. 켜는 단추의 `aria-label` 과 `title` 이 같은 문구를 쓴다. */
export const TOOL_LABEL_KEYS: Readonly<Record<CanvasTool, string>> = {
  select: 'dashboard.canvas.edit.toolSelect',
  anchor: 'dashboard.canvas.edit.toolAnchor',
  straight: 'dashboard.canvas.edit.toolStraight',
  elbow: 'dashboard.canvas.edit.toolElbow',
  curve: 'dashboard.canvas.edit.toolCurve',
  free: 'dashboard.canvas.edit.toolFree',
};

/**
 * 이 도구가 켜진 동안 **앵커가 보이는가**(REQ-02-b).
 *
 * 앵커는 편집 중에만, 그리고 **도구가 켜진 동안에만** 보인다. 표시 전용 패널에는 오버레이
 * 자체가 서지 않으므로 그쪽은 이 표를 지나지도 않는다(AC-60).
 *
 * 보이는 범위는 **최상위 전부**다. 고른 것에만 세우면 M8 이 곧 그것을 되돌려야 한다 —
 * 잇는 일은 요소 **둘** 사이에서 일어나므로, 출발 앵커를 고르는 순간 도착 앵커가 사라지는
 * 화면이 된다.
 */
export const TOOL_SHOWS_ANCHORS: Readonly<Record<CanvasTool, boolean>> = {
  select: false,
  anchor: true,
  // 연결선 도구 넷은 앵커를 **보여야** 한다 — 앵커에서 눌러 앵커에서 놓는 것이 그
  // 도구의 몸짓 자체이므로, 보이지 않으면 어디를 눌러야 할지 화면이 말하지 않는다.
  straight: true,
  elbow: true,
  curve: true,
  free: true,
};

/**
 * 이 도구에서 **더블클릭이 앵커를 뜻하는가**(REQ-02' · REQ-02'-c).
 *
 * 켜져 있으면 더블클릭은 **언제나** 앵커다 — 그룹 부품 위에서도 그렇다. 009 의 그룹 진입은
 * 그동안 쉰다(AC-27 은 도구가 **꺼진** 상태를 재는 조항이다). 두 뜻을 대상에 따라 나누면
 * (도형 위면 앵커 · 부품 위면 진입) 사용자는 같은 손짓이 무엇을 할지 **눌러 봐야** 알게
 * 되고, 그것이 REQ-05-c 가 "두 몸짓은 섞이지 않는다" 로 막으려는 바로 그 상태다.
 *
 * 그래서 두 뜻을 가르는 것은 **대상이 아니라 도구**다.
 */
export const TOOL_ANCHOR_GESTURE: Readonly<Record<CanvasTool, boolean>> = {
  select: false,
  anchor: true,
  // **여기가 위 표와 갈리는 자리다**(M3'b 가 예고한 그 갈라짐). 연결선 도구의 더블클릭은
  // 앵커가 아니라 **선 위의 중간점**이다(REQ-05 · M10). 참으로 두면 잇는 손이 두 번째
  // 누름마다 도형 위에 앵커를 흩뿌린다.
  straight: false,
  elbow: false,
  curve: false,
  free: false,
};

/**
 * 이 도구가 **연결선을 긋는가**, 긋는다면 어떤 `route` 로 긋는가 (REQ-03 · REQ-04).
 *
 * `null` 은 "긋지 않는다" 이며, 그래서 도구 여섯이 **둘로 갈린다** — 긋는 넷과 긋지 않는
 * 둘. 오버레이는 이 표 하나만 보고 갈래를 타므로 `tool === 'straight' || …` 같은 목록이
 * 그쪽에 생기지 않는다. 그런 목록은 다섯째 연결선 도구가 늘 때 **조용히** 빠지고, 그때
 * 화면은 "도구는 켜지는데 아무것도 그어지지 않는다" 만 말한다.
 *
 * 값이 도구 이름과 **같은 문자열**인 것은 우연이 아니라 설계다(§`CANVAS_TOOLS`). 그래도
 * `tool as ConnectorRoute` 로 줄여 적지 않는다 — `as` 는 두 이름이 갈라지는 날에도 조용히
 * 통과하고, 표는 그날 컴파일러를 부른다.
 *
 * 그리는 법이 넷이어도 **만드는 몸짓은 하나**다(M8). 중간점이 없는 동안 넷은 같은 그림이며
 * (REQ-04-b · AC-50), 점을 만드는 몸짓이 갈리는 것은 M10 · M11 의 몫이다.
 */
export const TOOL_CONNECTOR_ROUTE: Readonly<Record<CanvasTool, ConnectorRoute | null>> = {
  select: null,
  anchor: null,
  straight: 'straight',
  elbow: 'elbow',
  curve: 'curve',
  free: 'free',
};

/**
 * 도구 단추를 눌렀을 때의 다음 도구.
 *
 * 켜져 있는 것을 다시 누르면 **고르기로 돌아온다.** 끄는 길을 따로 두지 않는 것이 요점이다 —
 * 도구가 여섯인 지금 "끄기" 단추는 여섯 자리에 각각 서거나 한 자리에 따로 서야 하고, 어느
 * 쪽이든 켜는 단추와 끄는 단추가 두 벌이 된다.
 */
export function toggleTool(current: CanvasTool, pressed: CanvasTool): CanvasTool {
  return current === pressed ? DEFAULT_CANVAS_TOOL : pressed;
}
