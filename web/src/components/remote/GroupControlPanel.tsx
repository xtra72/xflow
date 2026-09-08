// GroupControlPanel 은 그룹 관리 화면 우측의 "단일 그룹 제어" 패널이다.
//
// - 이름변경 / 삭제(→미분류) — 명명된 그룹에서만(미분류는 기본 그룹이라 불가)
// - 그룹 일괄 원격 업데이트(목표 버전 + 재시작 토글)
// - 그룹 일괄 명령(domain/action/args)
// - 멤버 노드 목록
//
// 미분류(groupName="")는 rename/delete 를 숨기고 update/command 만 노출한다.

import { useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Loader2, Pencil, RefreshCw, Trash2, Users } from 'lucide-react';

import { ConfirmDialog } from '@/components/remote/ConfirmDialog';
import { NodeOnlineIndicator } from '@/components/remote/NodeOnlineIndicator';
import { NodeStatusBadge } from '@/components/remote/NodeStatusBadge';
import {
  useCommandGroup,
  useDeleteGroup,
  useReleases,
  useRenameGroup,
  useTargetVersion,
  useUpdateGroup,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { errorDetail } from '@/lib/remote/errorMessage';
import type {
  GroupDispatchResult,
  GroupUpdateRequest,
  GroupUpdateStrategy,
  ManagedNode,
  ReleaseRecord,
} from '@/types/remote';
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

/** 업데이트 전략 옵션(라디오 순서). */
const STRATEGIES: readonly GroupUpdateStrategy[] = ['latest', 'pin', 'per_arch'] as const;

/** latest 전략 채널 옵션. */
const CHANNELS: readonly string[] = ['stable', 'beta', 'nightly'] as const;

/** canonical "os/arch" 키(노드 보고값·자산 매칭과 일치). */
function archKey(os: string, arch: string): string {
  return `${os}/${arch}`;
}

/**
 * 전체 릴리스에서 스토어에 존재하는 distinct "os/arch" 집합을 추출한다(정렬됨).
 * per_arch 매트릭스의 행 목록이 된다.
 */
function distinctArches(releases: ReleaseRecord[]): string[] {
  const set = new Set<string>();
  for (const r of releases) {
    for (const a of r.assets) set.add(archKey(a.os, a.arch));
  }
  return Array.from(set).sort();
}

/**
 * 주어진 "os/arch" 자산을 가진 릴리스만 추려 버전 문자열 목록을 반환한다(게시 최신순).
 * per_arch 행의 버전 드롭다운 후보가 된다.
 */
function versionsForArch(releases: ReleaseRecord[], key: string): string[] {
  return releases
    .filter((r) => r.assets.some((a) => archKey(a.os, a.arch) === key))
    .slice()
    .sort((a, b) => b.published_at - a.published_at)
    .map((r) => r.version);
}

/**
 * pin 전략에서 선택된 버전(릴리스)이 자산을 갖지 않는 "os/arch" 목록을 반환한다.
 * 이 아키텍처 노드들은 서버가 건너뛰므로 사전 경고에 사용한다.
 */
function missingArchesForVersion(releases: ReleaseRecord[], version: string): string[] {
  if (version === '') return [];
  const all = distinctArches(releases);
  const rel = releases.find((r) => r.version === version);
  if (!rel) return [];
  const present = new Set(rel.assets.map((a) => archKey(a.os, a.arch)));
  return all.filter((k) => !present.has(k));
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
  const releasesQuery = useReleases();
  const rename = useRenameGroup();
  const remove = useDeleteGroup();
  const update = useUpdateGroup();
  const command = useCommandGroup();

  const [draft, setDraft] = useState(groupName);
  const [restart, setRestart] = useState(false);
  const [strategy, setStrategy] = useState<GroupUpdateStrategy>('pin');
  const [channel, setChannel] = useState<string>('stable');
  const [pinVersion, setPinVersion] = useState<string>('');
  const [archVersions, setArchVersions] = useState<Record<string, string>>({});
  const [cmdDomain, setCmdDomain] = useState('agent');
  const [cmdAction, setCmdAction] = useState('');
  const [cmdArgs, setCmdArgs] = useState('');
  const [pending, setPending] = useState<Pending>(null);

  useEffect(() => {
    setDraft(groupName);
  }, [groupName]);

  const target = targetQuery.data?.version ?? '';
  const releases = useMemo<ReleaseRecord[]>(
    () => releasesQuery.data ?? [],
    [releasesQuery.data],
  );

  /** 게시 최신순 정렬된 전체 버전(pin 드롭다운). */
  const allVersions = useMemo<string[]>(
    () =>
      releases
        .slice()
        .sort((a, b) => b.published_at - a.published_at)
        .map((r) => r.version),
    [releases],
  );
  /** per_arch 매트릭스 행(스토어에 존재하는 distinct os/arch). */
  const arches = useMemo<string[]>(() => distinctArches(releases), [releases]);
  /** pin 선택 버전이 자산을 갖지 않는 아키텍처(건너뜀 경고). */
  const pinMissing = useMemo<string[]>(
    () => missingArchesForVersion(releases, pinVersion),
    [releases, pinVersion],
  );

  // per_arch 행 기본값: 각 아키텍처의 최신 가용 버전으로 채운다(아직 미설정인 행만).
  useEffect(() => {
    if (arches.length === 0) return;
    setArchVersions((prev) => {
      const next: Record<string, string> = { ...prev };
      let changed = false;
      for (const key of arches) {
        if (next[key] === undefined) {
          const vs = versionsForArch(releases, key);
          if (vs.length > 0) {
            next[key] = vs[0]!;
            changed = true;
          }
        }
      }
      return changed ? next : prev;
    });
  }, [arches, releases]);

  /** 현재 전략에 맞는 그룹 업데이트 요청 본문을 구성한다. */
  const buildUpdateRequest = (): GroupUpdateRequest => {
    if (strategy === 'latest') {
      return { strategy: 'latest', channel, restart };
    }
    if (strategy === 'per_arch') {
      const version_by_arch: Record<string, string> = {};
      for (const key of arches) {
        const v = archVersions[key];
        if (v) version_by_arch[key] = v;
      }
      return { strategy: 'per_arch', version_by_arch, restart };
    }
    // pin: 명시 버전 우선, 없으면 서버 전역 목표 버전(레거시 동작 유지).
    return { strategy: 'pin', version: pinVersion || target || undefined, restart };
  };

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
        { name: groupName, req: buildUpdateRequest() },
        {
          onSuccess: (data) => {
            const ok = data.results.filter((r) => r.ok).length;
            const allOk = ok === data.results.length;
            // 일부/전부 실패 시 첫 실패 노드의 사유를 덧붙여 원인을 드러낸다.
            const firstErr = data.results.find((r) => !r.ok && r.error)?.error;
            const detail = !allOk && firstErr ? ` — ${firstErr}` : '';
            addNotification({
              type: allOk ? 'success' : 'error',
              message: `${t('remote.group.toast.updateDispatched')} (${summarize(data.results)})${detail}`,
            });
          },
          onError: (err: unknown) =>
            addNotification({
              type: 'error',
              message: t('remote.group.toast.opFailed') + errorDetail(err),
            }),
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

      {/* 일괄 업데이트 (아키텍처/OS 인지 전략) */}
      <section className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4">
        <div className="mb-3 flex items-center gap-2">
          <RefreshCw className="h-4 w-4 text-(--color-text-muted)" aria-hidden="true" />
          <h3 className="text-sm font-semibold text-(--color-text-primary)">
            {t('remote.group.manage.update')}
          </h3>
        </div>

        {/* 전략 선택 (세그먼트 라디오) */}
        <div
          role="radiogroup"
          aria-label={t('remote.group.update.strategyLabel')}
          className="mb-3 inline-flex rounded-md border border-(--color-border-strong) p-0.5"
        >
          {STRATEGIES.map((s) => {
            const selected = strategy === s;
            const label =
              s === 'latest'
                ? t('remote.group.update.strategyLatest')
                : s === 'pin'
                  ? t('remote.group.update.strategyPin')
                  : t('remote.group.update.strategyPerArch');
            return (
              <button
                key={s}
                type="button"
                role="radio"
                aria-checked={selected}
                onClick={() => setStrategy(s)}
                data-testid={`group-control-strategy-${s}`}
                className={`rounded px-2.5 py-1 text-xs font-medium transition-colors ${
                  selected
                    ? 'bg-blue-600 text-white dark:bg-blue-500'
                    : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
                }`}
              >
                {label}
              </button>
            );
          })}
        </div>

        {/* 전략별 입력 */}
        {strategy === 'latest' && (
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <label
              htmlFor="group-update-channel"
              className="text-xs text-(--color-text-muted)"
            >
              {t('remote.group.update.channel')}
            </label>
            <select
              id="group-update-channel"
              value={channel}
              onChange={(e) => setChannel(e.target.value)}
              data-testid="group-control-channel"
              className="rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary)"
            >
              {CHANNELS.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
            <p className="w-full text-xs text-(--color-text-muted)">
              {t('remote.group.update.latestHint')}
            </p>
          </div>
        )}

        {strategy === 'pin' && (
          <div className="mb-3 space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <label
                htmlFor="group-update-version"
                className="text-xs text-(--color-text-muted)"
              >
                {t('remote.group.update.version')}
              </label>
              <select
                id="group-update-version"
                value={pinVersion}
                onChange={(e) => setPinVersion(e.target.value)}
                data-testid="group-control-pin-version"
                className="rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary)"
              >
                <option value="">{t('remote.group.update.latestOption')}</option>
                {allVersions.map((v) => (
                  <option key={v} value={v}>
                    {v}
                  </option>
                ))}
              </select>
            </div>
            {pinMissing.length > 0 && (
              <p
                data-testid="group-control-missing-warn"
                className="flex items-start gap-1.5 text-xs text-amber-600 dark:text-amber-400"
              >
                <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                <span>
                  {t('remote.group.update.missingArchWarn')}: {pinMissing.join(', ')}
                </span>
              </p>
            )}
          </div>
        )}

        {strategy === 'per_arch' && (
          <div className="mb-3 space-y-1.5" data-testid="group-control-per-arch">
            {arches.length === 0 ? (
              <p className="text-xs text-(--color-text-muted)">
                {t('remote.group.update.noArches')}
              </p>
            ) : (
              arches.map((key) => {
                const versions = versionsForArch(releases, key);
                return (
                  <div key={key} className="flex items-center gap-2">
                    <span className="w-32 shrink-0 font-mono text-xs text-(--color-text-secondary)">
                      {key}
                    </span>
                    <select
                      value={archVersions[key] ?? ''}
                      onChange={(e) =>
                        setArchVersions((prev) => ({ ...prev, [key]: e.target.value }))
                      }
                      data-testid={`group-control-arch-${key}`}
                      className="rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary)"
                    >
                      {versions.map((v) => (
                        <option key={v} value={v}>
                          {v}
                        </option>
                      ))}
                    </select>
                  </div>
                );
              })
            )}
          </div>
        )}

        {/* 재시작 토글 + 실행 */}
        <div className="flex flex-wrap items-center gap-3">
          <label className="flex items-center gap-1.5 text-xs text-(--color-text-muted)">
            <input
              type="checkbox"
              checked={restart}
              onChange={(e) => setRestart(e.target.checked)}
              data-testid="group-control-restart"
              className="h-3.5 w-3.5 rounded border-(--color-border-strong) text-blue-600"
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
