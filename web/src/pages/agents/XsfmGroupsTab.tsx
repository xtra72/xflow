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
import { LINE_CODE_PATTERN } from '@/hooks/useLine';
import { useXsfmDevices, useStations, type AirDevice, type AirStation } from '@/hooks/useStation';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import { ConfirmDialog } from '@/components/property/ConfirmDialog';
import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { FacilityBulkControl } from '@/pages/dashboard/panels/facilityShared';

/** 필터 셀렉트에서 "미지정"(빈 station/place/line) 버킷을 나타내는 sentinel 값('' = 전체와 구분). */
const UNASSIGNED = '__unassigned__';

const inputCls =
  'block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)';
const labelCls = 'mb-1 block text-xs font-medium text-(--color-text-secondary)';

/** 그룹 폼 상태. 편집 시 group_id 가 채워지고(잠금), 추가 시 비어 있다. */
interface GroupFormState {
  /** 편집 대상 group_id(추가 모드에서는 ''). */
  groupId: string;
  name: string;
  /**
   * 사용자 지정 코드(SPEC-XSFM-LINE-001, RD-2). 추가 모드에서만 입력·전송한다(선택).
   * 편집 모드에서는 코드가 식별자이므로 변경 불가(입력 미노출).
   */
  code: string;
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
  const { data: stations = [] } = useStations(agentId);

  const addGroup = useAddGroup(agentId);
  const setGroup = useSetGroup(agentId);
  const removeGroup = useRemoveGroup(agentId);

  // 폼 상태: null=닫힘. groupId 가 있으면 편집(커스텀만), 없으면 추가.
  const [form, setForm] = useState<GroupFormState | null>(null);
  const [removeTarget, setRemoveTarget] = useState<Group | null>(null);

  // 멤버 선택 테이블 필터(모달 스코프, 런타임). 기본 '전체'(빈 값). UNASSIGNED = 미지정 버킷.
  const [filterLine, setFilterLine] = useState('');
  const [filterStation, setFilterStation] = useState('');
  const [filterPlace, setFilterPlace] = useState('');
  // 멤버 선택 테이블 정렬(라인/역사/위치). 기본 역사 오름차순, 동률은 이름(ko) tiebreak.
  const [memberSort, setMemberSort] = useState<SortState>({ field: 'station', direction: 'asc' });

  // station CODE → 레지스트리 엔트리 맵(라인 파생 · 표시명 해석). 라인은 디바이스가 아니라 역사에서 파생한다.
  const stationsByCode = useMemo(() => {
    const m = new Map<string, AirStation>();
    for (const s of stations) m.set(s.station, s);
    return m;
  }, [stations]);

  // 디바이스의 라인(호선)은 station→레지스트리 파생값이다(디바이스 필드가 아님). 미해석 시 ''.
  function deviceLine(d: AirDevice): string {
    return stationsByCode.get(d.station)?.line ?? '';
  }
  function stationDisplay(code: string): string {
    if (!code) return '-';
    return stationsByCode.get(code)?.display_name || code;
  }
  function placeDisplay(stationCode: string, placeCode: string): string {
    if (!placeCode) return '-';
    const p = stationsByCode.get(stationCode)?.places.find((x) => x.place === placeCode);
    return p?.display_name || placeCode;
  }

  // device_id → 표시 이름 맵(멤버 읽기 전용 뷰/편집 라벨 해석용).
  const deviceLabel = useMemo(() => {
    const m = new Map<string, string>();
    for (const d of devices) m.set(d.device_id, d.name || d.device_id);
    return m;
  }, [devices]);

  // 필터 옵션(디바이스 실측값 기준 — 빈 값이 있으면 미지정 버킷 노출). 라인은 파생값 distinct.
  const lineOptions = useMemo(() => {
    const set = new Set<string>();
    let hasUnassigned = false;
    for (const d of devices) {
      const ln = deviceLine(d);
      if (ln) set.add(ln);
      else hasUnassigned = true;
    }
    return { values: Array.from(set).sort((a, b) => a.localeCompare(b, 'ko')), hasUnassigned };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [devices, stationsByCode]);

  const stationOptions = useMemo(() => {
    const set = new Set<string>();
    let hasUnassigned = false;
    for (const d of devices) {
      if (d.station) set.add(d.station);
      else hasUnassigned = true;
    }
    const values = Array.from(set).sort((a, b) =>
      stationDisplay(a).localeCompare(stationDisplay(b), 'ko'),
    );
    return { values, hasUnassigned };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [devices, stationsByCode]);

  const placeOptions = useMemo(() => {
    const set = new Set<string>();
    let hasUnassigned = false;
    for (const d of devices) {
      if (d.place) set.add(d.place);
      else hasUnassigned = true;
    }
    return { values: Array.from(set).sort((a, b) => a.localeCompare(b, 'ko')), hasUnassigned };
  }, [devices]);

  // 필터 적용(AND): 라인(파생) + 역사(코드) + 위치(코드). UNASSIGNED 는 빈 값 매칭.
  const filteredMembers = useMemo(() => {
    return devices.filter((d) => {
      const ln = deviceLine(d);
      if (filterLine) {
        if (filterLine === UNASSIGNED ? ln !== '' : ln !== filterLine) return false;
      }
      if (filterStation) {
        if (filterStation === UNASSIGNED ? d.station !== '' : d.station !== filterStation) return false;
      }
      if (filterPlace) {
        if (filterPlace === UNASSIGNED ? d.place !== '' : d.place !== filterPlace) return false;
      }
      return true;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [devices, filterLine, filterStation, filterPlace, stationsByCode]);

  // 정렬(라인/역사/위치). 문자열 localeCompare('ko'), 동률은 이름 → device_id 결정적 tiebreak.
  const sortedMembers = useMemo(() => {
    const dir = memberSort.direction === 'asc' ? 1 : -1;
    const primary = (a: AirDevice, b: AirDevice): number => {
      switch (memberSort.field) {
        case 'line':
          return deviceLine(a).localeCompare(deviceLine(b), 'ko');
        case 'station':
          return stationDisplay(a.station).localeCompare(stationDisplay(b.station), 'ko');
        case 'place':
          return placeDisplay(a.station, a.place).localeCompare(placeDisplay(b.station, b.place), 'ko');
        default:
          return 0;
      }
    };
    return [...filteredMembers].sort((a, b) => {
      const c = primary(a, b);
      if (c !== 0) return c * dir;
      return (a.name || '').localeCompare(b.name || '', 'ko') || a.device_id.localeCompare(b.device_id);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filteredMembers, memberSort, stationsByCode]);

  function handleMemberSort(field: string) {
    setMemberSort((prev) =>
      prev.field === field
        ? { field, direction: prev.direction === 'asc' ? 'desc' : 'asc' }
        : { field, direction: 'asc' },
    );
  }

  // 결정적 순서 유지(백엔드가 type→name 정렬해 반환하지만 방어적으로 정렬).
  const sortedGroups = useMemo(() => {
    const typeOrder: Record<Group['type'], number> = { station: 0, line: 1, custom: 2 };
    return [...groups].sort((a, b) => {
      if (typeOrder[a.type] !== typeOrder[b.type]) return typeOrder[a.type] - typeOrder[b.type];
      return (a.name || '').localeCompare(b.name || '', 'ko');
    });
  }, [groups]);

  function openAdd() {
    setForm({ groupId: '', name: '', code: '', members: new Set() });
  }

  function openEdit(g: Group) {
    if (!isCustomGroup(g)) return; // 기본 그룹은 편집 불가(방어).
    setForm({ groupId: g.id, name: g.name, code: g.code, members: new Set(g.members) });
  }

  // 추가 모드에서 입력한 코드의 포맷 위반 여부(빈 코드는 허용 — 선택 필드). 편집 모드는 코드 잠금.
  const codeInvalid = !!form && !form.groupId && form.code.trim() !== '' && !LINE_CODE_PATTERN.test(form.code.trim());

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
      // 추가: name + 초기 members(선택) + code(선택, RD-2). code 제공 시 포맷 검증 후 전송하며,
      // 생략하면 기존 name 기반 동작을 유지한다(하위호환).
      const code = form.code.trim();
      if (code && !LINE_CODE_PATTERN.test(code)) {
        addNotification({ type: 'error', message: t('agents.detail.groups.codeFormatError') });
        return;
      }
      addGroup.mutate(
        code ? { name, members, code } : { name, members },
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
                    {/* 코드 표시(SPEC-XSFM-LINE-001) — 있을 때만 mono 배지로 노출. */}
                    {g.code && (
                      <span
                        data-testid={`group-code-${g.id}`}
                        className="shrink-0 truncate font-mono text-[10px] text-(--color-text-muted)"
                        title={g.code}
                      >
                        {g.code}
                      </span>
                    )}
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
            className="mx-4 flex max-h-[85vh] w-full max-w-2xl flex-col rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) shadow-xl"
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
              {/* 코드 입력(추가 모드만, 선택). 코드는 식별자이므로 편집 모드에서는 잠금(미노출). */}
              {!form.groupId && (
                <div>
                  <label className={labelCls}>{t('agents.detail.groups.code')}</label>
                  <input
                    type="text"
                    value={form.code}
                    placeholder={t('agents.detail.groups.codePlaceholder')}
                    onChange={(e) => setForm({ ...form, code: e.target.value })}
                    data-testid="group-code-input"
                    className={inputCls}
                  />
                  <p
                    data-testid="group-code-hint"
                    className={cn('mt-1 text-[11px]', codeInvalid ? 'text-red-500' : 'text-(--color-text-muted)')}
                  >
                    {t('agents.detail.groups.codeFormatHint')}
                  </p>
                </div>
              )}
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
                  <div data-testid="group-member-editor" className="space-y-2">
                    {/* 필터 바: 라인(파생) / 역사 / 위치 + 표시 개수. */}
                    <div className="flex flex-wrap items-center gap-2">
                      <select
                        value={filterLine}
                        onChange={(e) => setFilterLine(e.target.value)}
                        aria-label={t('agents.detail.groups.line')}
                        data-testid="group-member-filter-line"
                        className="rounded-md border border-(--color-border-strong) px-2 py-1.5 text-xs bg-(--color-bg-surface) text-(--color-text-primary)"
                      >
                        <option value="">{t('agents.detail.groups.filter.lineAll')}</option>
                        {lineOptions.values.map((ln) => (
                          <option key={ln} value={ln}>
                            {ln}
                          </option>
                        ))}
                        {lineOptions.hasUnassigned && (
                          <option value={UNASSIGNED}>{t('agents.detail.groups.filter.unassigned')}</option>
                        )}
                      </select>
                      <select
                        value={filterStation}
                        onChange={(e) => setFilterStation(e.target.value)}
                        aria-label={t('agents.detail.groups.station')}
                        data-testid="group-member-filter-station"
                        className="rounded-md border border-(--color-border-strong) px-2 py-1.5 text-xs bg-(--color-bg-surface) text-(--color-text-primary)"
                      >
                        <option value="">{t('agents.detail.groups.filter.stationAll')}</option>
                        {stationOptions.values.map((code) => (
                          <option key={code} value={code}>
                            {stationDisplay(code)}
                          </option>
                        ))}
                        {stationOptions.hasUnassigned && (
                          <option value={UNASSIGNED}>{t('agents.detail.groups.filter.unassigned')}</option>
                        )}
                      </select>
                      <select
                        value={filterPlace}
                        onChange={(e) => setFilterPlace(e.target.value)}
                        aria-label={t('agents.detail.groups.place')}
                        data-testid="group-member-filter-place"
                        className="rounded-md border border-(--color-border-strong) px-2 py-1.5 text-xs bg-(--color-bg-surface) text-(--color-text-primary)"
                      >
                        <option value="">{t('agents.detail.groups.filter.placeAll')}</option>
                        {placeOptions.values.map((code) => (
                          <option key={code} value={code}>
                            {code}
                          </option>
                        ))}
                        {placeOptions.hasUnassigned && (
                          <option value={UNASSIGNED}>{t('agents.detail.groups.filter.unassigned')}</option>
                        )}
                      </select>
                      <span className="whitespace-nowrap text-[11px] tabular-nums text-(--color-text-muted)">
                        {t('agents.detail.groups.filter.count')
                          .replace('{shown}', String(filteredMembers.length))
                          .replace('{total}', String(devices.length))}
                      </span>
                    </div>

                    {/* 멤버 선택 테이블: 선택 · 라인 · 역사 · 위치 · 이름. */}
                    {filteredMembers.length === 0 ? (
                      <p className="py-6 text-center text-xs text-(--color-text-muted)">
                        {t('agents.detail.groups.noMemberMatch')}
                      </p>
                    ) : (
                      <div className="max-h-72 overflow-auto rounded-md border border-(--color-border-default)">
                        <table className="w-full text-xs" data-testid="group-member-table">
                          <thead className="sticky top-0 bg-(--color-bg-elevated)">
                            <tr className="border-b border-(--color-border-default)">
                              <th className="w-8 px-2 py-1.5 text-left text-(--color-text-muted)">
                                {t('agents.detail.groups.select')}
                              </th>
                              <SortableHeader
                                label={t('agents.detail.groups.line')}
                                field="line"
                                currentSort={memberSort}
                                onSort={handleMemberSort}
                                className="px-2 py-1.5"
                              />
                              <SortableHeader
                                label={t('agents.detail.groups.station')}
                                field="station"
                                currentSort={memberSort}
                                onSort={handleMemberSort}
                                className="px-2 py-1.5"
                              />
                              <SortableHeader
                                label={t('agents.detail.groups.place')}
                                field="place"
                                currentSort={memberSort}
                                onSort={handleMemberSort}
                                className="px-2 py-1.5"
                              />
                              <th className="px-2 py-1.5 text-left text-(--color-text-muted)">
                                {t('agents.detail.groups.name')}
                              </th>
                            </tr>
                          </thead>
                          <tbody>
                            {sortedMembers.map((d) => (
                              <tr
                                key={d.device_id}
                                data-testid={`group-member-row-${d.device_id}`}
                                onClick={() => toggleMember(d.device_id)}
                                className="cursor-pointer border-b border-(--color-border-default) last:border-0 hover:bg-(--color-bg-elevated)"
                              >
                                <td className="w-8 px-2 py-1.5">
                                  <input
                                    type="checkbox"
                                    checked={form.members.has(d.device_id)}
                                    onChange={() => toggleMember(d.device_id)}
                                    onClick={(e) => e.stopPropagation()}
                                    data-testid={`group-member-checkbox-${d.device_id}`}
                                    className="h-3.5 w-3.5 cursor-pointer accent-blue-600"
                                  />
                                </td>
                                <td className="px-2 py-1.5 text-(--color-text-secondary)">
                                  {deviceLine(d) || '-'}
                                </td>
                                <td className="px-2 py-1.5 text-(--color-text-secondary)">
                                  {stationDisplay(d.station)}
                                </td>
                                <td className="px-2 py-1.5 text-(--color-text-secondary)">
                                  {placeDisplay(d.station, d.place)}
                                </td>
                                <td className="truncate px-2 py-1.5 text-(--color-text-primary)">
                                  {d.name || d.device_id}
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>
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
                disabled={submitting || !form.name.trim() || codeInvalid}
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
