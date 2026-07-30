// xsfm 그룹 관리 탭 (SPEC-XSFM-GROUP-001 Module 6, M6).
//
// 그룹(group)은 1급 엔티티이며, 이 탭은 (1) 그룹 목록(type 배지 + 멤버 수), (2) 커스텀 그룹
// 생성/수정/삭제(add_group/set_group/remove_group), (3) 커스텀 그룹의 멤버(디바이스) 편집,
// (4) 그룹 대상 일괄 제어(group_id 셀렉터)를 제공한다. 기본 그룹(type=station|line)은 파생·읽기
// 전용이라 이름/멤버 편집·삭제 UI 를 비활성/미노출한다(REQ-06-03). 일괄 제어와 집계 결과 표시는
// FacilityBulkControl/ControlResultView 를 재사용한다(REQ-06-04, 재구현 없음).

import { useMemo, useState } from 'react';
import { Layers, Lock, Pencil, Plus, Trash2, Users, X } from 'lucide-react';

import {
  isCustomGroup,
  useAddGroup,
  useGroups,
  useRemoveGroup,
  useSetGroup,
  type Group,
} from '@/hooks/useGroups';
import { useXsfmDevices, type AirDevice } from '@/hooks/useStation';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import { ConfirmDialog } from '@/components/property/ConfirmDialog';
import { FacilityBulkControl } from '@/pages/dashboard/panels/facilityShared';

const inputCls =
  'block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)';
const labelCls = 'mb-1 block text-xs font-medium text-(--color-text-secondary)';

/** 그룹 폼 상태. 편집 시 group_id 가 채워지고(잠금), 추가 시 비어 있다. */
interface GroupFormState {
  /** 편집 대상 group_id(추가 모드에서는 ''). */
  groupId: string;
  name: string;
  /** 선택된 멤버 device_id 집합. */
  members: Set<string>;
}

/** type 배지 색상(custom 파랑 · station 초록 · line 보라). */
const TYPE_BADGE: Record<Group['type'], string> = {
  custom: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  station: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
  line: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
};

export default function XsfmGroupsTab({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const { data: groups = [], isLoading } = useGroups(agentId);
  const { data: devices = [] } = useXsfmDevices(agentId);

  const addGroup = useAddGroup(agentId);
  const setGroup = useSetGroup(agentId);
  const removeGroup = useRemoveGroup(agentId);

  // 폼 상태: null=닫힘. groupId 가 있으면 편집(커스텀만), 없으면 추가.
  const [form, setForm] = useState<GroupFormState | null>(null);
  const [removeTarget, setRemoveTarget] = useState<Group | null>(null);

  // device_id → 표시 이름 맵(멤버 읽기 전용 뷰/편집 라벨 해석용).
  const deviceLabel = useMemo(() => {
    const m = new Map<string, string>();
    for (const d of devices) m.set(d.device_id, d.name || d.device_id);
    return m;
  }, [devices]);

  // 결정적 순서 유지(백엔드가 type→name 정렬해 반환하지만 방어적으로 정렬).
  const sortedGroups = useMemo(() => {
    const typeOrder: Record<Group['type'], number> = { station: 0, line: 1, custom: 2 };
    return [...groups].sort((a, b) => {
      if (typeOrder[a.type] !== typeOrder[b.type]) return typeOrder[a.type] - typeOrder[b.type];
      return (a.name || '').localeCompare(b.name || '', 'ko');
    });
  }, [groups]);

  function openAdd() {
    setForm({ groupId: '', name: '', members: new Set() });
  }

  function openEdit(g: Group) {
    if (!isCustomGroup(g)) return; // 기본 그룹은 편집 불가(방어).
    setForm({ groupId: g.id, name: g.name, members: new Set(g.members) });
  }

  function closeForm() {
    setForm(null);
  }

  function toggleMember(deviceId: string) {
    setForm((prev) => {
      if (!prev) return prev;
      const members = new Set(prev.members);
      if (members.has(deviceId)) members.delete(deviceId);
      else members.add(deviceId);
      return { ...prev, members };
    });
  }

  function notifyOpError(err: unknown) {
    addNotification({
      type: 'error',
      message: t('agents.detail.groups.opError').replace(
        '{message}',
        err instanceof Error ? err.message : t('agents.detail.groups.unknownError'),
      ),
    });
  }

  function handleSubmit() {
    if (!form) return;
    const name = form.name.trim();
    if (!name) {
      addNotification({ type: 'error', message: t('agents.detail.groups.nameRequired') });
      return;
    }
    const members = Array.from(form.members);

    if (form.groupId) {
      // 편집(커스텀만): 이름 + 멤버 부분 갱신.
      setGroup.mutate(
        { group_id: form.groupId, name, members },
        {
          onSuccess: () => {
            closeForm();
            addNotification({ type: 'success', message: t('agents.detail.groups.updateSuccess') });
          },
          onError: (err) => notifyOpError(err),
        },
      );
    } else {
      // 추가: name + 초기 members(선택).
      addGroup.mutate(
        { name, members },
        {
          onSuccess: () => {
            closeForm();
            addNotification({ type: 'success', message: t('agents.detail.groups.addSuccess') });
          },
          onError: (err) => notifyOpError(err),
        },
      );
    }
  }

  function confirmRemove() {
    if (!removeTarget) return;
    const target = removeTarget;
    removeGroup.mutate(target.id, {
      onSuccess: () => {
        setRemoveTarget(null);
        addNotification({ type: 'success', message: t('agents.detail.groups.removeSuccess') });
      },
      onError: (err) => {
        setRemoveTarget(null);
        notifyOpError(err);
      },
    });
  }

  const submitting = addGroup.isPending || setGroup.isPending;

  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-3 p-4 md:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="h-20 animate-pulse rounded-lg bg-(--color-bg-elevated)" />
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-3 p-4">
      {/* 헤더: 개수 + 커스텀 그룹 추가 버튼 */}
      <div className="flex items-center justify-between">
        <span className="text-xs text-(--color-text-muted)">
          {t('agents.detail.groups.groupsCount').replace('{count}', String(groups.length))}
        </span>
        <button
          type="button"
          onClick={openAdd}
          className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <Plus className="h-3.5 w-3.5" />
          {t('agents.detail.groups.addGroup')}
        </button>
      </div>

      {/* 그룹 목록 */}
      {groups.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-12 text-(--color-text-muted)">
          <Layers className="h-8 w-8 opacity-40" aria-hidden="true" />
          <p className="text-sm">{t('agents.detail.groups.noGroups')}</p>
        </div>
      ) : (
        <ul className="space-y-2" data-testid="group-list">
          {sortedGroups.map((g) => {
            const custom = isCustomGroup(g);
            return (
              <li
                key={g.id}
                data-testid={`group-item-${g.id}`}
                className="rounded-lg border border-(--color-border-default) px-3 py-2"
              >
                <div className="flex items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <span
                      data-testid={`group-type-${g.id}`}
                      className={cn(
                        'inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium',
                        TYPE_BADGE[g.type],
                      )}
                    >
                      {!custom && <Lock className="h-2.5 w-2.5" aria-hidden="true" />}
                      {t(`agents.detail.groups.type.${g.type}`)}
                    </span>
                    <span className="truncate text-sm font-medium text-(--color-text-primary)" title={g.name}>
                      {g.name}
                    </span>
                    <span
                      data-testid={`group-member-count-${g.id}`}
                      className="inline-flex shrink-0 items-center gap-1 text-[11px] text-(--color-text-muted)"
                    >
                      <Users className="h-3 w-3" aria-hidden="true" />
                      {g.member_count}
                    </span>
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    {/* 그룹 일괄 제어(group_id 셀렉터) — FacilityBulkControl 재사용(REQ-06-04). */}
                    <FacilityBulkControl
                      agentId={agentId}
                      selector={{ group_id: g.id }}
                      memberCount={g.member_count}
                    />
                    {/* 커스텀 그룹만 편집/삭제. 기본 그룹은 읽기 전용(REQ-06-03/03-05). */}
                    {custom && (
                      <div className="inline-flex items-center gap-1">
                        <button
                          type="button"
                          onClick={() => openEdit(g)}
                          aria-label={t('agents.detail.groups.edit')}
                          data-testid={`group-edit-${g.id}`}
                          className="rounded p-1 text-(--color-text-muted) hover:bg-(--color-bg-surface) hover:text-(--color-text-secondary)"
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() => setRemoveTarget(g)}
                          aria-label={t('agents.detail.groups.remove')}
                          data-testid={`group-remove-${g.id}`}
                          className="rounded p-1 text-(--color-text-muted) hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    )}
                  </div>
                </div>

                {/* 멤버 목록: 기본 그룹은 읽기 전용으로 표시(REQ-06-03). 커스텀은 편집 모달에서 관리. */}
                {!custom && g.members.length > 0 && (
                  <ul
                    data-testid={`group-members-readonly-${g.id}`}
                    className="mt-1.5 flex flex-wrap gap-1 border-t border-(--color-border-default) pt-1.5"
                  >
                    {g.members.map((deviceId) => (
                      <li
                        key={deviceId}
                        className="inline-flex items-center rounded-full bg-(--color-bg-elevated) px-1.5 py-0.5 text-[10px] text-(--color-text-secondary)"
                      >
                        {deviceLabel.get(deviceId) || deviceId}
                      </li>
                    ))}
                  </ul>
                )}
              </li>
            );
          })}
        </ul>
      )}

      {/* 추가/편집 폼 모달(커스텀 그룹) */}
      {form && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={closeForm}>
          <div
            className="mx-4 flex max-h-[85vh] w-full max-w-md flex-col rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-(--color-border-default) px-4 py-3">
              <h3 className="text-sm font-semibold text-(--color-text-primary)">
                {form.groupId ? t('agents.detail.groups.editGroup') : t('agents.detail.groups.addGroup')}
              </h3>
              <button
                type="button"
                onClick={closeForm}
                className="rounded-md p-1 text-(--color-text-muted) hover:bg-(--color-bg-surface)"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <div className="space-y-3 overflow-y-auto p-4">
              <div>
                <label className={labelCls}>
                  {t('agents.detail.groups.name')} <span className="text-red-500">*</span>
                </label>
                <input
                  type="text"
                  value={form.name}
                  placeholder={t('agents.detail.groups.namePlaceholder')}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  className={inputCls}
                />
              </div>
              <div>
                <label className={labelCls}>
                  {t('agents.detail.groups.members')}{' '}
                  <span className="text-(--color-text-muted)">
                    ({t('agents.detail.groups.memberSelected').replace('{count}', String(form.members.size))})
                  </span>
                </label>
                {devices.length === 0 ? (
                  <p className="text-xs text-(--color-text-muted)">{t('agents.detail.groups.noDevicesHint')}</p>
                ) : (
                  <ul
                    data-testid="group-member-editor"
                    className="max-h-56 space-y-1 overflow-y-auto rounded-md border border-(--color-border-default) p-2"
                  >
                    {devices.map((d: AirDevice) => (
                      <li key={d.device_id}>
                        <label className="flex cursor-pointer items-center gap-2 rounded px-1 py-0.5 hover:bg-(--color-bg-elevated)">
                          <input
                            type="checkbox"
                            checked={form.members.has(d.device_id)}
                            onChange={() => toggleMember(d.device_id)}
                            data-testid={`group-member-checkbox-${d.device_id}`}
                            className="h-3.5 w-3.5 cursor-pointer accent-blue-600"
                          />
                          <span className="truncate text-xs text-(--color-text-primary)">
                            {d.name || d.device_id}
                          </span>
                        </label>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-4 py-3">
              <button
                type="button"
                onClick={closeForm}
                className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) hover:bg-(--color-bg-surface)"
              >
                {t('common.cancel')}
              </button>
              <button
                type="button"
                onClick={handleSubmit}
                disabled={submitting || !form.name.trim()}
                data-testid="group-form-submit"
                className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500"
              >
                {submitting
                  ? t('agents.detail.groups.saving')
                  : form.groupId
                    ? t('agents.detail.groups.save')
                    : t('agents.detail.groups.add')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 삭제 확인 */}
      <ConfirmDialog
        isOpen={removeTarget !== null}
        onClose={() => setRemoveTarget(null)}
        onConfirm={confirmRemove}
        title={t('agents.detail.groups.removeConfirmTitle')}
        message={t('agents.detail.groups.removeConfirmMessage').replace('{name}', removeTarget?.name || '')}
        variant="danger"
        isSubmitting={removeGroup.isPending}
      />
    </div>
  );
}
