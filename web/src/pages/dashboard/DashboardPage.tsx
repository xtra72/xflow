// 대시보드 메인 페이지.
// 플로우 현황, 시스템 메트릭, 에이전트 상태를 위젯 형태로 표시하고
// WebSocket을 통해 실시간 업데이트를 수신한다.

import { useCallback, useEffect, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Plus, RefreshCw } from 'lucide-react';

import { useFlows, useWebSocket } from '@/hooks';
import { getMetrics } from '@/services/api/monitorService';
import { WS_MESSAGE_TYPES } from '@/services/ws/wsHandlers';
import type { FlowInfo } from '@/types/flow';

import CreateFlowModal from './CreateFlowModal';
import AgentStatusWidget from './widgets/AgentStatusWidget';
import RecentFlowsWidget from './widgets/RecentFlowsWidget';
import ResourceWidget from './widgets/ResourceWidget';
import SystemStatusWidget from './widgets/SystemStatusWidget';

/** 대시보드 페이지 컴포넌트 */
export default function DashboardPage() {
  const queryClient = useQueryClient();
  const [modalOpen, setModalOpen] = useState(false);

  // REQ-WEB-001-06-02: 플로우 목록과 메트릭을 병렬로 로드
  const {
    data: flowsData,
    isLoading: flowsLoading,
    error: flowsError,
  } = useFlows();

  const {
    data: metrics,
    isLoading: metricsLoading,
    error: metricsError,
  } = useQuery({
    queryKey: ['monitor', 'metrics'],
    queryFn: getMetrics,
    refetchInterval: 15000, // 15초마다 갱신
  });

  const flows: FlowInfo[] = flowsData?.data ?? [];
  const isLoading = flowsLoading || metricsLoading;

  // REQ-WEB-001-06-05: WebSocket 실시간 업데이트
  const { state: wsState, client: wsClient } = useWebSocket();

  useEffect(() => {
    if (wsState !== 'connected' || !wsClient) return;

    // 플로우 상태 변경 시 플로우 목록 갱신
    const handleFlowStatus = () => {
      queryClient.invalidateQueries({ queryKey: ['flows'] });
    };

    // 플로우 메트릭 수신 시 메트릭 갱신
    const handleFlowMetrics = () => {
      queryClient.invalidateQueries({ queryKey: ['monitor', 'metrics'] });
    };

    // 에이전트 상태 변경 시 에이전트 목록 갱신
    const handleAgentStatus = () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    };

    // 시스템 이벤트 수신 시 전체 갱신
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

  /** 수동 새로고침 핸들러 */
  const handleRefresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['flows'] });
    queryClient.invalidateQueries({ queryKey: ['monitor', 'metrics'] });
    queryClient.invalidateQueries({ queryKey: ['agents'] });
  }, [queryClient]);

  // 에러 상태 표시
  const hasError = flowsError || metricsError;

  return (
    <div className="space-y-6">
      {/* 헤더 영역 */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold text-gray-900 dark:text-white">대시보드</h2>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
            플로우 실행 현황과 시스템 상태를 확인합니다.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {/* WebSocket 연결 표시 */}
          <span
            className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${
              wsState === 'connected'
                ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                : 'bg-gray-100 text-gray-500 dark:bg-gray-700 dark:text-gray-400'
            }`}
          >
            <span
              className={`h-1.5 w-1.5 rounded-full ${
                wsState === 'connected' ? 'bg-green-500' : 'bg-gray-400'
              }`}
            />
            {wsState === 'connected' ? '실시간' : '오프라인'}
          </span>

          {/* 새로고침 버튼 */}
          <button
            type="button"
            onClick={handleRefresh}
            disabled={isLoading}
            className="rounded-md border border-gray-300 p-2 text-gray-600 transition-colors hover:bg-gray-50 disabled:opacity-50 dark:border-gray-600 dark:text-gray-400 dark:hover:bg-gray-700"
            aria-label="새로고침"
          >
            <RefreshCw className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`} />
          </button>

          {/* REQ-WEB-001-06-04: 새 플로우 버튼 */}
          <button
            type="button"
            onClick={() => setModalOpen(true)}
            className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            <Plus className="h-4 w-4" />
            새 플로우
          </button>
        </div>
      </div>

      {/* 에러 배너 */}
      {hasError && (
        <div className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400">
          데이터를 불러오는 중 오류가 발생했습니다. 새로고침을 시도해주세요.
        </div>
      )}

      {/* 로딩 스켈레톤 */}
      {isLoading && !flowsData && !metrics ? (
        <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <div
              key={i}
              className="h-48 animate-pulse rounded-lg bg-gray-200 dark:bg-gray-700"
            />
          ))}
        </div>
      ) : (
        /* 위젯 그리드: 모바일 1열, 데스크톱 2열 */
        <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
          {/* REQ-WEB-001-06-01: 시스템 상태 요약 */}
          <SystemStatusWidget flows={flows} />

          {/* REQ-WEB-001-06-01: 에이전트 현황 */}
          <AgentStatusWidget />

          {/* REQ-WEB-001-06-01: 최근 활성 플로우 */}
          <RecentFlowsWidget flows={flows} />

          {/* REQ-WEB-001-06-01: 시스템 리소스 개요 */}
          <ResourceWidget metrics={metrics} />
        </div>
      )}

      {/* REQ-WEB-001-06-04: 플로우 생성 모달 */}
      <CreateFlowModal open={modalOpen} onClose={() => setModalOpen(false)} />
    </div>
  );
}
