// GroupControlPanel 은 그룹 관리 화면 우측의 "단일 그룹 제어" 패널이다.
//
// - 이름변경 / 삭제(→미분류) — 명명된 그룹에서만(미분류는 기본 그룹이라 불가)
// - 그룹 일괄 원격 업데이트(목표 버전 + 재시작 토글)
// - 그룹 일괄 명령(domain/action/args)
// - 멤버 노드 목록
//
// 미분류(groupName="")는 rename/delete 를 숨기고 update/command 만 노출한다.

import { useEffect, useState } from 'react';
import { Loader2, Pencil, RefreshCw, Trash2, Users } from 'lucide-react';

import { ConfirmDialog } from '@/components/remote/ConfirmDialog';
import { NodeOnlineIndicator } from '@/components/remote/NodeOnlineIndicator';
import { NodeStatusBadge } from '@/components/remote/NodeStatusBadge';
import {
  useCommandGroup,
  useDeleteGroup,
  useRenameGroup,
  useTargetVersion,
  useUpdateGroup,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import type { GroupDispatchResult, ManagedNode } from '@/types/remote';
import { useUIStore } from '@/stores/uiStore';

interface GroupControlPanelProps {
  /** 선택된 그룹명("" = 미분류). */
  groupName: string;
  /** 그룹 멤버 노드. */
  members: ManagedNode[];
  /** 노드 선택(멤버 클릭 시 상세로 이동). */
  onSelectNode: (id: string) => void;
  /** 그룹 삭제/이름변경 후 상위 선택 초기화용. */
  onMutated: (nextGroup: string | null) => void;
}

function summarize(results: GroupDispatchResult[]): string {
  const ok = results.filter((r) => r.ok).length;
  return `${ok}/${results.length}`;
}

type Pending = 'delete' | 'update' | 'command' | null;

export function GroupControlPanel({
  groupName,
  members,
  onSelectNode,
  onMutated,
}: GroupControlPanelProps): React.JSX.Element {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const isUngrouped = groupName === '';
  const targetQuery = useTargetVersion();
  const rename = useRenameGroup();
  const remove = useDeleteGroup();
  const update = useUpdateGroup();
  const command = useCommandGroup();

  const [draft, setDraft] = useState(groupName);
  const [restart, setRestart] = useState(false);
  const [cmdDomain, setCmdDomain] = useState('agent');
  const [cmdAction, setCmdAction] = useState('');
  const [cmdArgs, setCmdArgs] = useState('');
  const [pending, setPending] = useState<Pending>(null);

  useEffect(() => {
    setDraft(groupName);
  }, [groupName]);

  const target = targetQuery.data?.version ?? '';

  const handleRename = (): void => {
    const next = draft.trim();
    if (next === '' || next === groupName) return;
    rename.mutate(
      { oldName: groupName, newName: next },
      {
        onSuccess: () => {
          addNotification({ type: 'success', message: t('remote.group.toast.renamed') });
          onMutated(next);
        },
        onError: () =>
          addNotification({ type: 'error', message: t('remote.group.toast.opFailed') }),
      },
    );
  };

  const submitCommand = (): void => {
    if (cmdAction.trim() === '') return;
    if (cmdArgs.trim() !== '') {
      try {
        JSON.parse(cmdArgs);
      } catch {
        addNotification({ type: 'error', message: t('remote.group.manage.invalidArgs') });
        return;
      }
    }
    setPending('command');
  };

  const confirm = (): void => {
    if (pending === 'delete') {
      remove.mutate(groupName, {
        onSuccess: () => {
          addNotification({ type: 'success', message: t('remote.group.toast.deleted') });
          onMutated(null);
        },
        onError: () =>
          addNotification({ type: 'error', message: t('remote.group.toast.opFailed') }),
      });
    } else if (pending === 'update') {
      update.mutate(
        { name: groupName, req: { version: target || undefined, restart } },
        {
          onSuccess: (data) =>
            addNotification({
              type: 'success',
              message: `${t('remote.group.toast.updateDispatched')} (${summarize(data.results)})`,
            }),
          onError: () =>
            addNotification({ type: 'error', message: t('remote.group.toast.opFailed') }),
        },
      );
    } else if (pending === 'command') {
      const args =
        cmdArgs.trim() === '' ? undefined : (JSON.parse(cmdArgs) as Record<string, unknown>);
      command.mutate(
        { name: groupName, req: { domain: cmdDomain, action: cmdAction.trim(), args } },
        {
          onSuccess: (data) =>
            addNotification({
              type: 'success',
              message: `${t('remote.group.toast.commandDispatched')} (${summarize(data.results)})`,
            }),
          onError: () =>
            addNotification({ type: 'error', message: t('remote.group.toast.opFailed') }),
        },
      );
    }
    setPending(null);
  };

  const busy = rename.isPending || remove.isPending || update.isPending || command.isPending;

  return (
    <div className="space-y-4" data-testid="group-control">
      {/* 헤더 + 이름변경/삭제 */}
      <section className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4">
        <div className="mb-3 flex items-center gap-2">
          <Users className="h-4 w-4 text-(--color-text-muted)" aria-hidden="true" />
          <h2 className="text-sm font-semibold text-(--color-text-primary)">
            {isUngrouped ? t('remote.groupManagement.ungrouped') : groupName}
          </h2>
          <span className="text-xs text-(--color-text-muted)">
            {members.length} {t('remote.groupManagement.memberCount')}
          </span>
        </div>
        {isUngrouped ? (
          <p className="text-xs text-(--color-text-muted)">
            {t('remote.groupManagement.ungroupedDesc')}
          </p>
        ) : (
          <div className="flex flex-wrap items-end gap-2">
            <input
              type="text"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              data-testid="group-control-rename-input"
              className="w-44 rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
            />
            <button
              type="button"
              onClick={handleRename}
              disabled={rename.isPending || draft.trim() === groupName}
              data-testid="group-control-rename"
              className="inline-flex items-center gap-1 rounded border border-(--color-border-strong) bg-(--color-bg-surface) px-2.5 py-1 text-sm font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated) disabled:opacity-50"
            >
              <Pencil className="h-3 w-3" aria-hidden="true" />
              {t('remote.group.manage.rename')}
            </button>
            <button
              type="button"
              onClick={() => setPending('delete')}
              disabled={remove.isPending}
              data-testid="group-control-delete"
              className="inline-flex items-center gap-1 rounded border border-red-200 bg-red-50 px-2.5 py-1 text-sm font-medium text-red-700 hover:bg-red-100 disabled:opacity-50 dark:border-red-800 dark:bg-red-950 dark:text-red-300"
            >
              <Trash2 className="h-3 w-3" aria-hidden="true" />
              {t('remote.group.manage.delete')}
            </button>
          </div>
        )}
      </section>

      {/* 일괄 업데이트 */}
      <section className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4">
        <div className="mb-2 flex items-center gap-2">
          <RefreshCw className="h-4 w-4 text-(--color-text-muted)" aria-hidden="true" />
          <h3 className="text-sm font-semibold text-(--color-text-primary)">
            {t('remote.group.manage.update')}
          </h3>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <label className="flex items-center gap-1.5 text-xs text-(--color-text-muted)">
            <input
              type="checkbox"
              checked={restart}
              onChange={(e) => setRestart(e.target.checked)}
              data-testid="group-control-restart"
              className="h-3.5 w-3.5 rounded border-gray-300 text-blue-600"
            />
            {t('remote.group.manage.restart')}
          </label>
          <button
            type="button"
            onClick={() => setPending('update')}
            disabled={update.isPending}
            data-testid="group-control-update"
            className="inline-flex items-center gap-1 rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            {update.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {t('remote.group.manage.update')}
          </button>
        </div>
      </section>

      {/* 일괄 명령 */}
      <section className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4">
        <h3 className="mb-2 text-sm font-semibold text-(--color-text-primary)">
          {t('remote.group.manage.command')}
        </h3>
        <div className="flex flex-wrap items-end gap-2">
          <select
            value={cmdDomain}
            onChange={(e) => setCmdDomain(e.target.value)}
            data-testid="group-control-domain"
            className="rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary)"
          >
            <option value="agent">agent</option>
            <option value="flow">flow</option>
            <option value="device">device</option>
          </select>
          <input
            type="text"
            value={cmdAction}
            onChange={(e) => setCmdAction(e.target.value)}
            placeholder={t('remote.group.manage.cmdActionPlaceholder')}
            data-testid="group-control-action"
            className="w-40 rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
          />
          <button
            type="button"
            onClick={submitCommand}
            disabled={command.isPending || cmdAction.trim() === ''}
            data-testid="group-control-command-send"
            className="inline-flex items-center gap-1 rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            {command.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {t('remote.group.manage.cmdSend')}
          </button>
        </div>
        <textarea
          value={cmdArgs}
          onChange={(e) => setCmdArgs(e.target.value)}
          placeholder={t('remote.group.manage.cmdArgsPlaceholder')}
          data-testid="group-control-args"
          rows={2}
          className="mt-2 w-full rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 font-mono text-xs text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
        />
      </section>

      {/* 멤버 노드 목록 */}
      <section className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4">
        <h3 className="mb-2 text-sm font-semibold text-(--color-text-primary)">
          {t('remote.groupManagement.members')}
        </h3>
        {members.length === 0 ? (
          <p className="text-xs text-(--color-text-muted)">{t('remote.groupManagement.noMembers')}</p>
        ) : (
          <ul className="divide-y divide-(--color-border-default)" data-testid="group-control-members">
            {members.map((n) => (
              <li key={n.instance_id}>
                <button
                  type="button"
                  onClick={() => onSelectNode(n.instance_id)}
                  className="flex w-full items-center gap-2 px-1 py-1.5 text-left text-sm hover:bg-(--color-bg-elevated)"
                >
                  <NodeOnlineIndicator online={n.online} showLabel={false} />
                  <span className="flex-1 truncate text-(--color-text-primary)">
                    {n.hostname || n.instance_id}
                  </span>
                  <span className="shrink-0 font-mono text-xs text-(--color-text-muted)">
                    {n.version || '-'}
                  </span>
                  <NodeStatusBadge status={n.status} />
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      {busy && <div className="sr-only">working</div>}

      <ConfirmDialog
        open={pending !== null}
        title={t(
          pending === 'delete'
            ? 'remote.group.manage.confirmDeleteTitle'
            : pending === 'command'
              ? 'remote.group.manage.confirmCommandTitle'
              : 'remote.group.manage.confirmUpdateTitle',
        )}
        description={t(
          pending === 'delete'
            ? 'remote.group.manage.confirmDeleteDesc'
            : pending === 'command'
              ? 'remote.group.manage.confirmCommandDesc'
              : 'remote.group.manage.confirmUpdateDesc',
        )}
        destructive={pending === 'delete'}
        pending={busy}
        onConfirm={confirm}
        onCancel={() => setPending(null)}
      />
    </div>
  );
}
