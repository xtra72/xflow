// UI 상태 관리 - 선택적 localStorage 영속화 포함.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export interface Notification {
  id: string;
  type: 'info' | 'success' | 'warning' | 'error';
  message: string;
  timestamp: number;
}

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
  | 'properties-grid';

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

/** 패널 타입별 기본 그리드 크기 */
function panelDefaultSize(type: PanelType): Pick<DashboardLayoutItem, 'w' | 'h' | 'minW' | 'minH'> {
  switch (type) {
    case 'stat':
      return { w: 2, h: 2, minW: 2, minH: 2 };
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
    case 'line-chart':
    case 'bar-chart':
      return { w: 5, h: 4, minW: 3, minH: 3 };
    case 'pie-chart':
      return { w: 3, h: 4, minW: 3, minH: 3 };
    case 'table':
      return { w: 5, h: 4, minW: 4, minH: 3 };
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
      return { type, title: '통계', config: { value: '', unit: '', label: '' } };
    case 'gauge':
      return { type, title: '게이지', config: { value: 75, min: 0, max: 100, unit: '%', gaugeType: 'simple' } };
    case 'line-chart':
      return { type, title: '라인 차트', config: { dataSource: '', period: '1h' } };
    case 'bar-chart':
      return { type, title: '바 차트', config: { dataSource: '', period: '1h' } };
    case 'pie-chart':
      return { type, title: '파이 차트', config: { dataSource: '' } };
    case 'text':
      return { type, title: '텍스트', config: { content: '', format: 'markdown' } };
    case 'table':
      return { type, title: '테이블', config: { dataSource: '', columns: [] } };
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
  }
}

// ---- 테마 모드 타입 ----

/** 테마 모드: system(OS 설정 따름) | day(라이트) | night(다크) | custom(사용자 정의) */
export type ThemeMode = 'system' | 'day' | 'night' | 'custom';

// ---- Store ----

interface UIState {
  sidebarCollapsed: boolean;
  theme: ThemeMode;
  /** 커스텀 테마 CSS 변수 토큰 (변수명 -> 값) */
  customThemeTokens: Record<string, string>;
  /** 대시보드 자동 갱신 주기 (초 단위). 기본값 10. */
  dashboardRefreshInterval: number;
  /** 멀티-대시보드 페이지 목록 */
  dashboardPages: DashboardPageConfig[];
  /** 현재 활성 대시보드 페이지 ID */
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
  notifications: Notification[];
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

  // 알림
  addNotification: (notification: Omit<Notification, 'id' | 'timestamp'>) => void;
  dismissNotification: (id: string) => void;
  clearNotifications: () => void;
}

let notificationCounter = 0;

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
      dashboardPages: [{ ...DEFAULT_DASHBOARD_PAGE, panels: [...DEFAULT_PANELS] }],
      activeDashboardId: 'default',
      dashboardEditMode: false,
      dashboardGridCols: 10,
      dashboardShowGridLines: true,
      deviceGridLayout: {},
      deviceGridEditMode: false,
      notifications: [],

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

      setDashboardGridCols: (cols) =>
        set({ dashboardGridCols: Math.max(4, Math.min(100, cols)) }),

      setDashboardShowGridLines: (show) =>
        set({ dashboardShowGridLines: show }),

      // 대시보드 페이지 CRUD

      addDashboardPage: (name) =>
        set((state) => {
          const newPage: DashboardPageConfig = {
            id: globalThis.crypto?.randomUUID?.() ?? Math.random().toString(36).slice(2) + Date.now().toString(36),
            name,
            isDefault: false,
            panels: [],
            layout: [],
          };
          return {
            dashboardPages: [...state.dashboardPages, newPage],
            activeDashboardId: newPage.id,
          };
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

          return { dashboardPages: pages, activeDashboardId: newActiveId };
        }),

      renameDashboardPage: (pageId, name) =>
        set((state) => ({
          dashboardPages: state.dashboardPages.map((p) =>
            p.id === pageId ? { ...p, name } : p,
          ),
        })),

      setDefaultDashboardPage: (pageId) =>
        set((state) => ({
          dashboardPages: state.dashboardPages.map((p) => ({
            ...p,
            isDefault: p.id === pageId,
          })),
        })),

      setActiveDashboard: (pageId) =>
        set({ activeDashboardId: pageId }),

      // 패널 CRUD (활성 대시보드 대상)

      addPanel: (type) =>
        set((state) => {
          const panelId = globalThis.crypto?.randomUUID?.() ?? Math.random().toString(36).slice(2) + Date.now().toString(36);
          const defaults = createDefaultPanel(type);
          const newPanel: PanelConfig = { id: panelId, ...defaults };
          const size = panelDefaultSize(type);
          const newLayoutItem: DashboardLayoutItem = {
            i: panelId,
            x: 0,
            y: Infinity, // react-grid-layout이 자동으로 하단에 배치
            ...size,
          };
          return updateActivePage(state, (page) => ({
            ...page,
            panels: [...page.panels, newPanel],
            layout: [...page.layout, newLayoutItem],
          }));
        }),

      addPanelWithConfig: (type, config, title) =>
        set((state) => {
          const panelId = globalThis.crypto?.randomUUID?.() ?? Math.random().toString(36).slice(2) + Date.now().toString(36);
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
          return updateActivePage(state, (page) => ({
            ...page,
            panels: [...page.panels, newPanel],
            layout: [...page.layout, newLayoutItem],
          }));
        }),

      removePanel: (panelId) =>
        set((state) =>
          updateActivePage(state, (page) => ({
            ...page,
            panels: page.panels.filter((p) => p.id !== panelId),
            layout: page.layout.filter((l) => l.i !== panelId),
          })),
        ),

      updatePanelConfig: (panelId, config) =>
        set((state) =>
          updateActivePage(state, (page) => ({
            ...page,
            panels: page.panels.map((p) =>
              p.id === panelId ? { ...p, config: { ...p.config, ...config } } : p,
            ),
          })),
        ),

      updatePanelTitle: (panelId, title) =>
        set((state) =>
          updateActivePage(state, (page) => ({
            ...page,
            panels: page.panels.map((p) =>
              p.id === panelId ? { ...p, title } : p,
            ),
          })),
        ),

      // 활성 대시보드 레이아웃

      setDashboardLayout: (layout) =>
        set((state) => updateActivePage(state, (page) => ({ ...page, layout }))),

      resetDashboardLayout: () =>
        set((state) =>
          updateActivePage(state, (page) => ({
            ...page,
            panels: [...DEFAULT_PANELS],
            layout: [...DEFAULT_DASHBOARD_LAYOUT],
          })),
        ),

      // 디바이스 그리드
      setDeviceGridLayout: (layout) =>
        set({ deviceGridLayout: layout }),

      setDeviceGridEditMode: (on) =>
        set({ deviceGridEditMode: on }),

      resetDeviceGridLayout: () =>
        set({ deviceGridLayout: {} }),

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
    }),
    {
      name: 'xflow-ui',
      version: 3,
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

        return state as unknown as UIState & UIActions;
      },
      partialize: (state) => ({
        sidebarCollapsed: state.sidebarCollapsed,
        theme: state.theme,
        customThemeTokens: state.customThemeTokens,
        dashboardRefreshInterval: state.dashboardRefreshInterval,
        dashboardPages: state.dashboardPages,
        activeDashboardId: state.activeDashboardId,
        deviceGridLayout: state.deviceGridLayout,
        dashboardGridCols: state.dashboardGridCols,
        dashboardShowGridLines: state.dashboardShowGridLines,
      }),
    },
  ),
);
