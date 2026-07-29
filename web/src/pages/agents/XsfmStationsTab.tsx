// xsfm 역사(station) + 위치(place) 관리 탭.
//
// SPEC-XSFM-001 Wave 2. xsfm 에이전트에서만 노출된다.
// 역사는 add_station/remove_station 으로, 위치는 각 역사 행 내부에서
// add_place/remove_place 로 관리한다("위치를 역사내에 등록" 요구사항).

import { useState } from 'react';
import { ChevronDown, ChevronRight, ListPlus, MapPin, Pencil, Plus, Trash2, X } from 'lucide-react';

import {
  EMPTY_REQUIRED,
  useAddPlace,
  useAddStation,
  useBulkAddPlaces,
  useBulkAddPlacesTop,
  useBulkAddStations,
  useRemovePlace,
  useRemoveStation,
  useStations,
  type AirStation,
  type BulkFailure,
  type BulkResult,
} from '@/hooks/useStation';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import { ConfirmDialog } from '@/components/property/ConfirmDialog';

const inputCls =
  'block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)';
const labelCls = 'mb-1 block text-xs font-medium text-(--color-text-secondary)';
const textareaCls =
  'block w-full rounded-md border border-(--color-border-strong) px-3 py-2 font-mono text-xs bg-(--color-bg-surface) text-(--color-text-primary)';

/**
 * 일괄 등록 패널 (역사/위치 공용). textarea + 제출 + 파싱 도움말 + 실패 목록.
 * best-effort 실행 결과의 실패 행은 formatFailure 로 변환해 인라인 표시한다.
 */
function BulkRegisterPanel({
  placeholder,
  formatHint,
  value,
  onChange,
  onSubmit,
  submitting,
  submitLabel,
  failures,
  formatFailure,
}: {
  placeholder: string;
  formatHint: string;
  value: string;
  onChange: (v: string) => void;
  onSubmit: () => void;
  submitting: boolean;
  submitLabel: string;
  failures: BulkFailure[] | null;
  formatFailure: (f: BulkFailure) => string;
}) {
  return (
    <div className="space-y-2">
      <textarea
        rows={5}
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        className={textareaCls}
      />
      <p className="text-[11px] text-(--color-text-muted)">{formatHint}</p>
      {failures && failures.length > 0 && (
        <ul className="space-y-0.5 rounded-md border border-red-200 bg-red-50 p-2 dark:border-red-900/40 dark:bg-red-900/20">
          {failures.map((f) => (
            <li key={f.line} className="text-[11px] text-red-600 dark:text-red-400">
              {formatFailure(f)}
            </li>
          ))}
        </ul>
      )}
      <div className="flex justify-end">
        <button
          type="button"
          onClick={onSubmit}
          disabled={submitting || value.trim() === ''}
          className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <ListPlus className="h-3.5 w-3.5" />
          {submitLabel}
        </button>
      </div>
    </div>
  );
}

interface StationFormState {
  station: string;
  line: string;
  display_name: string;
  order: string;
}
const EMPTY_STATION_FORM: StationFormState = { station: '', line: '', display_name: '', order: '' };

interface PlaceFormState {
  place: string;
  display_name: string;
  order: string;
}
const EMPTY_PLACE_FORM: PlaceFormState = { place: '', display_name: '', order: '' };

export default function XsfmStationsTab({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const { data: stations = [], isLoading } = useStations(agentId);
  const addStation = useAddStation(agentId);
  const removeStation = useRemoveStation(agentId);
  const addPlace = useAddPlace(agentId);
  const removePlace = useRemovePlace(agentId);
  const bulkAddStations = useBulkAddStations(agentId);
  const bulkAddPlaces = useBulkAddPlaces(agentId);
  const bulkAddPlacesTop = useBulkAddPlacesTop(agentId);

  // 역사 폼: null=닫힘, editing=편집 대상 station code(수정 시 코드 잠금).
  const [stationForm, setStationForm] = useState<StationFormState | null>(null);
  const [editingStation, setEditingStation] = useState<string | null>(null);
  // 위치 폼: 어느 역사에 추가할지 station code 를 키로 보관.
  const [placeForm, setPlaceForm] = useState<{ station: string; form: PlaceFormState } | null>(null);
  // 확장된 역사 행.
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  // 삭제 확인 대상.
  const [removeStationTarget, setRemoveStationTarget] = useState<AirStation | null>(null);
  const [removePlaceTarget, setRemovePlaceTarget] = useState<{ station: string; place: string; label: string } | null>(
    null,
  );
  // 일괄 등록(역사) 패널 상태.
  const [showStationBulk, setShowStationBulk] = useState(false);
  const [stationBulkText, setStationBulkText] = useState('');
  const [stationBulkFailures, setStationBulkFailures] = useState<BulkFailure[] | null>(null);
  // 일괄 등록(위치 최상위) 패널 상태 — 행에 station 포함, 아무 역사나 등록.
  const [showPlaceTopBulk, setShowPlaceTopBulk] = useState(false);
  const [placeTopBulkText, setPlaceTopBulkText] = useState('');
  const [placeTopBulkFailures, setPlaceTopBulkFailures] = useState<BulkFailure[] | null>(null);
  // 일괄 등록(위치) 패널 — 한 번에 한 역사만 열림.
  const [placeBulkStation, setPlaceBulkStation] = useState<string | null>(null);
  const [placeBulkText, setPlaceBulkText] = useState('');
  const [placeBulkFailures, setPlaceBulkFailures] = useState<BulkFailure[] | null>(null);

  function toggleExpand(station: string) {
    setExpanded((prev) => ({ ...prev, [station]: !prev[station] }));
  }

  // 일괄 실행 결과 → 요약 토스트. total===0 이면 입력 없음 오류.
  function reportBulk(result: BulkResult): boolean {
    if (result.total === 0) {
      addNotification({ type: 'error', message: t('agents.detail.stations.bulk.emptyInput') });
      return false;
    }
    if (result.failed.length === 0) {
      addNotification({
        type: 'success',
        message: t('agents.detail.stations.bulk.successToast').replace('{count}', String(result.ok)),
      });
      return true;
    }
    addNotification({
      type: 'error',
      message: t('agents.detail.stations.bulk.partialToast')
        .replace('{ok}', String(result.ok))
        .replace('{failed}', String(result.failed.length)),
    });
    return false;
  }

  // 실패 행 → 표시 문자열. EMPTY_REQUIRED sentinel 은 i18n 사유로 치환.
  function formatStationFailure(f: BulkFailure): string {
    const reason = f.reason === EMPTY_REQUIRED ? t('agents.detail.stations.bulk.emptyStation') : f.reason;
    return t('agents.detail.stations.bulk.rowError')
      .replace('{line}', String(f.line))
      .replace('{reason}', reason);
  }
  function formatPlaceFailure(f: BulkFailure): string {
    const reason = f.reason === EMPTY_REQUIRED ? t('agents.detail.stations.places.bulk.emptyPlace') : f.reason;
    return t('agents.detail.stations.bulk.rowError')
      .replace('{line}', String(f.line))
      .replace('{reason}', reason);
  }
  function formatPlaceTopFailure(f: BulkFailure): string {
    const reason = f.reason === EMPTY_REQUIRED ? t('agents.detail.stations.placesBulkTop.emptyRow') : f.reason;
    return t('agents.detail.stations.bulk.rowError')
      .replace('{line}', String(f.line))
      .replace('{reason}', reason);
  }

  async function submitStationBulk() {
    const result = await bulkAddStations.mutateAsync(stationBulkText);
    const fullSuccess = reportBulk(result);
    setStationBulkFailures(result.failed.length > 0 ? result.failed : null);
    if (fullSuccess) setStationBulkText('');
  }

  async function submitPlaceBulk(station: string) {
    const result = await bulkAddPlaces.mutateAsync({ station, text: placeBulkText });
    const fullSuccess = reportBulk(result);
    setPlaceBulkFailures(result.failed.length > 0 ? result.failed : null);
    if (fullSuccess) setPlaceBulkText('');
  }

  async function submitPlaceTopBulk() {
    const result = await bulkAddPlacesTop.mutateAsync(placeTopBulkText);
    const fullSuccess = reportBulk(result);
    setPlaceTopBulkFailures(result.failed.length > 0 ? result.failed : null);
    if (fullSuccess) setPlaceTopBulkText('');
  }

  function togglePlaceBulk(station: string) {
    setPlaceBulkFailures(null);
    setPlaceBulkText('');
    setPlaceBulkStation((prev) => (prev === station ? null : station));
  }

  // ---- 역사 CRUD ----

  function openAddStation() {
    setEditingStation(null);
    setStationForm({ ...EMPTY_STATION_FORM });
  }
  function openEditStation(s: AirStation) {
    setEditingStation(s.station);
    setStationForm({
      station: s.station,
      line: s.line ?? '',
      display_name: s.display_name ?? '',
      order: s.order ? String(s.order) : '',
    });
  }
  function closeStationForm() {
    setStationForm(null);
    setEditingStation(null);
  }

  function submitStation() {
    if (!stationForm) return;
    const station = stationForm.station.trim();
    if (!station) {
      addNotification({ type: 'error', message: t('agents.detail.stations.stationRequired') });
      return;
    }
    let orderNum = 0;
    if (stationForm.order.trim()) {
      orderNum = parseInt(stationForm.order.trim(), 10);
      if (Number.isNaN(orderNum)) {
        addNotification({ type: 'error', message: t('agents.detail.stations.orderError') });
        return;
      }
    }
    addStation.mutate(
      { station, line: stationForm.line.trim(), display_name: stationForm.display_name.trim(), order: orderNum },
      {
        onSuccess: () => {
          closeStationForm();
          addNotification({ type: 'success', message: t('agents.detail.stations.addSuccess') });
        },
        onError: (err) => notifyError(err),
      },
    );
  }

  function confirmRemoveStation() {
    if (!removeStationTarget) return;
    const target = removeStationTarget;
    removeStation.mutate(target.station, {
      onSuccess: () => {
        setRemoveStationTarget(null);
        addNotification({ type: 'success', message: t('agents.detail.stations.removeSuccess') });
      },
      onError: (err) => {
        setRemoveStationTarget(null);
        notifyError(err);
      },
    });
  }

  // ---- 위치 CRUD ----

  function openAddPlace(station: string) {
    setPlaceForm({ station, form: { ...EMPTY_PLACE_FORM } });
  }
  function closePlaceForm() {
    setPlaceForm(null);
  }

  function submitPlace() {
    if (!placeForm) return;
    const place = placeForm.form.place.trim();
    if (!place) {
      addNotification({ type: 'error', message: t('agents.detail.stations.places.placeRequired') });
      return;
    }
    let orderNum = 0;
    if (placeForm.form.order.trim()) {
      orderNum = parseInt(placeForm.form.order.trim(), 10);
      if (Number.isNaN(orderNum)) {
        addNotification({ type: 'error', message: t('agents.detail.stations.orderError') });
        return;
      }
    }
    addPlace.mutate(
      {
        station: placeForm.station,
        place,
        display_name: placeForm.form.display_name.trim(),
        order: orderNum,
      },
      {
        onSuccess: () => {
          closePlaceForm();
          addNotification({ type: 'success', message: t('agents.detail.stations.places.addSuccess') });
        },
        onError: (err) => notifyError(err),
      },
    );
  }

  function confirmRemovePlace() {
    if (!removePlaceTarget) return;
    const target = removePlaceTarget;
    removePlace.mutate(
      { station: target.station, place: target.place },
      {
        onSuccess: () => {
          setRemovePlaceTarget(null);
          addNotification({ type: 'success', message: t('agents.detail.stations.places.removeSuccess') });
        },
        onError: (err) => {
          setRemovePlaceTarget(null);
          notifyError(err);
        },
      },
    );
  }

  function notifyError(err: unknown) {
    addNotification({
      type: 'error',
      message: t('agents.detail.stations.opError').replace(
        '{message}',
        err instanceof Error ? err.message : t('agents.detail.stations.unknownError'),
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

  const stationSubmitting = addStation.isPending;

  return (
    <div className="space-y-3 p-4">
      {/* 헤더 */}
      <div className="flex items-center justify-between">
        <span className="text-xs text-(--color-text-muted)">
          {t('agents.detail.stations.stationsCount').replace('{count}', String(stations.length))}
        </span>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setShowStationBulk((v) => !v)}
            aria-expanded={showStationBulk}
            className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
          >
            <ListPlus className="h-3.5 w-3.5" />
            {t('agents.detail.stations.bulk.toggle')}
          </button>
          <button
            type="button"
            onClick={() => setShowPlaceTopBulk((v) => !v)}
            aria-expanded={showPlaceTopBulk}
            className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
          >
            <ListPlus className="h-3.5 w-3.5" />
            {t('agents.detail.stations.placesBulkTop.toggle')}
          </button>
          <button
            type="button"
            onClick={openAddStation}
            className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            <Plus className="h-3.5 w-3.5" />
            {t('agents.detail.stations.addStation')}
          </button>
        </div>
      </div>

      {/* 역사 일괄 등록 패널 */}
      {showStationBulk && (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-elevated) p-3">
          <h4 className="mb-2 text-xs font-semibold text-(--color-text-secondary)">
            {t('agents.detail.stations.bulk.title')}
          </h4>
          <BulkRegisterPanel
            placeholder={t('agents.detail.stations.bulk.placeholder')}
            formatHint={t('agents.detail.stations.bulk.formatHint')}
            value={stationBulkText}
            onChange={setStationBulkText}
            onSubmit={submitStationBulk}
            submitting={bulkAddStations.isPending}
            submitLabel={t('agents.detail.stations.bulk.submit')}
            failures={stationBulkFailures}
            formatFailure={formatStationFailure}
          />
        </div>
      )}

      {/* 위치 일괄 등록(최상위) 패널 — 행에 station 포함 */}
      {showPlaceTopBulk && (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-elevated) p-3">
          <h4 className="mb-2 text-xs font-semibold text-(--color-text-secondary)">
            {t('agents.detail.stations.placesBulkTop.title')}
          </h4>
          <BulkRegisterPanel
            placeholder={t('agents.detail.stations.placesBulkTop.placeholder')}
            formatHint={t('agents.detail.stations.placesBulkTop.formatHint')}
            value={placeTopBulkText}
            onChange={setPlaceTopBulkText}
            onSubmit={submitPlaceTopBulk}
            submitting={bulkAddPlacesTop.isPending}
            submitLabel={t('agents.detail.stations.placesBulkTop.submit')}
            failures={placeTopBulkFailures}
            formatFailure={formatPlaceTopFailure}
          />
        </div>
      )}

      {/* 역사 목록 */}
      {stations.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-12 text-(--color-text-muted)">
          <MapPin className="h-8 w-8 opacity-40" aria-hidden="true" />
          <p className="text-sm">{t('agents.detail.stations.noStations')}</p>
        </div>
      ) : (
        <div className="space-y-1.5">
          {/* 컬럼 헤더: 순서 / 이름 / 코드 / 라인 / 위치 / 액션 (역사 행과 동일 grid 트랙으로 정렬) */}
          <div className="grid grid-cols-[1.25rem_3rem_1fr_9rem_6rem_4rem_auto] items-center gap-2 px-3 pb-1 text-[10px] font-medium uppercase tracking-wide text-(--color-text-muted)">
            <span aria-hidden="true" />
            <span className="text-center">{t('agents.detail.stations.order')}</span>
            <span>{t('agents.detail.stations.displayName')}</span>
            <span>{t('agents.detail.stations.station')}</span>
            <span>{t('agents.detail.stations.line')}</span>
            <span className="text-center">{t('agents.detail.stations.placesColumn')}</span>
            <span className="text-right">{t('agents.detail.stations.actions')}</span>
          </div>
          {stations.map((s) => {
            const isOpen = expanded[s.station] ?? false;
            return (
              <div key={s.station} className="rounded-lg border border-(--color-border-default)">
                {/* 역사 행 헤더: 순서 → 이름 → 코드 → 라인 → 위치수 정렬 컬럼 (헤더 행과 동일 grid 트랙) */}
                <div className="grid grid-cols-[1.25rem_3rem_1fr_9rem_6rem_4rem_auto] items-center gap-2 px-3 py-2">
                  {/* 클릭 시 확장/접기. 앞 6개 컬럼(chevron~위치수)을 subgrid 로 span 하여 헤더와 정렬. */}
                  <button
                    type="button"
                    onClick={() => toggleExpand(s.station)}
                    aria-expanded={isOpen}
                    className="col-span-6 grid grid-cols-subgrid items-center gap-2 text-left"
                  >
                    <span className="flex items-center">
                      {isOpen ? (
                        <ChevronDown className="h-4 w-4 text-(--color-text-muted)" />
                      ) : (
                        <ChevronRight className="h-4 w-4 text-(--color-text-muted)" />
                      )}
                    </span>
                    <span className="text-center text-xs tabular-nums text-(--color-text-secondary)">{s.order}</span>
                    <span className="truncate text-sm font-medium text-(--color-text-primary)">
                      {s.display_name || s.station}
                    </span>
                    <span className="truncate font-mono text-xs text-(--color-text-muted)">{s.station}</span>
                    <span className="truncate text-xs text-(--color-text-secondary)">{s.line || '—'}</span>
                    <span className="text-center text-[10px] text-(--color-text-muted)">{s.places.length}</span>
                  </button>
                  {/* 편집/삭제 (토글 버튼 밖의 형제 요소, 액션 컬럼) */}
                  <div className="flex items-center justify-end gap-1">
                    <button
                      type="button"
                      onClick={() => openEditStation(s)}
                      aria-label={t('agents.detail.stations.edit')}
                      className="rounded p-1 text-(--color-text-muted) hover:bg-(--color-bg-surface) hover:text-(--color-text-secondary)"
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </button>
                    <button
                      type="button"
                      onClick={() => setRemoveStationTarget(s)}
                      aria-label={t('agents.detail.stations.remove')}
                      className="rounded p-1 text-(--color-text-muted) hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>

                {/* 위치 목록 (확장 시) */}
                {isOpen && (
                  <div className="border-t border-(--color-border-default) px-3 py-2">
                    <div className="mb-2 flex items-center justify-between">
                      <span className="text-xs font-medium text-(--color-text-muted)">
                        {t('agents.detail.stations.places.title')}
                      </span>
                      <div className="flex items-center gap-1.5">
                        <button
                          type="button"
                          onClick={() => togglePlaceBulk(s.station)}
                          aria-expanded={placeBulkStation === s.station}
                          className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-2 py-1 text-[11px] font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
                        >
                          <ListPlus className="h-3 w-3" />
                          {t('agents.detail.stations.places.bulk.toggle')}
                        </button>
                        <button
                          type="button"
                          onClick={() => openAddPlace(s.station)}
                          className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-2 py-1 text-[11px] font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
                        >
                          <Plus className="h-3 w-3" />
                          {t('agents.detail.stations.places.addPlace')}
                        </button>
                      </div>
                    </div>
                    {placeBulkStation === s.station && (
                      <div className="mb-2 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-2">
                        <h5 className="mb-2 text-[11px] font-semibold text-(--color-text-secondary)">
                          {t('agents.detail.stations.places.bulk.title')}
                        </h5>
                        <BulkRegisterPanel
                          placeholder={t('agents.detail.stations.places.bulk.placeholder')}
                          formatHint={t('agents.detail.stations.places.bulk.formatHint')}
                          value={placeBulkText}
                          onChange={setPlaceBulkText}
                          onSubmit={() => submitPlaceBulk(s.station)}
                          submitting={bulkAddPlaces.isPending}
                          submitLabel={t('agents.detail.stations.places.bulk.submit')}
                          failures={placeBulkFailures}
                          formatFailure={formatPlaceFailure}
                        />
                      </div>
                    )}
                    {s.places.length === 0 ? (
                      <p className="py-2 text-center text-xs text-(--color-text-muted)">
                        {t('agents.detail.stations.places.noPlaces')}
                      </p>
                    ) : (
                      <ul className="space-y-1">
                        {s.places.map((p) => (
                          <li
                            key={p.place}
                            className="flex items-center gap-2 rounded-md bg-(--color-bg-elevated) px-2 py-1.5"
                          >
                            <MapPin className="h-3 w-3 text-(--color-text-muted)" />
                            <span className="text-xs text-(--color-text-primary)">
                              {p.display_name || p.place}
                            </span>
                            <span className="font-mono text-[10px] text-(--color-text-muted)">{p.place}</span>
                            <span className="ml-auto text-[10px] text-(--color-text-muted)">#{p.order}</span>
                            <button
                              type="button"
                              onClick={() =>
                                setRemovePlaceTarget({
                                  station: s.station,
                                  place: p.place,
                                  label: p.display_name || p.place,
                                })
                              }
                              aria-label={t('agents.detail.stations.places.remove')}
                              className="rounded p-0.5 text-(--color-text-muted) hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20"
                            >
                              <Trash2 className="h-3 w-3" />
                            </button>
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {/* 역사 추가/편집 폼 모달 */}
      {stationForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={closeStationForm}>
          <div
            className="mx-4 w-full max-w-md rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-(--color-border-default) px-4 py-3">
              <h3 className="text-sm font-semibold text-(--color-text-primary)">
                {editingStation
                  ? t('agents.detail.stations.editStation')
                  : t('agents.detail.stations.addStation')}
              </h3>
              <button
                type="button"
                onClick={closeStationForm}
                className="rounded-md p-1 text-(--color-text-muted) hover:bg-(--color-bg-surface)"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <div className="space-y-3 p-4">
              <div>
                <label className={labelCls}>
                  {t('agents.detail.stations.station')} <span className="text-red-500">*</span>
                </label>
                <input
                  type="text"
                  value={stationForm.station}
                  disabled={!!editingStation}
                  placeholder={t('agents.detail.stations.stationPlaceholder')}
                  onChange={(e) => setStationForm({ ...stationForm, station: e.target.value })}
                  className={cn(inputCls, editingStation && 'cursor-not-allowed opacity-60')}
                />
              </div>
              <div>
                <label className={labelCls}>{t('agents.detail.stations.line')}</label>
                <input
                  type="text"
                  value={stationForm.line}
                  placeholder={t('agents.detail.stations.linePlaceholder')}
                  onChange={(e) => setStationForm({ ...stationForm, line: e.target.value })}
                  className={inputCls}
                />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelCls}>{t('agents.detail.stations.displayName')}</label>
                  <input
                    type="text"
                    value={stationForm.display_name}
                    placeholder={t('agents.detail.stations.displayNamePlaceholder')}
                    onChange={(e) => setStationForm({ ...stationForm, display_name: e.target.value })}
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className={labelCls}>{t('agents.detail.stations.order')}</label>
                  <input
                    type="number"
                    value={stationForm.order}
                    placeholder="0"
                    onChange={(e) => setStationForm({ ...stationForm, order: e.target.value })}
                    className={inputCls}
                  />
                </div>
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-4 py-3">
              <button
                type="button"
                onClick={closeStationForm}
                className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) hover:bg-(--color-bg-surface)"
              >
                {t('common.cancel')}
              </button>
              <button
                type="button"
                onClick={submitStation}
                disabled={stationSubmitting || !stationForm.station.trim()}
                className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500"
              >
                {stationSubmitting ? t('agents.detail.stations.saving') : t('agents.detail.stations.save')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 위치 추가 폼 모달 */}
      {placeForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={closePlaceForm}>
          <div
            className="mx-4 w-full max-w-md rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-(--color-border-default) px-4 py-3">
              <h3 className="text-sm font-semibold text-(--color-text-primary)">
                {t('agents.detail.stations.places.addPlace')}
                <span className="ml-2 font-mono text-xs text-(--color-text-muted)">{placeForm.station}</span>
              </h3>
              <button
                type="button"
                onClick={closePlaceForm}
                className="rounded-md p-1 text-(--color-text-muted) hover:bg-(--color-bg-surface)"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <div className="space-y-3 p-4">
              <div>
                <label className={labelCls}>
                  {t('agents.detail.stations.places.place')} <span className="text-red-500">*</span>
                </label>
                <input
                  type="text"
                  value={placeForm.form.place}
                  placeholder={t('agents.detail.stations.places.placePlaceholder')}
                  onChange={(e) => setPlaceForm({ ...placeForm, form: { ...placeForm.form, place: e.target.value } })}
                  className={inputCls}
                />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelCls}>{t('agents.detail.stations.places.displayName')}</label>
                  <input
                    type="text"
                    value={placeForm.form.display_name}
                    placeholder={t('agents.detail.stations.places.displayNamePlaceholder')}
                    onChange={(e) =>
                      setPlaceForm({ ...placeForm, form: { ...placeForm.form, display_name: e.target.value } })
                    }
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className={labelCls}>{t('agents.detail.stations.places.order')}</label>
                  <input
                    type="number"
                    value={placeForm.form.order}
                    placeholder="0"
                    onChange={(e) => setPlaceForm({ ...placeForm, form: { ...placeForm.form, order: e.target.value } })}
                    className={inputCls}
                  />
                </div>
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-4 py-3">
              <button
                type="button"
                onClick={closePlaceForm}
                className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) hover:bg-(--color-bg-surface)"
              >
                {t('common.cancel')}
              </button>
              <button
                type="button"
                onClick={submitPlace}
                disabled={addPlace.isPending || !placeForm.form.place.trim()}
                className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500"
              >
                {addPlace.isPending
                  ? t('agents.detail.stations.places.adding')
                  : t('agents.detail.stations.places.add')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 역사 삭제 확인 */}
      <ConfirmDialog
        isOpen={removeStationTarget !== null}
        onClose={() => setRemoveStationTarget(null)}
        onConfirm={confirmRemoveStation}
        title={t('agents.detail.stations.removeConfirmTitle')}
        message={t('agents.detail.stations.removeConfirmMessage').replace(
          '{name}',
          removeStationTarget?.display_name || removeStationTarget?.station || '',
        )}
        variant="danger"
        isSubmitting={removeStation.isPending}
      />

      {/* 위치 삭제 확인 */}
      <ConfirmDialog
        isOpen={removePlaceTarget !== null}
        onClose={() => setRemovePlaceTarget(null)}
        onConfirm={confirmRemovePlace}
        title={t('agents.detail.stations.places.removeConfirmTitle')}
        message={t('agents.detail.stations.places.removeConfirmMessage').replace(
          '{name}',
          removePlaceTarget?.label || '',
        )}
        variant="danger"
        isSubmitting={removePlace.isPending}
      />
    </div>
  );
}
