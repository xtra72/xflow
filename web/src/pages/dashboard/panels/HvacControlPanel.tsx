// HVAC 공조기 제어 패널.
// 센서 데이터(온도, 습도, CO2, 전력)와 환기 모드, 온·습도 설정, 스케줄을 표시한다.

import {
  Power,
  Wind,
  Thermometer,
  Droplets,
  Zap,
  Plus,
  Minus,
  Settings,
  Clock,
} from 'lucide-react';

import { useDeviceDetailTarget } from '@/hooks/useDetailTargets';
import { useDeviceRealtime } from '@/hooks/useDevice';
import { useDeviceCommandTarget } from '@/hooks/useDeviceCommandTarget';
import { useOptimisticToggle } from '@/hooks/useOptimisticToggle';
import { useTargetGating } from '@/hooks/useTargetGating';
import { useTranslation } from '@/lib/i18n';
import { remoteEditErrorMessage } from '@/lib/remote/editError';
import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { cn } from '@/lib/utils/cn';

interface HvacControlPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

/** 환기 모드 목록 */
const VENT_MODES = [
  { key: 'auto', label: '자동환기' },
  { key: 'supply', label: '급기' },
  { key: 'exhaust', label: '배기' },
  { key: 'heat-exchange', label: '전열교환' },
] as const;

type VentMode = (typeof VENT_MODES)[number]['key'];

/** 센서 카드 컴포넌트 */
function SensorCard({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className="flex flex-1 flex-col items-center gap-1 rounded-lg bg-(--color-bg-elevated) p-2">
      <div className="text-(--color-text-muted)">{icon}</div>
      <span className="text-[10px] text-(--color-text-muted)">{label}</span>
      <span className="text-sm font-semibold text-(--color-text-primary)">{value}</span>
    </div>
  );
}

/** 값 조절 컨트롤 (온도/습도 설정용) */
function ValueAdjuster({
  label,
  value,
  unit,
  onDecrement,
  onIncrement,
  disabled,
}: {
  label: string;
  value: string;
  unit: string;
  onDecrement: () => void;
  onIncrement: () => void;
  disabled?: boolean;
}) {
  return (
    <div className="flex flex-1 flex-col gap-1">
      <span className="text-[10px] text-(--color-text-muted)">{label}</span>
      <div className="flex items-center gap-1">
        <button
          type="button"
          onClick={onDecrement}
          disabled={disabled}
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-(--color-border-default) text-(--color-text-secondary) hover:bg-(--color-bg-elevated) disabled:opacity-50 disabled:pointer-events-none"
        >
          <Minus className="h-3.5 w-3.5" />
        </button>
        <span className="flex-1 text-center text-sm font-medium text-(--color-text-primary)">
          {value}{unit}
        </span>
        <button
          type="button"
          onClick={onIncrement}
          disabled={disabled}
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-(--color-border-default) text-(--color-text-secondary) hover:bg-(--color-bg-elevated) disabled:opacity-50 disabled:pointer-events-none"
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  );
}

/** HVAC 공조기 제어 패널 */
export default function HvacControlPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: HvacControlPanelProps) {
  const { t } = useTranslation();
  const deviceId = config.deviceId as string | undefined;

  // 원격 대시보드 target(SPEC-REMOTE-001 M10, REQ-L08): 원격이면 device.state(그룹 J)로
  // 상태를 읽고, 명령 쓰기는 그룹 D(execute)로 라우팅한다(REQ-D04/J03). 로컬은 불변.
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const gating = useTargetGating(target);
  const localDevice = useDeviceRealtime(remote ? '' : deviceId ?? '');
  const remoteDevice = useDeviceDetailTarget(target, deviceId ?? '');
  const device = remote ? remoteDevice.data : localDevice.data;
  const isLoading = remote ? remoteDevice.isLoading : localDevice.isLoading;

  // 디바이스 제어 명령(로컬: /execute, 원격: 그룹 D execute).
  const commandTarget = useDeviceCommandTarget(target);

  // 제어 상태 (디바이스 속성에서 읽기)
  const props = device?.state?.properties ?? {};
  const serverPower = props['power'] as boolean | undefined;
  // 낙관적 전원 토글: 즉시 UI 반영 → 서버 확인 후 동기화 / 타임아웃 시 복원
  // (hooks는 조건부 반환 이전에 호출해야 함)
  const { displayValue: power, setOptimistic: setOptimisticPower, isPendingConfirmation } =
    useOptimisticToggle(serverPower);

  const remoteBlocked = remote && !gating.canControl();
  const execute = (command: string, params: Record<string, unknown>) => {
    if (!deviceId || remoteBlocked) return;
    commandTarget.execute(deviceId, command, params);
  };

  // 디바이스 미설정 상태
  if (!deviceId) {
    return (
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <Wind className="mb-2 h-6 w-6 text-(--color-text-muted)" />
        <p className="text-xs text-(--color-text-muted)">디바이스가 설정되지 않았습니다.</p>
      </div>
    );
  }

  // 로딩 상태
  if (isLoading) {
    return (
      <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <div className="mb-2 flex shrink-0 items-center gap-2">
          <Wind className="h-4 w-4 text-(--color-text-muted)" />
          <span className="truncate text-sm font-medium text-(--color-text-primary)">{title}</span>
        </div>
        <div className="flex flex-1 items-center justify-center">
          <div className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-default) border-t-blue-600" />
        </div>
      </div>
    );
  }

  // 디바이스에서 센서 값 및 제어 상태 읽기 (없으면 기본값 사용)
  const indoorTemp = (props.indoor_temp as number) ?? 23.5;
  const humidity = (props.humidity as number) ?? 52;
  const co2 = (props.co2 as number) ?? 850;
  const powerUsage = (props.power_usage as number) ?? 3.2;

  const isPending = commandTarget.isPending || isPendingConfirmation || remoteBlocked;
  const powerOn = power ?? false;
  const targetTemp = (props['target_temperature'] as number) ?? 24;
  const targetHumidity = (props['target_humidity'] as number) ?? 50;
  const ventMode = (props['vent_mode'] as VentMode) ?? 'auto';

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 rounded-lg bg-(--color-bg-surface) p-4 shadow">
      {/* 1) 헤더: 아이콘 + 제목 + 상태 뱃지 + 전원 버튼 */}
      <div className="flex shrink-0 items-center justify-between">
        <div className="flex items-center gap-2">
          <Wind className="h-4 w-4 text-(--color-text-secondary)" />
          <span className="truncate text-sm font-medium text-(--color-text-primary)">{title}</span>
        </div>
        <div className="flex items-center gap-2">
          <span
            className={cn(
              'rounded-full px-2 py-0.5 text-[10px] font-medium',
              powerOn
                ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                : 'bg-gray-100 text-gray-500 dark:bg-gray-700/30 dark:text-gray-400',
            )}
          >
            {powerOn ? '운전 중' : '정지'}
          </span>
          <button
            type="button"
            disabled={isPending}
            onClick={() => {
              const nextPower = !powerOn;
              setOptimisticPower(nextPower);
              execute('set_power', { power: nextPower });
            }}
            className={cn(
              'flex h-7 w-7 items-center justify-center rounded-md transition-colors',
              powerOn
                ? 'bg-green-600 text-white hover:bg-green-700'
                : 'bg-gray-200 text-gray-500 hover:bg-gray-300 dark:bg-gray-700 dark:text-gray-400 dark:hover:bg-gray-600',
            )}
          >
            <Power className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      {/* 2) 센서 카드 (4열) */}
      <div className="flex shrink-0 gap-2">
        <SensorCard
          icon={<Thermometer className="h-4 w-4" />}
          label="실내 온도"
          value={`${indoorTemp.toFixed(1)}\u00B0C`}
        />
        <SensorCard
          icon={<Droplets className="h-4 w-4" />}
          label="습도"
          value={`${humidity}%`}
        />
        <SensorCard
          icon={<Wind className="h-4 w-4" />}
          label="CO2"
          value={`${co2}ppm`}
        />
        <SensorCard
          icon={<Zap className="h-4 w-4" />}
          label="전력"
          value={`${powerUsage.toFixed(1)}kW`}
        />
      </div>

      {/* 3) 환기 모드 (4버튼) */}
      <div className="flex shrink-0 gap-1.5">
        {VENT_MODES.map((mode) => (
          <button
            key={mode.key}
            type="button"
            disabled={isPending}
            onClick={() => execute('set_vent_mode', { vent_mode: mode.key })}
            className={cn(
              'flex-1 rounded-md px-1 py-1.5 text-[11px] font-medium transition-colors',
              ventMode === mode.key
                ? 'bg-blue-600 text-white'
                : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)/80',
            )}
          >
            {mode.label}
          </button>
        ))}
      </div>

      {/* 4) 온도·습도 설정 */}
      <div className="flex shrink-0 gap-3">
        <ValueAdjuster
          label="온도 설정"
          value={targetTemp.toFixed(1)}
          unit={'\u00B0C'}
          disabled={isPending}
          onDecrement={() => execute('target_temperature', { target_temperature: Math.max(16, +(targetTemp - 0.5).toFixed(1)) })}
          onIncrement={() => execute('target_temperature', { target_temperature: Math.min(30, +(targetTemp + 0.5).toFixed(1)) })}
        />
        <ValueAdjuster
          label="습도 설정"
          value={`${targetHumidity}`}
          unit="%"
          disabled={isPending}
          onDecrement={() => execute('set_humidity', { target_humidity: Math.max(30, targetHumidity - 5) })}
          onIncrement={() => execute('set_humidity', { target_humidity: Math.min(80, targetHumidity + 5) })}
        />
      </div>

      {/* 5) 스케줄 */}
      <div className="flex shrink-0 items-center justify-between rounded-lg bg-(--color-bg-elevated) px-3 py-2">
        <div className="flex items-center gap-2">
          <Clock className="h-3.5 w-3.5 text-(--color-text-muted)" />
          <span className="text-xs text-(--color-text-secondary)">08:00 - 18:00 (평일)</span>
        </div>
        <button
          type="button"
          className="flex h-6 w-6 items-center justify-center rounded-md text-(--color-text-muted) hover:bg-(--color-bg-surface)"
        >
          <Settings className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* 에러 표시 — 원격은 503/504/502/404 를 editError 로 매핑(REQ-L11). */}
      {commandTarget.error ? (
        <div className="shrink-0 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
          {remote
            ? remoteEditErrorMessage(commandTarget.error, t)
            : String((commandTarget.error as Error)?.message ?? commandTarget.error)}
        </div>
      ) : null}
    </div>
  );
}
