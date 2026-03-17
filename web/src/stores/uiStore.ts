// UI state management with selective localStorage persistence.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export interface Notification {
  id: string;
  type: 'info' | 'success' | 'warning' | 'error';
  message: string;
  timestamp: number;
}

// ---- Dashboard layout ----

export interface DashboardLayoutItem {
  i: string;
  x: number;
  y: number;
  w: number;
  h: number;
  minW?: number;
  minH?: number;
}

export const DEFAULT_DASHBOARD_LAYOUT: DashboardLayoutItem[] = [
  { i: 'flows',    x: 0, y: 0, w: 6, h: 4, minW: 4, minH: 3 },
  { i: 'agents',   x: 6, y: 0, w: 6, h: 4, minW: 4, minH: 3 },
  { i: 'resource', x: 0, y: 4, w: 12, h: 3, minW: 4, minH: 2 },
];

export const ALL_METRIC_KEYS = ['cpu', 'memory', 'throughput', 'errorRate'] as const;
export type MetricKey = (typeof ALL_METRIC_KEYS)[number];

const DEFAULT_VISIBLE_METRICS: MetricKey[] = ['cpu', 'memory', 'throughput', 'errorRate'];

// ---- Panel column keys ----

export const ALL_FLOW_COLUMNS = ['name', 'status', 'node_count', 'updated_at', 'actions'] as const;
export type FlowColumnKey = (typeof ALL_FLOW_COLUMNS)[number];

export const ALL_AGENT_COLUMNS = ['name', 'type', 'status', 'uptime', 'messages', 'actions'] as const;
export type AgentColumnKey = (typeof ALL_AGENT_COLUMNS)[number];

const DEFAULT_FLOW_PANEL_TITLE = '플로우 현황';
const DEFAULT_AGENT_PANEL_TITLE = '에이전트 현황';
const DEFAULT_RESOURCE_PANEL_TITLE = '프로세스 리소스';

// ---- 테마 모드 타입 ----

/** 테마 모드: system(OS 설정 따름) | day(라이트) | night(다크) | custom(사용자 정의) */
export type ThemeMode = 'system' | 'day' | 'night' | 'custom';

// ---- Store ----

interface UIState {
  sidebarCollapsed: boolean;
  theme: ThemeMode;
  /** 커스텀 테마 CSS 변수 토큰 (변수명 → 값) */
  customThemeTokens: Record<string, string>;
  /** 대시보드 자동 갱신 주기 (초 단위). 기본값 10. */
  dashboardRefreshInterval: number;
  /** react-grid-layout 레이아웃 */
  dashboardLayout: DashboardLayoutItem[];
  /** 프로세스 리소스 위젯에서 표시할 메트릭 키 */
  dashboardVisibleMetrics: MetricKey[];
  /** 대시보드 편집 모드 (비영속) */
  dashboardEditMode: boolean;
  /** 패널별 설정 */
  flowPanelTitle: string;
  flowVisibleColumns: FlowColumnKey[];
  agentPanelTitle: string;
  agentVisibleColumns: AgentColumnKey[];
  resourcePanelTitle: string;
  /** 디바이스 그리드 레이아웃 (deviceId → layout) */
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
  setDashboardRefreshInterval: (seconds: number) => void;
  setDashboardLayout: (layout: DashboardLayoutItem[]) => void;
  setDashboardVisibleMetrics: (keys: MetricKey[]) => void;
  setDashboardEditMode: (on: boolean) => void;
  resetDashboardLayout: () => void;
  setFlowPanelTitle: (title: string) => void;
  setFlowVisibleColumns: (cols: FlowColumnKey[]) => void;
  setAgentPanelTitle: (title: string) => void;
  setAgentVisibleColumns: (cols: AgentColumnKey[]) => void;
  setResourcePanelTitle: (title: string) => void;
  setDeviceGridLayout: (layout: Record<string, DashboardLayoutItem>) => void;
  setDeviceGridEditMode: (on: boolean) => void;
  resetDeviceGridLayout: () => void;
  addNotification: (notification: Omit<Notification, 'id' | 'timestamp'>) => void;
  dismissNotification: (id: string) => void;
  clearNotifications: () => void;
}

let notificationCounter = 0;

export const useUIStore = create<UIState & UIActions>()(
  persist(
    (set) => ({
      // State
      sidebarCollapsed: false,
      theme: 'system',
      customThemeTokens: {},
      dashboardRefreshInterval: 10,
      dashboardLayout: DEFAULT_DASHBOARD_LAYOUT,
      dashboardVisibleMetrics: DEFAULT_VISIBLE_METRICS,
      dashboardEditMode: false,
      flowPanelTitle: DEFAULT_FLOW_PANEL_TITLE,
      flowVisibleColumns: [...ALL_FLOW_COLUMNS],
      agentPanelTitle: DEFAULT_AGENT_PANEL_TITLE,
      agentVisibleColumns: [...ALL_AGENT_COLUMNS],
      resourcePanelTitle: DEFAULT_RESOURCE_PANEL_TITLE,
      deviceGridLayout: {},
      deviceGridEditMode: false,
      notifications: [],

      // Actions
      toggleSidebar: () =>
        set((state) => ({ sidebarCollapsed: !state.sidebarCollapsed })),

      setSidebarCollapsed: (collapsed) =>
        set({ sidebarCollapsed: collapsed }),

      setTheme: (theme) =>
        set({ theme }),

      setCustomThemeTokens: (tokens) =>
        set({ customThemeTokens: tokens }),

      resetCustomThemeTokens: () =>
        set({ customThemeTokens: {} }),

      setDashboardRefreshInterval: (seconds) =>
        set({ dashboardRefreshInterval: seconds }),

      setDashboardLayout: (layout) =>
        set({ dashboardLayout: layout }),

      setDashboardVisibleMetrics: (keys) =>
        set({ dashboardVisibleMetrics: keys }),

      setDashboardEditMode: (on) =>
        set({ dashboardEditMode: on }),

      resetDashboardLayout: () =>
        set({
          dashboardLayout: DEFAULT_DASHBOARD_LAYOUT,
          dashboardVisibleMetrics: DEFAULT_VISIBLE_METRICS,
          flowPanelTitle: DEFAULT_FLOW_PANEL_TITLE,
          flowVisibleColumns: [...ALL_FLOW_COLUMNS],
          agentPanelTitle: DEFAULT_AGENT_PANEL_TITLE,
          agentVisibleColumns: [...ALL_AGENT_COLUMNS],
          resourcePanelTitle: DEFAULT_RESOURCE_PANEL_TITLE,
        }),

      setFlowPanelTitle: (title) =>
        set({ flowPanelTitle: title }),

      setFlowVisibleColumns: (cols) =>
        set({ flowVisibleColumns: cols }),

      setAgentPanelTitle: (title) =>
        set({ agentPanelTitle: title }),

      setAgentVisibleColumns: (cols) =>
        set({ agentVisibleColumns: cols }),

      setResourcePanelTitle: (title) =>
        set({ resourcePanelTitle: title }),

      setDeviceGridLayout: (layout) =>
        set({ deviceGridLayout: layout }),

      setDeviceGridEditMode: (on) =>
        set({ deviceGridEditMode: on }),

      resetDeviceGridLayout: () =>
        set({ deviceGridLayout: {} }),

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
      version: 1,
      // v0 -> v1: light/dark -> day/night 마이그레이션
      migrate: (persistedState: unknown, version: number) => {
        const state = persistedState as Record<string, unknown>;
        if (version === 0) {
          if (state.theme === 'light') state.theme = 'day';
          if (state.theme === 'dark') state.theme = 'night';
          if (!state.customThemeTokens) state.customThemeTokens = {};
        }
        return state as unknown as UIState & UIActions;
      },
      partialize: (state) => ({
        sidebarCollapsed: state.sidebarCollapsed,
        theme: state.theme,
        customThemeTokens: state.customThemeTokens,
        dashboardRefreshInterval: state.dashboardRefreshInterval,
        dashboardLayout: state.dashboardLayout,
        dashboardVisibleMetrics: state.dashboardVisibleMetrics,
        flowPanelTitle: state.flowPanelTitle,
        flowVisibleColumns: state.flowVisibleColumns,
        agentPanelTitle: state.agentPanelTitle,
        agentVisibleColumns: state.agentVisibleColumns,
        resourcePanelTitle: state.resourcePanelTitle,
        deviceGridLayout: state.deviceGridLayout,
      }),
    },
  ),
);
