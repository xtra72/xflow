// SPEC-MODBUS-012 M2: MODBUS Gateway 대시보드 패널 공용 데이터 계층.
//
// modbus-gateway 에이전트의 exec 명령(list_devices / get_device_status ...)을
// 폴링으로 조회하는 React Query 훅과 응답 형상 타입, envelope 언랩 헬퍼를 제공한다.
// 6종 패널이 공유하며(관측 전용), 폴링 주기는 대시보드 공통 설정을 따른다.

import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';

import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { execAgent } from '@/services/api/agentService';
import { useUIStore } from '@/stores/uiStore';

// ---- 응답 형상 타입 (백엔드 agent.go processListDevices/processGetDeviceStatus 대응) ----

/** 영역별 정의 레지스터 개수. */
export interface ModbusRegisterCounts {
  coils: number;
  discrete_inputs: number;
  holding_registers: number;
  input_registers: number;
}

/** 서빙측(마스터→게이트웨이) 카운터. backing(게이트웨이→upstream)과는 별개다. */
export interface ModbusDeviceStats {
  read_count: number;
  write_count: number;
  error_count: number;
  last_access?: string;
}

/** list_devices 응답 내 개별 디바이스(SPEC-MODBUS-012 M3: backed/mode 추가). */
export interface ModbusDeviceListItem {
  unit_id: number;
  name: string;
  register_counts: ModbusRegisterCounts;
  status: string;
  /** 실제(upstream) 백킹 여부. true 인 항목만 실제 디바이스 목록에 표시한다(REQ-02-01). */
  backed: boolean;
  /** 백킹 모드. "direct" | "indirect" | "" (비백킹). */
  mode: string;
  stats: ModbusDeviceStats;
}

/** get_device_status.backing 서브객체(게이트웨이→upstream 관측 메트릭, REQ-06-02). */
export interface ModbusBacking {
  mode: string; // "direct" | "indirect"
  connected: boolean;
  request_count: number;
  error_count: number;
  avg_latency_ms: number;
  last_ok: number; // epoch ms, 0 = 없음
}

/** 영역별 스냅샷 맵(bool 영역은 addr→bool, 숫자 영역은 addr→uint16). */
export interface ModbusRegisterMap {
  coils?: Record<string, boolean>;
  discrete_inputs?: Record<string, boolean>;
  holding_registers?: Record<string, number>;
  input_registers?: Record<string, number>;
}

/** get_device_status 응답 전체. */
export interface ModbusDeviceStatus {
  unit_id: number;
  name: string;
  register_counts: ModbusRegisterCounts;
  register_map: ModbusRegisterMap;
  stats: ModbusDeviceStats;
  /** 백킹 디바이스면 서브객체, 순수 slave 면 null. */
  backing: ModbusBacking | null;
}

/** get_status 응답(버스/종합 통계, REQ-06). agent.go processGetStatus 대응. */
export interface ModbusStatus {
  listen_address: string;
  listen_port: number;
  unit_id: number;
  active_connections: number;
  max_connections: number;
  uptime_seconds: number;
}

/** list_clients 응답 내 개별 클라이언트. agent.go processListClients 대응. */
export interface ModbusClient {
  remote_addr: string;
  connected_at: string;
  unit_ids: number[];
  request_count: number;
  last_seen: string;
}

// ---- envelope 언랩 ----

/**
 * exec 응답 envelope 언랩. 값이 최상위 또는 result 하위에 위치할 수 있다
 * (AgentDetailPanel ClientsTab/ModbusDevicesSection 패턴 재사용).
 */
export function unwrapExec(res: unknown): Record<string, unknown> {
  const raw = (res ?? {}) as Record<string, unknown>;
  const inner = raw.result as Record<string, unknown> | undefined;
  return inner && typeof inner === 'object' ? inner : raw;
}

// ---- 패널 게이트(에이전트 바인딩 + 원격 감지) ----

/** 패널 렌더 게이트 상태. */
export interface ModbusGate {
  agentId: string;
  /** 원격 대시보드 타깃 여부(exec 프록시 부재 → graceful degrade). */
  remote: boolean;
  /** config.agentId 설정 여부. */
  bound: boolean;
  /** 폴링 가능 여부(bound && !remote). */
  enabled: boolean;
}

/** config.agentId + 원격 타깃 감지를 조합한 패널 게이트를 계산한다. */
export function useModbusGate(config: Record<string, unknown>): ModbusGate {
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const agentId = (config.agentId as string | undefined) ?? '';
  const bound = agentId.length > 0;
  return { agentId, remote, bound, enabled: bound && !remote };
}

/** 대시보드 공통 폴링 주기(ms). */
function useRefreshMs(): number {
  return useUIStore((s) => s.dashboardRefreshInterval) * 1000;
}

// ---- 폴링 훅 ----

/** list_devices 를 폴링하여 디바이스 목록을 반환한다(REQ-02/03). */
export function useModbusListDevices(
  agentId: string,
  enabled: boolean,
): { devices: ModbusDeviceListItem[]; isLoading: boolean; isError: boolean } {
  const refetchInterval = useRefreshMs();
  const q = useQuery({
    queryKey: ['modbus', agentId, 'list_devices'],
    queryFn: () => execAgent(agentId, { command: 'list_devices' }),
    enabled: enabled && agentId.length > 0,
    refetchInterval,
  });
  const devices = (unwrapExec(q.data).devices as ModbusDeviceListItem[] | undefined) ?? [];
  return { devices, isLoading: q.isLoading, isError: q.isError };
}

/** get_device_status(unit_id) 를 폴링하여 디바이스 상세(레지스터 맵 + backing)를 반환한다. */
export function useModbusDeviceStatus(
  agentId: string,
  unitId: number,
  enabled: boolean,
): { status: ModbusDeviceStatus | undefined; isLoading: boolean; isError: boolean } {
  const refetchInterval = useRefreshMs();
  const q = useQuery({
    queryKey: ['modbus', agentId, 'get_device_status', unitId],
    queryFn: () =>
      execAgent(agentId, { command: 'get_device_status', params: { unit_id: unitId } }),
    enabled: enabled && agentId.length > 0,
    refetchInterval,
  });
  const raw = unwrapExec(q.data) as unknown as ModbusDeviceStatus;
  const valid = raw && typeof raw.unit_id === 'number';
  return { status: valid ? raw : undefined, isLoading: q.isLoading, isError: q.isError };
}

/** get_map(unit_id) 을 폴링하여 레지스터 맵 스냅샷을 반환한다(REQ-04, 공유 컨테이너=unit 0). */
export function useModbusRegisterMap(
  agentId: string,
  unitId: number,
  enabled: boolean,
): {
  registerMap: ModbusRegisterMap | undefined;
  isLoading: boolean;
  isError: boolean;
} {
  const refetchInterval = useRefreshMs();
  const q = useQuery({
    queryKey: ['modbus', agentId, 'get_map', unitId],
    queryFn: () => execAgent(agentId, { command: 'get_map', params: { unit_id: unitId } }),
    enabled: enabled && agentId.length > 0,
    refetchInterval,
  });
  const registerMap = unwrapExec(q.data).register_map as ModbusRegisterMap | undefined;
  return { registerMap, isLoading: q.isLoading, isError: q.isError };
}

/** get_status 를 폴링하여 종합/버스 상태를 반환한다(REQ-06). */
export function useModbusStatus(
  agentId: string,
  enabled: boolean,
): { status: ModbusStatus | undefined; isLoading: boolean; isError: boolean } {
  const refetchInterval = useRefreshMs();
  const q = useQuery({
    queryKey: ['modbus', agentId, 'get_status'],
    queryFn: () => execAgent(agentId, { command: 'get_status' }),
    enabled: enabled && agentId.length > 0,
    refetchInterval,
  });
  const raw = unwrapExec(q.data) as unknown as ModbusStatus;
  const valid = raw && typeof raw.uptime_seconds === 'number';
  return { status: valid ? raw : undefined, isLoading: q.isLoading, isError: q.isError };
}

/** list_clients 를 폴링하여 연결된 TCP 클라이언트 목록을 반환한다(REQ-06-03). */
export function useModbusListClients(
  agentId: string,
  enabled: boolean,
): { clients: ModbusClient[]; isLoading: boolean; isError: boolean } {
  const refetchInterval = useRefreshMs();
  const q = useQuery({
    queryKey: ['modbus', agentId, 'list_clients'],
    queryFn: () => execAgent(agentId, { command: 'list_clients' }),
    enabled: enabled && agentId.length > 0,
    refetchInterval,
  });
  const clients = (unwrapExec(q.data).clients as ModbusClient[] | undefined) ?? [];
  return { clients, isLoading: q.isLoading, isError: q.isError };
}

// ---- 버스 통계 시계열(프론트 델타 누적, REQ-06-04 / AC-18) ----

/** 버스 시계열 한 포인트(per-min 환산 rate + 활성 커넥션). */
export interface ModbusBusPoint {
  /** 타임스탬프(epoch ms) — 차트 x축. */
  timestamp: number;
  reads_per_min: number;
  writes_per_min: number;
  errors_per_min: number;
  active_connections: number;
}

/** list_devices 합계(read/write/error). */
export function sumDeviceTotals(devices: ModbusDeviceListItem[]): {
  reads: number;
  writes: number;
  errors: number;
} {
  let reads = 0;
  let writes = 0;
  let errors = 0;
  for (const d of devices) {
    reads += d.stats.read_count;
    writes += d.stats.write_count;
    errors += d.stats.error_count;
  }
  return { reads, writes, errors };
}

/**
 * list_devices 합계의 폴링 간 델타를 per-min rate 로 환산하여 세션-로컬 링버퍼에 누적한다.
 * 백엔드에 시계열이 없으므로(get_status 무이력) 프론트에서 인접 폴 델타로 재구성한다(REQ-06-04).
 * errors_per_min 은 error_count 델타로 "CRC errors"(백엔드 카운터 부재)를 대체한다(§4 caveat).
 * 첫 관측은 기준선으로만 기록(포인트 미생성)한다.
 */
export function useModbusBusSeries(
  devices: ModbusDeviceListItem[],
  activeConnections: number,
  maxPoints = 60,
): ModbusBusPoint[] {
  const prev = useRef<{ reads: number; writes: number; errors: number; at: number } | null>(null);
  const [points, setPoints] = useState<ModbusBusPoint[]>([]);
  const totals = sumDeviceTotals(devices);
  // 폴 변화 감지 서명(합계 + 활성 커넥션). 배열 참조 변화에 의한 무한 렌더를 막는다.
  const sig = `${totals.reads}:${totals.writes}:${totals.errors}:${activeConnections}:${devices.length}`;

  useEffect(() => {
    const now = Date.now();
    const last = prev.current;
    prev.current = { reads: totals.reads, writes: totals.writes, errors: totals.errors, at: now };
    if (last === null) return; // 첫 관측: 기준선만 기록.
    const elapsedSec = Math.max(1, (now - last.at) / 1000);
    const perMin = (delta: number) => Math.max(0, (delta / elapsedSec) * 60);
    const point: ModbusBusPoint = {
      timestamp: now,
      reads_per_min: perMin(totals.reads - last.reads),
      writes_per_min: perMin(totals.writes - last.writes),
      errors_per_min: perMin(totals.errors - last.errors),
      active_connections: activeConnections,
    };
    setPoints((prevPoints) => {
      const next = [...prevPoints, point];
      return next.length > maxPoints ? next.slice(next.length - maxPoints) : next;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sig]);

  return points;
}

// ---- 표시 헬퍼 ----

/** unit_id 를 U01 형식으로 포맷한다. */
export function formatUnitLabel(unitId: number): string {
  return `U${String(unitId).padStart(2, '0')}`;
}

/** uptime(초)을 사람이 읽는 문자열로 포맷한다(예: "1h 2m", "2m 5s", "45s"). */
export function formatUptime(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${sec}s`;
  return `${sec}s`;
}

/** 영역 배지 정의(코드 → 축약 라벨). register_counts>0 인 영역만 배지로 표시한다. */
export const AREA_BADGES: { key: keyof ModbusRegisterCounts; label: string }[] = [
  { key: 'coils', label: 'CO' },
  { key: 'discrete_inputs', label: 'DI' },
  { key: 'input_registers', label: 'IR' },
  { key: 'holding_registers', label: 'HR' },
];
