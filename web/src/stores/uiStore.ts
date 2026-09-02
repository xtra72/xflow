// UI 상태 관리 - 선택적 localStorage 영속화 포함.
//
// SPEC-DASHBOARD-004 (M5):
// - 서버 상태 축은 **`dashboards: Dashboard[]` 하나**다. 구 모델의 스코프 2슬롯
//   (공유 스냅샷 · 개인 스냅샷과 그 활성 스코프 키)은 제거되었다 — 대시보드가 1급
//   엔티티가 되면서 "공유 묶음 / 내 묶음" 이라는 축 자체가 사라졌다
//   (spec.md §2.14 UB2 #1).
// - `dashboards` 는 목록 API 가 주는 **메타**(uid·name·version·can_edit 등)만 담는다.
//   본문(패널·레이아웃)은 `dashboardPages` 가 uid 를 키로 들고 있으며, 활성 대시보드
//   1장을 GET 할 때 채워진다. 두 배열은 `setDashboards` 가 uid 기준으로 정렬을 맞춘다.
// - `activeDashboardId` 는 이제 **활성 대시보드의 uid** 다. 서버의
//   `/dashboard-state.active_dashboard_uid` 와 대응한다.
// - 대시보드 구성은 localStorage 에 영속화되지 않는다. v0.2.0 첫 부팅 시 1회만 기존
//   localStorage 의 대시보드 키들을 제거하고 useDashboardSync 가 토스트로 안내한다.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

import type { Dashboard, DashboardContent, DashboardDetail } from '@/types/dashboard';
import { generateUUID } from '@/lib/utils/uuid';
// 신규 차트 패널의 기본 Store 소스 형상(설정 화면의 기본값과 같은 정본).
import { buildDefaultStoreSource } from '@/pages/dashboard/panels/charts/chartChannelTypes';
// SPEC-HEATMAP-PANEL-001: 히트맵 패널 기본 config 빌더(파서와 기본값 일치 보장).
import { buildDefaultHeatmapConfig } from '@/pages/dashboard/panels/heatmap/heatmapConfig';
// 모니터링 패널의 기본 표시 항목은 모니터링 페이지의 기본 레이아웃과 같은 값을 쓴다.
import { DEFAULT_LAYOUT as MONITOR_DEFAULT_LAYOUT } from '@/pages/monitoring/monitoringLayout';
import { STORAGE_ITEMS as SYSMETRICS_STORAGE_ITEMS } from '@/pages/dashboard/panels/sysmetrics/sysMetricsPanelConfig';
// 시스템 패널의 항목은 값 단위다(cpu.usage_percent 등) — 그룹 키를 쓰면 설정·미리보기가
// 값 카탈로그와 대조에 실패해 빈 목록이 된다.
import { DEFAULT_SYSTEM_FIELDS } from '@/pages/dashboard/panels/sysmetrics/sysMetricsFields';

export interface Notification {
  id: string;
  type: 'info' | 'success' | 'warning' | 'error';
  message: string;
  timestamp: number;
}

// ---------------------------------------------------------------------------
// v0.2.0 블랭크-슬레이트 마이그레이션 (SPEC-DASHBOARD-001 AC-15)
// ---------------------------------------------------------------------------

/** persist 키 (Zustand). */
const PERSIST_KEY = 'xflow-ui';

/** v0.2.0 마이그레이션 1회 보장 플래그 (localStorage). */
const MIGRATION_FLAG_KEY = 'xflow-ui:dashboard-migrated-v0.2';

/** v0.1 시절 localStorage 에 영속되었던 대시보드 관련 키. */
const LEGACY_DASHBOARD_KEYS: readonly string[] = [
  'dashboardPages',
  'activeDashboardId',
  'dashboardGridCols',
  'dashboardShowGridLines',
  'dashboardRefreshInterval',
  'deviceGridLayout',
];

/** v0.2 첫 부팅에서 마이그레이션이 실제로 수행되었음을 useDashboardSync 에 알리는 모듈 플래그. */
let migrationToastPending = false;

/**
 * v0.2 블랭크-슬레이트 마이그레이션을 시도한다.
 *
 * 동작:
 * - localStorage 의 마이그레이션 플래그가 이미 set 이면 no-op.
 * - 그렇지 않으면 persist 키(`xflow-ui`) 의 JSON 에서 v0.1 시절 6 개 키를
 *   삭제하고, 마이그레이션 플래그를 set 한 뒤 토스트 대기 플래그를 1로 만든다.
 *
 * @returns 마이그레이션이 실제로 수행되었는가? (테스트/디버깅용)
 */
export function runDashboardLocalStorageMigration(): boolean {
  // SSR/test 환경에서 localStorage 가 없을 수 있다.
  if (typeof globalThis === 'undefined' || typeof globalThis.localStorage === 'undefined') {
    return false;
  }
  const ls = globalThis.localStorage;
  try {
    if (ls.getItem(MIGRATION_FLAG_KEY)) {
      return false; // 이미 마이그레이션 됨
    }

    const raw = ls.getItem(PERSIST_KEY);
    if (raw) {
      try {
        const parsed = JSON.parse(raw) as { state?: Record<string, unknown> } | Record<string, unknown>;
        // Zustand persist 는 `{ state: {...}, version: N }` 형태로 저장한다.
        const container = parsed as Record<string, unknown>;
        const stateObj =
          parsed && typeof parsed === 'object' && 'state' in container && typeof container.state === 'object'
            ? (container.state as Record<string, unknown>)
            : container;

        let removedAny = false;
        for (const key of LEGACY_DASHBOARD_KEYS) {
          if (key in stateObj) {
            delete stateObj[key];
            removedAny = true;
          }
        }

        if (removedAny) {
          // 변경된 객체를 다시 쓴다.
          ls.setItem(PERSIST_KEY, JSON.stringify(parsed));
        }
      } catch {
        // JSON 파싱 실패 — persist key 가 손상된 경우 무시 (Zustand 가 새로 초기화한다).
      }
    }

    ls.setItem(MIGRATION_FLAG_KEY, '1');
    migrationToastPending = true;
    return true;
  } catch {
    // localStorage 사용 불가 시 무시
    return false;
  }
}

/** 마이그레이션 토스트가 대기 중인지 확인하고, 호출 즉시 소비한다 (1회 한정). */
export function consumeMigrationToastFlag(): boolean {
  if (migrationToastPending) {
    migrationToastPending = false;
    return true;
  }
  return false;
}

// 모듈 로드 시 1회 마이그레이션 실행 (테스트 환경에서 localStorage 가 없으면 no-op).
runDashboardLocalStorageMigration();

// ---- 대시보드 레이아웃 ----

export interface DashboardLayoutItem {
  i: string;
  x: number;
  y: number;
  w: number;
  h: number;
  minW?: number;
  minH?: number;
}

// ---- 패널 컬럼 키 ----

export const ALL_METRIC_KEYS = ['cpu', 'memory', 'throughput', 'errorRate'] as const;
export type MetricKey = (typeof ALL_METRIC_KEYS)[number];

export const ALL_FLOW_COLUMNS = ['name', 'status', 'node_count', 'updated_at', 'actions'] as const;
export type FlowColumnKey = (typeof ALL_FLOW_COLUMNS)[number];

export const ALL_AGENT_COLUMNS = ['name', 'type', 'status', 'uptime', 'messages', 'actions'] as const;
export type AgentColumnKey = (typeof ALL_AGENT_COLUMNS)[number];

// 디바이스 목록 컬럼은 디바이스 탭과 대시보드 패널이 같은 집합을 써야 하므로
// `@/hooks/useDeviceColumns` 의 ALL_DEVICE_COLUMNS / DeviceListColumnKey 를 SSOT 로 쓴다.
// (과거 여기 있던 5컬럼 사본은 탭이 8컬럼으로 늘어난 뒤에도 따라가지 못했다.)

// ---- 멀티-대시보드 타입 ----

/** 패널 유형 */
export type PanelType =
  | 'flows'
  | 'agents'
  | 'resource'
  | 'devices'
  | 'device'
  | 'logs'
  | 'stat'
  | 'gauge'
  | 'graph-chart'
  | 'bar-chart'
  | 'pie-chart'
  | 'text'
  | 'table'
  | 'ac-control'
  | 'hvac-control'
  | 'custom-control'
  | 'outdoor-control'
  | 'properties-grid'
  | 'facility-line'
  | 'facility-station'
  | 'facility-device'
  | 'facility-group'
  // SPEC-TRIGGER-PANEL-001 M2: trigger 노드 스케줄/페이로드 설정 패널.
  | 'trigger-config'
  // SPEC-TRIGGER-SCHED-001 M2: 설비 제어 예약 패널(규칙 테이블 + 모달). trigger-config 와 공존.
  | 'facility-schedule'
  // SPEC-MODBUS-012 M1: MODBUS Gateway 대시보드 패널 스위트 6종.
  // 모두 modbus-gateway 에이전트에 바인딩(config.agentId)되며 레지스터 맵 2종은 관측 전용이다.
  | 'modbus-real-devices' // 실제 연결(upstream 백킹) 디바이스 목록
  | 'modbus-virtual-devices' // 가상 디바이스(U01~) 목록
  | 'modbus-shared-registers' // 공유(unit 0) 레지스터 맵 그리드
  | 'modbus-device-registers' // 가상 디바이스(unitId) 레지스터 맵 그리드
  | 'modbus-bus-stats' // 버스 통계 미니차트
  | 'modbus-summary-stats' // 종합 통계 바
  // SPEC-DASHBOARD-002: 단일 에이전트(타입 무관) 상태·통계 패널.
  // config.agentId 로 임의 타입의 에이전트 하나에 바인딩되며 관측 전용이다.
  | 'agent-status'
  // SPEC-HEATMAP-PANEL-001 (MVP): store 태그 바인딩 온도 센서를 IDW 로 보간해
  // Canvas 2D 에 렌더하는 히트맵 패널. config 는 불투명 JSON(store_source + sensor_positions).
  | 'heatmap'
  // 모니터링 패널 4종. 모니터링 페이지와 같은 항목 어휘를 쓰며(`config.items`),
  // 실시간 스트림은 프로세스 전역 단일 구독(`monitorStream`)을 공유한다.
  // 기존 'resource'/'logs' 를 대체하며, 그 둘은 추가 메뉴에서만 내려가고 렌더는 유지된다.
  | 'monitor-stats'
  | 'monitor-metrics'
  // 네트워크는 출처(REST 누적 카운터)·단위시간·인터페이스별 계열이 모두 달라
  // 실시간 메트릭과 한 패널에 묶이지 않는다.
  | 'monitor-network'
  | 'monitor-logs'
  | 'monitor-events'
  // SPEC-SYSMETRICS-PANEL-001: sysmetrics 에이전트에 바인딩된 호스트 지표 패널 3종.
  // 위 monitor-* 와 출처가 다르다 — 이쪽은 호스트 전체(CPU·메모리·디스크·네트워크)를
  // 보고, monitor-* 는 xflowd 런타임을 본다. 대상(인터페이스·마운트) 선택이 비면
  // 종합, 고르면 개별이므로 "종합/개별"을 별도 타입으로 두지 않는다.
  | 'sysmetrics-system'
  | 'sysmetrics-network'
  | 'sysmetrics-storage';

/**
 * 옛 패널 타입 이름 → 현재 이름.
 *
 * 저장된 대시보드는 `line-chart` 를 담고 있다. 이름만 바뀌었을 뿐 같은 패널이므로,
 * 읽는 자리에서 옮겨 준다 — 저장을 강제로 다시 쓰지 않는다. 사용자가 그 대시보드를
 * 저장하는 순간 새 이름으로 자연히 넘어간다.
 */
const LEGACY_PANEL_TYPES: Record<string, PanelType> = {
  'line-chart': 'graph-chart',
};

/** 저장된 패널 타입을 현재 어휘로 옮긴다. 모르는 값은 그대로 둔다. */
export function normalizePanelType(type: string): PanelType {
  return LEGACY_PANEL_TYPES[type] ?? (type as PanelType);
}

/** 패널 목록의 타입을 일괄 정규화한다. 바뀔 것이 없으면 원본을 그대로 돌려준다. */
export function normalizePanels(panels: PanelConfig[]): PanelConfig[] {
  if (!panels.some((p) => p.type in LEGACY_PANEL_TYPES)) return panels;
  return panels.map((p) =>
    p.type in LEGACY_PANEL_TYPES ? { ...p, type: normalizePanelType(p.type) } : p,
  );
}


/** 개별 패널 설정 */
export interface PanelConfig {
  id: string;
  type: PanelType;
  title: string;
  /** 패널별 세부 설정 (타입에 따라 visibleColumns, visibleMetrics 등) */
  config: Record<string, unknown>;
}

/** 대시보드 페이지 설정 */
export interface DashboardPageConfig {
  id: string;
  name: string;
  isDefault: boolean;
  panels: PanelConfig[];
  layout: DashboardLayoutItem[];
}

// ---- 기본 패널/페이지 정의 ----

const DEFAULT_PANELS: PanelConfig[] = [
  {
    id: 'flows-default',
    type: 'flows',
    title: '플로우 현황',
    config: { visibleColumns: ['name', 'status', 'node_count', 'updated_at', 'actions'] },
  },
  {
    id: 'agents-default',
    type: 'agents',
    title: '에이전트 현황',
    config: { visibleColumns: ['name', 'type', 'status', 'uptime', 'messages', 'actions'] },
  },
  {
    id: 'resource-default',
    type: 'resource',
    title: '프로세스 상태',
    config: { visibleMetrics: ['cpu', 'memory', 'throughput', 'errorRate'] },
  },
];

const DEFAULT_DASHBOARD_LAYOUT: DashboardLayoutItem[] = [
  { i: 'flows-default', x: 0, y: 0, w: 5, h: 4, minW: 3, minH: 3 },
  { i: 'agents-default', x: 5, y: 0, w: 5, h: 4, minW: 3, minH: 3 },
  { i: 'resource-default', x: 0, y: 4, w: 10, h: 3, minW: 4, minH: 2 },
];

const DEFAULT_DASHBOARD_PAGE: DashboardPageConfig = {
  id: 'default',
  name: '대시보드',
  isDefault: true,
  panels: DEFAULT_PANELS,
  layout: DEFAULT_DASHBOARD_LAYOUT,
};

/** 그리드 배치 크기(생성 시점 기본값). */
type PanelGridSize = Pick<DashboardLayoutItem, 'w' | 'h' | 'minW' | 'minH'>;

/**
 * 패널 타입별 **생성 시점** 기본 그리드 크기.
 *
 * 그리드 셀은 **정사각형**이다 — 행 높이를 열 폭에서 계산한다(`DashboardPage` 의
 * `gridRowHeight`). 기본 10열이므로 폭 1400px 화면에서 한 칸은 약 140×140px 이고,
 * 패널 헤더가 그중 위쪽 ~36px 을 먹는다. 아래 값들은 그 셈을 기준으로 잡았다.
 *
 * 기준은 하나다: **만들자마자 크기를 조절하지 않아도 내용이 읽혀야 한다.** 종전에는
 * 여러 타입이 아래 표에 없어 일괄 폴백(5×4)으로 태어났고, 통계·게이지처럼 표에 있는
 * 것도 헤더를 빼면 내용이 들어갈 자리가 남지 않는 값이었다.
 *
 * `Record<PanelType, …>` 로 두어 **컴파일러가 전수성을 강제**한다. 종전의 `switch` +
 * `default` 는 새 타입이 조용히 일괄 폴백으로 떨어졌고, 그 폴백이 맞는지는 아무도
 * 확인하지 않았다 — sysmetrics 패널 3종이 실제로 그 상태였다.
 *
 * `minW`/`minH` 는 **그대로 둔다.** 여기는 "태어날 때의 크기"이지 "허용되는 최소"가
 * 아니다. 최소를 함께 올리면 사용자가 일부러 줄여 둔 패널을 다음 렌더에서 그리드가
 * 도로 키운다.
 *
 * 차트 계열 5종의 종전 값은 SPEC-CHART-001 REQ-M5-05 에서 왔다. 그 값들이 헤더·축·범례를
 * 셈에 넣지 않아 실사용에서 좁다는 피드백을 받아 이번에 키웠다.
 */
const PANEL_DEFAULT_SIZES: Record<PanelType, PanelGridSize> = {
  // --- 차트 계열 ---
  //
  // 통계는 종전 2×1 이었다. 한 칸 높이에서 헤더를 빼면 ~100px 이라 값 한 줄이 겨우
  // 들어가고, 다중 출력 타일 격자(SPEC-CHART-002)는 아예 보이지 않았다.
  stat: { w: 3, h: 2, minW: 2, minH: 1 },
  // 게이지는 원형이라 **정사각**이 맞다. 종전 2×3 은 폭이 원의 지름을 묶고 남는
  // 세로가 빈 채로 남았다.
  gauge: { w: 3, h: 3, minW: 2, minH: 2 },
  // 라인/영역/막대/캔들 공용. 축 라벨과 범례가 붙으면 3행에서는 그림이 절반이다.
  'graph-chart': { w: 6, h: 4, minW: 3, minH: 2 },
  // 카테고리 라벨이 가로로 늘어선다.
  'bar-chart': { w: 5, h: 4, minW: 3, minH: 2 },
  // 조각 + 범례. 파이 자체가 정사각을 요구한다.
  'pie-chart': { w: 4, h: 4, minW: 3, minH: 3 },
  // 컬럼 여러 개 + 헤더 행. 가로가 모자라면 컬럼이 잘린다.
  table: { w: 7, h: 5, minW: 4, minH: 3 },
  text: { w: 4, h: 3, minW: 2, minH: 2 },
  // 히트맵은 2차원 공간장을 그리므로 정사각이 자연스럽다.
  heatmap: { w: 5, h: 5, minW: 3, minH: 3 },

  // --- 목록/테이블 계열 ---
  //
  // 플로우·에이전트는 기본 대시보드가 쓰는 값과 같다(5×4 를 둘 나란히 = 10열).
  flows: { w: 5, h: 4, minW: 3, minH: 3 },
  agents: { w: 5, h: 4, minW: 3, minH: 3 },
  devices: { w: 5, h: 4, minW: 3, minH: 3 },
  // 로그는 컬럼이 많아 가로로 넓어야 읽힌다(monitor-logs 와 같은 근거).
  logs: { w: 8, h: 5, minW: 5, minH: 3 },
  // 지표 카드가 가로로 늘어서는 낮고 넓은 띠. 기본 대시보드는 전체 폭(10×3)을 준다.
  resource: { w: 6, h: 3, minW: 4, minH: 2 },

  // --- 단일 대상 카드 ---
  device: { w: 4, h: 5, minW: 3, minH: 3 },
  'properties-grid': { w: 4, h: 4, minW: 2, minH: 2 },
  // SPEC-DASHBOARD-002: 단일 에이전트 상태·통계(통계 타일 그리드 + 헤더).
  'agent-status': { w: 4, h: 5, minW: 3, minH: 3 },

  // --- 제어 카드 (세로로 긴 조작 패널) ---
  'ac-control': { w: 3, h: 5, minW: 2, minH: 4 },
  'outdoor-control': { w: 3, h: 5, minW: 2, minH: 4 },
  'hvac-control': { w: 5, h: 5, minW: 4, minH: 4 },
  'custom-control': { w: 3, h: 5, minW: 2, minH: 3 },

  // --- 설비 (SPEC-FACILITY-DASHBOARD-001 M5 / SPEC-XSFM-GROUP-001 M7) ---
  // 단일 기기는 에어컨 제어와 유사한 세로 카드.
  'facility-device': { w: 3, h: 5, minW: 2, minH: 4 },
  // 역사(통계 + 기기 목록 + 일괄 제어).
  'facility-station': { w: 5, h: 6, minW: 3, minH: 4 },
  // 호선(라인도 포함으로 더 넓게).
  'facility-line': { w: 8, h: 6, minW: 4, minH: 4 },
  // 그룹(그룹별 통계 + 일괄 제어 목록).
  'facility-group': { w: 5, h: 6, minW: 3, minH: 4 },
  // SPEC-TRIGGER-SCHED-001 M2: 예약 규칙 테이블(6컬럼). 가로로 넓은 카드.
  'facility-schedule': { w: 8, h: 6, minW: 5, minH: 4 },
  // SPEC-TRIGGER-PANEL-001 M2: 스케줄 리스트 + 카탈로그 편집을 담는 세로 카드.
  'trigger-config': { w: 4, h: 7, minW: 3, minH: 4 },

  // --- MODBUS Gateway (SPEC-MODBUS-012 M1) ---
  // 레지스터 맵 그리드는 넓게, 목록은 세로로, 미니차트/요약 바는 낮게.
  'modbus-shared-registers': { w: 6, h: 5, minW: 4, minH: 3 },
  'modbus-device-registers': { w: 6, h: 5, minW: 4, minH: 3 },
  'modbus-real-devices': { w: 4, h: 5, minW: 3, minH: 3 },
  'modbus-virtual-devices': { w: 4, h: 5, minW: 3, minH: 3 },
  'modbus-bus-stats': { w: 6, h: 3, minW: 3, minH: 2 },
  'modbus-summary-stats': { w: 8, h: 2, minW: 4, minH: 2 },

  // --- 모니터링 (xflowd 런타임) ---
  // 통계 카드 그리드 — 낮고 넓게.
  'monitor-stats': { w: 4, h: 3, minW: 2, minH: 2 },
  // 라인 차트 2열 배치 기준.
  'monitor-metrics': { w: 6, h: 4, minW: 3, minH: 3 },
  // 범례가 붙어 메트릭보다 넓어야 읽힌다.
  'monitor-network': { w: 6, h: 5, minW: 4, minH: 3 },
  // 로그 테이블은 컬럼이 6개라 가로로 넓어야 읽힌다.
  'monitor-logs': { w: 8, h: 5, minW: 5, minH: 3 },
  // 타임라인은 세로로 흐른다.
  'monitor-events': { w: 4, h: 5, minW: 3, minH: 3 },

  // --- sysmetrics (호스트 지표, SPEC-SYSMETRICS-PANEL-001) ---
  //
  // 세 타입 모두 종전에는 표에 없어 일괄 폴백(5×4)으로 태어났다. 각 패널이 선언한
  // 최소 항목 폭에서 필요한 칸 수를 되짚어 넣는다.
  //
  // 시스템: 기본 항목 4개 × 최소 카드 폭 160px = 640px → 5칸(700px)이 하한, 6칸이 여유.
  // 타일은 낮으므로 높이는 3칸이면 한 줄이 편하게 들어간다.
  'sysmetrics-system': { w: 6, h: 3, minW: 3, minH: 2 },
  // 네트워크: 기본 스타일이 라인이라 monitor-network 와 같은 근거로 넓고 높다.
  'sysmetrics-network': { w: 6, h: 5, minW: 4, minH: 3 },
  // 스토리지: 마운트마다 한 행(최소 행 폭 220px), 기본 스타일은 진행 막대.
  'sysmetrics-storage': { w: 5, h: 4, minW: 3, minH: 3 },
};

/** 표에 없는 타입(구 config 의 미지 문자열)이 들어왔을 때의 폴백. */
const FALLBACK_PANEL_SIZE: PanelGridSize = { w: 5, h: 4, minW: 3, minH: 3 };

/**
 * 패널 타입별 생성 시점 기본 크기.
 *
 * 표는 전수이지만 런타임에는 저장된 대시보드에서 미지의 타입 문자열이 올 수 있으므로
 * 폴백을 남긴다(`normalizePanelType` 이 걸러 주지만 그 밖의 경로가 생길 수 있다).
 */
function panelDefaultSize(type: PanelType): PanelGridSize {
  return PANEL_DEFAULT_SIZES[type] ?? FALLBACK_PANEL_SIZE;
}

/**
 * 신규 차트 계열 패널의 기본 데이터 소스 — 채널이 아니라 **Store** 로 시작한다.
 *
 * 히트맵(`buildDefaultHeatmapConfig`)이 이미 쓰던 방식을 통계/게이지/바/파이로 넓힌 것이다.
 * 이전에는 생성 위저드가 채널 이름을 **필수**로 물어봐서(`AddPanelDialog` 의 chart-config
 * 스텝) 신규 패널이 항상 channel 모드로 태어났고, store/tsdb 로 가려면 만든 뒤 설정에서
 * 소스를 다시 바꿔야 했다. 렌더 경로는 이미 세 소스를 모두 지원하고 있었으므로
 * (`usePanelSeriesData`) 남은 격차는 이 기본값 하나였다.
 *
 * `channel_name: ''` 은 **지우지 않는다.** 기본 store 소스는 시리즈가 비어 있어 비활성이고
 * (`isStoreSourceActive` false), 그 상태의 패널은 채널 경로로 폴백해 기존과 똑같은
 * "채널 미설정" 빈 상태를 보여준다. 키를 지우면 설정에서 채널 모드로 되돌렸을 때 편집할
 * 필드가 사라진다.
 */
function defaultChartSourceConfig(): Record<string, unknown> {
  return { data_source: 'store', store_source: buildDefaultStoreSource() };
}

/** 패널 타입별 기본값 생성 */
function createDefaultPanel(type: PanelType): Omit<PanelConfig, 'id'> {
  switch (type) {
    case 'flows':
      return { type, title: '플로우 현황', config: { visibleColumns: [...ALL_FLOW_COLUMNS] } };
    case 'agents':
      return { type, title: '에이전트 현황', config: { visibleColumns: [...ALL_AGENT_COLUMNS] } };
    case 'resource':
      return { type, title: '프로세스 상태', config: { visibleMetrics: [...ALL_METRIC_KEYS] } };
    case 'devices':
      return { type, title: '디바이스', config: {} };
    case 'device':
      return { type, title: '디바이스', config: {} };
    case 'logs':
      return { type, title: '로그', config: { maxLines: 100 } };
    // 모니터링 패널 4종 — 기본 표시 항목은 모니터링 페이지의 기본 레이아웃과 같다.
    case 'monitor-stats':
      return { type, title: '시스템 통계', config: { items: [...MONITOR_DEFAULT_LAYOUT.stats] } };
    case 'monitor-metrics':
      return { type, title: '실시간 메트릭', config: { items: [...MONITOR_DEFAULT_LAYOUT.metrics] } };
    case 'monitor-network':
      return {
        type,
        title: '네트워크',
        config: { items: [...MONITOR_DEFAULT_LAYOUT.network], interfaces: [], unitTime: 'sec' },
      };
    case 'monitor-logs':
      return { type, title: '시스템 로그', config: { items: [...MONITOR_DEFAULT_LAYOUT.logs] } };
    case 'monitor-events':
      return { type, title: '시스템 이벤트', config: { items: [...MONITOR_DEFAULT_LAYOUT.events] } };
    // sysmetrics 패널 3종 (SPEC-SYSMETRICS-PANEL-001).
    //
    // 대상 목록(interfaces / mountpoints)의 기본값은 **빈 배열 = 종합**이다. 기본을
    // "전체 개별"로 두면 마운트가 10개인 호스트에서 첫 화면부터 읽을 수 없다.
    //
    // agent_id 는 추가 다이얼로그의 선택 스텝이 채운다. 여기서는 키만 비워 둔다 —
    // 키가 없으면 설정 화면이 어떤 필드를 편집해야 할지 알 수 없다.
    case 'sysmetrics-system':
      return {
        type,
        title: '시스템 지표',
        config: { agent_id: '', agent_name: '', items: [...DEFAULT_SYSTEM_FIELDS], unitTime: 'sec' },
      };
    case 'sysmetrics-network':
      return {
        type,
        title: '네트워크 지표',
        config: { agent_id: '', agent_name: '', interfaces: [], unitTime: 'sec' },
      };
    case 'sysmetrics-storage':
      return {
        type,
        title: '스토리지 지표',
        config: { agent_id: '', agent_name: '', mountpoints: [], items: [...SYSMETRICS_STORAGE_ITEMS] },
      };
    case 'stat':
      // SPEC-CHART-001 §4.2.2 stat config
      return {
        type,
        title: '통계',
        config: {
          channel_name: '',
          display_field: 'value',
          unit: '',
          decimal_places: 2,
          ...defaultChartSourceConfig(),
        },
      };
    case 'gauge':
      // `series_reduce` 가 함께 있어야 store/tsdb 경로가 실제로 이긴다 — 게이지는
      // `data_source` + 소스 활성 + `series_reduce` 의 논리곱일 때만 레거시
      // `dataSources[]` 를 밀어낸다(SPEC-CHART-002 §2.9, `gaugeLegacyBinding.ts` 진리표).
      // 빠뜨리면 사용자가 시리즈를 골라도 게이지만 조용히 static `value` 를 계속 그린다.
      // `value: 75` 는 바인딩 이전의 표시값이므로 그대로 둔다(기존 동작 보존).
      return {
        type,
        title: '게이지',
        config: {
          value: 75,
          min: 0,
          max: 100,
          unit: '%',
          gaugeType: 'simple',
          series_reduce: 'last',
          ...defaultChartSourceConfig(),
        },
      };
    case 'graph-chart':
      // SPEC-CHART-001 §4.2.2 그래프 차트 config
      return {
        type,
        title: '라인 차트',
        config: { channel_name: '', display_field: 'value', max_points: 100, smooth: false },
      };
    case 'bar-chart':
      // SPEC-CHART-001 §4.2.2 bar-chart config
      return {
        type,
        title: '바 차트',
        config: {
          channel_name: '',
          display_field: 'value',
          label_field: 'labels.name',
          mode: 'category',
          bin_sec: 60,
          agg_func: 'avg',
          max_points: 20,
          ...defaultChartSourceConfig(),
        },
      };
    case 'pie-chart':
      // SPEC-CHART-001 §4.2.2 pie-chart config
      return {
        type,
        title: '파이 차트',
        config: {
          channel_name: '',
          display_field: 'value',
          label_field: 'labels.name',
          agg_func: 'sum',
          show_legend: true,
          show_percentage: true,
          max_points: 20,
          ...defaultChartSourceConfig(),
        },
      };
    case 'text':
      return { type, title: '텍스트', config: { content: '', format: 'markdown' } };
    case 'table':
      // SPEC-CHART-001 §4.2.2 table config.
      // 통계/게이지/바/파이와 같이 store 기본 소스로 태어난다 — 생성 시 채널 이름을
      // 묻지 않고, 채널/Store/TSDB 전환은 패널 설정의 데이터 소스 섹션에서 한다.
      return {
        type,
        title: '테이블',
        config: {
          channel_name: '',
          columns: [
            { field: 'timestamp', header: '시간', format: 'datetime' },
            { field: 'value', header: '값', format: 'number' },
          ],
          rows_per_page: 20,
          max_points: 200,
          ...defaultChartSourceConfig(),
        },
      };
    case 'ac-control':
      return { type, title: '에어컨 제어', config: { deviceId: '' } };
    case 'hvac-control':
      return { type, title: '공조기 제어', config: { deviceId: '' } };
    case 'custom-control':
      return { type, title: '커스텀 제어', config: { deviceId: '' } };
    case 'outdoor-control':
      return { type, title: '실외기 모니터링', config: { deviceId: '' } };
    case 'properties-grid':
      return { type, title: '디바이스 상태', config: { deviceId: '', gridCols: 3, visibleProperties: [] } };
    // SPEC-FACILITY-DASHBOARD-001 M5: 설비 패널 3종. config 는 agentId + 대상(라인/역사/기기).
    // 표시 옵션(nodeSize 5단계 / offlineAsOff / showStats / stationsPerRow)은 config-only 영속(UB-003).
    case 'facility-device':
      return { type, title: '설비 기기', config: { agentId: '', deviceId: '' } };
    case 'facility-station':
      return {
        type,
        title: '설비 역사',
        // deviceLabelMode: 개별 기기 라벨(placeIndex=위치+번호 기본 / name=기기 이름).
        // offlineAsOff: 오프라인을 꺼짐으로 표시(라인 패널과 동일 옵션, 기본 false).
        config: {
          agentId: '',
          station: '',
          showStats: true,
          deviceLabelMode: 'placeIndex',
          offlineAsOff: false,
        },
      };
    case 'facility-line':
      return {
        type,
        title: '설비 호선',
        // stationsPerRow: 0 = 자동(nodeSize 기반) 폴백. 1 이상 지정 시 1줄당 역사 수를 직접 제어.
        config: { agentId: '', line: '', nodeSize: '2', offlineAsOff: false, stationsPerRow: 0 },
      };
    // SPEC-XSFM-GROUP-001 M7: 설비 그룹 패널(역사/라인/커스텀 그룹을 한 종류로 표시·제어).
    // config 는 agentId 만 필요(그룹 전체를 나열하므로 단일 대상 없음). showStats 는 그룹 통계 토글.
    case 'facility-group':
      return { type, title: '설비 그룹', config: { agentId: '', showStats: true } };
    // SPEC-TRIGGER-PANEL-001 M2: trigger 노드 설정 패널.
    // config 는 대상 노드({flowId,nodeId}) + 대시보드-로컬 페이로드 카탈로그(RD-9).
    // 스케줄은 노드 config 가 SSOT 이므로 패널 config 에 복제하지 않는다(REQ-02-04).
    case 'trigger-config':
      return { type, title: '트리거 설정', config: { flowId: '', nodeId: '', payloadCatalog: {} } };
    // SPEC-TRIGGER-SCHED-001 M2: 설비 제어 예약 패널.
    // config 는 대상 trigger 노드({flowId,nodeId}) + TARGET 열거용 xsfm 에이전트(agentId).
    // 예약 규칙(스케줄 + 확장 메타 + 제어 payload)은 노드 config 가 SSOT 이므로 복제하지 않는다.
    case 'facility-schedule':
      return { type, title: '설비 제어 예약', config: { flowId: '', nodeId: '', agentId: '' } };
    // SPEC-MODBUS-012 M1: MODBUS Gateway 패널 6종. 모두 modbus-gateway 에이전트에 바인딩된다.
    // 가상 디바이스 레지스터 맵만 대상 unit(unitId)을 추가로 저장한다(2차 선택 스텝).
    case 'modbus-real-devices':
      return { type, title: '디바이스 상태', config: { agentId: '' } };
    case 'modbus-virtual-devices':
      return { type, title: '가상 디바이스', config: { agentId: '' } };
    case 'modbus-shared-registers':
      return { type, title: '공유 레지스터 맵', config: { agentId: '' } };
    case 'modbus-device-registers':
      return { type, title: '가상 디바이스 레지스터', config: { agentId: '', unitId: 0 } };
    case 'modbus-bus-stats':
      return { type, title: '버스 통계', config: { agentId: '' } };
    case 'modbus-summary-stats':
      return { type, title: '종합 통계', config: { agentId: '' } };
    // SPEC-DASHBOARD-002: 단일 에이전트(타입 무관) 상태 패널. config 는 agentId 만 필요.
    case 'agent-status':
      return { type, title: '에이전트 상태', config: { agentId: '' } };
    // SPEC-HEATMAP-PANEL-001: 히트맵 패널. 기본 config 는 store 태그 모드 + 빈 좌표 + 기본 IDW.
    case 'heatmap':
      return { type, title: '히트맵', config: buildDefaultHeatmapConfig() };
  }
}

// ---- 테마 모드 타입 ----

/** 테마 모드: system(OS 설정 따름) | day(라이트) | night(다크) | custom(사용자 정의) */
export type ThemeMode = 'system' | 'day' | 'night' | 'custom';

// ---- 원격 대시보드 렌더 모드 타입 (SPEC-REMOTE-001 M11.4) ----

/**
 * 원격 노드 대시보드의 렌더 모드.
 *
 * - 'responsive'(기본): 그리드가 관리자 콘텐츠 영역을 채운다(해상도 독립 —
 *   display_width/height 불필요). RemoteDashboardView 가 컨테이너 폭을 측정해
 *   그리드를 채우는 M11.3 이전 동작이다.
 * - 'fixed': 노드 해상도(display_width/height, 폴백 1920×1080) 고정 캔버스에
 *   렌더한 뒤 레터박스 스케일-투-핏 한다(M11.3 픽셀 충실 재현 — FixedCanvasScaler).
 *
 * 뷰 전역(노드별 아님) 환경설정이며 세션 간 영속된다.
 */
export type RemoteDashboardRenderMode = 'responsive' | 'fixed';

/**
 * 2026-05-31: 플로우 단위 표시 설정.
 * 플로우 에디터에서 노드의 포트 옆에 통계/이름을 표시할지 토글.
 */
export interface FlowDisplaySettings {
  /** 포트별 메시지 통계를 핸들 옆에 작은 수치로 표시 */
  showPortStats: boolean;
  /** 포트 이름을 핸들 옆에 텍스트로 표시 */
  showPortNames: boolean;
}

/** 새 플로우의 기본 표시 설정 */
export const DEFAULT_FLOW_DISPLAY_SETTINGS: FlowDisplaySettings = {
  showPortStats: false,
  showPortNames: false,
};

// ---- Store ----

interface UIState {
  sidebarCollapsed: boolean;
  theme: ThemeMode;
  /** 커스텀 테마 CSS 변수 토큰 (변수명 -> 값) */
  customThemeTokens: Record<string, string>;
  /** 대시보드 자동 갱신 주기 (초 단위). 기본값 10. */
  dashboardRefreshInterval: number;
  /**
   * 원격 노드 대시보드 렌더 모드 (뷰 전역, 영속). 기본값 'responsive'
   * (SPEC-REMOTE-001 M11.4). 'fixed' 는 노드 해상도 고정 캔버스를 사용한다.
   */
  remoteDashboardRenderMode: RemoteDashboardRenderMode;
  /**
   * 대시보드 본문 목록 — `dashboards` 와 uid 기준으로 1:1 대응한다.
   *
   * `panels` / `layout` 은 그 대시보드를 GET 하기 전까지 빈 배열이다. 목록 API 는
   * payload 를 주지 않기 때문이다(spec.md §2.3).
   */
  dashboardPages: DashboardPageConfig[];
  /** 활성 대시보드의 uid. 접근 가능한 대시보드가 0장이면 빈 문자열. */
  activeDashboardId: string;
  /** 대시보드 편집 모드 (비영속) */
  dashboardEditMode: boolean;
  /** 대시보드 그리드 칼럼 수 */
  dashboardGridCols: number;
  /**
   * 대시보드 그리드 컨테이너의 실측 폭(px, 비영속·비동기화).
   *
   * 셀 한 변을 이 폭에서 파생하므로(gridGeometry.gridCellSize) 패널 설정 화면이 "이 패널이
   * 대시보드에서 실제로 몇 대 몇인지"를 계산할 수 있다. 대시보드를 한 번도 열지 않았으면 0 이며
   * 호출부는 마진 무시 근사로 폴백한다.
   */
  dashboardGridWidth: number;
  /** 대시보드 그리드 라인 표시 여부 */
  dashboardShowGridLines: boolean;
  /** 디바이스 그리드 레이아웃 (deviceId -> layout) */
  deviceGridLayout: Record<string, DashboardLayoutItem>;
  /** 디바이스 그리드 편집 모드 (비영속) */
  deviceGridEditMode: boolean;
  /** 플로우 에디터: 노드 이동 시 그리드에 스냅 (영속) */
  editorSnapToGrid: boolean;
  /** 플로우 에디터: 스냅 그리드 간격 (px). Background dots gap 과 일치 (영속) */
  editorSnapGridSize: number;
  /**
   * 2026-05-31: 플로우 에디터 표시 설정 (per-flow-id, 영속).
   * 각 플로우 단위로 포트별 통계 / 포트 이름 표시 토글.
   * 키 = flow id, 값 = { showPortStats, showPortNames }.
   */
  flowDisplaySettings: Record<string, FlowDisplaySettings>;
  notifications: Notification[];

  // ---- SPEC-DASHBOARD-004: 서버 상태 단일 축 ----

  /**
   * 요청자가 접근 가능한 대시보드 메타 목록 (`GET /api/v1/dashboards`).
   *
   * 인가 판정(`can_edit` 등)은 서버가 실어 보낸 값을 그대로 쓴다 — 프론트에서
   * 재계산하면 규칙이 두 곳에 생겨 한쪽만 갱신된다(spec.md §2.3).
   */
  dashboards: Dashboard[];
}

interface UIActions {
  toggleSidebar: () => void;
  setSidebarCollapsed: (collapsed: boolean) => void;
  setTheme: (theme: ThemeMode) => void;
  setCustomThemeTokens: (tokens: Record<string, string>) => void;
  resetCustomThemeTokens: () => void;

  // 대시보드 전역 설정
  setDashboardRefreshInterval: (seconds: number) => void;
  setDashboardEditMode: (on: boolean) => void;
  /** 원격 대시보드 렌더 모드 설정 (SPEC-REMOTE-001 M11.4, 영속). */
  setRemoteDashboardRenderMode: (mode: RemoteDashboardRenderMode) => void;
  setDashboardGridCols: (cols: number) => void;
  setDashboardShowGridLines: (show: boolean) => void;

  // 대시보드 활성 전환.
  // SPEC-DASHBOARD-004 M8: 구 모델의 페이지 CRUD 4종
  // (addDashboardPage / removeDashboardPage / renameDashboardPage /
  //  setDefaultDashboardPage)은 서버 API 로 대체되어 호출자가 사라졌으므로 제거했다.
  // 생성·삭제·이름변경·기본지정은 이제 useCreateDashboard / useDashboardMutations 가
  // 서버 왕복 후 setDashboards · applyDashboardDetail 로 반영한다.
  setActiveDashboard: (pageId: string) => void;

  // 패널 CRUD (활성 대시보드 대상)
  addPanel: (type: PanelType) => void;
  addPanelWithConfig: (type: PanelType, config: Record<string, unknown>, title?: string) => void;
  removePanel: (panelId: string) => void;
  updatePanelConfig: (panelId: string, config: Record<string, unknown>) => void;
  updatePanelTitle: (panelId: string, title: string) => void;

  // 활성 대시보드 레이아웃 관리
  setDashboardLayout: (layout: DashboardLayoutItem[]) => void;
  resetDashboardLayout: () => void;
  /** 그리드 컨테이너 실측 폭 게시(대시보드 → 설정 화면). 같은 값이면 no-op. */
  setDashboardGridWidth: (width: number) => void;

  // 디바이스 그리드
  setDeviceGridLayout: (layout: Record<string, DashboardLayoutItem>) => void;
  setDeviceGridEditMode: (on: boolean) => void;
  resetDeviceGridLayout: () => void;

  // 플로우 에디터: 그리드 스냅 (v0.18.4)
  setEditorSnapToGrid: (on: boolean) => void;
  toggleEditorSnapToGrid: () => void;
  setEditorSnapGridSize: (size: number) => void;
  /** 2026-05-31: 플로우 표시 설정 갱신 (per-flow-id, partial merge). */
  setFlowDisplaySettings: (flowId: string, patch: Partial<FlowDisplaySettings>) => void;

  // 알림
  addNotification: (notification: Omit<Notification, 'id' | 'timestamp'>) => void;
  dismissNotification: (id: string) => void;
  clearNotifications: () => void;

  // ---- SPEC-DASHBOARD-004: 대시보드 단일 축 액션 ----

  /**
   * 목록 API 결과로 대시보드 축을 교체한다.
   *
   * 이미 본문을 받아둔 대시보드의 `panels` / `layout` 은 uid 기준으로 보존한다 —
   * 목록 재조회(권한 상실·삭제 복구)가 편집 중인 본문을 지워서는 안 된다.
   * 목록에서 사라진 대시보드는 본문도 함께 버린다.
   */
  setDashboards: (dashboards: Dashboard[]) => void;

  /**
   * 단건 조회·저장 응답을 반영한다 (메타 + 본문).
   *
   * 활성 대시보드이면 그 대시보드의 그리드 설정 3종도 함께 적용한다 — 그리드 설정은
   * 대시보드마다 다르므로(spec.md §2.1) 전환 시 이전 값이 남아서는 안 된다.
   */
  applyDashboardDetail: (detail: DashboardDetail) => void;

  /**
   * 저장 응답의 메타(특히 `version`)만 반영한다.
   *
   * 본문을 함께 덮지 않는 이유: PUT 이 비행 중일 때 사용자가 가한 변경을 서버
   * 에코가 되돌려버리기 때문이다. 보낸 본문은 이미 로컬에 있으므로 되받을 필요가 없다.
   */
  applyDashboardMeta: (meta: Dashboard) => void;
}

let notificationCounter = 0;

/** 대시보드 그리드 설정 기본값 — 서버 payload 가 값을 생략했을 때 쓴다. */
const DEFAULT_GRID_COLS = 10;
const DEFAULT_SHOW_GRID_LINES = true;
const DEFAULT_REFRESH_INTERVAL = 10;

/**
 * 활성 대시보드 1장의 본문(서버 PUT 형태)을 모은다.
 *
 * 구 모델의 `collectActivePayload` 를 대체한다 — 전송 단위가 "묶음 전체" 에서
 * "대시보드 1장" 으로 바뀌었으므로 `dashboardPages` 배열도, 사용자 UI 상태
 * (`activeDashboardId` · `deviceGridLayout`) 도 여기에 포함되지 않는다(spec.md §2.1).
 * 활성 대시보드가 없으면 null.
 */
export function collectActiveDashboardContent(state: UIState): DashboardContent | null {
  const page = state.dashboardPages.find((p) => p.id === state.activeDashboardId);
  if (!page) return null;
  return {
    panels: page.panels,
    layout: page.layout,
    gridCols: state.dashboardGridCols,
    showGridLines: state.dashboardShowGridLines,
    refreshInterval: state.dashboardRefreshInterval,
  };
}

/** 대시보드 메타에서 본문 페이지 항목을 만든다(기존 본문이 있으면 승계). */
function toDashboardPage(
  meta: Pick<Dashboard, 'uid' | 'name' | 'is_default'>,
  prev?: DashboardPageConfig,
): DashboardPageConfig {
  return {
    id: meta.uid,
    name: meta.name,
    isDefault: meta.is_default,
    panels: prev?.panels ?? [],
    layout: prev?.layout ?? [],
  };
}

/** 활성 대시보드 페이지를 찾는 헬퍼 */
function findActivePage(state: UIState): DashboardPageConfig | undefined {
  return state.dashboardPages.find((p) => p.id === state.activeDashboardId);
}

/** 활성 대시보드 페이지를 교체하는 헬퍼 */
function updateActivePage(
  state: UIState,
  updater: (page: DashboardPageConfig) => DashboardPageConfig,
): Partial<UIState> {
  const page = findActivePage(state);
  if (!page) return {};
  return {
    dashboardPages: state.dashboardPages.map((p) =>
      p.id === state.activeDashboardId ? updater(p) : p,
    ),
  };
}

export const useUIStore = create<UIState & UIActions>()(
  persist(
    (set) => ({
      // ---- State ----
      sidebarCollapsed: false,
      theme: 'system',
      customThemeTokens: {},
      dashboardRefreshInterval: 10,
      remoteDashboardRenderMode: 'responsive',
      dashboardPages: [{ ...DEFAULT_DASHBOARD_PAGE, panels: [...DEFAULT_PANELS] }],
      activeDashboardId: 'default',
      dashboardEditMode: false,
      dashboardGridCols: 10,
      dashboardGridWidth: 0,
      dashboardShowGridLines: true,
      deviceGridLayout: {},
      deviceGridEditMode: false,
      editorSnapToGrid: true,
      editorSnapGridSize: 16,
      flowDisplaySettings: {},
      notifications: [],

      // SPEC-DASHBOARD-004: 서버 목록이 도착하기 전까지는 빈 축이다. 위의 기본
      // dashboardPages 1장은 부팅 전 렌더용 자리표시자이며, setDashboards 가
      // 목록으로 통째로 교체한다.
      dashboards: [],

      // ---- Actions ----

      // 사이드바
      toggleSidebar: () =>
        set((state) => ({ sidebarCollapsed: !state.sidebarCollapsed })),

      setSidebarCollapsed: (collapsed) =>
        set({ sidebarCollapsed: collapsed }),

      // 테마
      setTheme: (theme) =>
        set({ theme }),

      setCustomThemeTokens: (tokens) =>
        set({ customThemeTokens: tokens }),

      resetCustomThemeTokens: () =>
        set({ customThemeTokens: {} }),

      // 대시보드 전역 설정
      setDashboardRefreshInterval: (seconds) =>
        set({ dashboardRefreshInterval: seconds }),

      setDashboardEditMode: (on) =>
        set({ dashboardEditMode: on }),

      setRemoteDashboardRenderMode: (mode) =>
        set({ remoteDashboardRenderMode: mode }),

      setDashboardGridCols: (cols) =>
        set({ dashboardGridCols: Math.max(4, Math.min(100, cols)) }),

      setDashboardShowGridLines: (show) =>
        set({ dashboardShowGridLines: show }),

      // 대시보드 활성 전환

      setActiveDashboard: (pageId) =>
        set({ activeDashboardId: pageId }),

      // 패널 CRUD (활성 대시보드 대상)

      addPanel: (type) =>
        set((state) => {
          const panelId = generateUUID();
          const defaults = createDefaultPanel(type);
          const newPanel: PanelConfig = { id: panelId, ...defaults };
          const size = panelDefaultSize(type);
          const newLayoutItem: DashboardLayoutItem = {
            i: panelId,
            x: 0,
            y: Infinity, // react-grid-layout이 자동으로 하단에 배치
            ...size,
          };
          const patch = updateActivePage(state, (page) => ({
            ...page,
            panels: [...page.panels, newPanel],
            layout: [...page.layout, newLayoutItem],
          }));
          return patch;
        }),

      addPanelWithConfig: (type, config, title) =>
        set((state) => {
          const panelId = generateUUID();
          const defaults = createDefaultPanel(type);
          const newPanel: PanelConfig = {
            id: panelId,
            ...defaults,
            config: { ...defaults.config, ...config },
            ...(title ? { title } : {}),
          };
          const size = panelDefaultSize(type);
          const newLayoutItem: DashboardLayoutItem = {
            i: panelId,
            x: 0,
            y: Infinity,
            ...size,
          };
          const patch = updateActivePage(state, (page) => ({
            ...page,
            panels: [...page.panels, newPanel],
            layout: [...page.layout, newLayoutItem],
          }));
          return patch;
        }),

      removePanel: (panelId) =>
        set((state) => {
          const patch = updateActivePage(state, (page) => ({
            ...page,
            panels: page.panels.filter((p) => p.id !== panelId),
            layout: page.layout.filter((l) => l.i !== panelId),
          }));
          return patch;
        }),

      updatePanelConfig: (panelId, config) =>
        set((state) => {
          const patch = updateActivePage(state, (page) => ({
            ...page,
            panels: page.panels.map((p) =>
              p.id === panelId ? { ...p, config: { ...p.config, ...config } } : p,
            ),
          }));
          return patch;
        }),

      updatePanelTitle: (panelId, title) =>
        set((state) => {
          const patch = updateActivePage(state, (page) => ({
            ...page,
            panels: page.panels.map((p) =>
              p.id === panelId ? { ...p, title } : p,
            ),
          }));
          return patch;
        }),

      // 활성 대시보드 레이아웃

      setDashboardLayout: (layout) =>
        set((state) => {
          const patch = updateActivePage(state, (page) => ({ ...page, layout }));
          return patch;
        }),

      resetDashboardLayout: () =>
        set((state) => {
          const patch = updateActivePage(state, (page) => ({
            ...page,
            panels: [...DEFAULT_PANELS],
            layout: [...DEFAULT_DASHBOARD_LAYOUT],
          }));
          return patch;
        }),

      // 그리드 실측 폭 게시. ResizeObserver 가 매 프레임 부르므로 동일 값이면 set 을 건너뛴다
      // (불필요한 구독자 재렌더 방지 — 이 값은 대시보드 렌더 경로에서도 읽힌다).
      setDashboardGridWidth: (width) =>
        set((state) => (state.dashboardGridWidth === width ? {} : { dashboardGridWidth: width })),

      // 디바이스 그리드
      setDeviceGridLayout: (layout) =>
        set({ deviceGridLayout: layout }),

      setDeviceGridEditMode: (on) =>
        set({ deviceGridEditMode: on }),

      resetDeviceGridLayout: () =>
        set({ deviceGridLayout: {} }),

      // 플로우 에디터: 그리드 스냅 (v0.18.4)
      setEditorSnapToGrid: (on) =>
        set({ editorSnapToGrid: on }),

      toggleEditorSnapToGrid: () =>
        set((state) => ({ editorSnapToGrid: !state.editorSnapToGrid })),

      setEditorSnapGridSize: (size) =>
        set({ editorSnapGridSize: Math.max(4, Math.min(128, size)) }),

      // 2026-05-31: 플로우 표시 설정 (per-flow-id) 갱신
      setFlowDisplaySettings: (flowId, patch) =>
        set((state) => ({
          flowDisplaySettings: {
            ...state.flowDisplaySettings,
            [flowId]: {
              ...DEFAULT_FLOW_DISPLAY_SETTINGS,
              ...state.flowDisplaySettings[flowId],
              ...patch,
            },
          },
        })),

      // 알림
      addNotification: (notification) =>
        set((state) => ({
          notifications: [
            ...state.notifications,
            {
              ...notification,
              id: `notification-${Date.now()}-${++notificationCounter}`,
              timestamp: Date.now(),
            },
          ],
        })),

      dismissNotification: (id) =>
        set((state) => ({
          notifications: state.notifications.filter((n) => n.id !== id),
        })),

      clearNotifications: () =>
        set({ notifications: [] }),

      // ---- SPEC-DASHBOARD-004: 대시보드 단일 축 액션 ----

      setDashboards: (dashboards) =>
        set((state) => {
          const prevByUid = new Map(state.dashboardPages.map((page) => [page.id, page]));
          return {
            dashboards,
            dashboardPages: dashboards.map((d) => toDashboardPage(d, prevByUid.get(d.uid))),
          };
        }),

      applyDashboardMeta: (meta) =>
        set((state) => ({
          dashboards: state.dashboards.some((d) => d.uid === meta.uid)
            ? state.dashboards.map((d) => (d.uid === meta.uid ? meta : d))
            : [...state.dashboards, meta],
          dashboardPages: state.dashboardPages.map((p) =>
            p.id === meta.uid ? { ...p, name: meta.name, isDefault: meta.is_default } : p,
          ),
        })),

      applyDashboardDetail: (detail) =>
        set((state) => {
          // payload 를 뺀 메타만 축에 남긴다 — 본문은 dashboardPages 가 소유한다.
          const { payload, ...meta } = detail;
          const exists = state.dashboards.some((d) => d.uid === meta.uid);
          const dashboards = exists
            ? state.dashboards.map((d) => (d.uid === meta.uid ? meta : d))
            : [...state.dashboards, meta];

          const page: DashboardPageConfig = {
            id: meta.uid,
            name: meta.name,
            isDefault: meta.is_default,
            // 서버가 옛 이름(line-chart)을 담고 있어도 여기서 현재 이름으로 읽는다.
            panels: normalizePanels(payload?.panels ?? []),
            layout: payload?.layout ?? [],
          };
          const hasPage = state.dashboardPages.some((p) => p.id === meta.uid);
          const dashboardPages = hasPage
            ? state.dashboardPages.map((p) => (p.id === meta.uid ? page : p))
            : [...state.dashboardPages, page];

          const next: Partial<UIState> = { dashboards, dashboardPages };
          if (state.activeDashboardId === meta.uid) {
            next.dashboardGridCols = payload?.gridCols ?? DEFAULT_GRID_COLS;
            next.dashboardShowGridLines = payload?.showGridLines ?? DEFAULT_SHOW_GRID_LINES;
            next.dashboardRefreshInterval = payload?.refreshInterval ?? DEFAULT_REFRESH_INTERVAL;
          }
          return next;
        }),
    }),
    {
      name: 'xflow-ui',
      version: 5,
      migrate: (persistedState: unknown, version: number) => {
        const state = persistedState as Record<string, unknown>;

        // v0 -> v1: light/dark -> day/night 테마 마이그레이션
        if (version === 0) {
          if (state.theme === 'light') state.theme = 'day';
          if (state.theme === 'dark') state.theme = 'night';
          if (!state.customThemeTokens) state.customThemeTokens = {};
        }

        // v1 -> v2: 단일 대시보드 -> 멀티 페이지 마이그레이션
        if (version < 2) {
          const pages: DashboardPageConfig[] = [
            {
              id: 'default',
              name: '대시보드',
              isDefault: true,
              panels: [
                {
                  id: 'flows-default',
                  type: 'flows' as PanelType,
                  title: (state.flowPanelTitle as string) || '플로우 현황',
                  config: {
                    visibleColumns:
                      (state.flowVisibleColumns as string[]) ||
                      ['name', 'status', 'node_count', 'updated_at', 'actions'],
                  },
                },
                {
                  id: 'agents-default',
                  type: 'agents' as PanelType,
                  title: (state.agentPanelTitle as string) || '에이전트 현황',
                  config: {
                    visibleColumns:
                      (state.agentVisibleColumns as string[]) ||
                      ['name', 'type', 'status', 'uptime', 'messages', 'actions'],
                  },
                },
                {
                  id: 'resource-default',
                  type: 'resource' as PanelType,
                  title: (state.resourcePanelTitle as string) || '프로세스 상태',
                  config: {
                    visibleMetrics:
                      (state.dashboardVisibleMetrics as string[]) ||
                      ['cpu', 'memory', 'throughput', 'errorRate'],
                  },
                },
              ],
              layout:
                (state.dashboardLayout as DashboardLayoutItem[]) || DEFAULT_DASHBOARD_LAYOUT,
            },
          ];
          state.dashboardPages = pages;
          state.activeDashboardId = 'default';

          // 이전 단일-대시보드 필드 제거
          delete state.dashboardLayout;
          delete state.dashboardVisibleMetrics;
          delete state.flowPanelTitle;
          delete state.flowVisibleColumns;
          delete state.agentPanelTitle;
          delete state.agentVisibleColumns;
          delete state.resourcePanelTitle;
        }

        // v2 -> v3: 그리드 설정 persist 추가
        if (version < 3) {
          if (state.dashboardGridCols === undefined) state.dashboardGridCols = 10;
          if (state.dashboardShowGridLines === undefined) state.dashboardShowGridLines = true;
        }

        // v3 -> v4: 차트 패널(5종) config 키 재정의 (SPEC-CHART-001 M5).
        // 기존 `dataSource` / `period` / `value` 등의 필드를 제거하고
        // `channel_name` 을 비어있는 문자열로 초기화한다. 사용자는 재설정 필요.
        if (version < 4) {
          const chartTypes = new Set(['stat', 'line-chart', 'bar-chart', 'pie-chart', 'table']);
          const pages = state.dashboardPages as DashboardPageConfig[] | undefined;
          if (Array.isArray(pages)) {
            for (const page of pages) {
              if (!Array.isArray(page.panels)) continue;
              for (const panel of page.panels) {
                if (chartTypes.has(panel.type)) {
                  const cfg = (panel.config ?? {}) as Record<string, unknown>;
                  // channel_name 이 없으면 초기화 (나머지는 손대지 않음)
                  if (typeof cfg.channel_name !== 'string') {
                    cfg.channel_name = '';
                  }
                  panel.config = cfg;
                }
              }
            }
          }
        }

        // v4 -> v5: 라인 차트 → 그래프 차트 (이름만 변경, 같은 패널).
        // 스타일 축(라인·영역·바·캔들)이 생기면서 이름이 한 스타일에 묶여 있는 것이
        // 어색해졌다. config 는 손대지 않는다 — 바뀐 것은 이름뿐이다.
        if (version < 5) {
          const pages = state.dashboardPages as DashboardPageConfig[] | undefined;
          if (Array.isArray(pages)) {
            for (const page of pages) {
              if (!Array.isArray(page.panels)) continue;
              for (const panel of page.panels) {
                if ((panel.type as string) === 'line-chart') panel.type = 'graph-chart';
              }
            }
          }
        }

        return state as unknown as UIState & UIActions;
      },
      // 대시보드 관련 키는 partialize 에서 제외한다.
      //   - dashboardPages, dashboards (서버 목록 + 본문)
      //   - dashboardGridCols, dashboardShowGridLines, dashboardRefreshInterval
      //     (대시보드 payload 에 속한다 — spec.md §2.1)
      //   - activeDashboardId, deviceGridLayout (서버 /dashboard-state 소유)
      // 기기별 환경설정만 영속한다.
      partialize: (state) => ({
        sidebarCollapsed: state.sidebarCollapsed,
        theme: state.theme,
        customThemeTokens: state.customThemeTokens,
        editorSnapToGrid: state.editorSnapToGrid,
        editorSnapGridSize: state.editorSnapGridSize,
        flowDisplaySettings: state.flowDisplaySettings,
        // SPEC-REMOTE-001 M11.4: 원격 대시보드 렌더 모드(뷰 전역 환경설정)는
        // 기기별로 영속한다(노드 snapshot 과 무관 — 서버 동기 대상 아님).
        remoteDashboardRenderMode: state.remoteDashboardRenderMode,
      }),
    },
  ),
);
