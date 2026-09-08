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
// @spec SPEC-CANVAS-002 REQ-04 / REQ-06

import { createContext, useCallback, useContext, useMemo, useState } from 'react';

import { EMPTY_SELECTION, type PanelSelection } from '../charts/panelEditSelection';

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
  return [...selection][0]!;
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
