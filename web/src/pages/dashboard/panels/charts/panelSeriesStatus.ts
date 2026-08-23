// 패널 시리즈 상태 4종 판정 — 빈 선택 / 빈 결과 / 부분 실패 / 전체 실패.
//
// spec.md §2.14 [S2] 는 네 상태가 화면에서 **서로 구분 가능**해야 한다고 규정한다.
// "빈 결과"와 "전체 실패"를 같은 글리프로 그리면 사용자가 센서 단선을 정상으로
// 오독하기 때문이다. 구분의 정본을 패널마다 각자 두면 그 규정이 지점 수만큼
// 갈라지므로, 판정을 **의존성 없는 순수 함수 하나**로 모은다.
//
// 이 모듈이 `usePanelSeriesData` 의 반환 형상에 상태를 실어 보내지 않고 별도 함수로
// 있는 이유는 하나다 — 훅의 반환 **키 집합**이 store 경로에서 `UseStoreChartDataResult`
// 와 정확히 같아야 한다(§2.3 의 두 번째 하중 지지점). 키를 하나라도 무조건 더하면
// 그 불변식이 깨진다. 그래서 상태는 결과를 **입력으로 받아** 파생한다.
//
// @spec SPEC-TSDB-002 §2.14 (S2) · §2.19 (U12)

import type { PanelSourceBinding } from './panelDataSource';

/**
 * 패널이 표시해야 할 시리즈 상태.
 *
 * `'channel'` 은 "이 축이 적용되지 않음" 을 뜻한다 — 채널 경로에는 시리즈 조회 자체가
 * 없으므로 네 상태 중 어느 것도 성립하지 않는다. `'ok'` 와 뭉개지 않는 이유는 호출부가
 * "표시할 것이 없다" 와 "정상 렌더" 를 구분해야 하기 때문이다.
 */
export type PanelSeriesDisplayState =
  | 'channel'
  | 'empty-selection'
  | 'empty-result'
  | 'partial-failure'
  | 'error'
  | 'ok';

/** 판정에 필요한 조회 결과의 최소 형상. 훅 결과를 그대로 넘겨도 된다. */
export interface PanelSeriesResultLike {
  status: string;
  entries: readonly unknown[];
  /** TSDB 경로에서만 채워진다. store 경로에는 부분 실패 개념이 없다(`Promise.all`). */
  partialFailureCount?: number;
  /** 참조된 에이전트가 지원 백엔드가 아닌가(§2.18). 오류 사유 구분용. */
  backendMismatch?: boolean;
}

/** 판정 결과. 배지 문구에 쓰이는 실패 개수를 함께 싣는다. */
export interface PanelSeriesDisplay {
  state: PanelSeriesDisplayState;
  /** 이번 조회에서 실패한 시리즈 개수. `'partial-failure'` 가 아니면 0 이다. */
  failureCount: number;
  /** 오류 사유가 백엔드 불일치인가(§2.18 · UB2-7). 오류 문구를 가른다. */
  backendMismatch: boolean;
}

/**
 * 소스 바인딩과 조회 결과에서 표시 상태를 판정한다. O(1) 순수 함수.
 *
 * 우선순위는 다음 순서로 고정한다. 두 조건이 동시에 참일 때 어느 쪽을 보여줄지가
 * 곧 사용자가 무엇을 먼저 고치게 되는지이므로, 순서 자체가 요구사항이다.
 *
 * | 순위 | 조건 | 상태 | 왜 이 순위인가 |
 * |------|------|------|----------------|
 * | 1 | 종류가 `channel` | `channel` | 시리즈 축이 없다 |
 * | 2 | 소스 비활성 | `empty-selection` | 아직 조회한 적이 없으므로 다른 상태가 성립할 수 없다 |
 * | 3 | `status === 'error'` | `error` | 전체 실패가 부분 실패를 이긴다 — 부분 배지를 띄우면 조회가 된 것처럼 보인다 |
 * | 4 | 실패 개수 > 0 | `partial-failure` | 행이 0개여도 실패 신호가 더 유익하다 |
 * | 5 | 연결됨 + 행 0개 | `empty-result` | **오류가 아니다.** 빈 차트를 그린다 |
 * | 6 | 그 외 | `ok` | |
 *
 * 3번이 4번보다 앞선다는 점이 §2.14 의 핵심이다. 전체 실패는 §2.19 에서 "부분 실패가
 * 아니라 전체 실패" 로 승격되어 올라오므로, 여기서 배지로 격하하면 오버레이가 사라진다.
 */
export function resolvePanelSeriesDisplay(
  binding: PanelSourceBinding,
  result: PanelSeriesResultLike,
): PanelSeriesDisplay {
  const failureCount = result.partialFailureCount ?? 0;
  const backendMismatch = result.backendMismatch === true;

  if (binding.kind === 'channel') {
    return { state: 'channel', failureCount: 0, backendMismatch: false };
  }
  if (!binding.active) {
    return { state: 'empty-selection', failureCount: 0, backendMismatch: false };
  }
  if (result.status === 'error') {
    return { state: 'error', failureCount, backendMismatch };
  }
  if (failureCount > 0) {
    return { state: 'partial-failure', failureCount, backendMismatch: false };
  }
  if (result.status === 'connected' && result.entries.length === 0) {
    return { state: 'empty-result', failureCount: 0, backendMismatch: false };
  }
  return { state: 'ok', failureCount: 0, backendMismatch: false };
}


// ===== 그룹 페이지 표시 판정 (SPEC-TSDB-004 §2.7) =====

/** 판정 입력 — 훅 결과의 groups 를 그대로 넘겨도 된다. */
export interface GroupPageInfoLike {
  total: number;
  page: number;
  pageCount: number;
  truncated: boolean;
}

/** 패널이 그려야 할 그룹 페이지 상태. */
export interface GroupPageDisplay {
  /** 표시할 것이 있는가. group by 항목이 없으면 false 다. */
  show: boolean;
  /** 전체 그룹 수(여러 항목이면 합). */
  total: number;
  /** 현재 페이지(0 기반). */
  page: number;
  /** 전체 페이지 수(여러 항목이면 최댓값). */
  pageCount: number;
  /** 이전/다음으로 갈 수 있는가. */
  canPrev: boolean;
  canNext: boolean;
  /**
   * 열거가 상한에 걸렸는가. `true` 면 페이지를 전부 넘겨도 일부 그룹에 도달하지
   * 못하므로 UI 는 좁히는 방법을 안내해야 한다(§2.7.4).
   */
  truncated: boolean;
}

/**
 * 그룹 페이지 표시 상태를 판정한다.
 *
 * 판정을 패널마다 두면 §2.14 가 상태 4종에 대해 겪은 분산이 그대로 반복되므로,
 * 여기 순수 함수 하나로 모은다.
 *
 * 여러 group by 항목이 한 패널에 있으면 **합/최댓값**으로 접는다 — 항목별 페이지를
 * 따로 넘기게 하면 조작 축이 항목 수만큼 늘어 화면이 읽히지 않는다. 페이지 커서는
 * 패널당 하나이며 모든 group by 항목에 같이 적용된다.
 */
export function resolveGroupPageDisplay(
  groups: readonly GroupPageInfoLike[] | undefined,
): GroupPageDisplay {
  if (!groups || groups.length === 0) {
    return {
      show: false,
      total: 0,
      page: 0,
      pageCount: 1,
      canPrev: false,
      canNext: false,
      truncated: false,
    };
  }
  const total = groups.reduce((n, g) => n + g.total, 0);
  const pageCount = groups.reduce((n, g) => Math.max(n, g.pageCount), 1);
  const page = groups[0]?.page ?? 0;
  return {
    show: true,
    total,
    page,
    pageCount,
    canPrev: page > 0,
    canNext: page < pageCount - 1,
    truncated: groups.some((g) => g.truncated),
  };
}
