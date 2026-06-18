// GroupManagementPanel 은 명명된 노드 그룹의 일괄 관리 UI 이다(그룹 관리).
//
// - 그룹 이름변경(인라인 입력 + 저장)
// - 그룹 삭제(확인 → 멤버를 "전체" 버킷으로 이동)
// - 그룹 일괄 원격 업데이트(확인 → 서버 목표 버전으로 그룹 내 승인·온라인 노드 업데이트)
//
// 가상 "전체"(빈 라벨) 버킷은 관리 대상이 아니므로 목록에서 제외한다.

import { useState } from 'react';
import { Loader2, Pencil, RefreshCw, Trash2, Users } from 'lucide-react';

import { ConfirmDialog } from '@/components/remote/ConfirmDialog';
import {
  useDeleteGroup,
  useRenameGroup,
  useTargetVersion,
  useUpdateGroup,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import type { NodeGroup } from '@/types/remote';
import { useUIStore } from '@/stores/uiStore';

interface GroupManagementPanelProps {
  /** /remote/groups 결과(전체 포함). 빈 라벨("전체")은 패널에서 제외된다. */
  groups: NodeGroup[];
}

type PendingAction =
  | { kind: 'delete'; group: string }
  | { kind: 'update'; group: string };

export function GroupManagementPanel({
  groups,
}: GroupManagementPanelProps): React.JSX.Element {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const targetQuery = useTargetVersion();
  const rename = useRenameGroup();
  const remove = useDeleteGroup();
  const update = useUpdateGroup();

  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [restart, setRestart] = useState(false);
  const [pending, setPending] = useState<PendingAction | null>(null);

  const named = groups.filter((g) => g.group_name !== '');
  const target = targetQuery.data?.version ?? '';

  const draftFor = (name: string): string => drafts[name] ?? name;

  const handleRename = (oldName: string): void => {
    const newName = draftFor(oldName).trim();
    if (newName === '' || newName === oldName) return;
    rename.mutate(
      { oldName, newName },
      {
        onSuccess: () => {
          addNotification({ type: 'success', message: t('remote.group.toast.renamed') });
          setDrafts((d) => {
            const next = { ...d };
            delete next[oldName];
            return next;
          });
        },
        onError: () =>
          addNotification({ type: 'error', message: t('remote.group.toast.opFailed') }),
      },
    );
  };

  const handleConfirm = (): void => {
    if (!pending) return;
    if (pending.kind === 'delete') {
      remove.mutate(pending.group, {
        onSuccess: () =>
          addNotification({ type: 'success', message: t('remote.group.toast.deleted') }),
        onError: () =>
          addNotification({ type: 'error', message: t('remote.group.toast.opFailed') }),
      });
    } else {
      update.mutate(
        { name: pending.group, req: { version: target || undefined, restart } },
        {
          onSuccess: () =>
            addNotification({
              type: 'success',
              message: t('remote.group.toast.updateDispatched'),
            }),
          onError: () =>
            addNotification({ type: 'error', message: t('remote.group.toast.opFailed') }),
        },
      );
    }
    setPending(null);
  };

  return (
    <section
      aria-labelledby="group-management-heading"
      className="w-full max-w-2xl rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
      data-testid="group-management"
    >
      <div className="mb-3 flex items-center gap-2">
        <Users className="h-4 w-4 text-(--color-text-muted)" aria-hidden="true" />
        <h2
          id="group-management-heading"
          className="text-sm font-semibold text-(--color-text-primary)"
        >
          {t('remote.group.manage.title')}
        </h2>
      </div>

      {/* 업데이트 후 재시작 옵션(그룹 일괄 업데이트에 공통 적용). */}
      <label className="mb-3 flex items-center gap-1.5 text-xs text-(--color-text-muted)">
        <input
          type="checkbox"
          checked={restart}
          onChange={(e) => setRestart(e.target.checked)}
          data-testid="group-update-restart"
          className="h-3.5 w-3.5 rounded border-gray-300 text-blue-600"
        />
        {t('remote.group.manage.restart')}
      </label>

      {named.length === 0 ? (
        <p className="text-xs text-(--color-text-muted)" data-testid="group-management-empty">
          {t('remote.group.manage.noGroups')}
        </p>
      ) : (
        <ul className="space-y-2" data-testid="group-management-list">
          {named.map((g) => (
            <li
              key={g.group_name}
              className="flex flex-wrap items-center gap-2 rounded border border-(--color-border-default) px-2.5 py-2"
              data-testid={`group-row-${g.group_name}`}
            >
              <input
                type="text"
                value={draftFor(g.group_name)}
                onChange={(e) =>
                  setDrafts((d) => ({ ...d, [g.group_name]: e.target.value }))
                }
                data-testid={`group-rename-input-${g.group_name}`}
                className="w-36 rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
              />
              <span className="text-xs text-(--color-text-muted)">
                {g.node_count} {t('remote.group.manage.node')}
              </span>
              <div className="ml-auto flex items-center gap-1.5">
                <button
                  type="button"
                  onClick={() => handleRename(g.group_name)}
                  disabled={
                    rename.isPending || draftFor(g.group_name).trim() === g.group_name
                  }
                  data-testid={`group-rename-${g.group_name}`}
                  className="inline-flex items-center gap-1 rounded border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1 text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated) disabled:opacity-50"
                >
                  <Pencil className="h-3 w-3" aria-hidden="true" />
                  {t('remote.group.manage.rename')}
                </button>
                <button
                  type="button"
                  onClick={() => setPending({ kind: 'update', group: g.group_name })}
                  disabled={update.isPending}
                  data-testid={`group-update-${g.group_name}`}
                  className="inline-flex items-center gap-1 rounded bg-blue-600 px-2 py-1 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
                >
                  <RefreshCw className="h-3 w-3" aria-hidden="true" />
                  {t('remote.group.manage.update')}
                </button>
                <button
                  type="button"
                  onClick={() => setPending({ kind: 'delete', group: g.group_name })}
                  disabled={remove.isPending}
                  data-testid={`group-delete-${g.group_name}`}
                  className="inline-flex items-center gap-1 rounded border border-red-200 bg-red-50 px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-100 disabled:opacity-50 dark:border-red-800 dark:bg-red-950 dark:text-red-300"
                >
                  <Trash2 className="h-3 w-3" aria-hidden="true" />
                  {t('remote.group.manage.delete')}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {(rename.isPending || remove.isPending || update.isPending) && (
        <div className="mt-2 flex items-center gap-1.5 text-xs text-(--color-text-muted)">
          <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
        </div>
      )}

      <ConfirmDialog
        open={pending !== null}
        title={t(
          pending?.kind === 'delete'
            ? 'remote.group.manage.confirmDeleteTitle'
            : 'remote.group.manage.confirmUpdateTitle',
        )}
        description={t(
          pending?.kind === 'delete'
            ? 'remote.group.manage.confirmDeleteDesc'
            : 'remote.group.manage.confirmUpdateDesc',
        )}
        destructive={pending?.kind === 'delete'}
        pending={remove.isPending || update.isPending}
        onConfirm={handleConfirm}
        onCancel={() => setPending(null)}
      />
    </section>
  );
}
