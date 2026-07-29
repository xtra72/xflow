// airpurifier 디바이스 관리 탭.
//
// SPEC-AIRPURIFIER-001 Wave 2. samsung/lgap DevicesTab 과 달리 airpurifier 는
// device_id 기반이며 역사(station)→위치(place) 계층 속성을 가진다. 역사/위치는
// list_stations 임베드 목록에서 SELECT 로 고른다(위치는 선택 역사의 places 에 종속).

import { useMemo, useState } from 'react';
import { HardDrive, Pencil, Plus, Trash2, X } from 'lucide-react';

import {
  useAddAirpurifierDevice,
  useAirpurifierDevices,
  useRemoveAirpurifierDevice,
  useSetAirpurifierDevice,
  useStations,
  type AirDevice,
} from '@/hooks/useStation';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import { ConfirmDialog } from '@/components/property/ConfirmDialog';

/** 폼 상태(문자열 위주 — 제출 시 파싱). */
interface DeviceFormState {
  device_id: string;
  name: string;
  station: string;
  place: string;
  index: string;
  group_id: string;
}

const EMPTY_FORM: DeviceFormState = {
  device_id: '',
  name: '',
  station: '',
  place: '',
  index: '',
  group_id: '',
};

const inputCls =
  'block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)';
const labelCls = 'mb-1 block text-xs font-medium text-(--color-text-secondary)';

export default function AirpurifierDevicesTab({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const { data: devices = [], isLoading } = useAirpurifierDevices(agentId);
  const { data: stations = [] } = useStations(agentId);

  const addDevice = useAddAirpurifierDevice(agentId);
  const setDevice = useSetAirpurifierDevice(agentId);
  const removeDevice = useRemoveAirpurifierDevice(agentId);

  // 폼 상태: null 이면 닫힘, editing 이 있으면 편집(기존 device_id), 없으면 추가.
  const [form, setForm] = useState<DeviceFormState | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  const [removeTarget, setRemoveTarget] = useState<AirDevice | null>(null);

  // 선택된 역사의 위치 목록(선택 역사 종속). 역사 미선택 시 빈 배열.
  const placeOptions = useMemo(() => {
    const st = stations.find((s) => s.station === form?.station);
    return st?.places ?? [];
  }, [stations, form?.station]);

  function openAdd() {
    setEditing(null);
    setForm({ ...EMPTY_FORM });
  }

  function openEdit(d: AirDevice) {
    setEditing(d.device_id);
    setForm({
      device_id: d.device_id,
      name: d.name ?? '',
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
    const deviceId = form.device_id.trim();
    if (!deviceId) {
      addNotification({ type: 'error', message: t('agents.detail.airDevices.deviceIdRequired') });
      return;
    }
    // index 파싱(빈 값=0). 숫자가 아니면 오류.
    let indexNum = 0;
    if (form.index.trim()) {
      indexNum = parseInt(form.index.trim(), 10);
      if (Number.isNaN(indexNum)) {
        addNotification({ type: 'error', message: t('agents.detail.airDevices.indexError') });
        return;
      }
    }

    const payload = {
      device_id: deviceId,
      name: form.name.trim(),
      station: form.station.trim(),
      place: form.place.trim(),
      index: indexNum,
      group_id: form.group_id.trim(),
    };

    const mutation = editing ? setDevice : addDevice;
    const successMsg = editing
      ? t('agents.detail.airDevices.updateSuccess')
      : t('agents.detail.airDevices.addSuccess');

    mutation.mutate(payload, {
      onSuccess: () => {
        closeForm();
        addNotification({ type: 'success', message: successMsg });
      },
      onError: (err) => {
        addNotification({
          type: 'error',
          message: t('agents.detail.airDevices.opError').replace(
            '{message}',
            err instanceof Error ? err.message : t('agents.detail.airDevices.unknownError'),
          ),
        });
      },
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
        addNotification({
          type: 'error',
          message: t('agents.detail.airDevices.opError').replace(
            '{message}',
            err instanceof Error ? err.message : t('agents.detail.airDevices.unknownError'),
          ),
        });
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

      {/* 목록 */}
      {devices.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-12 text-(--color-text-muted)">
          <HardDrive className="h-8 w-8 opacity-40" aria-hidden="true" />
          <p className="text-sm">{t('agents.detail.airDevices.noDevices')}</p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-(--color-border-default) bg-(--color-bg-elevated) text-left text-xs text-(--color-text-muted)">
                <th className="px-3 py-2 font-medium">{t('agents.detail.airDevices.deviceId')}</th>
                <th className="px-3 py-2 font-medium">{t('agents.detail.airDevices.name')}</th>
                <th className="px-3 py-2 font-medium">{t('agents.detail.airDevices.station')}</th>
                <th className="px-3 py-2 font-medium">{t('agents.detail.airDevices.place')}</th>
                <th className="px-3 py-2 text-right font-medium">{t('agents.detail.airDevices.index')}</th>
                <th className="px-3 py-2 font-medium">{t('agents.detail.airDevices.groupId')}</th>
                <th className="px-3 py-2 font-medium">{t('agents.detail.airDevices.status')}</th>
                <th className="px-3 py-2 text-right font-medium" />
              </tr>
            </thead>
            <tbody>
              {devices.map((d) => (
                <tr
                  key={d.device_id}
                  className="border-b border-(--color-border-default) last:border-0 hover:bg-(--color-bg-elevated)"
                >
                  <td className="px-3 py-2 font-mono text-xs text-(--color-text-primary)">{d.device_id}</td>
                  <td className="px-3 py-2 text-(--color-text-secondary)">{d.name || '-'}</td>
                  <td className="px-3 py-2 text-(--color-text-secondary)">{d.station || '-'}</td>
                  <td className="px-3 py-2 text-(--color-text-secondary)">{d.place || '-'}</td>
                  <td className="px-3 py-2 text-right text-(--color-text-secondary)">{d.index || 0}</td>
                  <td className="px-3 py-2 text-(--color-text-secondary)">{d.group_id || '-'}</td>
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
              <div>
                <label className={labelCls}>
                  {t('agents.detail.airDevices.deviceId')} <span className="text-red-500">*</span>
                </label>
                <input
                  type="text"
                  value={form.device_id}
                  disabled={!!editing}
                  placeholder={t('agents.detail.airDevices.deviceIdPlaceholder')}
                  onChange={(e) => setForm({ ...form, device_id: e.target.value })}
                  className={cn(inputCls, editing && 'cursor-not-allowed opacity-60')}
                />
              </div>
              <div>
                <label className={labelCls}>{t('agents.detail.airDevices.name')}</label>
                <input
                  type="text"
                  value={form.name}
                  placeholder={t('agents.detail.airDevices.namePlaceholder')}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  className={inputCls}
                />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelCls}>{t('agents.detail.airDevices.station')}</label>
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
                  <label className={labelCls}>{t('agents.detail.airDevices.place')}</label>
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
                  <label className={labelCls}>{t('agents.detail.airDevices.index')}</label>
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
                disabled={submitting || !form.device_id.trim()}
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
