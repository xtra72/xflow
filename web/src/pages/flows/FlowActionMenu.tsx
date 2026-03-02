// 플로우 액션 드롭다운 메뉴.
// 각 플로우 행에서 lifecycle 동작(시작, 중지, 재시작, 배포, 삭제)을 수행한다.

import { useEffect, useRef, useState } from 'react';
import {
  MoreVertical,
  Play,
  Square,
  RotateCcw,
  Upload,
  Trash2,
} from 'lucide-react';

import {
  useStartFlow,
  useStopFlow,
  useRestartFlow,
  useDeployFlow,
  useDeleteFlow,
} from '@/hooks';
import type { FlowInfo } from '@/types/flow';

interface FlowActionMenuProps {
  flow: FlowInfo;
  onAction?: () => void;
}

/**
 * 플로우 액션 드롭다운 메뉴.
 * 플로우 상태에 따라 활성화/비활성화되는 lifecycle 동작 목록을 제공한다.
 */
export default function FlowActionMenu({ flow, onAction }: FlowActionMenuProps) {
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  const startFlow = useStartFlow();
  const stopFlow = useStopFlow();
  const restartFlow = useRestartFlow();
  const deployFlow = useDeployFlow();
  const deleteFlow = useDeleteFlow();

  // 외부 클릭 시 드롭다운 닫기
  useEffect(() => {
    if (!open) return;

    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [open]);

  /** 상태에 따른 액션 비활성화 여부 */
  const isRunning = flow.status === 'Running';

  /** 액션 실행 후 드롭다운 닫기 */
  const execute = async (action: () => Promise<unknown>) => {
    setOpen(false);
    try {
      await action();
      onAction?.();
    } catch {
      // 에러는 mutation 상태에서 처리
    }
  };

  const handleStart = () => execute(() => startFlow.mutateAsync(flow.id));
  const handleStop = () => execute(() => stopFlow.mutateAsync(flow.id));
  const handleRestart = () => execute(() => restartFlow.mutateAsync(flow.id));
  const handleDeploy = () => execute(() => deployFlow.mutateAsync(flow.id));

  const handleDelete = () => {
    // 삭제 전 확인 대화상자
    const confirmed = window.confirm(
      `"${flow.name}" 플로우를 삭제하시겠습니까? 이 작업은 되돌릴 수 없습니다.`,
    );
    if (!confirmed) return;
    execute(() => deleteFlow.mutateAsync(flow.id));
  };

  return (
    <div ref={menuRef} className="relative">
      {/* 트리거 버튼 */}
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          setOpen((prev) => !prev);
        }}
        className="rounded-md p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-700 dark:hover:text-gray-300"
        aria-label="플로우 액션"
      >
        <MoreVertical className="h-4 w-4" />
      </button>

      {/* 드롭다운 메뉴 */}
      {open && (
        <div className="absolute right-0 z-10 mt-1 w-40 rounded-md border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-600 dark:bg-gray-800">
          {/* 시작 */}
          <button
            type="button"
            disabled={isRunning}
            onClick={(e) => {
              e.stopPropagation();
              handleStart();
            }}
            className="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-40 dark:text-gray-300 dark:hover:bg-gray-700"
          >
            <Play className="h-3.5 w-3.5" />
            시작
          </button>

          {/* 중지 */}
          <button
            type="button"
            disabled={!isRunning}
            onClick={(e) => {
              e.stopPropagation();
              handleStop();
            }}
            className="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-40 dark:text-gray-300 dark:hover:bg-gray-700"
          >
            <Square className="h-3.5 w-3.5" />
            중지
          </button>

          {/* 재시작 */}
          <button
            type="button"
            disabled={!isRunning}
            onClick={(e) => {
              e.stopPropagation();
              handleRestart();
            }}
            className="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-40 dark:text-gray-300 dark:hover:bg-gray-700"
          >
            <RotateCcw className="h-3.5 w-3.5" />
            재시작
          </button>

          {/* 배포 */}
          <button
            type="button"
            disabled={isRunning}
            onClick={(e) => {
              e.stopPropagation();
              handleDeploy();
            }}
            className="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-40 dark:text-gray-300 dark:hover:bg-gray-700"
          >
            <Upload className="h-3.5 w-3.5" />
            배포
          </button>

          {/* 구분선 */}
          <hr className="my-1 border-gray-200 dark:border-gray-600" />

          {/* 삭제 */}
          <button
            type="button"
            disabled={isRunning}
            onClick={(e) => {
              e.stopPropagation();
              handleDelete();
            }}
            className="flex w-full items-center gap-2 px-3 py-2 text-sm text-red-600 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-40 dark:text-red-400 dark:hover:bg-red-900/20"
          >
            <Trash2 className="h-3.5 w-3.5" />
            삭제
          </button>
        </div>
      )}
    </div>
  );
}
