// 대시보드 권한 부여 패널 (SPEC-DASHBOARD-004 M7 7.2, spec.md §2.9 E3 / §4.5).
//
// ACL 은 **전량 치환**이다 — 서버에 항목 단위 추가/삭제 경로가 없다(spec.md §4.5).
// 그래서 이 패널은 "현재 목록 전체" 를 편집하고 저장 시 그 전체를 PUT 한다.
// 화면의 목록이 곧 저장될 목록이며, 여기서 지운 항목은 저장과 함께 사라진다.
//
// 거부 사유는 사유별로 구분해 보여준다(acceptance.md AC-17):
//   - 저장 전: dashboardAclValidation 이 사유별 i18n 키를 돌려준다.
//   - 저장 후: 서버 400 은 서버가 보낸 사유를 그대로 덧붙인다. 서버 메시지를
//     파싱해 분류하지 않는다 — 메시지 문면은 계약이 아니다(adminErrors.ts 참조).
//
// 이 패널은 `can_grant` 인 대시보드에서만 열린다(spec.md §2.2 grant 판정).
// 호출부가 버튼을 비활성화하고, 서버도 grant 없는 요청을 403 으로 막는다.
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.9, §2.12 O1, §4.5)

import { useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Loader2, Plus, Trash2 } from 'lucide-react';

import { usePermission } from '@/hooks/usePermission';
import { useAdminRoles, useAdminUsers } from '@/hooks/useUserAdmin';
import { useTranslation } from '@/lib/i18n';
import {
  DashboardBadRequestError,
  DashboardForbiddenError,
  getAcl,
  putAcl,
} from '@/services/api/dashboardService';
import { useUIStore } from '@/stores/uiStore';
import type { Dashboard, DashboardAclEntry, DashboardAclLevel } from '@/types/dashboard';
import {
  SUBJECT_PREFIX_ROLE,
  SUBJECT_PREFIX_USER,
  validateAclEntries,
  type AclValidationIssue,
} from './dashboardAclValidation';

const INPUT_CLASS =
  'w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1.5 ' +
  'text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 ' +
  'focus:ring-blue-500 disabled:cursor-not-allowed disabled:opacity-50';

const TH_CLASS =
  'px-2 py-2 text-left text-xs font-medium uppercase tracking-wide text-(--color-text-muted)';

/** 편집 중인 ACL 1행. 접두사와 이름을 분리해 둔다 — 사용자가 접두사를 손으로 쓰지 않는다. */
interface AclDraftRow {
  kind: 'user' | 'role';
  name: string;
  level: DashboardAclLevel;
  /** 서버가 알려준 부여자. 새 행은 비어 있다(저장 시 서버가 채운다). */
  grantedBy: string;
}

/** 서버 응답 1건을 편집 행으로 바꾼다. 알 수 없는 접두사는 user 로 접지 않고 그대로 남긴다. */
function toDraftRow(entry: DashboardAclEntry): AclDraftRow {
  if (entry.subject.startsWith(SUBJECT_PREFIX_ROLE)) {
    return {
      kind: 'role',
      name: entry.subject.slice(SUBJECT_PREFIX_ROLE.length),
      level: entry.level,
      grantedBy: entry.granted_by ?? '',
    };
  }
  return {
    kind: 'user',
    name: entry.subject.startsWith(SUBJECT_PREFIX_USER)
      ? entry.subject.slice(SUBJECT_PREFIX_USER.length)
      : entry.subject,
    level: entry.level,
    grantedBy: entry.granted_by ?? '',
  };
}

/** 편집 행을 전송 형식으로 바꾼다. `granted_by` · `granted_at` 은 서버가 부여한다. */
function toEntry(row: AclDraftRow): DashboardAclEntry {
  const prefix = row.kind === 'role' ? SUBJECT_PREFIX_ROLE : SUBJECT_PREFIX_USER;
  return { subject: `${prefix}${row.name.trim()}`, level: row.level };
}

interface DashboardAclPanelProps {
  /** 권한을 편집할 대시보드. `can_grant` 가 true 인 항목만 전달된다. */
  dashboard: Dashboard;
  /** 닫기(저장 성공 포함). */
  onClose: () => void;
}

export default function DashboardAclPanel({
  dashboard,
  onClose,
}: DashboardAclPanelProps): React.JSX.Element {
  const { t } = useTranslation();
  const { hasPermission } = usePermission();
  const addNotification = useUIStore((s) => s.addNotification);

  const [rows, setRows] = useState<AclDraftRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [issue, setIssue] = useState<AclValidationIssue | null>(null);
  const [errorText, setErrorText] = useState<string | null>(null);

  // 대상 후보 목록. 목록 조회 권한이 없으면 서버가 403 이므로 아예 조회하지 않고
  // 이름 직접 입력으로 낮춘다(UsersPanel 의 역할 선택지와 동일한 처리).
  const canReadUsers = hasPermission('user.read');
  const canReadRoles = hasPermission('role.read');
  const usersQuery = useAdminUsers(canReadUsers);
  const rolesQuery = useAdminRoles(canReadRoles);

  const knownUsers = useMemo(
    () =>
      canReadUsers && usersQuery.data
        ? usersQuery.data.map((u) => u.username)
        : undefined,
    [canReadUsers, usersQuery.data],
  );
  const knownRoles = useMemo(
    () =>
      canReadRoles && rolesQuery.data ? rolesQuery.data.map((r) => r.name) : undefined,
    [canReadRoles, rolesQuery.data],
  );

  /**
   * 역할별 사용자 수 (spec.md §2.12 O1 — 선택 기능).
   *
   * 사용자 목록을 이미 대상 후보로 읽고 있으므로 추가 요청 없이 파생된다.
   * 목록을 못 읽으면 표시하지 않는다(추측하지 않는다).
   */
  const roleUserCounts = useMemo(() => {
    if (!canReadUsers || !usersQuery.data) return undefined;
    const counts = new Map<string, number>();
    for (const u of usersQuery.data) counts.set(u.role, (counts.get(u.role) ?? 0) + 1);
    return counts;
  }, [canReadUsers, usersQuery.data]);

  // 이름 후보를 읽지 못하는 상태 — 사용자에게 직접 입력임을 알린다.
  const subjectsUnavailable = knownUsers === undefined || knownRoles === undefined;

  // 최초 1회 서버 ACL 을 읽어 편집 상태를 만든다.
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    getAcl(dashboard.uid)
      .then((entries) => {
        if (cancelled) return;
        setRows(entries.map(toDraftRow));
        setErrorText(null);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setErrorText(
          t(
            err instanceof DashboardForbiddenError
              ? 'dashboard.acl.forbidden'
              : 'dashboard.acl.loadFailed',
          ),
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [dashboard.uid, t]);

  function updateRow(index: number, patch: Partial<AclDraftRow>): void {
    setIssue(null);
    setRows((prev) => prev.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  }

  function addRow(): void {
    setIssue(null);
    setRows((prev) => [...prev, { kind: 'user', name: '', level: 'view', grantedBy: '' }]);
  }

  function removeRow(index: number): void {
    setIssue(null);
    setRows((prev) => prev.filter((_, i) => i !== index));
  }

  async function handleSave(): Promise<void> {
    const entries = rows.map(toEntry);

    // 저장 전 검증 — 사유별로 구분된 안내를 준다(AC-17).
    const found = validateAclEntries(entries, {
      owner: dashboard.owner,
      knownUsers,
      knownRoles,
    });
    if (found) {
      setIssue(found);
      setErrorText(null);
      return;
    }

    setSaving(true);
    try {
      const saved = await putAcl(dashboard.uid, entries);
      setRows(saved.map(toDraftRow));
      addNotification({ type: 'success', message: t('dashboard.acl.saved') });
      onClose();
    } catch (err: unknown) {
      setIssue(null);
      if (err instanceof DashboardBadRequestError) {
        // 서버가 거부한 사유를 그대로 노출한다. 메시지를 파싱해 분류하지 않는다.
        setErrorText(`${t('dashboard.acl.rejected')}: ${err.message}`);
      } else if (err instanceof DashboardForbiddenError) {
        setErrorText(t('dashboard.acl.forbidden'));
      } else {
        setErrorText(t('dashboard.acl.saveFailed'));
      }
    } finally {
      setSaving(false);
    }
  }

  const issueText = issue
    ? t(issue.messageKey).replace('{value}', issue.value)
    : null;
  const alertText = issueText ?? errorText;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      role="dialog"
      aria-modal="true"
      aria-label={t('dashboard.acl.title')}
      data-testid="dashboard-acl-panel"
    >
      <div className="w-full max-w-2xl space-y-4 rounded-lg bg-(--color-bg-surface) p-6 shadow-xl">
        <header className="space-y-1">
          <h2 className="text-base font-semibold text-(--color-text-primary)">
            {t('dashboard.acl.title')}
          </h2>
          <p className="text-sm text-(--color-text-muted)">
            {t('dashboard.acl.subtitle').replace('{name}', dashboard.name)}
          </p>
        </header>

        {/* 전량 치환이라는 사실을 저장 전에 알린다 — 부분 수정으로 오해하면
            화면에서 지운 항목이 서버에도 사라진다는 것을 모른 채 저장한다. */}
        <p className="rounded-md border border-(--color-border-default) bg-(--color-bg-secondary) px-3 py-2 text-xs text-(--color-text-muted)">
          {t('dashboard.acl.replaceNotice')}
        </p>

        {/* visibility 가 acl 이 아니면 저장은 되지만 판정에서 무시된다(spec.md §2.9). */}
        {dashboard.visibility !== 'acl' && (
          <p
            data-testid="dashboard-acl-inactive"
            className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-400"
          >
            {t('dashboard.acl.inactiveNotice')}
          </p>
        )}

        {subjectsUnavailable && (
          <p className="text-xs text-(--color-text-muted)">
            {t('dashboard.acl.subjectsUnavailable')}
          </p>
        )}

        {alertText && (
          <div
            role="alert"
            data-testid="dashboard-acl-error"
            className="flex items-start gap-2 rounded-md border border-red-500/40 bg-red-500/10 px-3 py-2 text-sm text-red-600 dark:text-red-400"
          >
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
            <span>{alertText}</span>
          </div>
        )}

        {loading ? (
          <p className="inline-flex items-center gap-2 text-sm text-(--color-text-muted)">
            <Loader2 className="h-4 w-4 animate-spin" />
            {t('common.loading')}
          </p>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
            <table className="min-w-full divide-y divide-(--color-border-default)">
              <thead className="bg-(--color-bg-secondary)">
                <tr>
                  <th className={TH_CLASS}>{t('dashboard.acl.subjectKind')}</th>
                  <th className={TH_CLASS}>{t('dashboard.acl.subjectName')}</th>
                  <th className={TH_CLASS}>{t('dashboard.acl.level')}</th>
                  <th className={TH_CLASS}>{t('dashboard.acl.grantedBy')}</th>
                  <th className={TH_CLASS}>{t('common.actions')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-(--color-border-default)">
                {rows.length === 0 && (
                  <tr>
                    <td className="px-2 py-3 text-sm text-(--color-text-muted)" colSpan={5}>
                      {t('dashboard.acl.empty')}
                    </td>
                  </tr>
                )}
                {rows.map((row, index) => (
                  <tr key={index} data-testid={`dashboard-acl-row-${index}`}>
                    <td className="px-2 py-1.5">
                      <select
                        className={INPUT_CLASS}
                        aria-label={`${t('dashboard.acl.subjectKind')} ${index + 1}`}
                        value={row.kind}
                        onChange={(e) =>
                          updateRow(index, { kind: e.target.value as AclDraftRow['kind'] })
                        }
                      >
                        <option value="user">{t('dashboard.acl.kindUser')}</option>
                        <option value="role">{t('dashboard.acl.kindRole')}</option>
                      </select>
                    </td>
                    <td className="px-2 py-1.5">
                      {/* datalist 로 후보를 제안하되 자유 입력을 막지 않는다 —
                          목록 조회 권한이 없어도 이름을 넣을 수 있어야 한다. */}
                      <input
                        className={INPUT_CLASS}
                        aria-label={`${t('dashboard.acl.subjectName')} ${index + 1}`}
                        list={`dashboard-acl-${row.kind}-options`}
                        value={row.name}
                        onChange={(e) => updateRow(index, { name: e.target.value })}
                      />
                    </td>
                    <td className="px-2 py-1.5">
                      <select
                        className={INPUT_CLASS}
                        aria-label={`${t('dashboard.acl.level')} ${index + 1}`}
                        value={row.level}
                        onChange={(e) =>
                          updateRow(index, { level: e.target.value as DashboardAclLevel })
                        }
                      >
                        <option value="view">{t('dashboard.acl.levelView')}</option>
                        <option value="edit">{t('dashboard.acl.levelEdit')}</option>
                      </select>
                    </td>
                    <td className="px-2 py-1.5 text-sm text-(--color-text-muted)">
                      {row.kind === 'role' && roleUserCounts
                        ? t('dashboard.acl.roleUserCount').replace(
                            '{value}',
                            String(roleUserCounts.get(row.name.trim()) ?? 0),
                          )
                        : row.grantedBy}
                    </td>
                    <td className="px-2 py-1.5">
                      <button
                        type="button"
                        onClick={() => removeRow(index)}
                        aria-label={`${t('dashboard.acl.remove')} ${index + 1}`}
                        className="rounded p-1.5 text-red-500 hover:bg-red-500/10"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* 대상 후보 datalist — 목록을 읽지 못하면 비어 있고, 입력은 그대로 가능하다. */}
        <datalist id="dashboard-acl-user-options">
          {(knownUsers ?? []).map((name) => (
            <option key={name} value={name} />
          ))}
        </datalist>
        <datalist id="dashboard-acl-role-options">
          {(knownRoles ?? []).map((name) => (
            <option key={name} value={name} />
          ))}
        </datalist>

        <div className="flex items-center justify-between gap-2">
          <button
            type="button"
            onClick={addRow}
            className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-strong) px-3 py-2 text-sm text-(--color-text-primary) hover:bg-(--color-bg-elevated)"
          >
            <Plus className="h-4 w-4" />
            {t('dashboard.acl.add')}
          </button>

          <div className="flex gap-2">
            <button
              type="button"
              onClick={onClose}
              className="rounded-md border border-(--color-border-strong) px-3 py-2 text-sm text-(--color-text-primary)"
            >
              {t('common.cancel')}
            </button>
            <button
              type="button"
              onClick={() => void handleSave()}
              disabled={saving || loading}
              className="rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {t('common.save')}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
