// 디바이스 상세 패널 컴포넌트.
// 디바이스 상태 속성, 명령 실행, 메타데이터 편집 기능을 제공한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import {
  AlertCircle,
  ChevronsUpDown,
  Droplets,
  Edit2,
  Filter,
  Flame,
  Loader2,
  MapPin,
  Minus,
  Play,
  Plus,
  Power,
  RefreshCw,
  Snowflake,
  Tag,
  Thermometer,
  Wind,
  X,
} from 'lucide-react';

import { useDeviceRealtime, useExecuteCommand, useUpdateMetadata } from '@/hooks/useDevice';
import { cn } from '@/lib/utils/cn';
import { getPropertyLabel } from '@/lib/utils/deviceLabels';
import type { CommandSpec, ParamSpec } from '@/types/device';

interface DeviceDetailPanelProps {
  deviceId: string;
  hideState?: boolean;
}

/** 디바이스 행 확장 시 표시되는 상세 패널 */
export default function DeviceDetailPanel({ deviceId, hideState }: DeviceDetailPanelProps) {
  const { data: device, isLoading, error } = useDeviceRealtime(deviceId);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-5 w-5 animate-spin text-gray-400" />
        <span className="ml-2 text-sm text-gray-500 dark:text-gray-400">
          불러오는 중...
        </span>
      </div>
    );
  }

  if (error || !device) {
    return (
      <div className="flex items-center justify-center py-8 text-sm text-red-500 dark:text-red-400">
        <AlertCircle className="mr-2 h-4 w-4" />
        디바이스 정보를 불러올 수 없습니다.
      </div>
    );
  }

  return (
    <div className="space-y-6 px-6 py-4">
      {/* Section 1: 상태 속성 */}
      {!hideState && device.state?.properties && Object.keys(device.state.properties).length > 0 && (
        <StatePropertiesSection
          properties={device.state.properties}
          protocol={device.protocol}
          type={device.type}
          deviceId={deviceId}
        />
      )}

      {/* Section 2: 명령 */}
      {device.commands && device.commands.length > 0 && (
        <CommandsSection deviceId={deviceId} commands={device.commands} />
      )}

      {/* Section 3: 메타데이터 */}
      <MetadataSection
        deviceId={deviceId}
        metadata={{
          tags: device.metadata?.tags ?? [],
          location: device.metadata?.location ?? '',
          group: device.metadata?.group ?? '',
          labels: device.metadata?.labels ?? {},
        }}
      />
    </div>
  );
}

// ---- Section 1: 상태 속성 ----

/** 속성값 포맷팅 */
function formatPropertyValue(key: string, value: unknown): string {
  if (value === null || value === undefined) return '-';

  // 온도 관련 키는 C 단위 표시
  if (typeof value === 'number') {
    const lowerKey = key.toLowerCase();
    if (lowerKey.includes('temp')) return `${value}\u00B0C`;
    return String(value);
  }

  if (typeof value === 'boolean') return value ? 'ON' : 'OFF';

  return String(value);
}

interface StatePropertiesSectionProps {
  properties: Record<string, unknown>;
  protocol: string;
  type: string;
  compact?: boolean;
  deviceId?: string;
}

export function StatePropertiesSection({ properties, protocol, type, compact, deviceId }: StatePropertiesSectionProps) {
  if (protocol === 'nasa' && type === 'indoor') {
    return <NasaIndoorRemoteControl properties={properties} compact={compact} deviceId={deviceId} />;
  }
  return <GenericPropertiesGrid properties={properties} protocol={protocol} type={type} compact={compact} />;
}

// ---- NASA Indoor 리모컨 레이아웃 ----

const MODE_CONFIG: Record<string, { label: string; Icon: typeof Snowflake; active: string }> = {
  cool: {
    label: '냉방',
    Icon: Snowflake,
    active: 'border-blue-300 bg-blue-100 text-blue-700 dark:border-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
  },
  heat: {
    label: '난방',
    Icon: Flame,
    active: 'border-orange-300 bg-orange-100 text-orange-700 dark:border-orange-700 dark:bg-orange-900/40 dark:text-orange-300',
  },
  auto: {
    label: '자동',
    Icon: RefreshCw,
    active: 'border-green-300 bg-green-100 text-green-700 dark:border-green-700 dark:bg-green-900/40 dark:text-green-300',
  },
  dry: {
    label: '제습',
    Icon: Droplets,
    active: 'border-cyan-300 bg-cyan-100 text-cyan-700 dark:border-cyan-700 dark:bg-cyan-900/40 dark:text-cyan-300',
  },
  fan: {
    label: '팬',
    Icon: Wind,
    active: 'border-purple-300 bg-purple-100 text-purple-700 dark:border-purple-700 dark:bg-purple-900/40 dark:text-purple-300',
  },
};

const FAN_LABELS: Record<string, string> = { auto: '자동', low: '약', medium: '중', high: '강' };

function NasaIndoorRemoteControl({ properties, compact, deviceId }: { properties: Record<string, unknown>; compact?: boolean; deviceId?: string }) {
  const executeMutation = useExecuteCommand();
  const interactive = !!deviceId;

  const power = properties['power'] as boolean | undefined;
  const mode = properties['mode'] as string | undefined;
  const currentTemp = properties['current_temp'] as number | undefined;
  const targetTemp = properties['target_temp'] as number | undefined;
  const fanSpeed = properties['fan_speed'] as string | undefined;
  const swing = properties['swing_vertical'] as boolean | undefined;
  const filterAlarm = properties['filter_alarm'] as boolean | undefined;
  const errorCode = properties['error_code'] as number | undefined;

  // 제어 명령 후 실제 상태가 변경될 때까지 "적용 중" 표시를 유지한다.
  // WebSocket device.status 이벤트로 properties 가 갱신되면 자동 해제된다.
  const [commandPending, setCommandPending] = useState(false);
  const stateKey = `${power}-${mode}-${targetTemp}-${fanSpeed}`;
  const prevStateKey = useRef(stateKey);
  useEffect(() => {
    if (prevStateKey.current !== stateKey) {
      prevStateKey.current = stateKey;
      setCommandPending(false);
    }
  }, [stateKey]);
  // 안전장치: 하드웨어 무응답 시 3초 후 pending 해제
  useEffect(() => {
    if (!commandPending) return;
    const timer = setTimeout(() => setCommandPending(false), 3000);
    return () => clearTimeout(timer);
  }, [commandPending]);

  const isPending = executeMutation.isPending || commandPending;

  const isOff = power === false;
  const inactiveBadge = 'border-gray-200 bg-gray-50 text-gray-400 dark:border-gray-600 dark:bg-gray-700/50 dark:text-gray-500';

  const execute = (command: string, params: Record<string, unknown>) => {
    if (!deviceId) return;
    setCommandPending(true);
    executeMutation.mutate({ id: deviceId, req: { command, params } });
  };

  return (
    <div>
      {!compact && <h4 className="mb-3 text-sm font-semibold text-gray-900 dark:text-white">상태</h4>}
      <div className={cn('overflow-hidden', !compact && 'max-w-sm rounded-2xl border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800')}>
        {/* 헤더: 전원 + 에러코드 */}
        <div className="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-gray-700">
          <button
            type="button"
            onClick={interactive ? () => execute('set_power', { power: !power }) : undefined}
            disabled={!interactive || isPending}
            className={cn(
              'flex items-center gap-2 text-sm font-semibold transition-colors',
              power ? 'text-green-600 dark:text-green-400' : 'text-gray-400 dark:text-gray-500',
              interactive && 'hover:opacity-70',
              isPending && 'opacity-50',
            )}
          >
            {isPending ? <Loader2 className="h-5 w-5 animate-spin" /> : <Power className="h-5 w-5" />}
            {isPending ? '적용 중...' : power ? 'ON' : 'OFF'}
          </button>
          {errorCode != null && errorCode !== 0 && (
            <div className="flex items-center gap-1 text-xs font-medium text-red-500 dark:text-red-400">
              <AlertCircle className="h-3.5 w-3.5" />
              에러 {errorCode}
            </div>
          )}
        </div>

        {/* 온도 표시 */}
        <div className="border-b border-gray-100 px-4 py-5 text-center dark:border-gray-700">
          {currentTemp != null ? (
            <>
              <p className={cn('text-5xl font-bold tabular-nums', isOff ? 'text-gray-300 dark:text-gray-600' : 'text-gray-900 dark:text-white')}>
                {currentTemp}
                <span className="text-2xl font-normal text-gray-400">&deg;C</span>
              </p>
              <p className="mt-1 text-xs text-gray-400">현재 온도</p>
            </>
          ) : (
            <p className="text-2xl text-gray-300 dark:text-gray-600">--</p>
          )}

          {targetTemp != null && (
            <div className="mt-3 flex items-center justify-center gap-2 text-sm text-gray-600 dark:text-gray-300">
              <Thermometer className="h-4 w-4 text-blue-500" />
              {interactive && (
                <button
                  type="button"
                  onClick={() => execute('set_temperature', { target_temp: Math.max(16, targetTemp - 1) })}
                  disabled={isPending || isOff}
                  className="rounded-full p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 disabled:opacity-50 dark:hover:bg-gray-700 dark:hover:text-gray-300"
                >
                  <Minus className="h-3.5 w-3.5" />
                </button>
              )}
              <span className="min-w-[4rem] text-center font-medium">설정 {targetTemp}&deg;C</span>
              {interactive && (
                <button
                  type="button"
                  onClick={() => execute('set_temperature', { target_temp: Math.min(30, targetTemp + 1) })}
                  disabled={isPending || isOff}
                  className="rounded-full p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 disabled:opacity-50 dark:hover:bg-gray-700 dark:hover:text-gray-300"
                >
                  <Plus className="h-3.5 w-3.5" />
                </button>
              )}
            </div>
          )}
        </div>

        {/* 운전 모드 */}
        <div className="border-b border-gray-100 px-4 py-3 dark:border-gray-700">
          <div className="flex flex-wrap gap-2">
            {Object.entries(MODE_CONFIG).map(([key, cfg]) => {
              const isActive = mode === key;
              const { Icon } = cfg;
              return (
                <button
                  key={key}
                  type="button"
                  onClick={interactive ? () => execute('set_mode', { mode: key }) : undefined}
                  disabled={!interactive || isPending || isOff}
                  className={cn(
                    'inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-medium transition-colors',
                    isOff ? inactiveBadge : isActive ? cfg.active : inactiveBadge,
                    interactive && !isActive && !isOff && 'hover:border-gray-300 hover:bg-gray-100 dark:hover:border-gray-500 dark:hover:bg-gray-600/50',
                  )}
                >
                  <Icon className="h-3 w-3" />
                  {cfg.label}
                </button>
              );
            })}
          </div>
        </div>

        {/* 풍량 */}
        <div className="border-b border-gray-100 px-4 py-3 dark:border-gray-700">
          <div className="flex items-center gap-3">
            <Wind className="h-4 w-4 shrink-0 text-gray-400" />
            <span className="min-w-fit text-xs text-gray-500 dark:text-gray-400">풍량</span>
            <div className="flex gap-1.5">
              {(['auto', 'low', 'medium', 'high'] as const).map((speed) => (
                <button
                  key={speed}
                  type="button"
                  onClick={interactive ? () => execute('set_fan_speed', { fan_speed: speed }) : undefined}
                  disabled={!interactive || isPending || isOff}
                  className={cn(
                    'rounded px-2 py-0.5 text-xs font-medium transition-colors',
                    isOff
                      ? 'bg-gray-100 text-gray-300 dark:bg-gray-700 dark:text-gray-600'
                      : fanSpeed === speed
                        ? 'bg-blue-600 text-white'
                        : 'bg-gray-100 text-gray-400 dark:bg-gray-700 dark:text-gray-500',
                    interactive && fanSpeed !== speed && !isOff && 'hover:bg-gray-200 hover:text-gray-600 dark:hover:bg-gray-600 dark:hover:text-gray-300',
                  )}
                >
                  {FAN_LABELS[speed]}
                </button>
              ))}
            </div>
          </div>
        </div>

        {/* 상태 인디케이터 */}
        <div className="flex items-center gap-4 px-4 py-3">
          <div
            className={cn(
              'flex items-center gap-1 text-xs',
              swing ? 'font-medium text-blue-600 dark:text-blue-400' : 'text-gray-400',
            )}
          >
            <ChevronsUpDown className="h-3.5 w-3.5" />
            스윙 {swing ? 'ON' : 'OFF'}
          </div>
          <div
            className={cn(
              'flex items-center gap-1 text-xs',
              filterAlarm ? 'font-medium text-amber-600 dark:text-amber-400' : 'text-gray-400',
            )}
          >
            <Filter className="h-3.5 w-3.5" />
            {filterAlarm ? '필터 교체 필요' : '필터 정상'}
          </div>
        </div>
      </div>
    </div>
  );
}

// ---- 일반 디바이스 속성 그리드 ----

function GenericPropertiesGrid({
  properties,
  protocol,
  type,
  compact,
}: {
  properties: Record<string, unknown>;
  protocol: string;
  type: string;
  compact?: boolean;
}) {
  const entries = Object.entries(properties);

  return (
    <div className={compact ? 'px-4 py-3' : ''}>
      {!compact && <h4 className="mb-3 text-sm font-semibold text-gray-900 dark:text-white">상태 속성</h4>}
      <div className={cn('grid gap-3', compact ? 'grid-cols-2' : 'grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5')}>
        {entries.map(([key, value]) => (
          <div
            key={key}
            className="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-gray-600 dark:bg-gray-800"
          >
            <p className="text-xs text-gray-500 dark:text-gray-400">
              {getPropertyLabel(key, protocol, type)}
            </p>
            <p className="mt-0.5 text-sm font-medium text-gray-900 dark:text-white">
              {formatPropertyValue(key, value)}
            </p>
          </div>
        ))}
      </div>
    </div>
  );
}

// ---- Section 2: 명령 컨트롤 ----

function CommandsSection({
  deviceId,
  commands,
}: {
  deviceId: string;
  commands: CommandSpec[];
}) {
  return (
    <div>
      <h4 className="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
        제어
      </h4>
      <div className="space-y-3">
        {commands.map((cmd) => (
          <CommandControl key={cmd.name} deviceId={deviceId} command={cmd} />
        ))}
      </div>
    </div>
  );
}

/** 명령별 인라인 컨트롤 (파라미터 타입에 따라 적절한 UI 렌더링) */
function CommandControl({
  deviceId,
  command,
}: {
  deviceId: string;
  command: CommandSpec;
}) {
  const executeMutation = useExecuteCommand();
  const isPending = executeMutation.isPending;

  const execute = (params?: Record<string, unknown>) => {
    executeMutation.mutate({
      id: deviceId,
      req: { command: command.name, params },
    });
  };

  const params = command.params;

  // 파라미터 없음 → 단일 실행 버튼
  if (params.length === 0) {
    return (
      <CommandRow label={command.name} description={command.description}>
        <button
          type="button"
          onClick={() => execute()}
          disabled={isPending}
          className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          {isPending ? <Loader2 className="h-3 w-3 animate-spin" /> : <Play className="h-3 w-3" />}
          실행
        </button>
      </CommandRow>
    );
  }

  const firstParam = params[0];

  // 단일 bool 파라미터 → 토글 버튼
  if (params.length === 1 && firstParam && firstParam.type === 'bool') {
    return <BoolCommandControl command={command} param={firstParam} execute={execute} isPending={isPending} />;
  }

  // 단일 enum 파라미터 → 버튼 그룹 또는 셀렉트
  if (params.length === 1 && firstParam && firstParam.type === 'enum' && firstParam.enum) {
    const enumValues = firstParam.enum;
    // 4개 이하면 버튼 그룹, 5개 이상이면 셀렉트
    if (enumValues.length <= 4) {
      return <EnumButtonGroupControl command={command} param={firstParam} execute={execute} isPending={isPending} />;
    }
    return <EnumSelectControl command={command} param={firstParam} execute={execute} isPending={isPending} />;
  }

  // 단일 숫자 파라미터 → 슬라이더 + 값 표시
  if (params.length === 1 && firstParam && (firstParam.type === 'int' || firstParam.type === 'float')) {
    return <NumericCommandControl command={command} param={firstParam} execute={execute} isPending={isPending} />;
  }

  // 복합 파라미터 → 인라인 폼
  return <MultiParamCommandControl deviceId={deviceId} command={command} />;
}

/** 컨트롤 행 래퍼 (라벨 + 컨트롤) */
function CommandRow({
  label,
  description,
  children,
}: {
  label: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-center justify-between rounded-lg border border-gray-200 bg-white px-4 py-2.5 dark:border-gray-700 dark:bg-gray-800">
      <div className="mr-3 min-w-0">
        <p className="text-sm font-medium text-gray-900 dark:text-white">{label}</p>
        {description && (
          <p className="truncate text-xs text-gray-500 dark:text-gray-400">{description}</p>
        )}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  );
}

/** Bool 파라미터 → ON/OFF 토글 버튼 */
function BoolCommandControl({
  command,
  param,
  execute,
  isPending,
}: {
  command: CommandSpec;
  param: ParamSpec;
  execute: (params: Record<string, unknown>) => void;
  isPending: boolean;
}) {
  return (
    <CommandRow label={command.name} description={command.description}>
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={() => execute({ [param.name]: true })}
          disabled={isPending}
          className="rounded-md bg-green-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-green-700 disabled:opacity-50"
        >
          ON
        </button>
        <button
          type="button"
          onClick={() => execute({ [param.name]: false })}
          disabled={isPending}
          className="rounded-md bg-gray-500 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-gray-600 disabled:opacity-50"
        >
          OFF
        </button>
        {isPending && <Loader2 className="h-3 w-3 animate-spin text-gray-400" />}
      </div>
    </CommandRow>
  );
}

/** Enum 파라미터 (4개 이하) → 버튼 그룹 */
function EnumButtonGroupControl({
  command,
  param,
  execute,
  isPending,
}: {
  command: CommandSpec;
  param: ParamSpec;
  execute: (params: Record<string, unknown>) => void;
  isPending: boolean;
}) {
  return (
    <CommandRow label={command.name} description={command.description}>
      <div className="flex items-center gap-1">
        {param.enum!.map((opt) => (
          <button
            key={opt}
            type="button"
            onClick={() => execute({ [param.name]: opt })}
            disabled={isPending}
            className="rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-700 transition-colors hover:bg-blue-50 hover:border-blue-300 hover:text-blue-700 disabled:opacity-50 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-300 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
          >
            {opt}
          </button>
        ))}
        {isPending && <Loader2 className="h-3 w-3 animate-spin text-gray-400" />}
      </div>
    </CommandRow>
  );
}

/** Enum 파라미터 (5개 이상) → 셀렉트 드롭다운 */
function EnumSelectControl({
  command,
  param,
  execute,
  isPending,
}: {
  command: CommandSpec;
  param: ParamSpec;
  execute: (params: Record<string, unknown>) => void;
  isPending: boolean;
}) {
  return (
    <CommandRow label={command.name} description={command.description}>
      <div className="flex items-center gap-2">
        <select
          defaultValue=""
          onChange={(e) => {
            if (e.target.value) {
              execute({ [param.name]: e.target.value });
              e.target.value = '';
            }
          }}
          disabled={isPending}
          className="rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs text-gray-700 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-300"
        >
          <option value="">선택...</option>
          {param.enum!.map((opt) => (
            <option key={opt} value={opt}>{opt}</option>
          ))}
        </select>
        {isPending && <Loader2 className="h-3 w-3 animate-spin text-gray-400" />}
      </div>
    </CommandRow>
  );
}

/** 숫자 파라미터 → 슬라이더 또는 +/- 버튼 */
function NumericCommandControl({
  command,
  param,
  execute,
  isPending,
}: {
  command: CommandSpec;
  param: ParamSpec;
  execute: (params: Record<string, unknown>) => void;
  isPending: boolean;
}) {
  const hasRange = param.min != null && param.max != null;
  const step = param.type === 'float' ? 0.5 : 1;
  const [value, setValue] = useState(param.min ?? 0);

  return (
    <CommandRow label={command.name} description={command.description}>
      <div className="flex items-center gap-2">
        {hasRange ? (
          <>
            <button
              type="button"
              onClick={() => {
                const next = Math.max(param.min!, value - step);
                setValue(next);
                execute({ [param.name]: next });
              }}
              disabled={isPending}
              className="rounded-md border border-gray-300 p-1 text-gray-600 hover:bg-gray-100 disabled:opacity-50 dark:border-gray-600 dark:text-gray-400 dark:hover:bg-gray-700"
            >
              <Minus className="h-3.5 w-3.5" />
            </button>
            <input
              type="range"
              min={param.min}
              max={param.max}
              step={step}
              value={value}
              onChange={(e) => setValue(Number(e.target.value))}
              onMouseUp={() => execute({ [param.name]: value })}
              onTouchEnd={() => execute({ [param.name]: value })}
              disabled={isPending}
              className="h-1.5 w-20 cursor-pointer accent-blue-600 disabled:opacity-50"
            />
            <span className="min-w-[2rem] text-center text-xs font-medium text-gray-700 dark:text-gray-300">
              {value}
            </span>
            <button
              type="button"
              onClick={() => {
                const next = Math.min(param.max!, value + step);
                setValue(next);
                execute({ [param.name]: next });
              }}
              disabled={isPending}
              className="rounded-md border border-gray-300 p-1 text-gray-600 hover:bg-gray-100 disabled:opacity-50 dark:border-gray-600 dark:text-gray-400 dark:hover:bg-gray-700"
            >
              <Plus className="h-3.5 w-3.5" />
            </button>
          </>
        ) : (
          <>
            <input
              type="number"
              value={value}
              step={step}
              onChange={(e) => setValue(Number(e.target.value))}
              className="w-20 rounded-md border border-gray-300 px-2 py-1 text-xs text-gray-900 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
            />
            <button
              type="button"
              onClick={() => execute({ [param.name]: value })}
              disabled={isPending}
              className="inline-flex items-center rounded-md bg-blue-600 px-2.5 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {isPending ? <Loader2 className="h-3 w-3 animate-spin" /> : '적용'}
            </button>
          </>
        )}
        {isPending && hasRange && <Loader2 className="h-3 w-3 animate-spin text-gray-400" />}
      </div>
    </CommandRow>
  );
}

/** 복합 파라미터 → 인라인 폼 */
function MultiParamCommandControl({
  deviceId,
  command,
}: {
  deviceId: string;
  command: CommandSpec;
}) {
  const [paramValues, setParamValues] = useState<Record<string, unknown>>({});
  const [result, setResult] = useState<{ success: boolean; error?: string } | null>(null);
  const executeMutation = useExecuteCommand();

  const handleParamChange = useCallback((name: string, value: unknown) => {
    setParamValues((prev) => ({ ...prev, [name]: value }));
  }, []);

  const handleSubmit = async () => {
    setResult(null);
    try {
      await executeMutation.mutateAsync({
        id: deviceId,
        req: {
          command: command.name,
          params: Object.keys(paramValues).length > 0 ? paramValues : undefined,
        },
      });
      setResult({ success: true });
    } catch (err) {
      setResult({
        success: false,
        error: err instanceof Error ? err.message : '명령 실행에 실패했습니다.',
      });
    }
  };

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <p className="mb-2 text-sm font-medium text-gray-900 dark:text-white">{command.name}</p>
      {command.description && (
        <p className="mb-3 text-xs text-gray-500 dark:text-gray-400">{command.description}</p>
      )}
      <div className="mb-3 grid grid-cols-2 gap-2">
        {command.params.map((param) => (
          <InlineParamInput
            key={param.name}
            param={param}
            value={paramValues[param.name]}
            onChange={(val) => handleParamChange(param.name, val)}
          />
        ))}
      </div>
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={handleSubmit}
          disabled={executeMutation.isPending}
          className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
        >
          {executeMutation.isPending ? <Loader2 className="h-3 w-3 animate-spin" /> : <Play className="h-3 w-3" />}
          실행
        </button>
        {result && (
          <span className={cn('text-xs', result.success ? 'text-green-600' : 'text-red-600')}>
            {result.success ? '완료' : result.error}
          </span>
        )}
      </div>
    </div>
  );
}

/** 인라인 파라미터 입력 (컴팩트) */
function InlineParamInput({
  param,
  value,
  onChange,
}: {
  param: ParamSpec;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const inputBase =
    'w-full rounded-md border border-gray-300 px-2 py-1 text-xs text-gray-900 dark:border-gray-600 dark:bg-gray-700 dark:text-white';

  if (param.type === 'enum' && param.enum) {
    return (
      <div>
        <label className="mb-0.5 block text-xs text-gray-500 dark:text-gray-400">{param.name}</label>
        <select value={(value as string) ?? ''} onChange={(e) => onChange(e.target.value)} className={inputBase}>
          <option value="">선택...</option>
          {param.enum.map((opt) => (
            <option key={opt} value={opt}>{opt}</option>
          ))}
        </select>
      </div>
    );
  }

  if (param.type === 'bool') {
    return (
      <div className="flex items-center gap-2">
        <label className="text-xs text-gray-500 dark:text-gray-400">{param.name}</label>
        <button
          type="button"
          onClick={() => onChange(!(value as boolean))}
          className={cn(
            'relative inline-flex h-5 w-9 items-center rounded-full transition-colors',
            value ? 'bg-blue-600' : 'bg-gray-300 dark:bg-gray-600',
          )}
        >
          <span
            className={cn(
              'inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform',
              value ? 'translate-x-4.5' : 'translate-x-0.5',
            )}
          />
        </button>
      </div>
    );
  }

  if (param.type === 'int' || param.type === 'float') {
    return (
      <div>
        <label className="mb-0.5 block text-xs text-gray-500 dark:text-gray-400">
          {param.name}{param.min != null && param.max != null ? ` (${param.min}~${param.max})` : ''}
        </label>
        <input
          type="number"
          value={(value as number) ?? ''}
          min={param.min}
          max={param.max}
          step={param.type === 'float' ? 0.1 : 1}
          onChange={(e) => {
            const v = e.target.value;
            onChange(v === '' ? undefined : param.type === 'int' ? parseInt(v, 10) : parseFloat(v));
          }}
          className={inputBase}
        />
      </div>
    );
  }

  return (
    <div>
      <label className="mb-0.5 block text-xs text-gray-500 dark:text-gray-400">{param.name}</label>
      <input
        type="text"
        value={(value as string) ?? ''}
        onChange={(e) => onChange(e.target.value)}
        className={inputBase}
      />
    </div>
  );
}

// ---- Section 3: 메타데이터 ----

interface MetadataSectionProps {
  deviceId: string;
  metadata: {
    tags: string[];
    location: string;
    group: string;
    labels: Record<string, string>;
  };
}

function MetadataSection({ deviceId, metadata }: MetadataSectionProps) {
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({
    location: metadata.location,
    group: metadata.group,
    tagsStr: metadata.tags.join(', '),
    labels: { ...metadata.labels },
  });
  const [newLabelKey, setNewLabelKey] = useState('');
  const [newLabelValue, setNewLabelValue] = useState('');
  const updateMutation = useUpdateMetadata();

  const handleSave = async () => {
    const tags = form.tagsStr
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean);

    try {
      await updateMutation.mutateAsync({
        id: deviceId,
        metadata: {
          location: form.location,
          group: form.group,
          tags,
          labels: form.labels,
        },
      });
      setEditing(false);
    } catch {
      // 에러는 React Query에서 처리
    }
  };

  const handleAddLabel = () => {
    if (!newLabelKey.trim()) return;
    setForm((prev) => ({
      ...prev,
      labels: { ...prev.labels, [newLabelKey.trim()]: newLabelValue.trim() },
    }));
    setNewLabelKey('');
    setNewLabelValue('');
  };

  const handleRemoveLabel = (key: string) => {
    setForm((prev) => {
      const next = { ...prev.labels };
      delete next[key];
      return { ...prev, labels: next };
    });
  };

  const inputBase =
    'block w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-900 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:focus:border-blue-400';

  return (
    <div>
      <div className="mb-3 flex items-center justify-between">
        <h4 className="text-sm font-semibold text-gray-900 dark:text-white">
          메타데이터
        </h4>
        {!editing && (
          <button
            type="button"
            onClick={() => setEditing(true)}
            className="inline-flex items-center gap-1 text-xs text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
          >
            <Edit2 className="h-3 w-3" />
            편집
          </button>
        )}
      </div>

      {!editing ? (
        /* 읽기 모드 */
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
          <div className="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-gray-600 dark:bg-gray-800">
            <p className="flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400">
              <MapPin className="h-3 w-3" />
              위치
            </p>
            <p className="mt-0.5 text-sm text-gray-900 dark:text-white">
              {metadata.location || '-'}
            </p>
          </div>
          <div className="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-gray-600 dark:bg-gray-800">
            <p className="text-xs text-gray-500 dark:text-gray-400">그룹</p>
            <p className="mt-0.5 text-sm text-gray-900 dark:text-white">
              {metadata.group || '-'}
            </p>
          </div>
          <div className="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-gray-600 dark:bg-gray-800">
            <p className="flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400">
              <Tag className="h-3 w-3" />
              태그
            </p>
            <p className="mt-0.5 text-sm text-gray-900 dark:text-white">
              {metadata.tags.length > 0 ? metadata.tags.join(', ') : '-'}
            </p>
          </div>
          {Object.entries(metadata.labels).map(([k, v]) => (
            <div
              key={k}
              className="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-gray-600 dark:bg-gray-800"
            >
              <p className="text-xs text-gray-500 dark:text-gray-400">{k}</p>
              <p className="mt-0.5 text-sm text-gray-900 dark:text-white">{v}</p>
            </div>
          ))}
        </div>
      ) : (
        /* 편집 모드 */
        <div className="space-y-3">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
                위치
              </label>
              <input
                type="text"
                value={form.location}
                onChange={(e) => setForm((prev) => ({ ...prev, location: e.target.value }))}
                className={inputBase}
                placeholder="예: 1층 로비"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
                그룹
              </label>
              <input
                type="text"
                value={form.group}
                onChange={(e) => setForm((prev) => ({ ...prev, group: e.target.value }))}
                className={inputBase}
                placeholder="예: zone-a"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
                태그 (콤마 구분)
              </label>
              <input
                type="text"
                value={form.tagsStr}
                onChange={(e) => setForm((prev) => ({ ...prev, tagsStr: e.target.value }))}
                className={inputBase}
                placeholder="예: hvac, 1층, 로비"
              />
            </div>
          </div>

          {/* 라벨 목록 */}
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
              라벨
            </label>
            {Object.entries(form.labels).length > 0 && (
              <div className="mb-2 space-y-1">
                {Object.entries(form.labels).map(([k, v]) => (
                  <div key={k} className="flex items-center gap-2">
                    <span className="rounded bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-gray-700 dark:text-gray-300">
                      {k}: {v}
                    </span>
                    <button
                      type="button"
                      onClick={() => handleRemoveLabel(k)}
                      className="text-red-500 hover:text-red-700 dark:text-red-400"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </div>
                ))}
              </div>
            )}
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={newLabelKey}
                onChange={(e) => setNewLabelKey(e.target.value)}
                className={cn(inputBase, 'w-32')}
                placeholder="키"
              />
              <input
                type="text"
                value={newLabelValue}
                onChange={(e) => setNewLabelValue(e.target.value)}
                className={cn(inputBase, 'w-32')}
                placeholder="값"
              />
              <button
                type="button"
                onClick={handleAddLabel}
                className="rounded-md border border-gray-300 px-2 py-1.5 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
              >
                추가
              </button>
            </div>
          </div>

          {/* 저장/취소 버튼 */}
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={handleSave}
              disabled={updateMutation.isPending}
              className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              {updateMutation.isPending && <Loader2 className="h-3 w-3 animate-spin" />}
              저장
            </button>
            <button
              type="button"
              onClick={() => setEditing(false)}
              className="rounded-md border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
            >
              취소
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
