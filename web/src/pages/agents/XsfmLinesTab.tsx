// xsfm 라인(line) 관리 탭 (SPEC-XSFM-LINE-001 Module 6, M6).
//
// 라인(line)은 1급 엔티티이며, 이 탭은 (1) 라인 목록(코드 + 이름 + 정렬, Order 오름차순),
// (2) 라인 추가(코드 + 이름 입력, 코드 포맷 힌트 `^[a-z0-9][a-z0-9_-]*$`), (3) 라인 삭제
// (참조 역사 존재 시 백엔드 `ErrLineInUse` 를 친화적 메시지로 노출, RD-5)를 제공한다.
// 빈 라인(참조 역사 0개)도 유효 엔티티로 표시한다(REQ-06-04). XsfmStationsTab 패턴 미러.

import { useState } from 'react';
import { Pencil, Plus, Trash2, TrainFront, X } from 'lucide-react';

import {
  isLineInUseError,
  isValidLineCode,
  useAddLine,
  useLines,
  useRemoveLine,
  type Line,
} from '@/hooks/useLine';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import { ConfirmDialog } from '@/components/property/ConfirmDialog';

const inputCls =
  'block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)';
const labelCls = 'mb-1 block text-xs font-medium text-(--color-text-secondary)';

interface LineFormState {
  code: string;
  name: string;
  order: string;
}
const EMPTY_LINE_FORM: LineFormState = { code: '', name: '', order: '' };

export default function XsfmLinesTab({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const { data: lines = [], isLoading } = useLines(agentId);
  const addLine = useAddLine(agentId);
  const removeLine = useRemoveLine(agentId);

  // 라인 폼: null=닫힘, editing=편집 대상 라인 code(수정 시 코드 잠금 → upsert 갱신).
  const [lineForm, setLineForm] = useState<LineFormState | null>(null);
  const [editingLine, setEditingLine] = useState<string | null>(null);
  // 삭제 확인 대상.
  const [removeTarget, setRemoveTarget] = useState<Line | null>(null);

  // 코드 포맷 위반 여부(편집 모드에서는 코드 잠금이므로 검사 생략).
  const codeInvalid = !!lineForm && !editingLine && lineForm.code.trim() !== '' && !isValidLineCode(lineForm.code.trim());

  function openAddLine() {
    setEditingLine(null);
    setLineForm({ ...EMPTY_LINE_FORM });
  }
  function openEditLine(l: Line) {
    setEditingLine(l.code);
    setLineForm({ code: l.code, name: l.name ?? '', order: l.order ? String(l.order) : '' });
  }
  function closeLineForm() {
    setLineForm(null);
    setEditingLine(null);
  }

  function submitLine() {
    if (!lineForm) return;
    const code = lineForm.code.trim();
    if (!code) {
      addNotification({ type: 'error', message: t('agents.detail.lines.codeRequired') });
      return;
    }
    // 신규 코드 포맷 검증(편집 모드는 코드 잠금이라 스킵). 백엔드가 최종 강제하지만 선제 차단.
    if (!editingLine && !isValidLineCode(code)) {
      addNotification({ type: 'error', message: t('agents.detail.lines.codeFormatError') });
      return;
    }
    const name = lineForm.name.trim();
    if (!name) {
      addNotification({ type: 'error', message: t('agents.detail.lines.nameRequired') });
      return;
    }
    let orderNum = 0;
    if (lineForm.order.trim()) {
      orderNum = parseInt(lineForm.order.trim(), 10);
      if (Number.isNaN(orderNum)) {
        addNotification({ type: 'error', message: t('agents.detail.lines.orderError') });
        return;
      }
    }
    addLine.mutate(
      { code, name, order: orderNum },
      {
        onSuccess: () => {
          closeLineForm();
          addNotification({ type: 'success', message: t('agents.detail.lines.addSuccess') });
        },
        onError: (err) => notifyError(err),
      },
    );
  }

  function confirmRemove() {
    if (!removeTarget) return;
    const target = removeTarget;
    removeLine.mutate(target.code, {
      onSuccess: () => {
        setRemoveTarget(null);
        addNotification({ type: 'success', message: t('agents.detail.lines.removeSuccess') });
      },
      onError: (err) => {
        setRemoveTarget(null);
        // 참조 역사 존재로 인한 거부(ErrLineInUse, RD-5)는 친화적 메시지로 안내.
        if (isLineInUseError(err)) {
          addNotification({ type: 'error', message: t('agents.detail.lines.inUseError') });
          return;
        }
        notifyError(err);
      },
    });
  }

  function notifyError(err: unknown) {
    addNotification({
      type: 'error',
      message: t('agents.detail.lines.opError').replace(
        '{message}',
        err instanceof Error ? err.message : t('agents.detail.lines.unknownError'),
      ),
    });
  }

  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-3 p-4 md:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="h-16 animate-pulse rounded-lg bg-(--color-bg-elevated)" />
        ))}
      </div>
    );
  }

  const submitting = addLine.isPending;

  return (
    <div className="space-y-3 p-4">
      {/* 헤더: 개수 + 라인 추가 버튼 */}
      <div className="flex items-center justify-between">
        <span className="text-xs text-(--color-text-muted)">
          {t('agents.detail.lines.linesCount').replace('{count}', String(lines.length))}
        </span>
        <button
          type="button"
          onClick={openAddLine}
          className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <Plus className="h-3.5 w-3.5" />
          {t('agents.detail.lines.addLine')}
        </button>
      </div>

      {/* 라인 목록 */}
      {lines.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-12 text-(--color-text-muted)">
          <TrainFront className="h-8 w-8 opacity-40" aria-hidden="true" />
          <p className="text-sm">{t('agents.detail.lines.noLines')}</p>
        </div>
      ) : (
        <div className="space-y-1.5">
          {/* 컬럼 헤더: 순서 / 이름 / 코드 / 액션 */}
          <div className="grid grid-cols-[3rem_1fr_9rem_4rem] items-center gap-2 px-3 pb-1 text-[10px] font-medium uppercase tracking-wide text-(--color-text-muted)">
            <span className="text-center">{t('agents.detail.lines.order')}</span>
            <span>{t('agents.detail.lines.displayName')}</span>
            <span>{t('agents.detail.lines.code')}</span>
            <span className="text-right">{t('agents.detail.lines.actions')}</span>
          </div>
          <ul className="space-y-1.5" data-testid="line-list">
            {lines.map((l) => (
              <li
                key={l.code}
                data-testid={`line-item-${l.code}`}
                className="grid grid-cols-[3rem_1fr_9rem_4rem] items-center gap-2 rounded-lg border border-(--color-border-default) px-3 py-2"
              >
                <span className="text-center text-xs tabular-nums text-(--color-text-secondary)">{l.order}</span>
                <span className="truncate text-sm font-medium text-(--color-text-primary)">{l.name || l.code}</span>
                <span className="truncate font-mono text-xs text-(--color-text-muted)">{l.code}</span>
                <div className="flex items-center justify-end gap-1">
                  <button
                    type="button"
                    onClick={() => openEditLine(l)}
                    aria-label={t('agents.detail.lines.edit')}
                    data-testid={`line-edit-${l.code}`}
                    className="rounded p-1 text-(--color-text-muted) hover:bg-(--color-bg-surface) hover:text-(--color-text-secondary)"
                  >
                    <Pencil className="h-3.5 w-3.5" />
                  </button>
                  <button
                    type="button"
                    onClick={() => setRemoveTarget(l)}
                    aria-label={t('agents.detail.lines.remove')}
                    data-testid={`line-remove-${l.code}`}
                    className="rounded p-1 text-(--color-text-muted) hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* 라인 추가/편집 폼 모달 */}
      {lineForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={closeLineForm}>
          <div
            className="mx-4 w-full max-w-md rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-(--color-border-default) px-4 py-3">
              <h3 className="text-sm font-semibold text-(--color-text-primary)">
                {editingLine ? t('agents.detail.lines.editLine') : t('agents.detail.lines.addLine')}
              </h3>
              <button
                type="button"
                onClick={closeLineForm}
                className="rounded-md p-1 text-(--color-text-muted) hover:bg-(--color-bg-surface)"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <div className="space-y-3 p-4">
              <div>
                <label className={labelCls}>
                  {t('agents.detail.lines.code')} <span className="text-red-500">*</span>
                </label>
                <input
                  type="text"
                  value={lineForm.code}
                  disabled={!!editingLine}
                  placeholder={t('agents.detail.lines.codePlaceholder')}
                  onChange={(e) => setLineForm({ ...lineForm, code: e.target.value })}
                  data-testid="line-code-input"
                  className={cn(inputCls, editingLine && 'cursor-not-allowed opacity-60')}
                />
                {/* 코드 포맷 힌트: 위반 시 붉은색 경고, 그 외 회색 안내. 편집 모드는 코드 잠금이라 미노출. */}
                {!editingLine && (
                  <p
                    data-testid="line-code-hint"
                    className={cn('mt-1 text-[11px]', codeInvalid ? 'text-red-500' : 'text-(--color-text-muted)')}
                  >
                    {t('agents.detail.lines.codeFormatHint')}
                  </p>
                )}
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelCls}>
                    {t('agents.detail.lines.displayName')} <span className="text-red-500">*</span>
                  </label>
                  <input
                    type="text"
                    value={lineForm.name}
                    placeholder={t('agents.detail.lines.displayNamePlaceholder')}
                    onChange={(e) => setLineForm({ ...lineForm, name: e.target.value })}
                    data-testid="line-name-input"
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className={labelCls}>{t('agents.detail.lines.order')}</label>
                  <input
                    type="number"
                    value={lineForm.order}
                    placeholder="0"
                    onChange={(e) => setLineForm({ ...lineForm, order: e.target.value })}
                    data-testid="line-order-input"
                    className={inputCls}
                  />
                </div>
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-4 py-3">
              <button
                type="button"
                onClick={closeLineForm}
                className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) hover:bg-(--color-bg-surface)"
              >
                {t('common.cancel')}
              </button>
              <button
                type="button"
                onClick={submitLine}
                disabled={submitting || !lineForm.code.trim() || !lineForm.name.trim() || codeInvalid}
                data-testid="line-form-submit"
                className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500"
              >
                {submitting ? t('agents.detail.lines.saving') : t('agents.detail.lines.save')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 라인 삭제 확인 */}
      <ConfirmDialog
        isOpen={removeTarget !== null}
        onClose={() => setRemoveTarget(null)}
        onConfirm={confirmRemove}
        title={t('agents.detail.lines.removeConfirmTitle')}
        message={t('agents.detail.lines.removeConfirmMessage').replace(
          '{name}',
          removeTarget?.name || removeTarget?.code || '',
        )}
        variant="danger"
        isSubmitting={removeLine.isPending}
      />
    </div>
  );
}
