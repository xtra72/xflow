// 에이전트 상태 요약 위젯.
// 전체/활성/비활성 에이전트 수를 표시한다.

import { Bot } from 'lucide-react';

import { useAgents } from '@/hooks';

/** 에이전트 현황을 카드 형태로 보여주는 위젯 */
export default function AgentStatusWidget() {
  const { data, isLoading } = useAgents();
  const agents = data?.data ?? [];

  // 연결 상태 또는 status 기반으로 활성/비활성 집계
  const activeCount = agents.filter(
    (a) => a.connected === true || a.status === 'running',
  ).length;
  const inactiveCount = agents.length - activeCount;

  return (
    <div className="rounded-lg bg-white p-6 shadow dark:bg-gray-800">
      <h3 className="mb-4 text-lg font-semibold text-gray-900 dark:text-white">
        에이전트 현황
      </h3>

      {isLoading ? (
        <div className="flex items-center justify-center py-8">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600" />
        </div>
      ) : (
        <div className="flex items-center gap-6">
          {/* 에이전트 아이콘 */}
          <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-full bg-blue-100 dark:bg-blue-900/30">
            <Bot className="h-7 w-7 text-blue-600 dark:text-blue-400" />
          </div>

          {/* 수치 표시 */}
          <div className="grid flex-1 grid-cols-3 gap-2 text-center">
            <div>
              <p className="text-2xl font-bold text-gray-900 dark:text-white">
                {agents.length}
              </p>
              <p className="text-xs text-gray-500 dark:text-gray-400">전체</p>
            </div>
            <div>
              <p className="text-2xl font-bold text-green-600 dark:text-green-400">
                {activeCount}
              </p>
              <p className="text-xs text-gray-500 dark:text-gray-400">활성</p>
            </div>
            <div>
              <p className="text-2xl font-bold text-gray-400 dark:text-gray-500">
                {inactiveCount}
              </p>
              <p className="text-xs text-gray-500 dark:text-gray-400">비활성</p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
