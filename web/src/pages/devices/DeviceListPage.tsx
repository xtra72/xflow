// 디바이스 관리 페이지.
// react-grid-layout 기반 대시보드 스타일 그리드로 디바이스를 표시한다.
// 편집 모드에서 카드를 드래그/리사이즈할 수 있고, 레이아웃은 localStorage에 영속된다.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import GridLayout from 'react-grid-layout';
import { Check, ChevronDown, ChevronRight, HardDrive, Pencil, Plus, RotateCcw, Search, Trash2, X } from 'lucide-react';

import 'react-grid-layout/css/styles.css';
import 'react-resizable/css/styles.css';

import { useAgents, useExecAgent } from '@/hooks/useAgent';
import { useDevice, useDevicesRealtime } from '@/hooks/useDevice';
import { getDeviceTypeLabel } from '@/lib/utils/deviceLabels';
import { cn } from '@/lib/utils/cn';
import {
  useUIStore,
  type DashboardLayoutItem,
} from '@/stores/uiStore';
import type { DeviceInfo, DeviceListParams } from '@/types/device';

import DeviceDetailPanel, { StatePropertiesSection } from './DeviceDetailPanel';
import DeviceStatusBadge from './DeviceStatusBadge';

/** 그리드 설정 */
const GRID_COLS = 12;
const GRID_ROW_HEIGHT = 60;
const GRID_MARGIN: [number, number] = [12, 12];

/** 기본 카드 크기 (12열 중 4칸 = 1/3 너비) */
const DEFAULT_W = 4;
const DEFAULT_H = 4;
const MIN_W = 2;
const MIN_H = 2;

/** 상대 시간 포맷 (예: "3분 전") */
function formatRelativeTime(dateStr: string): string {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  const then = date.getTime();
  if (isNaN(then)) return '-';

  // Go zero time ("0001-01-01T00:00:00Z") 등 유효하지 않은 과거 날짜 처리
  if (date.getUTCFullYear() < 2000) return '-';

  const now = Date.now();
  const diffMs = now - then;
  if (diffMs < 0) return '방금';

  const seconds = Math.floor(diffMs / 1000);
  if (seconds < 60) return `${seconds}초 전`;

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}분 전`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}시간 전`;

  const days = Math.floor(hours / 24);
  return `${days}일 전`;
}

export default function DeviceListPage() {
  // 필터 상태
  const [filters, setFilters] = useState<DeviceListParams>({});
  const [searchQuery, setSearchQuery] = useState('');

  const { data, isLoading, error, refetch } = useDevicesRealtime(filters);

  // 상세 패널 상태 (슬라이드-인)
  const [selectedDeviceId, setSelectedDeviceId] = useState<string | null>(null);

  // 디바이스 추가 다이얼로그
  const [showAddDialog, setShowAddDialog] = useState(false);

  // UI store (디바이스 그리드 레이아웃)
  const deviceGridLayout = useUIStore((s) => s.deviceGridLayout);
  const setDeviceGridLayout = useUIStore((s) => s.setDeviceGridLayout);
  const editMode = useUIStore((s) => s.deviceGridEditMode);
  const setEditMode = useUIStore((s) => s.setDeviceGridEditMode);
  const resetLayout = useUIStore((s) => s.resetDeviceGridLayout);

  // 컨테이너 너비 측정 (react-grid-layout 필수)
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(1200);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width);
      }
    });
    observer.observe(el);
    setContainerWidth(el.clientWidth);
    return () => observer.disconnect();
  }, []);

  const devices: DeviceInfo[] = data?.data ?? [];

  // 클라이언트 측 검색 필터
  const filteredDevices = useMemo(() => {
    if (!searchQuery.trim()) return devices;
    const q = searchQuery.toLowerCase();
    return devices.filter(
      (d) =>
        d.name.toLowerCase().includes(q) ||
        d.id.toLowerCase().includes(q) ||
        d.type.toLowerCase().includes(q) ||
        d.agent_name.toLowerCase().includes(q),
    );
  }, [devices, searchQuery]);

  // 이름 기준 정렬
  const sortedDevices = useMemo(() => {
    return [...filteredDevices].sort((a, b) =>
      (a.name || a.id).localeCompare(b.name || b.id),
    );
  }, [filteredDevices]);

  // 동적 레이아웃 생성: 저장된 레이아웃 + 새 디바이스 자동 배치
  const layout = useMemo(() => {
    return sortedDevices.map((device, idx) => {
      const saved = deviceGridLayout[device.id];
      if (saved) return { ...saved, i: device.id, minW: MIN_W, minH: MIN_H };
      // 새 디바이스: 3열 배치
      const col = idx % 3;
      const row = Math.floor(idx / 3);
      return {
        i: device.id,
        x: col * DEFAULT_W,
        y: row * DEFAULT_H,
        w: DEFAULT_W,
        h: DEFAULT_H,
        minW: MIN_W,
        minH: MIN_H,
      };
    });
  }, [sortedDevices, deviceGridLayout]);

  /** 레이아웃 변경 핸들러 */
  const handleLayoutChange = useCallback(
    (newLayout: DashboardLayoutItem[]) => {
      const map: Record<string, DashboardLayoutItem> = {};
      for (const item of newLayout) {
        map[item.i] = item;
      }
      setDeviceGridLayout(map);
    },
    [setDeviceGridLayout],
  );

  /** 필터 변경 핸들러 */
  const handleFilterChange = (key: keyof DeviceListParams, value: string) => {
    setFilters((prev) => {
      const next = { ...prev };
      if (!value) {
        delete next[key];
      } else if (key === 'online') {
        (next as Record<string, unknown>)[key] = value === 'true';
      } else {
        (next as Record<string, unknown>)[key] = value;
      }
      return next;
    });
  };

  return (
    <div className="space-y-4" ref={containerRef}>
      {/* 헤더 */}
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold text-(--color-text-primary)">디바이스</h2>

        <div className="flex items-center gap-2">
          {/* 디바이스 추가 */}
          <button
            type="button"
            onClick={() => setShowAddDialog(true)}
            className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-2 text-xs font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            <Plus className="h-4 w-4" />
            추가
          </button>
          {/* 편집 모드 토글 */}
          <button
            type="button"
            onClick={() => setEditMode(!editMode)}
            className={cn(
              'rounded-md border p-2 transition-colors',
              editMode
                ? 'border-blue-500 bg-blue-50 text-blue-600 dark:border-blue-400 dark:bg-blue-900/20 dark:text-blue-400'
                : 'border-gray-300 text-gray-600 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-400 dark:hover:bg-gray-700',
            )}
            aria-label={editMode ? '편집 완료' : '레이아웃 편집'}
          >
            {editMode ? <Check className="h-4 w-4" /> : <Pencil className="h-4 w-4" />}
          </button>
        </div>
      </div>

      {/* 편집 모드 설정 바 */}
      {editMode && (
        <div className="flex items-center justify-between rounded-lg border border-blue-200 bg-blue-50 p-3 dark:border-blue-800 dark:bg-blue-900/20">
          <span className="text-xs text-blue-700 dark:text-blue-300">
            카드를 드래그하여 위치를 이동하고, 모서리를 드래그하여 크기를 조절할 수 있습니다.
          </span>
          <button
            type="button"
            onClick={resetLayout}
            className="inline-flex items-center gap-1 rounded-md border border-gray-300 px-2.5 py-1 text-xs text-gray-600 transition-colors hover:bg-white dark:border-gray-600 dark:text-gray-400 dark:hover:bg-gray-700"
          >
            <RotateCcw className="h-3 w-3" />
            초기화
          </button>
        </div>
      )}

      {/* 필터 바 */}
      <div className="flex flex-wrap items-center gap-3">
        {/* 검색 */}
        <div className="relative">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="이름, ID, 타입 검색..."
            className="rounded-md border border-gray-300 py-2 pl-9 pr-3 text-sm text-(--color-text-primary) placeholder-gray-400 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:placeholder-gray-500 dark:focus:border-blue-400"
          />
        </div>

        {/* 프로토콜 필터 */}
        <select
          value={filters.protocol ?? ''}
          onChange={(e) => handleFilterChange('protocol', e.target.value)}
          className="rounded-md border border-gray-300 px-3 py-2 text-sm text-(--color-text-secondary) dark:border-gray-600 dark:bg-gray-700"
        >
          <option value="">전체 프로토콜</option>
          <option value="nasa">NASA</option>
          <option value="modbus">Modbus</option>
        </select>

        {/* 타입 필터 */}
        <select
          value={filters.type ?? ''}
          onChange={(e) => handleFilterChange('type', e.target.value)}
          className="rounded-md border border-gray-300 px-3 py-2 text-sm text-(--color-text-secondary) dark:border-gray-600 dark:bg-gray-700"
        >
          <option value="">전체 타입</option>
          <option value="indoor">실내기</option>
          <option value="outdoor">실외기</option>
          <option value="sensor">센서</option>
          <option value="controller">컨트롤러</option>
          <option value="gateway">게이트웨이</option>
        </select>

        {/* 온라인 필터 */}
        <select
          value={filters.online != null ? String(filters.online) : ''}
          onChange={(e) => handleFilterChange('online', e.target.value)}
          className="rounded-md border border-gray-300 px-3 py-2 text-sm text-(--color-text-secondary) dark:border-gray-600 dark:bg-gray-700"
        >
          <option value="">전체 상태</option>
          <option value="true">온라인</option>
          <option value="false">오프라인</option>
        </select>

        {/* 에이전트 필터 */}
        <input
          type="text"
          value={filters.agent ?? ''}
          onChange={(e) => handleFilterChange('agent', e.target.value)}
          placeholder="에이전트 필터"
          className="rounded-md border border-gray-300 px-3 py-2 text-sm text-(--color-text-secondary) placeholder-gray-400 dark:border-gray-600 dark:bg-gray-700 dark:placeholder-gray-500"
        />
      </div>

      {/* 로딩 스켈레톤 */}
      {isLoading && (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <div
              key={i}
              className="h-56 animate-pulse rounded-lg bg-(--color-bg-elevated)"
            />
          ))}
        </div>
      )}

      {/* 에러 상태 */}
      {error && !isLoading && (
        <div className="rounded-lg border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20">
          <p className="text-sm text-red-600 dark:text-red-400">
            디바이스 목록을 불러오는데 실패했습니다.
          </p>
          <button
            type="button"
            onClick={() => refetch()}
            className="mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700"
          >
            다시 시도
          </button>
        </div>
      )}

      {/* 빈 상태 */}
      {!isLoading && !error && devices.length === 0 && (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-12 text-center">
          <HardDrive className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" />
          <h3 className="mt-4 text-lg font-medium text-(--color-text-primary)">
            등록된 디바이스가 없습니다
          </h3>
          <p className="mt-2 text-sm text-(--color-text-muted)">
            에이전트를 시작하면 디바이스가 자동으로 검색됩니다.
          </p>
        </div>
      )}

      {/* 검색 결과 없음 */}
      {!isLoading && !error && devices.length > 0 && sortedDevices.length === 0 && (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-8 text-center">
          <p className="text-sm text-(--color-text-muted)">
            검색 조건에 맞는 디바이스가 없습니다.
          </p>
        </div>
      )}

      {/* 디바이스 그리드 (react-grid-layout) */}
      {!isLoading && !error && sortedDevices.length > 0 && (
        <GridLayout
          layout={layout}
          width={containerWidth}
          gridConfig={{
            cols: GRID_COLS,
            rowHeight: GRID_ROW_HEIGHT,
            margin: GRID_MARGIN,
            containerPadding: [0, 0],
          }}
          dragConfig={{
            enabled: editMode,
            handle: '.device-drag-handle',
          }}
          resizeConfig={{
            enabled: editMode,
            handles: ['se'],
          }}
          onLayoutChange={(newLayout) =>
            handleLayoutChange(newLayout as DashboardLayoutItem[])
          }
        >
          {sortedDevices.map((device) => (
            <div key={device.id} className="flex flex-col overflow-hidden">
              {editMode && <DragHandle />}
              <DeviceGridCard
                device={device}
                onSelect={() => setSelectedDeviceId(device.id)}
              />
            </div>
          ))}
        </GridLayout>
      )}

      {/* 상세 패널 (슬라이드-인 Sheet) */}
      {selectedDeviceId && (
        <DeviceDetailSheet
          deviceId={selectedDeviceId}
          onClose={() => setSelectedDeviceId(null)}
        />
      )}

      {/* 디바이스 추가 다이얼로그 */}
      {showAddDialog && (
        <AddDeviceDialog onClose={() => setShowAddDialog(false)} />
      )}
    </div>
  );
}

// ---- 디바이스 그리드 카드 컴포넌트 ----

interface DeviceGridCardProps {
  device: DeviceInfo;
  onSelect: () => void;
}

function DeviceGridCard({ device, onSelect }: DeviceGridCardProps) {
  const { data: detail } = useDevice(device.id);
  const properties = detail?.state?.properties;
  const hasProperties = properties && Object.keys(properties).length > 0;

  return (
    <div
      className={cn(
        'flex h-full flex-col overflow-hidden rounded-lg border bg-(--color-bg-surface) transition-all',
        device.online
          ? 'border-green-200 dark:border-green-800/50'
          : 'border-(--color-border-default)',
      )}
    >
      {/* 카드 헤더 */}
      <div
        onClick={onSelect}
        className="flex shrink-0 cursor-pointer items-start justify-between px-4 py-3 hover:bg-gray-50/50 dark:hover:bg-gray-800/50"
      >
        <div className="flex items-center gap-2">
          <span
            className={cn(
              'h-2.5 w-2.5 shrink-0 rounded-full',
              device.online ? 'bg-green-500' : 'bg-gray-400',
            )}
          />
          <span className="text-sm font-semibold text-(--color-text-primary)">
            {device.name || device.id}
          </span>
        </div>
        <DeviceStatusBadge online={device.online} />
      </div>

      {/* 상태 속성 (리모컨 패널) */}
      {hasProperties && (
        <div className="min-h-0 flex-1 overflow-y-auto">
          <StatePropertiesSection
            properties={properties}
            protocol={device.protocol}
            type={device.type}
            compact
            deviceId={device.id}
          />
        </div>
      )}

      {/* 카드 푸터 */}
      <div
        onClick={onSelect}
        className="flex shrink-0 cursor-pointer items-center justify-between border-t border-(--color-border-default) px-4 py-2 hover:bg-gray-50/50 dark:hover:bg-gray-800/50"
      >
        <div className="flex items-center gap-2 text-xs text-(--color-text-muted)">
          <span>{getDeviceTypeLabel(device.type)}</span>
          <span>&middot;</span>
          <span
            className={cn(
              'rounded-full px-1.5 py-0.5 text-xs font-medium',
              device.protocol === 'nasa'
                ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                : 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
            )}
          >
            {device.protocol.toUpperCase()}
          </span>
          <span>&middot;</span>
          <span>{device.agent_name}</span>
        </div>
        <p className="text-xs text-(--color-text-muted)">
          {formatRelativeTime(device.last_seen)}
        </p>
      </div>
    </div>
  );
}

// ---- 상세 패널 Sheet (슬라이드-인) ----

interface DeviceDetailSheetProps {
  deviceId: string;
  onClose: () => void;
}

function DeviceDetailSheet({ deviceId, onClose }: DeviceDetailSheetProps) {
  return (
    <>
      {/* 배경 오버레이 */}
      <div
        className="fixed inset-0 z-40 bg-black/20 backdrop-blur-sm"
        onClick={onClose}
      />
      {/* 슬라이드-인 패널 */}
      <div className="fixed inset-y-0 right-0 z-50 flex w-full max-w-lg flex-col border-l border-(--color-border-default) bg-(--color-bg-surface) shadow-2xl">
        {/* Sheet 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-4 py-3">
          <h3 className="text-sm font-semibold text-(--color-text-primary)">디바이스 상세</h3>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        {/* 상세 내용 */}
        <div className="flex-1 overflow-y-auto">
          <DeviceDetailPanel deviceId={deviceId} />
        </div>
      </div>
    </>
  );
}

// ---- 편집 모드 드래그 핸들 ----

function DragHandle() {
  return (
    <div className="device-drag-handle flex h-5 shrink-0 cursor-grab items-center justify-center rounded-t-lg bg-gray-200/80 active:cursor-grabbing dark:bg-gray-600/80">
      <div className="flex gap-1">
        <span className="h-1 w-1 rounded-full bg-gray-400 dark:bg-gray-500" />
        <span className="h-1 w-1 rounded-full bg-gray-400 dark:bg-gray-500" />
        <span className="h-1 w-1 rounded-full bg-gray-400 dark:bg-gray-500" />
      </div>
    </div>
  );
}

// ---- 디바이스 추가 다이얼로그 ----

/** 레지스터 영역 라벨 */
const MODBUS_AREA_LABELS: Record<string, string> = {
  coils: 'Coils (FC01/05)',
  discrete_inputs: 'Discrete Inputs (FC02)',
  holding_registers: 'Holding Registers (FC03/06)',
  input_registers: 'Input Registers (FC04)',
};
const MODBUS_AREA_ORDER = ['coils', 'discrete_inputs', 'holding_registers', 'input_registers'] as const;

type RegBlock = { start: string; count: string };

const DEFAULT_REG_AREAS: Record<string, RegBlock[]> = {
  holding_registers: [{ start: '0', count: '100' }],
  input_registers: [],
  coils: [],
  discrete_inputs: [],
};

function AddDeviceDialog({ onClose }: { onClose: () => void }) {
  const { data: agentsData } = useAgents();
  const execAgent = useExecAgent();
  const addNotification = useUIStore((s) => s.addNotification);

  // 디바이스 추가 지원 에이전트 필터링 (samsung-nasa + modbus-tcp-server)
  const supportedAgents = useMemo(() => {
    const agents = agentsData?.data ?? [];
    return agents.filter((a) => a.type === 'samsung-nasa' || a.type === 'modbus-tcp-server');
  }, [agentsData]);

  const [selectedAgentId, setSelectedAgentId] = useState('');

  // 선택된 에이전트 타입 결정
  const selectedAgentType = useMemo(() => {
    if (!selectedAgentId) return null;
    return supportedAgents.find((a) => a.id === selectedAgentId)?.type ?? null;
  }, [selectedAgentId, supportedAgents]);

  // --- NASA 폼 상태 ---
  const [nasaAddress, setNasaAddress] = useState('');
  const [nasaDeviceId, setNasaDeviceId] = useState('');
  const [nasaDeviceType, setNasaDeviceType] = useState('');

  // --- Modbus 폼 상태 ---
  const [modbusUnitId, setModbusUnitId] = useState('');
  const [modbusName, setModbusName] = useState('');
  const [modbusRegAreas, setModbusRegAreas] = useState<Record<string, RegBlock[]>>({ ...DEFAULT_REG_AREAS });
  const [expandedAreas, setExpandedAreas] = useState<Record<string, boolean>>({});

  function resetForm() {
    setNasaAddress('');
    setNasaDeviceId('');
    setNasaDeviceType('');
    setModbusUnitId('');
    setModbusName('');
    setModbusRegAreas({
      holding_registers: [{ start: '0', count: '100' }],
      input_registers: [],
      coils: [],
      discrete_inputs: [],
    });
    setExpandedAreas({});
  }

  function handleSubmit() {
    if (!selectedAgentId) return;

    if (selectedAgentType === 'samsung-nasa') {
      if (!nasaAddress.trim()) return;
      execAgent.mutate(
        {
          id: selectedAgentId,
          req: {
            command: 'add_device',
            params: {
              address: nasaAddress.trim(),
              ...(nasaDeviceId.trim() && { device_id: nasaDeviceId.trim() }),
              ...(nasaDeviceType && { device_type: nasaDeviceType }),
            },
          },
        },
        {
          onSuccess: () => {
            addNotification({ type: 'success', message: '디바이스가 추가되었습니다' });
            onClose();
          },
          onError: (err) => {
            addNotification({ type: 'error', message: `디바이스 추가 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
          },
        },
      );
    } else if (selectedAgentType === 'modbus-tcp-server') {
      const unitId = parseInt(modbusUnitId, 10);
      if (isNaN(unitId) || unitId < 1 || unitId > 247) {
        addNotification({ type: 'error', message: '유닛 ID는 1~247 범위여야 합니다' });
        return;
      }

      const params: Record<string, unknown> = { unit_id: unitId };
      if (modbusName.trim()) params.name = modbusName.trim();

      // 레지스터 맵 조립 (다중 블록 지원)
      const regMap: Record<string, unknown> = {};
      for (const [area, blocks] of Object.entries(modbusRegAreas)) {
        if (!blocks || blocks.length === 0) continue;
        const parsed: { start_address: number; count: number }[] = [];
        for (const blk of blocks) {
          const start = parseInt(blk.start, 10);
          const cnt = parseInt(blk.count, 10);
          if (isNaN(start) || isNaN(cnt) || cnt <= 0) {
            addNotification({ type: 'error', message: `${MODBUS_AREA_LABELS[area] ?? area}: 올바른 주소와 개수를 입력하세요` });
            return;
          }
          parsed.push({ start_address: start, count: cnt });
        }
        regMap[area] = parsed.length === 1 ? parsed[0] : parsed;
      }
      if (Object.keys(regMap).length === 0) {
        addNotification({ type: 'error', message: '최소 하나의 레지스터 영역을 활성화하세요' });
        return;
      }
      params.register_map = regMap;

      execAgent.mutate(
        { id: selectedAgentId, req: { command: 'add_device', params } },
        {
          onSuccess: (res) => {
            const result = res as { result?: { success?: boolean; error?: string } };
            if (result?.result?.success === false) {
              addNotification({ type: 'error', message: result.result.error ?? '디바이스 추가 실패' });
            } else {
              addNotification({ type: 'success', message: `Modbus 디바이스 (Unit ${unitId})가 추가되었습니다` });
              onClose();
            }
          },
          onError: (err) => {
            addNotification({ type: 'error', message: `디바이스 추가 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
          },
        },
      );
    }
  }

  // 제출 버튼 활성화 조건
  const canSubmit = (() => {
    if (!selectedAgentId || execAgent.isPending) return false;
    if (selectedAgentType === 'samsung-nasa') return !!nasaAddress.trim();
    if (selectedAgentType === 'modbus-tcp-server') return !!modbusUnitId.trim();
    return false;
  })();

  return (
    <>
      {/* 오버레이 */}
      <div
        className="fixed inset-0 z-40 bg-black/30 backdrop-blur-sm"
        onClick={onClose}
      />
      {/* 다이얼로그 */}
      <div className="fixed inset-x-0 top-1/2 z-50 mx-auto w-full max-w-lg -translate-y-1/2 rounded-xl border border-(--color-border-default) bg-(--color-bg-surface) p-6 shadow-2xl">
        <div className="mb-4 flex items-center justify-between">
          <h3 className="text-lg font-semibold text-(--color-text-primary)">디바이스 추가</h3>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-700"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="max-h-[60vh] space-y-3 overflow-y-auto pr-1">
          {/* 에이전트 선택 */}
          <div>
            <label className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
              에이전트
            </label>
            <select
              value={selectedAgentId}
              onChange={(e) => {
                setSelectedAgentId(e.target.value);
                resetForm();
              }}
              className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
            >
              <option value="">에이전트 선택...</option>
              {supportedAgents.map((a) => {
                const typeLabel = a.type === 'modbus-tcp-server' ? 'Modbus' : 'NASA';
                return (
                  <option key={a.id} value={a.id}>
                    {a.name} [{typeLabel}] ({a.status === 'running' ? '실행 중' : '중지'})
                  </option>
                );
              })}
            </select>
            {supportedAgents.length === 0 && (
              <p className="mt-1 text-xs text-gray-500">디바이스 추가를 지원하는 에이전트가 없습니다</p>
            )}
          </div>

          {/* ===== NASA 폼 ===== */}
          {selectedAgentType === 'samsung-nasa' && (
            <>
              <div>
                <label className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
                  디바이스 주소
                </label>
                <input
                  type="text"
                  placeholder="예: 20 00 03"
                  value={nasaAddress}
                  onChange={(e) => setNasaAddress(e.target.value)}
                  className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                />
              </div>
              <div>
                <label className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
                  디바이스 ID (선택)
                </label>
                <input
                  type="text"
                  placeholder="고유 식별자"
                  value={nasaDeviceId}
                  onChange={(e) => setNasaDeviceId(e.target.value)}
                  className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                />
              </div>
              <div>
                <label className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
                  디바이스 타입
                </label>
                <select
                  value={nasaDeviceType}
                  onChange={(e) => setNasaDeviceType(e.target.value)}
                  className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                >
                  <option value="">자동 감지</option>
                  <option value="indoor">실내기</option>
                  <option value="outdoor">실외기</option>
                </select>
              </div>
              <p className="text-xs text-(--color-text-muted)">
                동적으로 추가된 디바이스는 에이전트 재시작 시 초기화됩니다.
              </p>
            </>
          )}

          {/* ===== Modbus 폼 ===== */}
          {selectedAgentType === 'modbus-tcp-server' && (
            <>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
                    Unit ID (1~247)
                  </label>
                  <input
                    type="number"
                    min={1}
                    max={247}
                    placeholder="1"
                    value={modbusUnitId}
                    onChange={(e) => setModbusUnitId(e.target.value)}
                    className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
                    이름 (선택)
                  </label>
                  <input
                    type="text"
                    placeholder="Device-1"
                    value={modbusName}
                    onChange={(e) => setModbusName(e.target.value)}
                    className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                  />
                </div>
              </div>

              {/* 레지스터 맵 설정 */}
              <div>
                <label className="mb-2 block text-sm font-medium text-(--color-text-secondary)">
                  레지스터 맵
                </label>
                <div className="space-y-2">
                  {MODBUS_AREA_ORDER.map((area) => {
                    const blocks = modbusRegAreas[area] ?? [];
                    const enabled = blocks.length > 0;
                    const expanded = expandedAreas[area] ?? false;

                    return (
                      <div key={area} className="rounded-md border border-(--color-border-default)">
                        {/* 영역 헤더 */}
                        <div className="flex items-center gap-2 px-3 py-2">
                          <input
                            type="checkbox"
                            checked={enabled}
                            onChange={(e) => {
                              const checked = e.target.checked;
                              setModbusRegAreas((prev) => ({
                                ...prev,
                                [area]: checked
                                  ? [{ start: area.startsWith('coil') || area.startsWith('discrete') ? '0' : '0', count: area.startsWith('coil') || area.startsWith('discrete') ? '8' : '100' }]
                                  : [],
                              }));
                              if (checked) setExpandedAreas((prev) => ({ ...prev, [area]: true }));
                            }}
                            className="h-3.5 w-3.5 rounded border-gray-300"
                          />
                          <button
                            type="button"
                            onClick={() => enabled && setExpandedAreas((prev) => ({ ...prev, [area]: !prev[area] }))}
                            className="flex flex-1 items-center gap-1 text-left text-xs font-medium text-(--color-text-secondary)"
                            disabled={!enabled}
                          >
                            {enabled && (expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />)}
                            {MODBUS_AREA_LABELS[area]}
                            {enabled && <span className="ml-auto text-[10px] text-gray-400">{blocks.length}개 블록</span>}
                          </button>
                        </div>

                        {/* 블록 목록 */}
                        {enabled && expanded && (
                          <div className="border-t border-(--color-border-default) px-3 pb-2 pt-1">
                            {blocks.map((blk, idx) => (
                              <div key={idx} className="mt-1 flex items-center gap-2">
                                <span className="w-6 text-right text-[10px] text-gray-400">#{idx + 1}</span>
                                <input
                                  type="number"
                                  min={0}
                                  placeholder="시작"
                                  value={blk.start}
                                  onChange={(e) => {
                                    const v = e.target.value;
                                    setModbusRegAreas((prev) => {
                                      const arr = [...(prev[area] ?? [])];
                                      arr[idx] = { start: v, count: arr[idx]?.count ?? '0' };
                                      return { ...prev, [area]: arr };
                                    });
                                  }}
                                  className="w-20 rounded border border-gray-300 px-2 py-1 text-xs dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                                />
                                <span className="text-[10px] text-gray-400">~</span>
                                <input
                                  type="number"
                                  min={1}
                                  placeholder="개수"
                                  value={blk.count}
                                  onChange={(e) => {
                                    const v = e.target.value;
                                    setModbusRegAreas((prev) => {
                                      const arr = [...(prev[area] ?? [])];
                                      arr[idx] = { start: arr[idx]?.start ?? '0', count: v };
                                      return { ...prev, [area]: arr };
                                    });
                                  }}
                                  className="w-20 rounded border border-gray-300 px-2 py-1 text-xs dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                                />
                                <span className="text-[10px] text-gray-400">개</span>
                                {blocks.length > 1 && (
                                  <button
                                    type="button"
                                    onClick={() => {
                                      setModbusRegAreas((prev) => {
                                        const arr = (prev[area] ?? []).filter((_, i) => i !== idx);
                                        return { ...prev, [area]: arr };
                                      });
                                    }}
                                    className="rounded p-0.5 text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20"
                                  >
                                    <Trash2 className="h-3 w-3" />
                                  </button>
                                )}
                              </div>
                            ))}
                            <button
                              type="button"
                              onClick={() => {
                                const defCount = area.startsWith('coil') || area.startsWith('discrete') ? '8' : '100';
                                const lastBlk = blocks[blocks.length - 1];
                                const nextStart = lastBlk ? String(parseInt(lastBlk.start, 10) + parseInt(lastBlk.count, 10)) : '0';
                                setModbusRegAreas((prev) => ({
                                  ...prev,
                                  [area]: [...(prev[area] ?? []), { start: nextStart, count: defCount }],
                                }));
                              }}
                              className="mt-1 text-[10px] font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400"
                            >
                              + 블록 추가
                            </button>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>

              <p className="text-xs text-(--color-text-muted)">
                동적으로 추가된 디바이스는 에이전트 재시작 시 초기화됩니다.
              </p>
            </>
          )}
        </div>

        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-(--color-text-secondary) hover:bg-gray-50 dark:border-gray-600 dark:hover:bg-gray-700"
          >
            취소
          </button>
          <button
            type="button"
            onClick={handleSubmit}
            disabled={!canSubmit}
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500"
          >
            {execAgent.isPending ? '추가 중...' : '추가'}
          </button>
        </div>
      </div>
    </>
  );
}
