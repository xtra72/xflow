// UI 상태 관리 - 선택적 localStorage 영속화 포함.
//
// SPEC-DASHBOARD-001 v0.2.0:
// - 대시보드 구성(`dashboardPages`, `activeDashboardId`, 그리드 설정,
//   `deviceGridLayout`) 은 더 이상 localStorage 에 영속화되지 않으며,
//   `sharedSnapshot` / `mineSnapshot` 두 슬롯에서 서버 snapshot 으로 관리된다.
// - 기존 컴포넌트가 직접 접근하는 legacy 필드(`dashboardPages` 등) 는
//   읽기 호환을 위해 상태로 유지하되, 활성 스코프(`activeDashboardScope`)
//   의 snapshot.payload 와 mutation 시 동기 갱신된다.
// - v0.2.0 첫 부팅 시 1회만 기존 localStorage 의 대시보드 키들을 제거하고
//   useDashboardSync 가 토스트로 안내한다 (`migrationToastPendingFlag()` 참조).

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

import type { DashboardScope, DashboardSnapshot } from '@/types/dashboard';
import { generateUUID } from '@/lib/utils/uuid';
// SPEC-HEATMAP-PANEL-001: 히트맵 패널 기본 config 빌더(파서와 기본값 일치 보장).
import { buildDefaultHeatmapConfig } from '@/pages/dashboard/panels/heatmap/heatmapConfig';

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

/** 활성 스코프 sessionStorage 키 (탭 새로고침 시 유지). */
const ACTIVE_SCOPE_SESSION_KEY = 'xflow-ui:active-dashboard-scope';

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

/** 활성 스코프를 sessionStorage 에서 읽는다. 없거나 잘못된 값이면 null. */
export function readActiveScopeFromSession(): DashboardScope | 'shared' | 'mine' | null {
  if (typeof globalThis === 'undefined' || typeof globalThis.sessionStorage === 'undefined') {
    return null;
  }
  try {
    const v = globalThis.sessionStorage.getItem(ACTIVE_SCOPE_SESSION_KEY);
    if (v === 'shared' || v === 'mine') return v;
    return null;
  } catch {
    return null;
  }
}

/** 활성 스코프를 sessionStorage 에 기록한다. */
function writeActiveScopeToSession(scope: 'shared' | 'mine'): void {
  if (typeof globalThis === 'undefined' || typeof globalThis.sessionStorage === 'undefined') {
    return;
  }
  try {
    globalThis.sessionStorage.setItem(ACTIVE_SCOPE_SESSION_KEY, scope);
  } catch {
    // 무시
  }
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

export const ALL_DEVICE_COLUMNS = ['name', 'type', 'status', 'agent', 'last_seen'] as const;
export type DeviceColumnKey = (typeof ALL_DEVICE_COLUMNS)[number];

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
  | 'line-chart'
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
  | 'heatmap';

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
    title: '프로세스 리소스',
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

/** 패널 타입별 기본 그리드 크기.
 *
 * 차트 계열 5종 (stat/line-chart/bar-chart/pie-chart/table) 크기는
 * SPEC-CHART-001 REQ-M5-05 에 정의되어 있다.
 */
function panelDefaultSize(type: PanelType): Pick<DashboardLayoutItem, 'w' | 'h' | 'minW' | 'minH'> {
  switch (type) {
    case 'stat':
      // SPEC REQ-M5-05: stat {w:2, h:1}
      return { w: 2, h: 1, minW: 2, minH: 1 };
    case 'gauge':
      return { w: 2, h: 3, minW: 2, minH: 2 };
    case 'text':
      return { w: 3, h: 2, minW: 2, minH: 2 };
    case 'ac-control':
      return { w: 3, h: 5, minW: 2, minH: 4 };
    case 'hvac-control':
      return { w: 5, h: 5, minW: 4, minH: 4 };
    case 'custom-control':
      return { w: 3, h: 5, minW: 2, minH: 3 };
    case 'properties-grid':
      return { w: 4, h: 4, minW: 2, minH: 2 };
    // SPEC-FACILITY-DASHBOARD-001 M5: 설비 패널 3종
    case 'facility-device':
      // 단일 기기(에어컨 제어와 유사한 세로 카드).
      return { w: 3, h: 5, minW: 2, minH: 4 };
    case 'facility-station':
      // 역사(통계 + 기기 목록 + 일괄 제어).
      return { w: 5, h: 6, minW: 3, minH: 4 };
    case 'facility-line':
      // 호선(라인도 포함으로 더 넓게).
      return { w: 8, h: 6, minW: 4, minH: 4 };
    case 'facility-group':
      // 그룹(그룹별 통계 + 일괄 제어 목록). SPEC-XSFM-GROUP-001 M7.
      return { w: 5, h: 6, minW: 3, minH: 4 };
    // SPEC-TRIGGER-PANEL-001 M2: 스케줄 리스트 + 카탈로그 편집을 담는 세로 카드.
    case 'trigger-config':
      return { w: 4, h: 7, minW: 3, minH: 4 };
    // SPEC-TRIGGER-SCHED-001 M2: 예약 규칙 테이블(6컬럼). 가로로 넓은 카드.
    case 'facility-schedule':
      return { w: 8, h: 6, minW: 5, minH: 4 };
    // SPEC-MODBUS-012 M1: MODBUS Gateway 패널 6종(목업 레이아웃 부합).
    // 레지스터 맵 그리드는 넓게, 목록은 세로로, 미니차트/요약 바는 낮게.
    case 'modbus-shared-registers':
    case 'modbus-device-registers':
      return { w: 6, h: 5, minW: 4, minH: 3 };
    case 'modbus-real-devices':
    case 'modbus-virtual-devices':
      return { w: 4, h: 5, minW: 3, minH: 3 };
    case 'modbus-bus-stats':
      return { w: 6, h: 3, minW: 3, minH: 2 };
    case 'modbus-summary-stats':
      return { w: 8, h: 2, minW: 4, minH: 2 };
    // SPEC-DASHBOARD-002: 단일 에이전트 상태·통계 패널(통계 타일 그리드 + 헤더).
    case 'agent-status':
      return { w: 4, h: 5, minW: 3, minH: 3 };
    // SPEC-HEATMAP-PANEL-001: 히트맵 패널(권장 기본 크기 {w:5,h:4}).
    case 'heatmap':
      return { w: 5, h: 4, minW: 3, minH: 3 };
    case 'line-chart':
      // SPEC REQ-M5-05: line-chart {w:6, h:3}
      return { w: 6, h: 3, minW: 3, minH: 2 };
    case 'bar-chart':
      // SPEC REQ-M5-05: bar-chart {w:4, h:3}
      return { w: 4, h: 3, minW: 3, minH: 2 };
    case 'pie-chart':
      // SPEC REQ-M5-05: pie-chart {w:3, h:3}
      return { w: 3, h: 3, minW: 3, minH: 3 };
    case 'table':
      // SPEC REQ-M5-05: table {w:6, h:4}
      return { w: 6, h: 4, minW: 4, minH: 3 };
    default:
      return { w: 5, h: 4, minW: 3, minH: 3 };
  }
}

/** 패널 타입별 기본값 생성 */
function createDefaultPanel(type: PanelType): Omit<PanelConfig, 'id'> {
  switch (type) {
    case 'flows':
      return { type, title: '플로우 현황', config: { visibleColumns: [...ALL_FLOW_COLUMNS] } };
    case 'agents':
      return { type, title: '에이전트 현황', config: { visibleColumns: [...ALL_AGENT_COLUMNS] } };
    case 'resource':
      return { type, title: '프로세스 리소스', config: { visibleMetrics: [...ALL_METRIC_KEYS] } };
    case 'devices':
      return { type, title: '디바이스', config: {} };
    case 'device':
      return { type, title: '디바이스', config: {} };
    case 'logs':
      return { type, title: '로그', config: { maxLines: 100 } };
    case 'stat':
      // SPEC-CHART-001 §4.2.2 stat config
      return {
        type,
        title: '통계',
        config: { channel_name: '', display_field: 'value', unit: '', decimal_places: 2 },
      };
    case 'gauge':
      return { type, title: '게이지', config: { value: 75, min: 0, max: 100, unit: '%', gaugeType: 'simple' } };
    case 'line-chart':
      // SPEC-CHART-001 §4.2.2 line-chart config
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
        },
      };
    case 'text':
      return { type, title: '텍스트', config: { content: '', format: 'markdown' } };
    case 'table':
      // SPEC-CHART-001 §4.2.2 table config
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
      return { type, title: '속성 그리드', config: { deviceId: '', gridCols: 3, visibleProperties: [] } };
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
      return { type, title: '실제 디바이스', config: { agentId: '' } };
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
  /** 멀티-대시보드 페이지 목록 — 활성 스코프 snapshot.payload 와 동기. */
  dashboardPages: DashboardPageConfig[];
  /** 현재 활성 대시보드 페이지 ID — 활성 스코프 snapshot.payload 와 동기. */
  activeDashboardId: string;
  /** 대시보드 편집 모드 (비영속) */
  dashboardEditMode: boolean;
  /** 대시보드 그리드 칼럼 수 */
  dashboardGridCols: number;
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

  // ---- SPEC-DASHBOARD-001 v0.2.0: 서버 snapshot 슬롯 ----

  /** 공유(global) 대시보드 snapshot. 부팅 GET 404 시 version=0 의 기본 snapshot. */
  sharedSnapshot: DashboardSnapshot | null;
  /** 본인(user) 대시보드 snapshot. */
  mineSnapshot: DashboardSnapshot | null;
  /** 활성 스코프. UI 탭 ("공유" / "내 대시보드") 와 1:1. */
  activeDashboardScope: 'shared' | 'mine';
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

  // 대시보드 페이지 CRUD
  addDashboardPage: (name: string) => void;
  removeDashboardPage: (pageId: string) => void;
  renameDashboardPage: (pageId: string, name: string) => void;
  setDefaultDashboardPage: (pageId: string) => void;
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

  // ---- SPEC-DASHBOARD-001 v0.2.0: snapshot 슬롯 액션 ----

  /**
   * 공유(global) snapshot 을 교체한다. 활성 스코프가 'shared' 이면 legacy 필드
   * (dashboardPages 등) 도 함께 sync 된다.
   *
   * @param opts.fromServer 서버 응답으로 인한 적용임을 표시 (useDashboardSync 가 사용).
   */
  setSharedSnapshot: (snapshot: DashboardSnapshot | null, opts?: { fromServer?: boolean }) => void;

  /** 본인(user) snapshot 을 교체한다. 활성 스코프가 'mine' 이면 legacy 필드 sync. */
  setMineSnapshot: (snapshot: DashboardSnapshot | null, opts?: { fromServer?: boolean }) => void;

  /** 활성 스코프를 변경한다 — 해당 snapshot.payload 를 legacy 필드로 복사. sessionStorage 영속. */
  setActiveDashboardScope: (scope: 'shared' | 'mine') => void;
}

let notificationCounter = 0;

/** snapshot.payload 를 legacy state 필드 형태로 투영한다 (snapshot → legacy sync). */
function projectPayloadToLegacy(payload: import('@/types/dashboard').DashboardPayload): Partial<UIState> {
  return {
    dashboardPages: payload.dashboardPages,
    activeDashboardId: payload.activeDashboardId,
    dashboardGridCols: payload.dashboardGridCols,
    dashboardShowGridLines: payload.dashboardShowGridLines,
    dashboardRefreshInterval: payload.dashboardRefreshInterval,
    deviceGridLayout: payload.deviceGridLayout,
  };
}

/** 현재 store 상태에서 활성 페이로드(서버 PUT 형태) 를 모은다 (legacy → payload). */
export function collectActivePayload(
  state: UIState,
): import('@/types/dashboard').DashboardPayload {
  return {
    dashboardPages: state.dashboardPages,
    activeDashboardId: state.activeDashboardId,
    dashboardGridCols: state.dashboardGridCols,
    dashboardShowGridLines: state.dashboardShowGridLines,
    dashboardRefreshInterval: state.dashboardRefreshInterval,
    deviceGridLayout: state.deviceGridLayout,
  };
}

/** 활성 스코프의 snapshot 을 반환한다 (없으면 null). */
export function getActiveSnapshot(state: UIState): DashboardSnapshot | null {
  return state.activeDashboardScope === 'shared' ? state.sharedSnapshot : state.mineSnapshot;
}

/** 활성 스코프의 dashboardPages 를 반환 (snapshot 우선, fallback: state.dashboardPages, 최후: DEFAULT). */
export function getActiveDashboardPages(state: UIState): DashboardPageConfig[] {
  const snap = getActiveSnapshot(state);
  if (snap) return snap.payload.dashboardPages;
  if (state.dashboardPages.length > 0) return state.dashboardPages;
  return [{ ...DEFAULT_DASHBOARD_PAGE, panels: [...DEFAULT_PANELS] }];
}

/** 빌트인 기본 페이로드 (서버 GET 404 시 fallback 용). */
export function buildDefaultDashboardPayload(): import('@/types/dashboard').DashboardPayload {
  return {
    dashboardPages: [{ ...DEFAULT_DASHBOARD_PAGE, panels: [...DEFAULT_PANELS] }],
    activeDashboardId: 'default',
    dashboardGridCols: 10,
    dashboardShowGridLines: true,
    dashboardRefreshInterval: 10,
    deviceGridLayout: {},
  };
}

/** 빌트인 기본 snapshot (version=0 = "서버에 아직 없음" 의 marker). */
export function buildDefaultSnapshot(
  scope: 'global' | 'user',
  owner: string | null,
): DashboardSnapshot {
  return {
    scope,
    owner,
    version: 0,
    updatedAt: 0,
    payload: buildDefaultDashboardPayload(),
  };
}

/**
 * legacy 필드 변경 (`dashboardPages` 등) 을 활성 스코프 snapshot.payload 로 mirror 한다.
 *
 * - 활성 snapshot 이 없으면 빌트인 기본값을 기반으로 새 snapshot (version=0) 을 만든다.
 * - version/updatedAt 은 그대로 둔다 — 서버 PUT 응답으로 갱신될 예정.
 */
function mirrorLegacyToActiveSnapshot(
  prev: UIState,
  patch: Partial<UIState>,
): Partial<UIState> {
  // mutation 결과를 적용한 가상 state 를 만들어 payload 를 추출한다.
  const merged: UIState = { ...prev, ...patch };
  const payload = collectActivePayload(merged);
  const active = merged.activeDashboardScope;

  if (active === 'shared') {
    const base = merged.sharedSnapshot ?? buildDefaultSnapshot('global', null);
    return {
      ...patch,
      sharedSnapshot: { ...base, payload },
    };
  } else {
    const base = merged.mineSnapshot ?? buildDefaultSnapshot('user', null);
    return {
      ...patch,
      mineSnapshot: { ...base, payload },
    };
  }
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
      dashboardShowGridLines: true,
      deviceGridLayout: {},
      deviceGridEditMode: false,
      editorSnapToGrid: true,
      editorSnapGridSize: 16,
      flowDisplaySettings: {},
      notifications: [],

      // v0.2.0: snapshot 슬롯 — sessionStorage 우선, 없으면 'shared' 기본.
      sharedSnapshot: null,
      mineSnapshot: null,
      activeDashboardScope: (readActiveScopeFromSession() ?? 'shared') as 'shared' | 'mine',

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
        set((state) => mirrorLegacyToActiveSnapshot(state, { dashboardRefreshInterval: seconds })),

      setDashboardEditMode: (on) =>
        set({ dashboardEditMode: on }),

      setRemoteDashboardRenderMode: (mode) =>
        set({ remoteDashboardRenderMode: mode }),

      setDashboardGridCols: (cols) =>
        set((state) =>
          mirrorLegacyToActiveSnapshot(state, {
            dashboardGridCols: Math.max(4, Math.min(100, cols)),
          }),
        ),

      setDashboardShowGridLines: (show) =>
        set((state) => mirrorLegacyToActiveSnapshot(state, { dashboardShowGridLines: show })),

      // 대시보드 페이지 CRUD

      addDashboardPage: (name) =>
        set((state) => {
          const newPage: DashboardPageConfig = {
            id: generateUUID(),
            name,
            isDefault: false,
            panels: [],
            layout: [],
          };
          return mirrorLegacyToActiveSnapshot(state, {
            dashboardPages: [...state.dashboardPages, newPage],
            activeDashboardId: newPage.id,
          });
        }),

      removeDashboardPage: (pageId) =>
        set((state) => {
          // 페이지가 1개뿐이면 삭제 차단
          if (state.dashboardPages.length <= 1) return state;

          const target = state.dashboardPages.find((p) => p.id === pageId);
          if (!target) return state;

          let pages = state.dashboardPages.filter((p) => p.id !== pageId);

          // 기본 페이지를 삭제한 경우 다른 페이지를 기본으로 설정
          if (target.isDefault && pages.length > 0) {
            pages = pages.map((p, idx) => (idx === 0 ? { ...p, isDefault: true } : p));
          }

          // 활성 페이지를 삭제한 경우 기본 페이지로 전환
          let newActiveId = state.activeDashboardId;
          if (pageId === state.activeDashboardId) {
            const defaultPage = pages.find((p) => p.isDefault);
            newActiveId = defaultPage ? defaultPage.id : pages[0]!.id;
          }

          return mirrorLegacyToActiveSnapshot(state, {
            dashboardPages: pages,
            activeDashboardId: newActiveId,
          });
        }),

      renameDashboardPage: (pageId, name) =>
        set((state) =>
          mirrorLegacyToActiveSnapshot(state, {
            dashboardPages: state.dashboardPages.map((p) =>
              p.id === pageId ? { ...p, name } : p,
            ),
          }),
        ),

      setDefaultDashboardPage: (pageId) =>
        set((state) =>
          mirrorLegacyToActiveSnapshot(state, {
            dashboardPages: state.dashboardPages.map((p) => ({
              ...p,
              isDefault: p.id === pageId,
            })),
          }),
        ),

      setActiveDashboard: (pageId) =>
        set((state) => mirrorLegacyToActiveSnapshot(state, { activeDashboardId: pageId })),

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
          return mirrorLegacyToActiveSnapshot(state, patch);
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
          return mirrorLegacyToActiveSnapshot(state, patch);
        }),

      removePanel: (panelId) =>
        set((state) => {
          const patch = updateActivePage(state, (page) => ({
            ...page,
            panels: page.panels.filter((p) => p.id !== panelId),
            layout: page.layout.filter((l) => l.i !== panelId),
          }));
          return mirrorLegacyToActiveSnapshot(state, patch);
        }),

      updatePanelConfig: (panelId, config) =>
        set((state) => {
          const patch = updateActivePage(state, (page) => ({
            ...page,
            panels: page.panels.map((p) =>
              p.id === panelId ? { ...p, config: { ...p.config, ...config } } : p,
            ),
          }));
          return mirrorLegacyToActiveSnapshot(state, patch);
        }),

      updatePanelTitle: (panelId, title) =>
        set((state) => {
          const patch = updateActivePage(state, (page) => ({
            ...page,
            panels: page.panels.map((p) =>
              p.id === panelId ? { ...p, title } : p,
            ),
          }));
          return mirrorLegacyToActiveSnapshot(state, patch);
        }),

      // 활성 대시보드 레이아웃

      setDashboardLayout: (layout) =>
        set((state) => {
          const patch = updateActivePage(state, (page) => ({ ...page, layout }));
          return mirrorLegacyToActiveSnapshot(state, patch);
        }),

      resetDashboardLayout: () =>
        set((state) => {
          const patch = updateActivePage(state, (page) => ({
            ...page,
            panels: [...DEFAULT_PANELS],
            layout: [...DEFAULT_DASHBOARD_LAYOUT],
          }));
          return mirrorLegacyToActiveSnapshot(state, patch);
        }),

      // 디바이스 그리드
      setDeviceGridLayout: (layout) =>
        set((state) => mirrorLegacyToActiveSnapshot(state, { deviceGridLayout: layout })),

      setDeviceGridEditMode: (on) =>
        set({ deviceGridEditMode: on }),

      resetDeviceGridLayout: () =>
        set((state) => mirrorLegacyToActiveSnapshot(state, { deviceGridLayout: {} })),

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

      // ---- SPEC-DASHBOARD-001 v0.2.0: snapshot 액션 ----

      setSharedSnapshot: (snapshot, _opts) =>
        set((state) => {
          const next: Partial<UIState> = { sharedSnapshot: snapshot };
          if (state.activeDashboardScope === 'shared' && snapshot) {
            Object.assign(next, projectPayloadToLegacy(snapshot.payload));
          }
          return next as UIState;
        }),

      setMineSnapshot: (snapshot, _opts) =>
        set((state) => {
          const next: Partial<UIState> = { mineSnapshot: snapshot };
          if (state.activeDashboardScope === 'mine' && snapshot) {
            Object.assign(next, projectPayloadToLegacy(snapshot.payload));
          }
          return next as UIState;
        }),

      setActiveDashboardScope: (scope) =>
        set((state) => {
          writeActiveScopeToSession(scope);
          const target = scope === 'shared' ? state.sharedSnapshot : state.mineSnapshot;
          const legacy = target ? projectPayloadToLegacy(target.payload) : {};
          return { activeDashboardScope: scope, ...legacy } as Partial<UIState> as UIState;
        }),
    }),
    {
      name: 'xflow-ui',
      version: 4,
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
                  title: (state.resourcePanelTitle as string) || '프로세스 리소스',
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

        return state as unknown as UIState & UIActions;
      },
      // SPEC-DASHBOARD-001 v0.2.0: 대시보드 관련 키 6 개를 partialize 에서 제외.
      //   - dashboardPages, activeDashboardId
      //   - dashboardGridCols, dashboardShowGridLines, dashboardRefreshInterval
      //   - deviceGridLayout
      // 위 값들은 서버 snapshot (`sharedSnapshot` / `mineSnapshot`) 으로 관리되며
      // 활성 스코프에서 derive 된다. 기기별 환경설정만 영속.
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
