// 플로우 설정 다이얼로그.
//
// 2026-05-31: 플로우 단위 설정을 한 곳에서 편집한다.
//   - 이름(name) / 설명(description): 서버 상태(useUpdateFlow). blur 시 저장.
//   - showPortStats: 노드의 각 포트 옆에 메시지 통계 수치 표시
//   - showPortNames: 노드의 각 포트 옆에 포트 이름 텍스트 표시
//
// 이름·설명은 서버에 영속(PUT /flows/{id})되고, 표시 토글은 (flowId) 별로
// uiStore 의 flowDisplaySettings 에 localStorage 영속화된다.
// ConfirmDialog 와 동일한 모달 레이아웃 / 키보드·배경 클릭 패턴.

import { useCallback, useEffect, useState } from 'react';
import { X } from 'lucide-react';

import { useFlow, useUpdateFlow } from '@/hooks/useFlow';
import { useTranslation } from '@/lib/i18n';
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
  const { t } = useTranslation();
  const settings = useUIStore(
    (state) => state.flowDisplaySettings[flowId] ?? DEFAULT_FLOW_DISPLAY_SETTINGS,
  );
  const setFlowDisplaySettings = useUIStore((state) => state.setFlowDisplaySettings);

  // 서버 상태(이름·설명) — useFlow 로 조회, useUpdateFlow 로 저장.
  const { data: flowData } = useFlow(flowId);
  const updateFlow = useUpdateFlow();
  const serverName = flowData?.name ?? '';
  const serverDescription = flowData?.description ?? '';

  // 입력 중인 값은 로컬 상태로 보관하고 blur 시 서버에 반영한다.
  const [nameDraft, setNameDraft] = useState(serverName);
  const [descDraft, setDescDraft] = useState(serverDescription);

  // 모달이 열릴 때(혹은 서버 값이 바뀌었을 때) 드래프트를 서버 값으로 동기화한다.
  useEffect(() => {
    if (isOpen) {
      setNameDraft(serverName);
      setDescDraft(serverDescription);
    }
  }, [isOpen, serverName, serverDescription]);

  // 이름 커밋 — trim 후 비어 있지 않고 변경된 경우에만 저장한다. 빈 값이면
  // 원래 이름으로 되돌린다.
  const commitName = () => {
    const trimmed = nameDraft.trim();
    if (!trimmed) {
      setNameDraft(serverName);
      return;
    }
    if (trimmed !== serverName) {
      updateFlow.mutate({ id: flowId, req: { name: trimmed } });
    }
  };

  // 설명 커밋 — 변경된 경우에만 저장한다. 빈 값(설명 제거)도 허용한다.
  const commitDescription = () => {
    if (descDraft !== serverDescription) {
      updateFlow.mutate({ id: flowId, req: { description: descDraft } });
    }
  };

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
            {t('editor.settings.title')}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)"
            aria-label={t('common.close')}
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 본문 */}
        <div className="flex flex-col gap-3 px-5 py-4 text-sm text-(--color-text-primary)">
          {/* 이름·설명 편집 (서버 상태) — blur 시 저장한다. */}
          <div className="flex flex-col gap-1.5">
            <label
              htmlFor="flow-settings-name"
              className="text-xs font-medium text-(--color-text-secondary)"
            >
              {t('editor.settings.nameLabel')}
            </label>
            <input
              id="flow-settings-name"
              type="text"
              value={nameDraft}
              onChange={(e) => setNameDraft(e.target.value)}
              onBlur={commitName}
              // 입력 중 에디터 전역 단축키(Delete/Ctrl+Z 등)가 발동하지 않게 한다.
              onKeyDown={(e) => {
                e.stopPropagation();
                if (e.key === 'Enter') {
                  e.preventDefault();
                  e.currentTarget.blur();
                }
              }}
              placeholder={t('editor.settings.namePlaceholder')}
              className="rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-400 focus:ring-1 focus:ring-blue-400"
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <label
              htmlFor="flow-settings-description"
              className="text-xs font-medium text-(--color-text-secondary)"
            >
              {t('editor.settings.descLabel')}
            </label>
            <textarea
              id="flow-settings-description"
              value={descDraft}
              onChange={(e) => setDescDraft(e.target.value)}
              onBlur={commitDescription}
              onKeyDown={(e) => e.stopPropagation()}
              rows={3}
              placeholder={t('editor.settings.descPlaceholder')}
              className="resize-y rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-400 focus:ring-1 focus:ring-blue-400"
            />
          </div>

          <div className="my-1 h-px bg-(--color-border-default)" />

          <p className="text-xs text-(--color-text-secondary)">
            {t('editor.settings.scopeNote')}
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
              <div className="font-medium">{t('editor.settings.showPortStats')}</div>
              <div className="mt-0.5 text-xs text-(--color-text-secondary)">
                {t('editor.settings.showPortStatsDesc')}
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
              <div className="font-medium">{t('editor.settings.showPortNames')}</div>
              <div className="mt-0.5 text-xs text-(--color-text-secondary)">
                {t('editor.settings.showPortNamesDesc')}
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
            {t('common.close')}
          </button>
        </div>
      </div>
    </div>
  );
}
