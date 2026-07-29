// airpurifier 디바이스 관리 탭.
//
// SPEC-AIRPURIFIER-001 Wave 2 (identity 모델 개정). 백엔드가 device_id(UUID)를 생성하고
// name(복합 키 "station:place:index")을 계산한다. 프런트엔드는 station/place/index/group_id
// 만 입력하며, 목록의 station/place CODE 는 역사 레지스트리(useStations)로 표시명을 해석한다.

import { useMemo, useState } from 'react';
import { HardDrive, Pencil, Plus, Trash2, X } from 'lucide-react';

import {
  useAddAirpurifierDevice,
  useAirpurifierDevices,
  useRemoveAirpurifierDevice,
  useSetAirpurifierDevice,
  useStations,
  type AirDevice,
  type AirStation,
} from '@/hooks/useStation';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import { ConfirmDialog } from '@/components/property/ConfirmDialog';

/** 폼 상태(문자열 위주 — 제출 시 파싱). device_id/name 은 입력하지 않는다(백엔드 생성/계산). */
interface DeviceFormState {
  station: string;
  place: string;
  index: string;
  group_id: string;
}

const EMPTY_FORM: DeviceFormState = { station: '', place: '', index: '', group_id: '' };

const inputCls =
  'block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)';
const labelCls = 'mb-1 block text-xs font-medium text-(--color-text-secondary)';

/** UUID 를 축약 표시(앞 8자 + …). 전체 값은 title 툴팁으로 노출. */
function shortId(id: string): string {
  return id.length > 8 ? `${id.slice(0, 8)}…` : id;
}

export default function AirpurifierDevicesTab({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const { data: devices = [], isLoading } = useAirpurifierDevices(agentId);
  const { data: stations = [] } = useStations(agentId);

  const addDevice = useAddAirpurifierDevice(agentId);
  const setDevice = useSetAirpurifierDevice(agentId);
  const removeDevice = useRemoveAirpurifierDevice(agentId);

  // station CODE → 엔트리 맵(표시명 해석 + place 조회). 미등록 코드는 원문 fallback.
  const stationsByCode = useMemo(() => {
    const m = new Map<string, AirStation>();
    for (const s of stations) m.set(s.station, s);
    return m;
  }, [stations]);

  function stationDisplay(code: string): string {
    if (!code) return '-';
    return stationsByCode.get(code)?.display_name || code;
  }
  function placeDisplay(stationCode: string, placeCode: string): string {
    if (!placeCode) return '-';
    const p = stationsByCode.get(stationCode)?.places.find((x) => x.place === placeCode);
    return p?.display_name || placeCode;
  }

  // 폼 상태: null=닫힘, editing 이 있으면 편집 대상 device_id(UUID), 없으면 추가.
  const [form, setForm] = useState<DeviceFormState | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  const [removeTarget, setRemoveTarget] = useState<AirDevice | null>(null);

  // 필터 상태(클라이언트 사이드): 라인 / 역사(코드) / 검색어. 기본 '전체'(빈 값).
  const [filterLine, setFilterLine] = useState('');
  const [filterStation, setFilterStation] = useState('');
  const [filterText, setFilterText] = useState('');

  // 선택된 역사의 위치 목록(선택 역사 종속). 역사 미선택 시 빈 배열.
  const placeOptions = useMemo(() => {
    return stationsByCode.get(form?.station ?? '')?.places ?? [];
  }, [stationsByCode, form?.station]);

  // 라인 필터 옵션: stations 의 distinct line (빈 값 제외, 정렬).
  const distinctLines = useMemo(() => {
    const set = new Set<string>();
    for (const s of stations) if (s.line) set.add(s.line);
    return Array.from(set).sort();
  }, [stations]);

  // 역사 필터 옵션: 선택 라인에 속한 역사 (라인 미선택 시 전체).
  const stationFilterOptions = useMemo(
    () => (filterLine ? stations.filter((s) => s.line === filterLine) : stations),
    [stations, filterLine],
  );

  // AND 결합 필터: 라인(디바이스 station→line) + 역사(코드) + 검색어.
  // 검색어는 name + device_id + 해석된 역사/위치 표시명에 대소문자 무시 부분일치.
  const filteredDevices = useMemo(() => {
    const q = filterText.trim().toLowerCase();
    return devices.filter((d) => {
      const st = stationsByCode.get(d.station);
      if (filterLine && (st?.line ?? '') !== filterLine) return false;
      if (filterStation && d.station !== filterStation) return false;
      if (q) {
        const stationName = st?.display_name || d.station;
        const placeName = st?.places.find((x) => x.place === d.place)?.display_name || d.place;
        const hay = `${d.name} ${d.device_id} ${stationName} ${placeName}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    });
  }, [devices, filterLine, filterStation, filterText, stationsByCode]);

  function openAdd() {
    setEditing(null);
    setForm({ ...EMPTY_FORM });
  }

  function openEdit(d: AirDevice) {
    setEditing(d.device_id);
    setForm({
      station: d.station ?? '',
      place: d.place ?? '',
      index: d.index ? String(d.index) : '',
      group_id: d.group_id ?? '',
    });
  }

  function closeForm() {
    setForm(null);
    setEditing(null);
  }

  function handleSubmit() {
    if (!form) return;
    const station = form.station.trim();
    const place = form.place.trim();
    if (!station || !place) {
      addNotification({ type: 'error', message: t('agents.detail.airDevices.stationPlaceRequired') });
      return;
    }
    if (form.index.trim() === '') {
      addNotification({ type: 'error', message: t('agents.detail.airDevices.indexRequired') });
      return;
    }
    const indexNum = parseInt(form.index.trim(), 10);
    if (Number.isNaN(indexNum)) {
      addNotification({ type: 'error', message: t('agents.detail.airDevices.indexError') });
      return;
    }
    const groupId = form.group_id.trim();

    if (editing) {
      setDevice.mutate(
        { device_id: editing, station, place, index: indexNum, group_id: groupId },
        {
          onSuccess: () => {
            closeForm();
            addNotification({ type: 'success', message: t('agents.detail.airDevices.updateSuccess') });
          },
          onError: (err) => notifyOpError(err),
        },
      );
    } else {
      addDevice.mutate(
        { station, place, index: indexNum, group_id: groupId },
        {
          onSuccess: (res) => {
            closeForm();
            addNotification({
              type: 'success',
              message: t('agents.detail.airDevices.addSuccess').replace('{name}', res.name || res.device_id),
            });
          },
          onError: (err) => notifyOpError(err),
        },
      );
    }
  }

  function notifyOpError(err: unknown) {
    addNotification({
      type: 'error',
      message: t('agents.detail.airDevices.opError').replace(
        '{message}',
        err instanceof Error ? err.message : t('agents.detail.airDevices.unknownError'),
      ),
    });
  }

  function confirmRemove() {
    if (!removeTarget) return;
    const target = removeTarget;
    removeDevice.mutate(target.device_id, {
      onSuccess: () => {
        setRemoveTarget(null);
        addNotification({ type: 'success', message: t('agents.detail.airDevices.removeSuccess') });
      },
      onError: (err) => {
        setRemoveTarget(null);
        notifyOpError(err);
      },
    });
  }

  const submitting = addDevice.isPending || setDevice.isPending;

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
      {/* 헤더: 개수 + 추가 버튼 */}
      <div className="flex items-center justify-between">
        <span className="text-xs text-(--color-text-muted)">
          {t('agents.detail.airDevices.devicesCount').replace('{count}', String(devices.length))}
        </span>
        <button
          type="button"
          onClick={openAdd}
          className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <Plus className="h-3.5 w-3.5" />
          {t('agents.detail.airDevices.addDevice')}
        </button>
      </div>

      {/* 필터 바: 라인 / 역사 / 검색 + 필터 개수 (기기가 있을 때만) */}
      {devices.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <select
            value={filterLine}
            onChange={(e) => {
              setFilterLine(e.target.value);
              setFilterStation('');
            }}
            aria-label={t('agents.detail.airDevices.line')}
            className="rounded-md border border-(--color-border-strong) px-2 py-1.5 text-xs bg-(--color-bg-surface) text-(--color-text-primary)"
          >
            <option value="">{t('agents.detail.airDevices.filter.lineAll')}</option>
            {distinctLines.map((ln) => (
              <option key={ln} value={ln}>
                {ln}
              </option>
            ))}
          </select>
          <select
            value={filterStation}
            onChange={(e) => setFilterStation(e.target.value)}
            aria-label={t('agents.detail.airDevices.station')}
            className="rounded-md border border-(--color-border-strong) px-2 py-1.5 text-xs bg-(--color-bg-surface) text-(--color-text-primary)"
          >
            <option value="">{t('agents.detail.airDevices.filter.stationAll')}</option>
            {stationFilterOptions.map((s) => (
              <option key={s.station} value={s.station}>
                {s.display_name || s.station}
              </option>
            ))}
          </select>
          <input
            type="text"
            value={filterText}
            onChange={(e) => setFilterText(e.target.value)}
            placeholder={t('agents.detail.airDevices.filter.searchPlaceholder')}
            className="min-w-40 flex-1 rounded-md border border-(--color-border-strong) px-2 py-1.5 text-xs bg-(--color-bg-surface) text-(--color-text-primary)"
          />
          <span className="whitespace-nowrap text-[11px] tabular-nums text-(--color-text-muted)">
            {t('agents.detail.airDevices.filter.count')
              .replace('{shown}', String(filteredDevices.length))
              .replace('{total}', String(devices.length))}
          </span>
        </div>
      )}

      {/* 목록: 이름 → 아이디 → 역사 → 위치 → 인덱스 → 상태 → 액션 */}
      {devices.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-12 text-(--color-text-muted)">
          <HardDrive className="h-8 w-8 opacity-40" aria-hidden="true" />
          <p className="text-sm">{t('agents.detail.airDevices.noDevices')}</p>
        </div>
      ) : filteredDevices.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-12 text-(--color-text-muted)">
          <HardDrive className="h-8 w-8 opacity-40" aria-hidden="true" />
          <p className="text-sm">{t('agents.detail.airDevices.filter.noMatch')}</p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-(--color-border-default) bg-(--color-bg-elevated) text-left text-xs text-(--color-text-muted)">
                <th className="px-3 py-2 font-medium">{t('agents.detail.airDevices.name')}</th>
                <th className="px-3 py-2 font-medium">{t('agents.detail.airDevices.deviceId')}</th>
                <th className="px-3 py-2 font-medium">{t('agents.detail.airDevices.station')}</th>
                <th className="px-3 py-2 font-medium">{t('agents.detail.airDevices.place')}</th>
                <th className="px-3 py-2 text-right font-medium">{t('agents.detail.airDevices.index')}</th>
                <th className="px-3 py-2 font-medium">{t('agents.detail.airDevices.status')}</th>
                <th className="px-3 py-2 text-right font-medium">{t('agents.detail.airDevices.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {filteredDevices.map((d) => (
                <tr
                  key={d.device_id}
                  className="border-b border-(--color-border-default) last:border-0 hover:bg-(--color-bg-elevated)"
                >
                  <td className="px-3 py-2 font-mono text-xs text-(--color-text-primary)">{d.name || '-'}</td>
                  <td className="px-3 py-2 font-mono text-xs text-(--color-text-muted)" title={d.device_id}>
                    {shortId(d.device_id)}
                  </td>
                  <td className="px-3 py-2 text-(--color-text-secondary)">{stationDisplay(d.station)}</td>
                  <td className="px-3 py-2 text-(--color-text-secondary)">{placeDisplay(d.station, d.place)}</td>
                  <td className="px-3 py-2 text-right text-(--color-text-secondary)">{d.index || 0}</td>
                  <td className="px-3 py-2">
                    <span
                      className={cn(
                        'inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium',
                        d.online
                          ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                          : 'bg-gray-100 text-gray-500 dark:bg-gray-700 dark:text-gray-400',
                      )}
                    >
                      {d.online ? t('agents.detail.airDevices.online') : t('agents.detail.airDevices.offline')}
                    </span>
                  </td>
                  <td className="px-3 py-2 text-right">
                    <div className="inline-flex items-center gap-1">
                      <button
                        type="button"
                        onClick={() => openEdit(d)}
                        aria-label={t('agents.detail.airDevices.edit')}
                        className="rounded p-1 text-(--color-text-muted) hover:bg-(--color-bg-surface) hover:text-(--color-text-secondary)"
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>
                      <button
                        type="button"
                        onClick={() => setRemoveTarget(d)}
                        aria-label={t('agents.detail.airDevices.remove')}
                        className="rounded p-1 text-(--color-text-muted) hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* 추가/편집 폼 모달 */}
      {form && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
          onClick={closeForm}
        >
          <div
            className="mx-4 w-full max-w-md rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-(--color-border-default) px-4 py-3">
              <h3 className="text-sm font-semibold text-(--color-text-primary)">
                {editing
                  ? t('agents.detail.airDevices.editDevice')
                  : t('agents.detail.airDevices.addDevice')}
              </h3>
              <button
                type="button"
                onClick={closeForm}
                className="rounded-md p-1 text-(--color-text-muted) hover:bg-(--color-bg-surface)"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <div className="space-y-3 p-4">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelCls}>
                    {t('agents.detail.airDevices.station')} <span className="text-red-500">*</span>
                  </label>
                  <select
                    value={form.station}
                    onChange={(e) => setForm({ ...form, station: e.target.value, place: '' })}
                    className={inputCls}
                  >
                    <option value="">{t('agents.detail.airDevices.selectStation')}</option>
                    {stations.map((s) => (
                      <option key={s.station} value={s.station}>
                        {s.display_name ? `${s.display_name} (${s.station})` : s.station}
                      </option>
                    ))}
                  </select>
                  {stations.length === 0 && (
                    <p className="mt-1 text-xs text-(--color-text-muted)">
                      {t('agents.detail.airDevices.noStationsHint')}
                    </p>
                  )}
                </div>
                <div>
                  <label className={labelCls}>
                    {t('agents.detail.airDevices.place')} <span className="text-red-500">*</span>
                  </label>
                  <select
                    value={form.place}
                    disabled={!form.station}
                    onChange={(e) => setForm({ ...form, place: e.target.value })}
                    className={cn(inputCls, !form.station && 'cursor-not-allowed opacity-60')}
                  >
                    <option value="">{t('agents.detail.airDevices.selectPlace')}</option>
                    {placeOptions.map((p) => (
                      <option key={p.place} value={p.place}>
                        {p.display_name ? `${p.display_name} (${p.place})` : p.place}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelCls}>
                    {t('agents.detail.airDevices.index')} <span className="text-red-500">*</span>
                  </label>
                  <input
                    type="number"
                    value={form.index}
                    placeholder="0"
                    onChange={(e) => setForm({ ...form, index: e.target.value })}
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className={labelCls}>{t('agents.detail.airDevices.groupId')}</label>
                  <input
                    type="text"
                    value={form.group_id}
                    placeholder={t('agents.detail.airDevices.groupIdPlaceholder')}
                    onChange={(e) => setForm({ ...form, group_id: e.target.value })}
                    className={inputCls}
                  />
                </div>
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
                disabled={submitting || !form.station || !form.place || form.index.trim() === ''}
                className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500"
              >
                {submitting
                  ? t('agents.detail.airDevices.saving')
                  : editing
                    ? t('agents.detail.airDevices.save')
                    : t('agents.detail.airDevices.add')}
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
        title={t('agents.detail.airDevices.removeConfirmTitle')}
        message={t('agents.detail.airDevices.removeConfirmMessage').replace(
          '{name}',
          removeTarget?.name || removeTarget?.device_id || '',
        )}
        variant="danger"
        isSubmitting={removeDevice.isPending}
      />
    </div>
  );
}
