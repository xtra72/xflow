// 패널 추가 다이얼로그.
// 패널 유형을 선택하여 활성 대시보드에 추가한다.

import { useCallback, useEffect } from 'react';
import { GitBranch, Bot, Activity, HardDrive, ScrollText, X } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

import { useUIStore, type PanelType } from '@/stores/uiStore';

/** 패널 유형 옵션 */
interface PanelOption {
  type: PanelType;
  icon: LucideIcon;
  label: string;
  description: string;
}

const PANEL_OPTIONS: PanelOption[] = [
  {
    type: 'flows',
    icon: GitBranch,
    label: '플로우 현황',
    description: '플로우 목록과 실행 상태를 표시합니다.',
  },
  {
    type: 'agents',
    icon: Bot,
    label: '에이전트 현황',
    description: '에이전트 목록과 상태를 표시합니다.',
  },
  {
    type: 'resource',
    icon: Activity,
    label: '프로세스 리소스',
    description: 'CPU, 메모리 등 시스템 메트릭을 표시합니다.',
  },
  {
    type: 'devices',
    icon: HardDrive,
    label: '디바이스',
    description: '등록된 디바이스 목록과 상태를 표시합니다.',
  },
  {
    type: 'logs',
    icon: ScrollText,
    label: '로그',
    description: '실시간 로그 스트림을 표시합니다.',
  },
];

interface AddPanelDialogProps {
  open: boolean;
  onClose: () => void;
}

/** 패널 추가 모달 */
export default function AddPanelDialog({ open, onClose }: AddPanelDialogProps) {
  const addPanel = useUIStore((s) => s.addPanel);

  // ESC 키로 닫기
  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [open, onClose]);

  // 배경 클릭 시 닫기
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget) onClose();
    },
    [onClose],
  );

  // 패널 선택 처리
  const handleSelect = (type: PanelType) => {
    addPanel(type);
    onClose();
  };

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="add-panel-dialog-title"
    >
      <div className="mx-4 w-full max-w-md rounded-lg bg-(--color-bg-surface) shadow-xl">
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-6 py-4">
          <h2
            id="add-panel-dialog-title"
            className="text-lg font-semibold text-(--color-text-primary)"
          >
            패널 추가
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label="닫기"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* 패널 유형 목록 */}
        <div className="space-y-2 px-6 py-4">
          {PANEL_OPTIONS.map((option) => {
            const Icon = option.icon;
            return (
              <button
                key={option.type}
                type="button"
                onClick={() => handleSelect(option.type)}
                className="flex w-full items-center gap-3 rounded-lg border border-(--color-border-default) p-3 text-left transition-colors hover:border-blue-300 hover:bg-blue-50 dark:hover:border-blue-600 dark:hover:bg-blue-900/20"
              >
                <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-(--color-bg-elevated)">
                  <Icon className="h-5 w-5 text-(--color-text-secondary)" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-(--color-text-primary)">
                    {option.label}
                  </p>
                  <p className="text-xs text-(--color-text-muted)">
                    {option.description}
                  </p>
                </div>
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
