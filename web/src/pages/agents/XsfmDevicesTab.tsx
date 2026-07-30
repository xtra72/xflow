// xsfm 디바이스 관리 탭.
//
// SPEC-XSFM-001 Wave 2 (identity 모델 개정). 백엔드가 device_id(UUID)를 생성하고
// name(복합 키 "station:place:index")을 계산한다. 프런트엔드는 station/place/index/group_id
// 만 입력하며, 목록의 station/place CODE 는 역사 레지스트리(useStations)로 표시명을 해석한다.

import { useMemo, useState } from 'react';
import { HardDrive, ListPlus, Pencil, Plus, Trash2, X } from 'lucide-react';

import {
  EMPTY_REQUIRED,
  INVALID_INDEX,
  useAddXsfmDevice,
  useBulkAddDevices,
  useBulkRemoveDevices,
  useXsfmDevices,
  useRemoveXsfmDevice,
  useSetXsfmDevice,
  useStations,
  type AirDevice,
  type AirDeviceCreate,
  type AirDeviceUpdate,
  type AirStation,
  type BulkFailure,
  type BulkResult,
} from '@/hooks/useStation';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import { ConfirmDialog } from '@/components/property/ConfirmDialog';
import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import BulkRegisterPanel from './BulkRegisterPanel';
import TablePagination from './TablePagination';

/** 폼 상태(문자열 위주 — 제출 시 파싱). device_id 는 입력하지 않는다(백엔드 생성).
 *  name 은 선택 — 비우면 백엔드가 station:place:index 로 자동 계산, 입력하면 override(sticky). */
interface DeviceFormState {
  station: string;
  place: string;
  index: string;
  group_id: string;
  name: string;
}

const EMPTY_FORM: DeviceFormState = { station: '', place: '', index: '', group_id: '', name: '' };

const inputCls =
  'block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)';
const labelCls = 'mb-1 block text-xs font-medium text-(--color-text-secondary)';

export default function XsfmDevicesTab({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const { data: devices = [], isLoading } = useXsfmDevices(agentId);
  const { data: stations = [] } = useStations(agentId);

  const addDevice = useAddXsfmDevice(agentId);
  const setDevice = useSetXsfmDevice(agentId);
  const removeDevice = useRemoveXsfmDevice(agentId);
  const bulkAddDevices = useBulkAddDevices(agentId);
  const bulkRemoveDevices = useBulkRemoveDevices(agentId);

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
  // 편집 진입 시점의 기존 name 스냅샷(부분갱신 dirty 판정용). 추가 모드에서는 미사용.
  const [editingName, setEditingName] = useState('');
  const [removeTarget, setRemoveTarget] = useState<AirDevice | null>(null);

  // 다중선택 상태(device_id Set) + 일괄 삭제 확인 다이얼로그 표시 여부.
  // Source="config" 디바이스는 백엔드가 삭제를 거부하므로 선택 대상에서 제외한다(체크박스 disabled).
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [showBulkRemove, setShowBulkRemove] = useState(false);

  // 일괄 등록 패널 상태(역사 탭과 동일 패턴).
  const [showBulk, setShowBulk] = useState(false);
  const [bulkText, setBulkText] = useState('');
  const [bulkFailures, setBulkFailures] = useState<BulkFailure[] | null>(null);

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

  // 정렬 상태(클라이언트 사이드). 기본: 이름 오름차순.
  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  // 페이지네이션 상태(클라이언트 사이드). 기본 페이지 크기 10.
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  function handleSort(field: string) {
    setSort((prev) =>
      prev.field === field
        ? { field, direction: prev.direction === 'asc' ? 'desc' : 'asc' }
        : { field, direction: 'asc' },
    );
    setPage(1);
  }

  // 필터된 목록에 정렬 적용(원본 배열 불변 — 복사 후 정렬).
  // 컬럼별 comparator: 문자열은 localeCompare('ko'), 인덱스는 수치, 상태는 online-우선(asc).
  // 동률은 이름 → device_id 로 결정적 tiebreak(항상 오름차순, 방향과 무관).
  const sortedDevices = useMemo(() => {
    const dir = sort.direction === 'asc' ? 1 : -1;
    const stationName = (d: AirDevice) => stationsByCode.get(d.station)?.display_name || d.station || '';
    const placeName = (d: AirDevice) => {
      const st = stationsByCode.get(d.station);
      return st?.places.find((x) => x.place === d.place)?.display_name || d.place || '';
    };
    const primary = (a: AirDevice, b: AirDevice): number => {
      switch (sort.field) {
        case 'name':
          return (a.name || '').localeCompare(b.name || '', 'ko');
        case 'device_id':
          return a.device_id.localeCompare(b.device_id);
        case 'station':
          return stationName(a).localeCompare(stationName(b), 'ko');
        case 'place':
          return placeName(a).localeCompare(placeName(b), 'ko');
        case 'status':
          // asc: online 먼저(online=true 가 앞). desc 는 dir 로 반전.
          return a.online === b.online ? 0 : a.online ? -1 : 1;
        default:
          return 0;
      }
    };
    return [...filteredDevices].sort((a, b) => {
      const c = primary(a, b);
      if (c !== 0) return c * dir;
      return (a.name || '').localeCompare(b.name || '', 'ko') || a.device_id.localeCompare(b.device_id);
    });
  }, [filteredDevices, sort, stationsByCode]);

  // 페이지네이션: 정렬/필터된 전체에서 현재 페이지 슬라이스만 렌더한다.
  const totalPages = Math.max(1, Math.ceil(sortedDevices.length / pageSize));
  const safePage = Math.min(page, totalPages);
  const startIndex = (safePage - 1) * pageSize;
  const pagedDevices = useMemo(
    () => sortedDevices.slice(startIndex, startIndex + pageSize),
    [sortedDevices, startIndex, pageSize],
  );

  // 현재 페이지에 보이는 행 중 선택 가능한(=config 아님) 디바이스. 전체선택 기준.
  const selectableVisible = useMemo(
    () => pagedDevices.filter((d) => d.source !== 'config'),
    [pagedDevices],
  );
  const allSelected =
    selectableVisible.length > 0 && selectableVisible.every((d) => selected.has(d.device_id));
  const someSelected = selectableVisible.some((d) => selected.has(d.device_id));

  // 전체선택 토글: 이미 전부 선택이면 보이는 선택가능 행을 해제, 아니면 모두 선택.
  function toggleSelectAll() {
    setSelected((prev) => {
      const next = new Set(prev);
      if (allSelected) for (const d of selectableVisible) next.delete(d.device_id);
      else for (const d of selectableVisible) next.add(d.device_id);
      return next;
    });
  }

  function toggleRow(deviceId: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(deviceId)) next.delete(deviceId);
      else next.add(deviceId);
      return next;
    });
  }

  // 일괄 삭제 결과 → 요약 토스트(성공/부분성공). 선택 삭제는 줄 개념이 없어 개수만 보고한다.
  function reportBulkRemove(result: BulkResult) {
    if (result.failed.length === 0) {
      addNotification({
        type: 'success',
        message: t('agents.detail.airDevices.bulkRemove.successToast').replace('{count}', String(result.ok)),
      });
      return;
    }
    addNotification({
      type: 'error',
      message: t('agents.detail.airDevices.bulkRemove.partialToast')
        .replace('{ok}', String(result.ok))
        .replace('{failed}', String(result.failed.length)),
    });
  }

  // 확인 다이얼로그에서 확정 시: 선택된 각 device_id 에 remove_device 를 반복 호출 → 선택 해제 + 갱신.
  function confirmBulkRemove() {
    const ids = Array.from(selected);
    if (ids.length === 0) {
      setShowBulkRemove(false);
      return;
    }
    bulkRemoveDevices.mutate(ids, {
      onSuccess: (result) => {
        setShowBulkRemove(false);
        setSelected(new Set());
        reportBulkRemove(result);
      },
    });
  }

  function openAdd() {
    setEditing(null);
    setEditingName('');
    setForm({ ...EMPTY_FORM });
  }

  function openEdit(d: AirDevice) {
    setEditing(d.device_id);
    setEditingName(d.name ?? '');
    setForm({
      station: d.station ?? '',
      place: d.place ?? '',
      index: d.index ? String(d.index) : '',
      group_id: d.group_id ?? '',
      name: d.name ?? '',
    });
  }

  function closeForm() {
    setForm(null);
    setEditing(null);
    setEditingName('');
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
    const name = form.name.trim();

    if (editing) {
      // 부분갱신 정확성: 기존 name 과 달라졌을 때만 name 을 전송한다. 자동이름 디바이스를
      // 사용자가 손대지 않았으면 name 키를 omit 하여 override 로 잘못 고정되는 것을 막는다.
      // 빈값으로 바꿨으면 override 해제 의도이므로 name:'' 를 전송한다.
      const update: AirDeviceUpdate = { device_id: editing, station, place, index: indexNum, group_id: groupId };
      if (name !== editingName.trim()) update.name = name;
      setDevice.mutate(update, {
        onSuccess: () => {
          closeForm();
          addNotification({ type: 'success', message: t('agents.detail.airDevices.updateSuccess') });
        },
        onError: (err) => notifyOpError(err),
      });
    } else {
      // 추가: name 이 비어있지 않을 때만 전송(비면 omit → 백엔드가 composeName 자동 계산).
      const create: AirDeviceCreate = { station, place, index: indexNum, group_id: groupId };
      if (name) create.name = name;
      addDevice.mutate(create, {
        onSuccess: (res) => {
          closeForm();
          addNotification({
            type: 'success',
            message: t('agents.detail.airDevices.addSuccess').replace('{name}', res.name || res.device_id),
          });
        },
        onError: (err) => notifyOpError(err),
      });
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

  // 일괄 실행 결과 → 요약 토스트. total===0 이면 입력 없음 오류. 전건 성공 시 true.
  function reportBulk(result: BulkResult): boolean {
    if (result.total === 0) {
      addNotification({ type: 'error', message: t('agents.detail.airDevices.bulk.emptyInput') });
      return false;
    }
    if (result.failed.length === 0) {
      addNotification({
        type: 'success',
        message: t('agents.detail.airDevices.bulk.successToast').replace('{count}', String(result.ok)),
      });
      return true;
    }
    addNotification({
      type: 'error',
      message: t('agents.detail.airDevices.bulk.partialToast')
        .replace('{ok}', String(result.ok))
        .replace('{failed}', String(result.failed.length)),
    });
    return false;
  }

  // 실패 행 → 표시 문자열. sentinel(EMPTY_REQUIRED / INVALID_INDEX)은 i18n 사유로 치환.
  function formatDeviceFailure(f: BulkFailure): string {
    let reason = f.reason;
    if (f.reason === EMPTY_REQUIRED) reason = t('agents.detail.airDevices.bulk.emptyRow');
    else if (f.reason === INVALID_INDEX) reason = t('agents.detail.airDevices.bulk.invalidIndex');
    return t('agents.detail.airDevices.bulk.rowError')
      .replace('{line}', String(f.line))
      .replace('{reason}', reason);
  }

  async function submitDeviceBulk() {
    const result = await bulkAddDevices.mutateAsync(bulkText);
    const fullSuccess = reportBulk(result);
    setBulkFailures(result.failed.length > 0 ? result.failed : null);
    if (fullSuccess) setBulkText('');
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
        <div className="flex items-center gap-2">
          {devices.length > 0 && (
            <button
              type="button"
              onClick={() => setShowBulkRemove(true)}
              disabled={selected.size === 0}
              className="inline-flex items-center gap-1 rounded-md border border-red-300 px-3 py-1.5 text-xs font-medium text-red-600 transition-colors hover:bg-red-50 disabled:opacity-40 disabled:hover:bg-transparent dark:border-red-800 dark:text-red-400 dark:hover:bg-red-900/20"
            >
              <Trash2 className="h-3.5 w-3.5" />
              {t('agents.detail.airDevices.bulkRemove.button').replace('{count}', String(selected.size))}
            </button>
          )}
          <button
            type="button"
            onClick={() => setShowBulk((v) => !v)}
            aria-expanded={showBulk}
            className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
          >
            <ListPlus className="h-3.5 w-3.5" />
            {t('agents.detail.airDevices.bulk.toggle')}
          </button>
          <button
            type="button"
            onClick={openAdd}
            className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            <Plus className="h-3.5 w-3.5" />
            {t('agents.detail.airDevices.addDevice')}
          </button>
        </div>
      </div>

      {/* 일괄 등록 패널 */}
      {showBulk && (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-elevated) p-3">
          <h4 className="mb-2 text-xs font-semibold text-(--color-text-secondary)">
            {t('agents.detail.airDevices.bulk.title')}
          </h4>
          <BulkRegisterPanel
            placeholder={t('agents.detail.airDevices.bulk.placeholder')}
            formatHint={t('agents.detail.airDevices.bulk.formatHint')}
            value={bulkText}
            onChange={setBulkText}
            onSubmit={submitDeviceBulk}
            submitting={bulkAddDevices.isPending}
            submitLabel={t('agents.detail.airDevices.bulk.submit')}
            failures={bulkFailures}
            formatFailure={formatDeviceFailure}
          />
        </div>
      )}

      {/* 필터 바: 라인 / 역사 / 검색 + 필터 개수 (기기가 있을 때만) */}
      {devices.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <select
            value={filterLine}
            onChange={(e) => {
              setFilterLine(e.target.value);
              setFilterStation('');
              setPage(1);
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
            onChange={(e) => {
              setFilterStation(e.target.value);
              setPage(1);
            }}
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
            onChange={(e) => {
              setFilterText(e.target.value);
              setPage(1);
            }}
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

      {/* 목록: 이름 → 아이디 → 역사 → 위치 → 상태 → 액션 */}
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
        <>
          {/* 페이지네이션 컨트롤 (정렬/필터 뒤에 슬라이스) */}
          <TablePagination
            page={safePage}
            pageSize={pageSize}
            totalItems={sortedDevices.length}
            onPageChange={setPage}
            onPageSizeChange={(size) => {
              setPageSize(size);
              setPage(1);
            }}
          />
          <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-(--color-border-default) bg-(--color-bg-elevated)">
                <th className="w-8 px-3 py-2">
                  <input
                    type="checkbox"
                    aria-label={t('agents.detail.airDevices.selectAll')}
                    checked={allSelected}
                    ref={(el) => {
                      if (el) el.indeterminate = someSelected && !allSelected;
                    }}
                    onChange={toggleSelectAll}
                    disabled={selectableVisible.length === 0}
                    className="h-3.5 w-3.5 cursor-pointer accent-blue-600 disabled:cursor-not-allowed disabled:opacity-40"
                  />
                </th>
                <SortableHeader label={t('agents.detail.airDevices.name')} field="name" currentSort={sort} onSort={handleSort} className="px-3 py-2" />
                <SortableHeader label={t('agents.detail.airDevices.deviceId')} field="device_id" currentSort={sort} onSort={handleSort} className="px-3 py-2" />
                <SortableHeader label={t('agents.detail.airDevices.station')} field="station" currentSort={sort} onSort={handleSort} className="px-3 py-2" />
                <SortableHeader label={t('agents.detail.airDevices.place')} field="place" currentSort={sort} onSort={handleSort} className="px-3 py-2" />
                <SortableHeader label={t('agents.detail.airDevices.status')} field="status" currentSort={sort} onSort={handleSort} className="px-3 py-2" />
                <th className="px-3 py-2 text-right text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
                  {t('agents.detail.airDevices.actions')}
                </th>
              </tr>
            </thead>
            <tbody>
              {pagedDevices.map((d) => (
                <tr
                  key={d.device_id}
                  className="border-b border-(--color-border-default) last:border-0 hover:bg-(--color-bg-elevated)"
                >
                  <td className="w-8 px-3 py-2">
                    {d.source === 'config' ? (
                      <span title={t('agents.detail.airDevices.configProtected')} className="inline-flex">
                        <input
                          type="checkbox"
                          disabled
                          aria-label={t('agents.detail.airDevices.selectRow')}
                          className="h-3.5 w-3.5 cursor-not-allowed opacity-40"
                        />
                      </span>
                    ) : (
                      <input
                        type="checkbox"
                        aria-label={t('agents.detail.airDevices.selectRow')}
                        checked={selected.has(d.device_id)}
                        onChange={() => toggleRow(d.device_id)}
                        className="h-3.5 w-3.5 cursor-pointer accent-blue-600"
                      />
                    )}
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-(--color-text-primary)">{d.name || '-'}</td>
                  <td className="px-3 py-2 font-mono text-xs break-all text-(--color-text-muted)">
                    {d.device_id}
                  </td>
                  <td className="px-3 py-2 text-(--color-text-secondary)">{stationDisplay(d.station)}</td>
                  <td className="px-3 py-2 text-(--color-text-secondary)">{placeDisplay(d.station, d.place)}</td>
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
        </>
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
              <div>
                <label className={labelCls}>{t('agents.detail.airDevices.name')}</label>
                <input
                  type="text"
                  value={form.name}
                  placeholder={t('agents.detail.airDevices.namePlaceholder')}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  className={inputCls}
                />
                <p className="mt-1 text-xs text-(--color-text-muted)">
                  {t('agents.detail.airDevices.nameHint')}
                </p>
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

      {/* 일괄 삭제 확인 */}
      <ConfirmDialog
        isOpen={showBulkRemove}
        onClose={() => setShowBulkRemove(false)}
        onConfirm={confirmBulkRemove}
        title={t('agents.detail.airDevices.bulkRemove.confirmTitle')}
        message={t('agents.detail.airDevices.bulkRemove.confirmMessage').replace(
          '{count}',
          String(selected.size),
        )}
        variant="danger"
        isSubmitting={bulkRemoveDevices.isPending}
      />
    </div>
  );
}
