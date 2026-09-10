// 스크래치패드 항목의 형상과 그 관용 파서 (SPEC-CANVAS-008 M8 · REQ-05 · REQ-08).
//
// **항목의 몸체는 `CanvasElement[]` 이지 컨테이너 노드가 아니다.** 형상만 보면 그것은
// SPEC-CANVAS-004 의 `group` 이지만 004 는 아직 한 줄도 구현되지 않았고, 그 스키마는
// 캐스케이드 4단 · 중첩 투영 3종 · 복합 프레임 키를 함께 끌고 온다 — 스크래치패드가
// 요구하는 것은 그중 하나도 없다. 그래서 평평한 목록을 저장하고 놓기는 형제 요소들을
// 옮겨 찍는다(spec.md §004 의 `group` 에 기대지 않는다 (c)).
//
// **그 선택은 004 가 착륙해도 한 바이트를 바꾸지 않는다**(REQ-08). `GroupElement.parts` 의
// 타입이 정확히 `CanvasElement[]` 이므로 저장 형상이 그대로 서고, 004 는 "그룹으로 놓기"
// 라는 **놓기 방식 하나**를 더할 뿐이다. 반대 방향은 성립하지 않는다 — 지금 `group` 을
// 저장 형상으로 삼으면 아직 배달되지 않았고 개정이 예정된 스키마 위에 영속 자료를 얼린다.
//
// **DOM 무의존이다.** React 도 i18n 도 `localStorage` 도 부르지 않으므로 jsdom 없이 전량
// 단위 시험된다 — `canvasHitTest.ts` · `canvasEditArrange.ts` 와 같은 규율이다. 저장소를
// 실제로 만지는 일은 옆 파일(`scratchpadStore.ts`)의 몫이다.
//
// **요소 파서를 두 번째로 만들지 않는다**(spec.md §스크래치패드 — 저장 형상과 사는 곳).
// 항목 안의 요소는 001 의 `parseElements` 를 **그대로** 지난다. 그 함수가 이 SPEC 에서
// export 된 유일한 이유가 그것이며, 형상만 베낀 사본을 두면 "config 에서는 살아나는데
// 서랍에서는 사라지는 요소" 가 생긴다.
//
// @spec SPEC-CANVAS-008 REQ-05 · REQ-08 · AC-08

import { parseElements, type CanvasElement, type Geometry } from '../canvasConfig';
import type { CanvasBox, CanvasPoint } from '../canvasGeometry';

// --- 형상 ---------------------------------------------------------------

/**
 * 스크래치패드 항목 하나 — 사용자가 캔버스에서 조립해 서랍에 넣어 둔 것.
 *
 * `origin` 을 따로 드는 이유: 놓을 때 "놓인 지점을 좌상단으로" 옮기려면 저장 시점의
 * 좌상단이 있어야 델타가 나온다(REQ-04). 요소들에서 매번 다시 계산할 수도 있지만, 그러면
 * 저장한 것과 놓는 것이 **서로 다른 시점의 계산**을 보게 되고 그 어긋남은 화면에서만
 * 드러난다. 값으로 못박아 둔다.
 */
export interface ScratchpadEntry {
  /** 기기 안에서 유일. 발급은 저장소의 몫이다. */
  id: string;
  /**
   * 사용자가 고칠 수 있는 이름. **빈 문자열은 "아직 이름이 없다"** 는 뜻이며 그때 화면이
   * 자동 이름을 보인다(REQ-06).
   *
   * 자동 이름을 여기에 **넣지 않는 것**에 뜻이 있다: 자동 이름은 번역이 필요한 문구이고,
   * 이 파일은 i18n 을 알지 못한다. 저장 시점의 로케일로 이름을 굳혀 두면 언어를 바꾼
   * 뒤에도 옛 로케일의 이름이 서랍에 남는다.
   */
  name: string;
  /** epoch ms — 이 저장소의 시각 규약(프로젝트 전역: 시각은 언제나 epoch millis). */
  created: number;
  /** 저장 시점 묶음의 좌상단(정수 캔버스 단위). */
  origin: CanvasPoint;
  /** 사본. 004 의 `GroupElement.parts` 와 **같은 타입**이다(REQ-08). */
  elements: CanvasElement[];
}

// --- 예산 ---------------------------------------------------------------

/**
 * 항목 수 상한. `localStorage` 는 출처당 대략 5MB 이고 그 몫을 `xflow-ui` 와 나눠 쓴다.
 *
 * 넘으면 **거절하고 말한다** — 오래된 항목을 몰래 지우지 않는다(REQ-05). 조용한 축출은
 * "저장했는데 나중에 보니 없더라" 로 나타나고, 그 실패는 사용자가 원인을 찾을 수 없다.
 */
export const SCRATCHPAD_MAX_ENTRIES = 50;

/** 총 바이트 상한. 위와 같은 정책 — 넘으면 거절하고 말한다. */
export const SCRATCHPAD_MAX_BYTES = 256 * 1024;

/**
 * 항목 목록의 **UTF-8 바이트 수**.
 *
 * `JSON.stringify(...).length` 를 쓰지 않는다 — 그것은 UTF-16 코드 단위 수이고, 한글
 * 이름 한 글자는 코드 단위로 1 이지만 저장소가 실제로 무는 것은 **3 바이트**다. 이름을
 * 한글로 적는 사용자에게 예산이 세 배로 보이는 셈이며, 그 어긋남은 예산 근처에서만
 * 드러난다(= 가장 늦게 드러난다).
 */
export function scratchpadBytes(entries: readonly ScratchpadEntry[]): number {
  return new TextEncoder().encode(JSON.stringify(entries)).length;
}

// --- 기하 -------------------------------------------------------------

/**
 * 기하 하나가 차지하는 캔버스 단위 상자.
 *
 * 상자 기하(rect · ellipse · path)의 `x`/`y` 를 **그대로** 왼쪽 위로 쓴다. 파서가
 * `isDegenerateBox` 로 `w`·`h` 가 `MIN_ELEMENT_EXTENT` 이상임을 이미 보장하므로 음수 폭이
 * 여기로 들어올 길이 없고, `Math.min(x, x + w)` 는 **검증되지 않는 가드**가 되어 읽는
 * 사람에게 "음수 폭이 올 수도 있다" 고 거짓말한다.
 *
 * 문구 요소는 넓이가 없다 — 글자의 실제 너비는 측정 결과이고(`textWidths`) 그것은 화면의
 * 값이지 저술의 값이 아니다. 묶음의 좌상단을 정하는 데에 측정값을 끌어들이면 같은 항목이
 * 폰트에 따라 다른 자리에 놓인다.
 */
function geometryBox(geometry: Geometry): CanvasBox {
  if ('w' in geometry) return { x: geometry.x, y: geometry.y, w: geometry.w, h: geometry.h };
  if ('x1' in geometry) {
    const x = Math.min(geometry.x1, geometry.x2);
    const y = Math.min(geometry.y1, geometry.y2);
    return {
      x,
      y,
      w: Math.max(geometry.x1, geometry.x2) - x,
      h: Math.max(geometry.y1, geometry.y2) - y,
    };
  }
  return { x: geometry.x, y: geometry.y, w: 0, h: 0 };
}

/**
 * 요소들이 이루는 바깥 상자(정수 캔버스 단위).
 *
 * 빈 목록이면 원점의 0 상자다. 그 값이 쓰이는 자리는 없다 — 저장은 빈 선택을 거절하고
 * (REQ-03) 놓기는 빈 항목을 만들지 않는다 — 그러나 `-Infinity` 를 밖으로 흘리는 것보다
 * 낫다. `reduce` 의 초기값 없는 형태는 빈 배열에서 던진다.
 */
export function elementsBounds(elements: readonly CanvasElement[]): CanvasBox {
  if (elements.length === 0) return { x: 0, y: 0, w: 0, h: 0 };
  let left = Number.POSITIVE_INFINITY;
  let top = Number.POSITIVE_INFINITY;
  let right = Number.NEGATIVE_INFINITY;
  let bottom = Number.NEGATIVE_INFINITY;
  for (const el of elements) {
    const box = geometryBox(el.geometry);
    left = Math.min(left, box.x);
    top = Math.min(top, box.y);
    right = Math.max(right, box.x + box.w);
    bottom = Math.max(bottom, box.y + box.h);
  }
  return { x: left, y: top, w: right - left, h: bottom - top };
}

/**
 * 묶음의 좌상단. `elementsBounds` 에서 뽑아 쓴다 — 좌상단을 재는 규칙이 둘이 되면
 * "저장할 때 잡은 자리" 와 "놓을 때 미는 자리" 가 갈라진다.
 */
export function elementsOrigin(elements: readonly CanvasElement[]): CanvasPoint {
  const box = elementsBounds(elements);
  return { x: box.x, y: box.y };
}

// --- 사본 ---------------------------------------------------------------

/**
 * 요소들의 **깊은 사본**.
 *
 * JSON 왕복인 것에 뜻이 있다: 서랍에 들어가는 것은 어차피 JSON 을 지나므로, 저장하는
 * 순간의 메모리 값과 **다시 읽었을 때의 값이 같아진다**. `structuredClone` 이면 메모리
 * 에서만 살아남는 필드(예: `undefined` 를 명시적으로 든 키)가 생겨 두 값이 갈라진다.
 *
 * 얕은 사본이 안 되는 이유는 공유다 — `style` · `path` · `rules` 를 그대로 나눠 가지면
 * 캔버스에서 놓인 요소를 고칠 때 서랍 속 원본까지 함께 바뀔 수 있다.
 */
export function cloneElements(elements: readonly CanvasElement[]): CanvasElement[] {
  return JSON.parse(JSON.stringify(elements)) as CanvasElement[];
}

// --- 파서 ---------------------------------------------------------------

/** 유한한 수만 통과시킨다. 좌표는 정수로 굳힌다 — 저장소 자료도 손으로 고쳐질 수 있다. */
function coordinate(v: unknown): number | undefined {
  return typeof v === 'number' && Number.isFinite(v) ? Math.round(v) : undefined;
}

/** 좌상단. 두 축 가운데 하나라도 읽히지 않으면 **부재**다 — 반쪽 원점은 번역을 어긋낸다. */
function parseOrigin(raw: unknown): CanvasPoint | undefined {
  if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) return undefined;
  const r = raw as Record<string, unknown>;
  const x = coordinate(r.x);
  const y = coordinate(r.y);
  return x === undefined || y === undefined ? undefined : { x, y };
}

/**
 * 항목 1건. **정체성이 성립하지 않으면 버린다**(001 파서와 같은 규율: 버리는 것은 정체성이
 * 없을 때뿐이다).
 *
 * 정체성은 둘이다 — `id` 가 있고, **그릴 요소가 하나라도 살아남는가**. 요소가 모두 떨어진
 * 항목은 목록에 이름만 남아 놓아도 아무 일이 없는 행이 되며, 그것은 사용자가 화면에서
 * 고칠 수 없는 상태다.
 *
 * 나머지는 전부 되살린다: 이름이 없으면 빈 이름(자동 이름이 뜬다), 시각이 없으면 0,
 * 좌상단이 없으면 **요소에서 다시 계산한다**. 계산으로 메울 수 있는 것을 탈락 사유로
 * 삼지 않는 것이 001 이 기하 손상에 대해 세운 정책과 같은 규율이다.
 */
export function parseScratchpadEntry(raw: unknown): ScratchpadEntry | null {
  if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const r = raw as Record<string, unknown>;
  if (typeof r.id !== 'string' || r.id === '') return null;

  const elements = parseElements(r.elements);
  if (elements.length === 0) return null;

  return {
    id: r.id,
    name: typeof r.name === 'string' ? r.name : '',
    created: typeof r.created === 'number' && Number.isFinite(r.created) ? r.created : 0,
    origin: parseOrigin(r.origin) ?? elementsOrigin(elements),
    elements,
  };
}

/**
 * 항목 목록. **예외를 던지지 않는다**(REQ-05). 배열이 아니면 빈 목록이고, 항목 단위로
 * 살린다 — 하나가 깨졌다고 나머지 마흔아홉을 잃지 않는다.
 *
 * id 중복은 **먼저 온 것이 이긴다** — `parseElements` 가 요소에 대해 세운 규칙과 같다.
 * 목록의 행이 id 로 지목되므로 중복이 남으면 이름 바꾸기와 지우기가 어느 쪽을 가리키는지
 * 정할 수 없다.
 */
export function parseScratchpad(raw: unknown): ScratchpadEntry[] {
  if (!Array.isArray(raw)) return [];
  const out: ScratchpadEntry[] = [];
  const seen = new Set<string>();
  for (const item of raw) {
    const entry = parseScratchpadEntry(item);
    if (entry === null) continue;
    if (seen.has(entry.id)) continue;
    seen.add(entry.id);
    out.push(entry);
  }
  return out;
}

/** 목록 안에서 쓰이지 않은 항목 id. 결정적이라 시험이 값을 예측할 수 있다. */
export function nextScratchpadId(entries: readonly ScratchpadEntry[]): string {
  const used = new Set(entries.map((e) => e.id));
  let n = 1;
  while (used.has(`sp-${n}`)) n++;
  return `sp-${n}`;
}
