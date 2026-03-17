// 대시보드 메인 페이지.
// 플로우 현황, 시스템 메트릭, 에이전트 상태를 위젯 형태로 표시하고
// WebSocket을 통해 실시간 업데이트를 수신한다.
// react-grid-layout으로 패널 드래그/리사이즈를 지원한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import GridLayout from 'react-grid-layout';
import { Pencil, RefreshCw, RotateCcw, Check } from 'lucide-react';

import 'react-grid-layout/css/styles.css';
import 'react-resizable/css/styles.css';

import { useFlows, useWebSocket } from '@/hooks';
import { getMetrics } from '@/services/api/monitorService';
import {
  useUIStore,
  type DashboardLayoutItem,
} from '@/stores/uiStore';
import { WS_MESSAGE_TYPES } from '@/services/ws/wsHandlers';
import type { FlowInfo } from '@/types/flow';

import AgentPanel from './panels/AgentPanel';
import FlowPanel from './panels/FlowPanel';
import ResourceWidget from './widgets/ResourceWidget';

/** 갱신 주기 옵션 (초) */
const INTERVAL_OPTIONS = [5, 10, 15, 30, 60] as const;

/** 그리드 설정 */
const GRID_COLS = 12;
const GRID_ROW_HEIGHT = 80;
const GRID_MARGIN: [number, number] = [16, 16];

/** 대시보드 페이지 컴포넌트 */
export default function DashboardPage() {
  const queryClient = useQueryClient();

  // UI store
  const refreshInterval = useUIStore((s) => s.dashboardRefreshInterval);
  const setRefreshInterval = useUIStore((s) => s.setDashboardRefreshInterval);
  const layout = useUIStore((s) => s.dashboardLayout);
  const setLayout = useUIStore((s) => s.setDashboardLayout);
  const editMode = useUIStore((s) => s.dashboardEditMode);
  const setEditMode = useUIStore((s) => s.setDashboardEditMode);
  const resetLayout = useUIStore((s) => s.resetDashboardLayout);
  const refreshMs = refreshInterval * 1000;

  // 컨테이너 너비 측정
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(1200);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width);
      }
    });
    observer.observe(el);
    setContainerWidth(el.clientWidth);
    return () => observer.disconnect();
  }, []);

  // 데이터 로드
  const {
    data: flowsData,
    isLoading: flowsLoading,
    error: flowsError,
  } = useFlows();

  const {
    data: metrics,
    isLoading: metricsLoading,
  } = useQuery({
    queryKey: ['monitor', 'metrics'],
    queryFn: getMetrics,
    refetchInterval: refreshMs,
  });

  const flows: FlowInfo[] = flowsData?.data ?? [];
  const isLoading = flowsLoading || metricsLoading;

  // WebSocket 실시간 업데이트
  const { state: wsState, client: wsClient } = useWebSocket();

  useEffect(() => {
    if (wsState !== 'connected' || !wsClient) return;

    const handleFlowStatus = () => {
      queryClient.invalidateQueries({ queryKey: ['flows'] });
    };
    const handleFlowMetrics = () => {
      queryClient.invalidateQueries({ queryKey: ['monitor', 'metrics'] });
    };
    const handleAgentStatus = () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    };
    const handleSystemEvent = () => {
      queryClient.invalidateQueries({ queryKey: ['flows'] });
      queryClient.invalidateQueries({ queryKey: ['monitor', 'metrics'] });
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    };

    wsClient.on(WS_MESSAGE_TYPES.FLOW_STATUS, handleFlowStatus);
    wsClient.on(WS_MESSAGE_TYPES.FLOW_METRICS, handleFlowMetrics);
    wsClient.on(WS_MESSAGE_TYPES.AGENT_STATUS, handleAgentStatus);
    wsClient.on(WS_MESSAGE_TYPES.SYSTEM_EVENT, handleSystemEvent);

    return () => {
      wsClient.off(WS_MESSAGE_TYPES.FLOW_STATUS, handleFlowStatus);
      wsClient.off(WS_MESSAGE_TYPES.FLOW_METRICS, handleFlowMetrics);
      wsClient.off(WS_MESSAGE_TYPES.AGENT_STATUS, handleAgentStatus);
      wsClient.off(WS_MESSAGE_TYPES.SYSTEM_EVENT, handleSystemEvent);
    };
  }, [wsState, wsClient, queryClient]);

  const handleRefresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['flows'] });
    queryClient.invalidateQueries({ queryKey: ['monitor', 'metrics'] });
    queryClient.invalidateQueries({ queryKey: ['agents'] });
  }, [queryClient]);

  /** 레이아웃 변경 핸들러 */
  const handleLayoutChange = useCallback(
    (newLayout: DashboardLayoutItem[]) => {
      setLayout(newLayout);
    },
    [setLayout],
  );

  const hasError = flowsError;

  return (
    <div className="space-y-4" ref={containerRef}>
      {/* 헤더 영역 */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold text-(--color-text-primary)">대시보드</h2>
          <p className="mt-1 text-sm text-(--color-text-muted)">
            플로우 실행 현황과 시스템 상태를 확인합니다.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {/* WebSocket 연결 표시 */}
          <span
            className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${
              wsState === 'connected'
                ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                : 'bg-(--color-bg-elevated) text-(--color-text-muted)'
            }`}
          >
            <span
              className={`h-1.5 w-1.5 rounded-full ${
                wsState === 'connected' ? 'bg-green-500' : 'bg-gray-400'
              }`}
            />
            {wsState === 'connected' ? '실시간' : '오프라인'}
          </span>

          {/* 갱신 주기 선택 */}
          <select
            value={refreshInterval}
            onChange={(e) => setRefreshInterval(Number(e.target.value))}
            className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1.5 text-xs text-(--color-text-secondary)"
            aria-label="갱신 주기"
          >
            {INTERVAL_OPTIONS.map((sec) => (
              <option key={sec} value={sec}>
                {sec}초
              </option>
            ))}
          </select>

          {/* 새로고침 */}
          <button
            type="button"
            onClick={handleRefresh}
            disabled={isLoading}
            className="rounded-md border border-(--color-border-strong) p-2 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-50"
            aria-label="새로고침"
          >
            <RefreshCw className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`} />
          </button>

          {/* 편집 모드 토글 */}
          <button
            type="button"
            onClick={() => setEditMode(!editMode)}
            className={`rounded-md border p-2 transition-colors ${
              editMode
                ? 'border-blue-500 bg-blue-50 text-blue-600 dark:border-blue-400 dark:bg-blue-900/20 dark:text-blue-400'
                : 'border-(--color-border-strong) text-(--color-text-muted) hover:bg-(--color-bg-elevated)'
            }`}
            aria-label={editMode ? '편집 완료' : '레이아웃 편집'}
          >
            {editMode ? <Check className="h-4 w-4" /> : <Pencil className="h-4 w-4" />}
          </button>
        </div>
      </div>

      {/* 편집 모드 설정 바 */}
      {editMode && (
        <div className="flex items-center justify-end rounded-lg border border-blue-200 bg-blue-50 p-3 dark:border-blue-800 dark:bg-blue-900/20">
          <button
            type="button"
            onClick={resetLayout}
            className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-2.5 py-1 text-xs text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated)"
          >
            <RotateCcw className="h-3 w-3" />
            초기화
          </button>
        </div>
      )}

      {/* 에러 배너 */}
      {hasError && (
        <div className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400">
          데이터를 불러오는 중 오류가 발생했습니다. 새로고침을 시도해주세요.
        </div>
      )}

      {/* 로딩 스켈레톤 */}
      {isLoading && !flowsData && !metrics ? (
        <div className="space-y-4">
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div className="h-96 animate-pulse rounded-lg bg-(--color-bg-elevated)" />
            <div className="h-96 animate-pulse rounded-lg bg-(--color-bg-elevated)" />
          </div>
          <div className="h-48 animate-pulse rounded-lg bg-(--color-bg-elevated)" />
        </div>
      ) : (
        <GridLayout
          layout={layout}
          width={containerWidth}
          gridConfig={{
            cols: GRID_COLS,
            rowHeight: GRID_ROW_HEIGHT,
            margin: GRID_MARGIN,
            containerPadding: [0, 0],
          }}
          dragConfig={{
            enabled: editMode,
            handle: '.dashboard-drag-handle',
          }}
          resizeConfig={{
            enabled: editMode,
            handles: ['se'],
          }}
          onLayoutChange={(newLayout) => handleLayoutChange(newLayout as DashboardLayoutItem[])}
        >
          <div key="flows" className="flex flex-col overflow-hidden">
            {editMode && <DragHandle />}
            <FlowPanel flows={flows} />
          </div>
          <div key="agents" className="flex flex-col overflow-hidden">
            {editMode && <DragHandle />}
            <AgentPanel />
          </div>
          <div key="resource" className="flex flex-col overflow-hidden">
            {editMode && <DragHandle />}
            <ResourceWidget metrics={metrics} />
          </div>
        </GridLayout>
      )}
    </div>
  );
}

/** 편집 모드 드래그 핸들 */
function DragHandle() {
  return (
    <div className="dashboard-drag-handle flex h-6 cursor-grab items-center justify-center rounded-t-lg bg-(--color-bg-elevated)/80 active:cursor-grabbing">
      <div className="flex gap-1">
        <span className="h-1 w-1 rounded-full bg-(--color-text-muted)" />
        <span className="h-1 w-1 rounded-full bg-(--color-text-muted)" />
        <span className="h-1 w-1 rounded-full bg-(--color-text-muted)" />
      </div>
    </div>
  );
}
