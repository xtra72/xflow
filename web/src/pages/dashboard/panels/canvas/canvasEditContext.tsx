// 캔버스 편집 선택 상태 공유 (SPEC-CANVAS-002 T4).
//
// 미리보기(`CanvasPanel`)와 요소 목록 편집기(`CanvasElementsEditor`)는
// `PanelSettingsDialog.tsx` 안의 **서로 다른 자리**에 마운트된다. 그 파일(8,000행+)에
// 상태를 더하는 것은 001 §위험 R4 를 되살리는 일이므로, 상태를 여기 작은 모듈에 두고
// 다이얼로그에는 두 마운트 지점을 감싸는 줄만 더한다(그 감싸기는 T11 의 몫이다).
//
// **provider 없이도 동작해야 한다.** 대시보드에 놓인 캔버스 패널 곁에는 목록 편집기가
// 없다 — 그때 선택은 오버레이만 움직이면 되므로 **컨텍스트 기본값이 곧 로컬 선택**이다
// (spec.md §클릭 → 속성 편집). 그래서 컨텍스트의 기본값을 `null` 로 두고, 소비 훅이
// provider 가 없을 때 자기 로컬 상태로 떨어진다. "provider 를 잊으면 조용히 죽는" 형태를
// 만들지 않기 위한 설계다.
//
// **선택은 런타임 상태이며 config 에 저장하지 않는다.** `panelEditSelection.ts` 헤더가
// 이미 못박은 규율을 그대로 물려받는다 — 저장하면 다음에 열 때도 무엇인가 골라져 있고,
// 그것을 푸는 방법이 화면에 없다(가정 A4).
//
// **선택 키는 언제나 최상위 배열 원소의 id 다**(REQ-06). SPEC-CANVAS-004 는 부품에
// 복합 키(`groupId/partId`)를 쓰되 최상위 키는 001·002 와 같게 유지하기로 이미 설계되어
// 있으므로, 지금 이 키를 쓰면 004 가 키를 갈아 끼울 일이 없다.
//
// 이 파일은 **컴포넌트를 내보내지 않는다.** React 19 에서는 컨텍스트 객체 자체가
// provider 이므로(`<CanvasEditSelectionContext value={...}>`) 감싸기용 컴포넌트가 필요
// 없고, 그 덕에 `react-refresh/only-export-components`(Fast Refresh) 경고도 생기지 않는다
// — `panelChromeContext.ts` 가 Provider 를 별도 파일로 뺀 것과 같은 이유를, 별도 파일
// 없이 해결한 것이다.
//
// **이 파일에는 채널이 둘 있다.** 선택(위)과 **라이브 시리즈 키 집합**(아래)이다. 둘을 한
// 값으로 합치지 않은 이유는 소비자가 다르기 때문이다 — 선택은 포인터를 끄는 동안 매 프레임
// 바뀌는 뜨거운 상태이고, 시리즈 키 집합은 조회 결과가 실제로 갈릴 때만 바뀌는 차가운
// 상태다. 한 값에 합치면 폴링 한 번이 오버레이 전체를 다시 그리게 하고, 반대로 선택 한
// 번이 목록 편집기의 선택지를 다시 계산하게 한다. 컨텍스트를 둘로 두면 각자 자기 것이
// 바뀔 때만 깨어난다.
//
// @spec SPEC-CANVAS-002 REQ-04 / REQ-06

import { createContext, useCallback, useContext, useMemo, useState } from 'react';

import { EMPTY_SELECTION, type PanelSelection } from '../charts/panelEditSelection';
import { parseFrameKey } from './group/frameKey';

// --- 타입 ---------------------------------------------------------------

/** 고른 요소들. 키는 최상위 배열 원소의 id 다(REQ-06). */
export type CanvasSelection = PanelSelection<string>;

/** 캔버스 편집이 공유하는 것 전부. */
export interface CanvasEditSelectionValue {
  /** 지금 골라진 요소 id 들. 순서는 뜻이 없다. */
  selection: CanvasSelection;
  /** 선택 교체. 값 자체를 넘긴다 — 다음 선택 계산은 `nextSelection` 이 소유한다. */
  setSelection: (next: CanvasSelection) => void;
  /**
   * **캔버스가 자동으로 펼친 행**의 id. 단일 선택일 때만 값이 있고, 0개·2개 이상이면
   * `null` 이다.
   *
   * 목록 편집기(T10)가 이 값을 소비한다. 둘 이상 선택되면 아무 행도 자동으로 펼치지
   * 않는 규칙(AC-06)이 여기 한 줄로 들어 있다 — 펼침이 쌓이면 이미 고친 "설정이 모두
   * 펼쳐져 복잡하다" 로 되돌아간다. 사용자가 손으로 펼친 행은 이 값과 무관하므로
   * 접히지 않는다(목록 편집기가 자기 `expandedIds` 와 이 한 개를 따로 든다).
   */
  autoExpandedId: string | null;
}

// --- 컨텍스트 -----------------------------------------------------------

/**
 * 선택 공유 컨텍스트. 기본값이 `null` 인 것이 **provider 없이 동작한다**는 계약 그
 * 자체다 — `null` 은 "공유할 상대가 없다" 는 뜻이고, 그때 소비 훅이 로컬 상태로 떨어진다.
 *
 * React 19 이므로 감싸는 쪽은 이 객체를 그대로 쓴다:
 * `<CanvasEditSelectionContext value={state}>…</CanvasEditSelectionContext>` (T11).
 */
export const CanvasEditSelectionContext = createContext<CanvasEditSelectionValue | null>(null);

// --- 순수 도우미 ---------------------------------------------------------

/**
 * 선택으로부터 **캔버스가 펼칠 행** 하나를 정한다.
 *
 * 상태를 하나 더 들지 않고 선택에서 파생시키는 이유: 두 값을 따로 들면 선택과 펼침이
 * 어긋난 중간 상태가 생기고("골라져 있는데 다른 행이 펼쳐져 있다") 그 상태를 되돌릴
 * 경로가 화면에 없다.
 */
export function canvasAutoExpandedId(selection: CanvasSelection): string | null {
  if (selection.size !== 1) return null;
  // 배열 전개 뒤 첫 원소 — 크기가 1 임을 위에서 이미 가렸으므로 반드시 있다.
  //
  // **부품 키가 오면 그 그룹의 id 를 낸다**(SPEC-CANVAS-009 M3). 목록에서 펼칠 수 있는
  // 행은 최상위 노드의 행뿐이고, 부품 행은 그 그룹 행이 펼쳐져야 비로소 보인다. 복합 키를
  // 그대로 내려보내면 `rowRefs` 와 `isExpanded` 가 어느 행과도 만나지 못해 **캔버스에서
  // 부품을 골라도 목록이 꼼짝하지 않는다.**
  //
  // 분해는 `parseFrameKey` 가 한다 — 구분자 리터럴이 이 파일에 없다(AC-04).
  return parseFrameKey([...selection][0]!).nodeId;
}

// --- 훅 -----------------------------------------------------------------

/**
 * 선택 상태 한 벌을 만든다. **감싸는 쪽(T11)과 provider 없는 소비자가 같은 구현을 쓴다** —
 * 두 벌이 되면 "다이얼로그에서는 되는데 대시보드에서는 안 된다" 가 생긴다.
 */
export function useCanvasEditSelectionState(): CanvasEditSelectionValue {
  const [selection, setSelectionState] = useState<CanvasSelection>(EMPTY_SELECTION);
  // 참조 안정성이 필요하다 — 오버레이의 효과 의존성에 들어간다.
  const setSelection = useCallback((next: CanvasSelection) => setSelectionState(next), []);
  return useMemo(
    () => ({ selection, setSelection, autoExpandedId: canvasAutoExpandedId(selection) }),
    [selection, setSelection],
  );
}

/**
 * 선택을 읽고 쓴다. **provider 가 없으면 로컬 선택**이다(대시보드에 놓인 패널).
 *
 * 로컬 상태를 provider 유무와 관계없이 **언제나** 만든다 — 훅 호출 수는 렌더마다 같아야
 * 하기 때문이다. provider 가 있을 때 그 `useState` 는 쓰이지 않는 값 하나를 들 뿐이며,
 * 그 대가로 "provider 를 잊으면 조용히 아무 일도 일어나지 않는" 형태를 피한다.
 */
export function useCanvasEditSelection(): CanvasEditSelectionValue {
  const shared = useContext(CanvasEditSelectionContext);
  const local = useCanvasEditSelectionState();
  return shared ?? local;
}

// --- 라이브 시리즈 채널 ---------------------------------------------------
//
// **왜 채널이 필요한가.** 바인딩 선택지의 값과 패널이 판독값을 찾는 키는 **같은 공간**
// 이어야 한다. 그런데 두 곳이 그 공간을 따로 계산하고 있었다: 목록 편집기는 config 만
// 보고 store 시리즈의 동일성 키를 냈고, 패널은 조회 결과가 config 의 참조 수와 1:1 일
// 때만 그 동일성 키를 쓰고 아닐 때는 **조회 이름**을 썼다. 태그 하나가 여러 컬럼으로
// 펼쳐지는 흔한 경우가 곧 그 "아닐 때" 이므로, 사용자가 고른 키는 어느 판독값과도 만나지
// 못하고 `{value}` 가 영영 결측 표기로 남았다.
//
// 고치는 방향은 "편집기가 더 잘 추측한다" 가 아니다 — 추측이 둘이면 언제든 다시
// 갈라진다. **판독값을 실제로 키잉한 그 집합을 패널이 내놓고 편집기가 그대로 쓴다.**
// 규칙 표의 `matchesRule` 를 편집기와 렌더러가 함께 쓰는 것과 같은 규율이다.

/** 바인딩 선택지 한 줄 — 값은 판독값 키, 표시는 사람이 읽는 이름이다. */
export interface CanvasSeriesOption {
  /** `binding.series` 에 저장될 키. 패널의 판독값 맵 키와 **글자 그대로** 같다. */
  id: string;
  /** 드롭다운에 보일 이름. 훅이 정한 시리즈 표시명이다. */
  label: string;
}

/**
 * 아직 아무도 내놓지 않은 상태.
 *
 * 참조가 고정인 것이 요점이다 — 소비자가 매 렌더 새 빈 배열을 받으면 그것을 의존성으로
 * 든 memo 가 매번 무효가 된다.
 */
const NO_LIVE_SERIES: readonly CanvasSeriesOption[] = [];

/**
 * 아무도 듣고 있지 않을 때의 발행. 참조가 고정이라 효과 의존성에 그대로 들어간다.
 *
 * 이름을 소문자로 시작하는 것은 취향이 아니다 — 대문자로 시작하는 화살표 함수는
 * `react-refresh/only-export-components` 가 컴포넌트로 읽어, 이 파일이 지키기로 한
 * "컴포넌트를 내보내지 않는다"(위 머리말)가 경고 하나로 깨진 것처럼 보인다.
 */
const noopPublish = (_next: readonly CanvasSeriesOption[]): void => {};

/** 라이브 시리즈 채널이 나르는 것 전부. */
export interface CanvasLiveSeriesValue {
  /** 패널이 마지막으로 내놓은 키 집합. 아무도 내놓지 않았으면 빈 배열이다. */
  options: readonly CanvasSeriesOption[];
  /** 패널이 자기 판독값 키 집합을 내놓는다. **값이 같으면 아무 일도 일어나지 않는다.** */
  publish: (next: readonly CanvasSeriesOption[]) => void;
}

/**
 * 라이브 시리즈 공유 컨텍스트. 선택 컨텍스트와 같은 이유로 기본값이 `null` 이다 —
 * provider 가 없으면 발행은 무동작이고 소비는 빈 목록이며, 그때 편집기는 config 에서
 * 뽑은 목록으로 떨어진다(그 폴백은 편집기가 소유한다).
 */
export const CanvasLiveSeriesContext = createContext<CanvasLiveSeriesValue | null>(null);

/**
 * 두 선택지 목록이 **값으로** 같은가.
 *
 * 참조 비교가 아니라 값 비교인 것이 이 채널의 안전장치다. 패널의 판독값 맵은 폴링마다
 * 새로 만들어지므로 참조로 재면 매 주기 발행이 일어나고, 그 발행이 상태를 갈면 렌더가
 * 다시 돌아 다시 발행하는 고리가 생긴다. 순서도 함께 본다 — 시리즈 순서가 곧 드롭다운
 * 순서이고 사용자가 그 순서로 고른다.
 */
export function sameSeriesOptions(
  a: readonly CanvasSeriesOption[],
  b: readonly CanvasSeriesOption[],
): boolean {
  if (a === b) return true;
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    if (a[i]!.id !== b[i]!.id || a[i]!.label !== b[i]!.label) return false;
  }
  return true;
}

/**
 * 라이브 시리즈 상태 한 벌을 만든다. 감싸는 쪽(설정 다이얼로그)이 부른다.
 *
 * 값이 같으면 `setState` 가 **이전 값 그대로**를 돌려주므로 React 가 렌더를 건너뛴다 —
 * 발행 고리를 끊는 자리가 여기 한 줄이다.
 */
export function useCanvasLiveSeriesState(): CanvasLiveSeriesValue {
  const [options, setOptions] = useState<readonly CanvasSeriesOption[]>(NO_LIVE_SERIES);
  const publish = useCallback((next: readonly CanvasSeriesOption[]) => {
    setOptions((prev) => (sameSeriesOptions(prev, next) ? prev : next));
  }, []);
  return useMemo(() => ({ options, publish }), [options, publish]);
}

/** 지금 살아 있는 키 집합을 읽는다. provider 가 없으면 빈 목록이다(편집기가 폴백한다). */
export function useCanvasLiveSeries(): readonly CanvasSeriesOption[] {
  return useContext(CanvasLiveSeriesContext)?.options ?? NO_LIVE_SERIES;
}

/** 키 집합을 내놓는 통로. provider 가 없으면 무동작이다(대시보드에 놓인 패널). */
export function useCanvasLiveSeriesPublisher(): (next: readonly CanvasSeriesOption[]) => void {
  return useContext(CanvasLiveSeriesContext)?.publish ?? noopPublish;
}
