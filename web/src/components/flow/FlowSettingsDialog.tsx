// 플로우 단위 표시 설정 다이얼로그.
//
// 2026-05-31: 플로우 에디터의 시각화 옵션을 토글한다.
//   - showPortStats: 노드의 각 포트 옆에 메시지 통계 수치 표시
//   - showPortNames: 노드의 각 포트 옆에 포트 이름 텍스트 표시
//
// 설정은 (flowId) 별로 분리되며 uiStore 의 flowDisplaySettings 에 localStorage
// 영속화된다. ConfirmDialog 와 동일한 모달 레이아웃 / 키보드·배경 클릭 패턴.

import { useCallback, useEffect } from 'react';
import { X } from 'lucide-react';

import { DEFAULT_FLOW_DISPLAY_SETTINGS, useUIStore } from '@/stores/uiStore';

interface FlowSettingsDialogProps {
  /** 모달 표시 여부 */
  isOpen: boolean;
  /** 모달 닫기 핸들러 (취소·Esc·배경 클릭). */
  onClose: () => void;
  /** 대상 플로우 ID — 설정 키. */
  flowId: string;
}

export function FlowSettingsDialog({ isOpen, onClose, flowId }: FlowSettingsDialogProps) {
  const settings = useUIStore(
    (state) => state.flowDisplaySettings[flowId] ?? DEFAULT_FLOW_DISPLAY_SETTINGS,
  );
  const setFlowDisplaySettings = useUIStore((state) => state.setFlowDisplaySettings);

  // Esc 키로 닫기.
  useEffect(() => {
    if (!isOpen) return;
    const handleKeyDown = (e: globalThis.KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  // 배경 클릭 시 닫기.
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget) onClose();
    },
    [onClose],
  );

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="flow-settings-dialog-title"
    >
      <div
        className="mx-4 flex w-full max-w-[480px] flex-col rounded-lg bg-(--color-bg-surface) shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-3">
          <h2
            id="flow-settings-dialog-title"
            className="text-base font-semibold text-(--color-text-primary)"
          >
            플로우 표시 설정
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label="닫기"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 본문 */}
        <div className="flex flex-col gap-3 px-5 py-4 text-sm text-(--color-text-primary)">
          <p className="text-xs text-(--color-text-secondary)">
            본 플로우에 대해서만 적용됩니다. 다른 플로우의 표시는 영향받지 않습니다.
          </p>

          {/* 포트별 통계 표시 토글 */}
          <label className="flex items-start gap-3 rounded-md border border-(--color-border-default) px-3 py-2.5 hover:bg-(--color-bg-elevated)">
            <input
              type="checkbox"
              checked={settings.showPortStats}
              onChange={(e) =>
                setFlowDisplaySettings(flowId, { showPortStats: e.target.checked })
              }
              className="mt-0.5 h-4 w-4"
            />
            <div className="flex-1">
              <div className="font-medium">포트별 통계 표시</div>
              <div className="mt-0.5 text-xs text-(--color-text-secondary)">
                각 포트 옆에 메시지 통계 수치를 표시합니다. 비활성 시 노드 하단의 누계만 표시.
              </div>
            </div>
          </label>

          {/* 포트 이름 표시 토글 */}
          <label className="flex items-start gap-3 rounded-md border border-(--color-border-default) px-3 py-2.5 hover:bg-(--color-bg-elevated)">
            <input
              type="checkbox"
              checked={settings.showPortNames}
              onChange={(e) =>
                setFlowDisplaySettings(flowId, { showPortNames: e.target.checked })
              }
              className="mt-0.5 h-4 w-4"
            />
            <div className="flex-1">
              <div className="font-medium">포트 이름 표시</div>
              <div className="mt-0.5 text-xs text-(--color-text-secondary)">
                각 포트 옆에 포트 이름 텍스트를 표시합니다 (in / out / error 등).
              </div>
            </div>
          </label>
        </div>

        {/* 푸터 */}
        <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
          >
            닫기
          </button>
        </div>
      </div>
    </div>
  );
}
