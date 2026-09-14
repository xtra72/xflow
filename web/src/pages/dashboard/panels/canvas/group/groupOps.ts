// 묶기와 풀기 (SPEC-CANVAS-004 M5).
//
// **순수 함수 둘이고 DOM 을 모른다.** 배열을 받아 배열을 돌려주며, 어느 컨트롤이 그것을
// 부르는지 알지 못한다(그 배선은 M6 의 몫이다).
//
// ## 이 파일이 **하지 않는** 것 셋
//
//   1. **부품의 저장 좌표를 그룹 크기 조절 때 고치지 않는다.** 그 일은 여기 없다 —
//      그룹 상자 쓰기는 `patchNodeGeometry` 의 `case 'group':` 한 갈래를 지나고, 그
//      통로는 `el.geometry` 만 쓴다(가정 A17 · 008 불변식 J2 와 같은 자리). 그래서 이동 ·
//      8핸들 · 정렬 · 붙임 · 방향키가 한 줄도 새로 짜지 않고 그룹에 걸린다.
//   2. **`로컬 ÷ EXTENT × 상자변` 을 적지 않는다.** 그 나눗셈은 `projectPathPoints` 안
//      두 자리뿐이며, 여기는 `groupCoords` 의 변환을 부를 뿐이다(불변식 G2 · 008 J3).
//   3. **그룹 상자를 파생시키지 않는다.** 상자는 묶는 순간 계산해 적는 **저장된 값**이다
//      (가정 A16). 파생 상자 위에서는 그룹 크기 조절이 정의되지 않는다 — 8핸들로 늘린
//      즉시 파생이 되받아 원래 크기로 되돌아간다.
//
// ## 묶기와 풀기는 서로의 역이 **아니다**
//
// 좌표는 왕복하지만(상자변 ≤ `GROUP_LOCAL_EXTENT` 에서 정확히, 그 위에서는
// `상자변 ÷ (2 × EXTENT)` 안에서 — 가정 A15) 겉모습은 왕복하지 않는다. **그룹의 `rules`
// 가 버려지기 때문이다.** 규칙은 값이 아니라 **판정**이므로 N 개 부품에 복사하면 프레임당
// 평가가 1회에서 N회로 늘고(REQ-04 가 금지한 그것), 그중 한 부품의 표만 나중에 고쳐지는
// 순간 "같이 흐려지던 것이 따로 논다". 그 손실은 조용히 일어나서는 안 되며, 화면이 풀기
// **전에** 말한다 — 이 모듈은 그 말을 위한 사실을 `rulesLostByUngroup` 으로 내준다(A19).
//
// @spec SPEC-CANVAS-004 REQ-07

import {
  MIN_ELEMENT_EXTENT,
  type CanvasElement,
  type ElementStyle,
  type Geometry,
} from '../canvasConfig';
import { nextElementId } from '../canvasElementFactory';
import type { CanvasBox } from '../canvasGeometry';
import { elementsBounds } from '../scratchpad/scratchpadTypes';
import { frameKey } from './frameKey';
import { toAbsoluteGeometry, toLocalGeometry, widenDegenerateBox } from './groupCoords';
import { isGroup, type CanvasNode, type GroupElement } from './groupTypes';

// --- 거절 ------------------------------------------------------------------

/**
 * 묶기를 거절하는 사유. **화면이 이 값을 문구로 옮긴다**(REQ-07 — 거절은 조용하지 않다).
 *
 * - `nested` — 고른 것에 그룹이 섞여 있다(가정 A6). 조용히 평평하게 펴지도, 중첩을
 *   허용하지도 않는다. 중첩 상한이 없으면 좌표 오차가 깊이마다 곱해진다 — 한 겹에서는
 *   보이지 않는 값이 세 겹에서는 보인다.
 * - `tooFew` — 고른 것이 둘 미만이다. 부품 하나짜리 그룹은 정체성도 캐스케이드도 주지
 *   않으면서 목록에 층만 더한다.
 */
export type GroupRefusal = 'nested' | 'tooFew';

/** 풀기를 거절하는 사유. 고른 것이 그룹이 아니거나 아예 없을 때다. */
export type UngroupRefusal = 'notGroup';

/**
 * 묶기의 결과.
 *
 * **거절이면 `nodes` 가 입력 배열 그 참조다.** 값이 같은 새 배열이 아니라 같은 참조인 것이
 * 요점이다 — "한 바이트도 바뀌지 않았다" 를 호출부가 `===` 하나로 알 수 있고, 표면이
 * 쓸데없이 다시 그려지지 않는다.
 */
export interface GroupOutcome {
  nodes: readonly CanvasNode[];
  /** 만들어진 그룹 id. 거절이면 `undefined` 다. 배선이 이것 하나를 선택으로 세운다. */
  groupId?: string;
  /** 거절 사유. 성공이면 `undefined` 다. */
  refusal?: GroupRefusal;
}

/** 풀기의 결과. 거절이면 `nodes` 가 입력 배열 그 참조다(묶기와 같은 규율). */
export interface UngroupOutcome {
  nodes: readonly CanvasNode[];
  /** 최상위로 올라온 부품들의 **새** id. 배선이 이 전부를 선택으로 세운다. */
  liftedIds: readonly string[];
  refusal?: UngroupRefusal;
}

// --- 겉모습 접기 -----------------------------------------------------------

/**
 * `opacity` 를 0..1 로 죈다. 001 파서의 규율을 그대로 따른다 — 손상 값은 곱셈에 들이지
 * 않는다(비유한 수를 곱하면 `NaN` 이 부품 스타일에 눌러앉아 저장 왕복을 견딘다).
 */
function clampOpacity(v: number): number {
  if (!Number.isFinite(v)) return 1;
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

/** 저술된 `opacity` 만 곱셈에 들어온다. 미지정과 손상 값을 여기서 한 번에 가른다. */
function authoredOpacity(v: number | undefined): number | undefined {
  return v === undefined ? undefined : clampOpacity(v);
}

/**
 * 풀기의 겉모습 접기 — **4단 법칙이 아니라 2단이다.**
 *
 * 규칙 층(1·2층)이 없기 때문이다. 풀 때 살아 있는 것은 저술(3·4층)뿐이고, 그 둘 사이에서는
 * **구체성이 이긴다** — 부품이 적은 것이 그룹이 적은 것을 덮는다.
 *
 * **`opacity` 만 곱셈이다.** 그 속성의 법이 곱셈이므로(가정 A11) 베이킹도 곱셈이어야 한다.
 * 흐린 그룹 안의 흐린 부품은 **더** 흐려야 하고, 승자 독식이면 그중 하나가 사라진다.
 *
 * **양쪽 미지정이면 `opacity` 키를 만들지 않는다.** `1` 을 채우면 무동작 보장이 깨진다 —
 * "쓰지 않으면 001 과 결과가 **키 집합까지** 같다"(불변식 G12)가 그 이유이고, 여기서
 * `1` 을 채우는 것과 캐스케이드에서 채우는 것은 **같은 결함**이다.
 */
export function bakeStyle(
  groupStyle: ElementStyle | undefined,
  partStyle: ElementStyle,
): ElementStyle {
  // `opacity` 를 뺀 나머지는 속성별 `부품 ?? 그룹` 이다. 스프레드로 적으면 부품이 명시적
  // `undefined` 키를 들었을 때 그룹 값을 **지워** 버리므로, 정의된 값만 옮긴다.
  const baked: ElementStyle = {};
  const merge = (from: ElementStyle | undefined): void => {
    if (from === undefined) return;
    for (const [k, v] of Object.entries(from) as [keyof ElementStyle, unknown][]) {
      if (k === 'opacity' || v === undefined) continue;
      (baked as Record<string, unknown>)[k] = v;
    }
  };
  merge(groupStyle);
  merge(partStyle);

  const g = authoredOpacity(groupStyle?.opacity);
  const p = authoredOpacity(partStyle.opacity);
  if (g !== undefined || p !== undefined) {
    baked.opacity = clampOpacity((g ?? 1) * (p ?? 1));
  }
  return baked;
}

/**
 * 풀기가 **버리는** 규칙 행의 수. 0 이면 잃을 것이 없다.
 *
 * 화면은 이 수가 0 보다 클 때만 안내를 띄운다(REQ-07). **판정을 화면이 다시 짓지 않도록**
 * 여기서 내주는 것이 요점이다 — 같은 판정이 둘이 되면 "안내는 떴는데 실제로는 안 버렸다"
 * 나 그 반대가 가능해진다.
 */
export function rulesLostByUngroup(node: CanvasNode | undefined): number {
  if (node === undefined || !isGroup(node)) return 0;
  return node.rules?.length ?? 0;
}

// --- 기하만 갈아 끼운 사본 --------------------------------------------------

/**
 * 요소의 **기하만** 갈아 끼운 사본. 나머지 필드는 그대로 실려 간다.
 *
 * `kind` 로 갈라 놓은 것은 타입을 달래기 위해서가 아니라 **기하 형상별 오버로드를 실제로
 * 고르기 위해서**다. 합집합(`Geometry`)으로 부르면 좁혀지지 않은 결과가 돌아와 요소의
 * 기하 자리에 들어갈 수 없고, 그것을 단언으로 뚫으면 선 기하가 상자 자리에 앉는 실수를
 * 컴파일러가 더는 잡지 못한다(가정 A6 이 적은 그 자리다).
 *
 * `default:` 를 두지 않는다 — 여섯 번째 기하 형상이 생기면 컴파일러가 이 자리를 가리킨다.
 *
 * ## 변환할 기하를 **밖에서 줄 수 있다** (SPEC-CANVAS-009 M4)
 *
 * `source` 는 기본값이 `el.geometry` 라 묶기·풀기의 호출은 한 글자도 바뀌지 않는다.
 * 부품 기하 쓰기(`patchPartGeometry`)만 **캔버스 단위의 새 기하**를 실어 보낸다.
 *
 * **두 번째 역투영을 짓지 않는 것이 요점이다**(불변식 G2). 부품 편집이 제 나름의 역투영을
 * 적으면 풀기와 편집이 반올림·퇴화 처리에서 갈라질 수 있고, 그 갈라짐은 "풀었을 때와
 * 편집했을 때 좌표가 1 다르다" 로만 보인다. 같은 함수를 지나면 그 어긋남이 표현 불가능하다.
 *
 * 형상이 맞지 않으면(선 기하를 사각형 부품에 주는 따위) **받은 요소를 그대로** 돌려준다.
 * 그 조합은 호출부의 실수이지 관용할 입력이 아니고, 그때 기하를 갈아 끼우면 선 기하가
 * 상자 자리에 앉는다 — 예외를 내지 않는 것은 REQ-07 의 규율이다.
 */
function withGeometry(
  el: CanvasElement,
  box: CanvasBox,
  convert: typeof toLocalGeometry | typeof toAbsoluteGeometry,
  source: Geometry = el.geometry,
): CanvasElement {
  switch (el.kind) {
    case 'rect':
    case 'ellipse':
    case 'path':
      return 'w' in source ? { ...el, geometry: convert(source, box) } : el;
    case 'line':
      return 'x1' in source ? { ...el, geometry: convert(source, box) } : el;
    case 'text':
      return !('w' in source) && !('x1' in source)
        ? { ...el, geometry: convert(source, box) }
        : el;
  }
}

// --- 묶기 ------------------------------------------------------------------

/**
 * 고른 최상위 원소들을 그룹 하나로 묶는다.
 *
 * 상자는 `elementsBounds`(008 이 스크래치패드를 위해 이미 export 한 함수)로 잰다.
 * **다시 만들지 않는 것**이 요점이다 — 그 함수는 `w` 가 없는 문구 요소를 넓이 0 으로 보는
 * 규칙까지 이미 정해 두었고("글자의 실제 너비는 측정 결과이고 그것은 화면의 값이지 저술의
 * 값이 아니다"), 그래서 **묶기가 폰트에 따라 다른 상자를 내지 않는다.**
 *
 * 퇴화 넓히기는 **`toLocal` 을 부르기 전에** 일어난다. 가로선 둘만 고르면 상자 높이가 0 이고,
 * 그때 나누는 수가 여전히 0 이라 모든 부품의 로컬 y 가 폴백 0 으로 내려앉아 **상자 왼쪽 위
 * 모서리에 찌부러진다.** 그 결함은 예외를 내지 않고 저장 왕복을 견디며 화면으로만 드러난다
 * (007 0.2.0 이 같은 함정에서 같은 결론에 도달했다).
 *
 * 새 그룹은 **고른 것들 가운데 가장 뒤에 있던 것의 자리**에 선다 — 묶었다고 그림이 다른
 * 요소 앞뒤로 튀어서는 안 된다.
 */
export function groupNodes(
  nodes: readonly CanvasNode[],
  selectedIds: ReadonlySet<string>,
): GroupOutcome {
  const pickedIndices: number[] = [];
  for (let i = 0; i < nodes.length; i++) {
    if (selectedIds.has(nodes[i]!.id)) pickedIndices.push(i);
  }
  const picked = pickedIndices.map((i) => nodes[i]!);

  // 거절 둘. **순서가 뜻을 갖는다** — 그룹이 섞인 채로 하나만 고른 경우에 화면이 말해야
  // 하는 것은 "둘 이상 고르라" 가 아니라 "그룹은 못 묶는다" 다(고쳐야 할 것이 다르다).
  if (picked.some(isGroup)) return { nodes, refusal: 'nested' };
  if (picked.length < 2) return { nodes, refusal: 'tooFew' };

  const parts = picked as readonly CanvasElement[];
  const box = widenDegenerateBox(elementsBounds(parts));
  const groupId = nextElementId(nodes);

  // 부품은 **기하만** 옮긴 사본이다. `style` · `rules` · `binding` · `path` 는 그대로
  // 실려 간다 — 묶기는 겉모습을 건드리지 않는다(겉모습이 접히는 것은 풀 때뿐이다).
  // 경로 명령은 **제 상자 로컬**이라 그룹과 무관하고, 그래서 한 글자도 바뀌지 않는다.
  const group: GroupElement = {
    id: groupId,
    kind: 'group',
    geometry: { x: box.x, y: box.y, w: box.w, h: box.h },
    parts: parts.map((el) => withGeometry(el, box, toLocalGeometry)),
  } satisfies GroupElement;

  const last = pickedIndices[pickedIndices.length - 1];
  const next: CanvasNode[] = [];
  for (let i = 0; i < nodes.length; i++) {
    if (i === last) {
      next.push(group);
      continue;
    }
    if (selectedIds.has(nodes[i]!.id)) continue;
    next.push(nodes[i]!);
  }
  return { nodes: next, groupId };
}

// --- 풀기 ------------------------------------------------------------------

/**
 * 그룹 하나를 풀어 부품을 최상위로 올린다.
 *
 * 부품은 **그룹이 있던 배열 자리에 순서대로** 펼쳐진다. 좌표는 그룹의 **캔버스 단위 상자**로
 * 되돌리고(px 상자가 아니다 — 묶기와 푸는 쪽이 같은 함수를 지난다), id 는 `nextElementId`
 * 로 다시 발급한다. 부품 id 는 그룹 안에서만 유일했으므로 최상위에서는 충돌할 수 있다.
 *
 * **id 발급은 자라는 배열을 본다.** 고정된 원본을 보면 풀린 부품 셋이 전부 같은 id 를
 * 받는다 — 008 의 가져오기가 같은 자리에서 같은 함정을 적어 두었다.
 *
 * 그룹의 `style` · `binding` · `tween` 은 갈 곳이 없으므로 부품에 베이킹된다.
 * **`rules` 는 버린다**(A19 — 파일 머리말이 그 이유를 적었다).
 */
export function ungroupNode(nodes: readonly CanvasNode[], groupId: string): UngroupOutcome {
  const at = nodes.findIndex((n) => n.id === groupId);
  const target = at < 0 ? undefined : nodes[at];
  if (target === undefined || !isGroup(target)) {
    return { nodes, liftedIds: [], refusal: 'notGroup' };
  }

  const box: CanvasBox = target.geometry;
  const head = nodes.slice(0, at);
  const tail = nodes.slice(at + 1);
  const lifted: CanvasElement[] = [];
  for (const part of target.parts) {
    // **`...part` 로 편다 — `...target` 이 아니다.** 그룹 쪽을 펴면 `parts` · `symbol` ·
    // `rules` 같은 그룹 전용 필드가 부품에 흘러 들어가고, 스프레드는 초과 속성 검사를
    // 우회하므로 타입이 울지 않는다.
    const el: CanvasElement = withGeometry(part, box, toAbsoluteGeometry);
    el.id = nextElementId([...head, ...lifted, ...tail]);
    el.style = bakeStyle(target.style, part.style);
    if (el.binding === undefined && target.binding !== undefined) el.binding = target.binding;
    if (el.tween === undefined && target.tween !== undefined) el.tween = target.tween;
    lifted.push(el);
  }

  return {
    nodes: [...head, ...lifted, ...tail],
    liftedIds: lifted.map((el) => el.id),
  };
}

// --- 부품 찾기와 캔버스 단위 읽기 (SPEC-CANVAS-009 M3 · M4 · M5) ------------

/** 그룹 안에서 부품 하나를 찾은 결과. 그룹도 함께 내는 것은 상자가 거기 있기 때문이다. */
export interface PartLocation {
  group: GroupElement;
  part: CanvasElement;
  /** 최상위 배열에서 그룹의 자리. 분리가 "그룹 바로 뒤" 를 계산하는 데 쓴다. */
  groupIndex: number;
  /** `parts` 안에서 부품의 자리. */
  partIndex: number;
}

/**
 * 부품 하나를 찾는다. **찾기의 유일한 자리**다.
 *
 * 없는 그룹 · 그룹이 아닌 노드 · 없는 부품 전부 `undefined` 이며 예외를 내지 않는다
 * (REQ-07). 소비 측이 저마다 `nodes.find(...)` + `parts.find(...)` 를 적으면 그 판정이
 * 여럿이 되고, 그중 하나가 `isGroup` 확인을 빠뜨리는 날 최상위 요소의 `parts` 를 읽으려
 * 든다.
 */
export function findPart(
  nodes: readonly CanvasNode[],
  groupId: string,
  partId: string,
): PartLocation | undefined {
  const groupIndex = nodes.findIndex((n) => n.id === groupId);
  const group = groupIndex < 0 ? undefined : nodes[groupIndex];
  if (group === undefined || !isGroup(group)) return undefined;
  const partIndex = group.parts.findIndex((p) => p.id === partId);
  const part = partIndex < 0 ? undefined : group.parts[partIndex];
  if (part === undefined) return undefined;
  return { group, part, groupIndex, partIndex };
}

/**
 * 부품을 **캔버스 단위 좌표를 가진 요소**로 본 사본 — 선택 · 윤곽 · 8핸들 · 목록 수치 칸이
 * 모두 이것 하나를 본다(SPEC-CANVAS-009 M3 · M5).
 *
 * ## 왜 id 가 복합 키인가
 *
 * 오버레이의 네 통로(윤곽 상자 · 핸들 자리 · 드래그 상태 · 글자 폭 조회)는 전부 `el.id` 로
 * 프레임 상태를 뒤진다. 부품의 원래 id(`body`)를 실어 보내면 그 조회가 최상위 요소 `body`
 * 를 만나거나 아무것도 만나지 못하고, **문구 부품의 폭이 폴백으로 내려앉아** "글자를
 * 클릭하면 가끔 안 잡힌다" 가 된다(`frameKey` 머리말이 적은 그 부류다). 복합 키를 실으면
 * 그 넷이 한 글자도 바뀌지 않고 부품에 걸린다.
 *
 * **의사 노드이지 저장되는 값이 아니다.** 이 사본은 화면이 재고 그리는 데만 쓰이며, 저장
 * 좌표를 고치는 길은 아래 `patchPartGeometry` 하나뿐이다.
 */
export function partInCanvasUnits(
  nodes: readonly CanvasNode[],
  groupId: string,
  partId: string,
): CanvasElement | undefined {
  const found = findPart(nodes, groupId, partId);
  if (found === undefined) return undefined;
  const el = withGeometry(found.part, found.group.geometry, toAbsoluteGeometry);
  return { ...el, id: frameKey(groupId, partId) };
}

// --- 부품 기하 쓰기 (SPEC-CANVAS-009 M4) -----------------------------------

/**
 * 부품 하나의 **저장 좌표**를 캔버스 단위 기하로부터 갱신한다.
 *
 * ## 이 함수가 **하지 않는** 것 셋
 *
 *   1. **그룹 상자를 건드리지 않는다.** 상자는 저장된 값이지 부품에서 파생되는 값이
 *      아니다(004 가정 A16 · 009 REQ-03-a). 부품을 옮겼다고 상자가 따라 자라면 8핸들로
 *      늘린 크기가 그 다음 부품 이동에서 되돌아간다.
 *   2. **상자 밖으로 나가는 것을 막지 않는다**(REQ-03-b). 004 가 이미 "부품을 상자 밖으로
 *      밀어내면 그림이 상자를 넘친다" 를 대가로 받아들였고, 여기서 clamp 하면 그 대가만
 *      숨긴 채 사용자의 손이 벽에 걸린다.
 *   3. **새 역투영을 짓지 않는다.** 위 `withGeometry` 의 **두 번째 호출자**가 될 뿐이다
 *      (불변식 G2 — 좌표 공간을 넘는 자리를 넷으로 늘리지 않는다).
 *
 * 형제 노드와 형제 부품은 **참조 그대로** 실려 가고 새 배열이 나온다(004
 * `patchNodeGeometry` 의 규율). 바꿀 것이 없으면 **받은 배열 그 참조**를 돌려주므로,
 * 호출부가 `===` 하나로 "쓸 일이 없다" 를 안다.
 */
export function patchPartGeometry(
  nodes: readonly CanvasNode[],
  groupId: string,
  partId: string,
  nextAbsolute: Geometry,
): readonly CanvasNode[] {
  const found = findPart(nodes, groupId, partId);
  if (found === undefined) return nodes;

  const nextPart = withGeometry(found.part, found.group.geometry, toLocalGeometry, nextAbsolute);
  // 형상이 맞지 않아 `withGeometry` 가 받은 요소를 그대로 돌려준 경우다. 새 배열을 짓지
  // 않는다 — 한 글자도 바뀌지 않았음을 참조가 말한다.
  if (nextPart === found.part) return nodes;

  const parts = found.group.parts.map((p, i) => (i === found.partIndex ? nextPart : p));
  const group: GroupElement = { ...found.group, parts };
  return nodes.map((n, i) => (i === found.groupIndex ? group : n));
}

// --- 부품 분리 (SPEC-CANVAS-009 M6) ----------------------------------------

/**
 * 분리가 **버리는** 그룹 규칙 행의 수. 화면은 이 수가 0 보다 클 때만 확인을 묻는다.
 *
 * **판정을 화면이 다시 짓지 않는다**(REQ-05-c — 004 REQ-07 의 규율 그대로). 같은 판정이
 * 둘이 되면 "안내는 떴는데 실제로는 안 버렸다" 와 그 반대가 함께 가능해진다.
 *
 * 부품이 셋 이상이라 그룹이 살아남는 경우에도 **0 이 아니다**. 규칙 행이 표에서 지워지는
 * 것은 아니지만, 나간 부품에게는 그 N 행이 더는 걸리지 않는다 — 잃는 쪽은 그룹이 아니라
 * **그 부품**이고, 사용자가 분리 전에 알아야 하는 것은 그쪽이다.
 */
export function rulesLostByDetach(
  nodes: readonly CanvasNode[],
  groupId: string,
  partId: string,
): number {
  const found = findPart(nodes, groupId, partId);
  if (found === undefined) return 0;
  return rulesLostByUngroup(found.group);
}

/**
 * 부품 **하나**를 그룹 밖으로 빼낸다 — 풀기의 부분 적용이다(REQ-05).
 *
 * ## 부품이 둘 이하면 그대로 **푼다**
 *
 * 004 는 부품 하나짜리 그룹을 **읽기는** 허용하되 **만드는 것**은 거절한다(`tooFew`).
 * 분리가 그 금지된 상태를 새로 만들면 안 되므로(가정 A5), 남을 부품이 1 개 이하면
 * `ungroupNode` 를 그대로 부른다 — 조건만 여기서 가르고 **일은 그 함수가 한다.**
 *
 * 이것이 "같은 함수를 쓴다"(REQ-05 · AC-33)의 절반이고, 나머지 절반은 아래 ≥ 3 갈래가
 * `withGeometry` · `bakeStyle` · `nextElementId` 라는 **바로 그 셋**을 부르는 것이다.
 * 좌표 환산도 스타일 굽기도 새로 적지 않는다.
 *
 * ## 올라온 부품은 **그룹 바로 뒤**에 선다
 *
 * 그룹의 부품은 그룹 자리에서 연달아 그려지므로(`walkDrawables` 의 2단 순서), 그룹 바로
 * 뒤가 곧 그 부품이 있던 그리기 순서다. 맨 뒤에 붙이면 분리한 순간 그림이 다른 요소
 * 위로 튀어 오른다.
 *
 * 거절(`notGroup`)이면 `nodes` 가 **입력 배열 그 참조**다(묶기·풀기와 같은 규율).
 */
export function detachPart(
  nodes: readonly CanvasNode[],
  groupId: string,
  partId: string,
): UngroupOutcome {
  const found = findPart(nodes, groupId, partId);
  if (found === undefined) return { nodes, liftedIds: [], refusal: 'notGroup' };

  // 남을 부품이 1 개 이하 → 그룹을 남겨 둘 이유가 없다. 004 가 만들기를 거절하는 그
  // 상태(부품 1개 그룹)를 분리가 새로 지어서는 안 된다(A5 · REQ-05-a · REQ-05-b).
  if (found.group.parts.length <= 2) return ungroupNode(nodes, groupId);

  const box: CanvasBox = found.group.geometry;
  const el: CanvasElement = withGeometry(found.part, box, toAbsoluteGeometry);
  el.id = nextElementId(nodes);
  el.style = bakeStyle(found.group.style, found.part.style);
  if (el.binding === undefined && found.group.binding !== undefined) el.binding = found.group.binding;
  if (el.tween === undefined && found.group.tween !== undefined) el.tween = found.group.tween;

  const group: GroupElement = {
    ...found.group,
    parts: found.group.parts.filter((_, i) => i !== found.partIndex),
  };

  const next: CanvasNode[] = [...nodes];
  next[found.groupIndex] = group;
  next.splice(found.groupIndex + 1, 0, el);
  return { nodes: next, liftedIds: [el.id] };
}

// --- 최소 크기 재수출 ------------------------------------------------------

/**
 * 퇴화 넓히기가 쓰는 최소 변. 시험이 상자 두 변을 이 값과 견주므로 여기서 한 번 더 이름을
 * 준다 — **값을 베끼지 않는다**(001 이 정한 그 상수 그대로다).
 */
export { MIN_ELEMENT_EXTENT };
