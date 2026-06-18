// GroupManagementPanel 은 명명된 노드 그룹의 일괄 관리 UI 이다(그룹 관리).
//
// - 그룹 이름변경(인라인 입력 + 저장)
// - 그룹 삭제(확인 → 멤버를 "전체" 버킷으로 이동)
// - 그룹 일괄 원격 업데이트(확인 → 서버 목표 버전으로 그룹 내 승인·온라인 노드 업데이트)
//
// 가상 "전체"(빈 라벨) 버킷은 관리 대상이 아니므로 목록에서 제외한다.

import { useState } from 'react';
import { Loader2, Pencil, Plus, RefreshCw, Trash2, Users } from 'lucide-react';

import { ConfirmDialog } from '@/components/remote/ConfirmDialog';
import {
  useCommandGroup,
  useDeleteGroup,
  useRenameGroup,
  useSetNodeGroup,
  useTargetVersion,
  useUpdateGroup,
} from '@/hooks/useRemote';
import type { GroupDispatchResult, ManagedNode } from '@/types/remote';
import { useTranslation } from '@/lib/i18n';
import type { NodeGroup } from '@/types/remote';
import { useUIStore } from '@/stores/uiStore';

interface GroupManagementPanelProps {
  /** /remote/groups 결과(전체 포함). 빈 라벨("전체")은 패널에서 제외된다. */
  groups: NodeGroup[];
  /** 전체 노드 목록(그룹 생성 시 멤버 다중선택용). */
  nodes: ManagedNode[];
}

type PendingAction =
  | { kind: 'delete'; group: string }
  | { kind: 'update'; group: string }
  | { kind: 'command'; group: string };

/** 일괄 명령 결과 요약 문자열(ok/total). */
function summarize(results: GroupDispatchResult[]): string {
  const ok = results.filter((r) => r.ok).length;
  return `${ok}/${results.length}`;
}

export function GroupManagementPanel({
  groups,
  nodes,
}: GroupManagementPanelProps): React.JSX.Element {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const targetQuery = useTargetVersion();
  const rename = useRenameGroup();
  const remove = useDeleteGroup();
  const update = useUpdateGroup();
  const command = useCommandGroup();
  const setNodeGroup = useSetNodeGroup();

  // 그룹 생성 폼 상태(새 이름 + 멤버 다중선택).
  const [createName, setCreateName] = useState('');
  const [selectedNodes, setSelectedNodes] = useState<Set<string>>(new Set());
  const [creating, setCreating] = useState(false);

  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [restart, setRestart] = useState(false);
  const [pending, setPending] = useState<PendingAction | null>(null);

  // 일괄 명령 폼 상태.
  const [cmdGroup, setCmdGroup] = useState('');
  const [cmdDomain, setCmdDomain] = useState('agent');
  const [cmdAction, setCmdAction] = useState('');
  const [cmdArgs, setCmdArgs] = useState('');

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

  // 그룹 생성: 선택한 노드들을 새 그룹명으로 일괄 배정한다(첫 배정이 곧 생성 — 파생 모델).
  const handleCreate = async (): Promise<void> => {
    const name = createName.trim();
    if (name === '' || selectedNodes.size === 0) return;
    setCreating(true);
    try {
      await Promise.all(
        Array.from(selectedNodes).map((instanceID) =>
          setNodeGroup.mutateAsync({ instanceID, groupName: name }),
        ),
      );
      addNotification({ type: 'success', message: t('remote.group.toast.created') });
      setCreateName('');
      setSelectedNodes(new Set());
    } catch {
      addNotification({ type: 'error', message: t('remote.group.toast.opFailed') });
    } finally {
      setCreating(false);
    }
  };

  const toggleNode = (id: string): void => {
    setSelectedNodes((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  // 일괄 명령 전송 시도: 그룹/액션 검증 + args JSON 파싱 후 확인 다이얼로그를 연다.
  const handleCommandSubmit = (): void => {
    if (cmdGroup === '' || cmdAction.trim() === '') return;
    if (cmdArgs.trim() !== '') {
      try {
        JSON.parse(cmdArgs);
      } catch {
        addNotification({ type: 'error', message: t('remote.group.manage.invalidArgs') });
        return;
      }
    }
    setPending({ kind: 'command', group: cmdGroup });
  };

  const handleConfirm = (): void => {
    if (!pending) return;
    if (pending.kind === 'command') {
      const args =
        cmdArgs.trim() === ''
          ? undefined
          : (JSON.parse(cmdArgs) as Record<string, unknown>);
      command.mutate(
        { name: pending.group, req: { domain: cmdDomain, action: cmdAction.trim(), args } },
        {
          onSuccess: (data) =>
            addNotification({
              type: 'success',
              message: `${t('remote.group.toast.commandDispatched')} (${t('remote.group.manage.cmdResult')}: ${summarize(data.results)})`,
            }),
          onError: () =>
            addNotification({ type: 'error', message: t('remote.group.toast.opFailed') }),
        },
      );
      setPending(null);
      return;
    }
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

      {/* 그룹 생성: 새 이름 + 노드 다중선택 → 일괄 배정(첫 배정이 곧 생성). */}
      <div
        className="mb-4 rounded border border-(--color-border-default) p-2.5"
        data-testid="group-create-form"
      >
        <div className="mb-2 flex items-center gap-1.5">
          <Plus className="h-3.5 w-3.5 text-(--color-text-muted)" aria-hidden="true" />
          <h3 className="text-xs font-semibold uppercase tracking-wider text-(--color-text-muted)">
            {t('remote.group.manage.create')}
          </h3>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <input
            type="text"
            value={createName}
            onChange={(e) => setCreateName(e.target.value)}
            placeholder={t('remote.group.manage.createPlaceholder')}
            data-testid="group-create-name"
            className="w-44 rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
          />
          <button
            type="button"
            onClick={() => void handleCreate()}
            disabled={creating || createName.trim() === '' || selectedNodes.size === 0}
            data-testid="group-create-submit"
            className="inline-flex items-center gap-1 rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            {creating && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {t('remote.group.manage.createButton')} ({selectedNodes.size})
          </button>
        </div>
        {/* 멤버 다중선택 — 새 그룹으로 이동할 노드. */}
        {nodes.length === 0 ? (
          <p className="mt-2 text-xs text-(--color-text-muted)" data-testid="group-create-nonodes">
            {t('remote.group.manage.noNodes')}
          </p>
        ) : (
          <ul className="mt-2 max-h-40 overflow-y-auto rounded border border-(--color-border-default)">
            {nodes.map((n) => (
              <li key={n.instance_id}>
                <label className="flex cursor-pointer items-center gap-2 px-2 py-1 text-xs hover:bg-(--color-bg-elevated)">
                  <input
                    type="checkbox"
                    checked={selectedNodes.has(n.instance_id)}
                    onChange={() => toggleNode(n.instance_id)}
                    data-testid={`group-create-node-${n.instance_id}`}
                    className="h-3.5 w-3.5 rounded border-gray-300 text-blue-600"
                  />
                  <span className="flex-1 truncate text-(--color-text-primary)">
                    {n.hostname || n.instance_id}
                  </span>
                  <span className="shrink-0 text-(--color-text-muted)">
                    {n.group_name || t('remote.group.manage.ungrouped')}
                  </span>
                </label>
              </li>
            ))}
          </ul>
        )}
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

      {/* 일괄 명령 폼: 대상 그룹 + 도메인/액션/인자(JSON) → 그룹 내 전 노드에 동일 전송. */}
      {named.length > 0 && (
        <div
          className="mt-4 border-t border-(--color-border-default) pt-3"
          data-testid="group-command-form"
        >
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-(--color-text-muted)">
            {t('remote.group.manage.command')}
          </h3>
          <div className="flex flex-wrap items-end gap-2">
            <label className="flex flex-col gap-1 text-xs text-(--color-text-muted)">
              {t('remote.group.manage.cmdGroup')}
              <select
                value={cmdGroup}
                onChange={(e) => setCmdGroup(e.target.value)}
                data-testid="group-command-group"
                className="rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary)"
              >
                <option value="">—</option>
                {named.map((g) => (
                  <option key={g.group_name} value={g.group_name}>
                    {g.group_name}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1 text-xs text-(--color-text-muted)">
              {t('remote.group.manage.cmdDomain')}
              <select
                value={cmdDomain}
                onChange={(e) => setCmdDomain(e.target.value)}
                data-testid="group-command-domain"
                className="rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary)"
              >
                <option value="agent">agent</option>
                <option value="flow">flow</option>
                <option value="device">device</option>
              </select>
            </label>
            <label className="flex flex-col gap-1 text-xs text-(--color-text-muted)">
              {t('remote.group.manage.cmdAction')}
              <input
                type="text"
                value={cmdAction}
                onChange={(e) => setCmdAction(e.target.value)}
                placeholder={t('remote.group.manage.cmdActionPlaceholder')}
                data-testid="group-command-action"
                className="w-40 rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
              />
            </label>
            <button
              type="button"
              onClick={handleCommandSubmit}
              disabled={command.isPending || cmdGroup === '' || cmdAction.trim() === ''}
              data-testid="group-command-send"
              className="inline-flex items-center gap-1 rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              {command.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('remote.group.manage.cmdSend')}
            </button>
          </div>
          <label className="mt-2 flex flex-col gap-1 text-xs text-(--color-text-muted)">
            {t('remote.group.manage.cmdArgs')}
            <textarea
              value={cmdArgs}
              onChange={(e) => setCmdArgs(e.target.value)}
              placeholder={t('remote.group.manage.cmdArgsPlaceholder')}
              data-testid="group-command-args"
              rows={2}
              className="w-full rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 font-mono text-xs text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
            />
          </label>
        </div>
      )}

      <ConfirmDialog
        open={pending !== null}
        title={t(
          pending?.kind === 'delete'
            ? 'remote.group.manage.confirmDeleteTitle'
            : pending?.kind === 'command'
              ? 'remote.group.manage.confirmCommandTitle'
              : 'remote.group.manage.confirmUpdateTitle',
        )}
        description={t(
          pending?.kind === 'delete'
            ? 'remote.group.manage.confirmDeleteDesc'
            : pending?.kind === 'command'
              ? 'remote.group.manage.confirmCommandDesc'
              : 'remote.group.manage.confirmUpdateDesc',
        )}
        destructive={pending?.kind === 'delete'}
        pending={remove.isPending || update.isPending || command.isPending}
        onConfirm={handleConfirm}
        onCancel={() => setPending(null)}
      />
    </section>
  );
}
