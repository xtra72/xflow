// 실외기/제어기 모니터링 패널 컴포넌트.
// 압축기 주파수, 압축기 용량, 운전 모드, 상태 플래그를 표시한다.
// 읽기 전용(passive-monitor) 패널이므로 제어 버튼이 없다.

import { Activity, Cpu, Eye, Gauge, HardDrive, Moon } from 'lucide-react';

import { useDeviceRealtime } from '@/hooks/useDevice';
import { cn } from '@/lib/utils/cn';

// ---- 타입 정의 ----

interface OutdoorControlPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

/** 운전 모드 */
type OpMode = 'cool' | 'heat' | 'auto' | 'dry' | 'fan';

/** 모드별 뱃지 색상 */
const MODE_COLORS: Record<OpMode, string> = {
  cool: 'bg-blue-50 text-blue-600 ring-blue-200 dark:bg-blue-900/20 dark:text-blue-400 dark:ring-blue-700',
  heat: 'bg-orange-50 text-orange-600 ring-orange-200 dark:bg-orange-900/20 dark:text-orange-400 dark:ring-orange-700',
  auto: 'bg-green-50 text-green-600 ring-green-200 dark:bg-green-900/20 dark:text-green-400 dark:ring-green-700',
  dry: 'bg-purple-50 text-purple-600 ring-purple-200 dark:bg-purple-900/20 dark:text-purple-400 dark:ring-purple-700',
  fan: 'bg-slate-50 text-slate-500 ring-slate-200 dark:bg-slate-800 dark:text-slate-400 dark:ring-slate-600',
};

/** 모드 라벨 */
const MODE_LABELS: Record<OpMode, string> = {
  cool: '냉방',
  heat: '난방',
  auto: '자동',
  dry: '제습',
  fan: '팬',
};

// ---- 상태 인디케이터 설정 ----

interface StatusIndicator {
  key: string;
  label: string;
  activeColor: string;
  activeBg: string;
}

const STATUS_INDICATORS: StatusIndicator[] = [
  { key: 'compressor_run', label: '압축기', activeColor: 'bg-green-500', activeBg: 'bg-green-50 dark:bg-green-900/20' },
  { key: 'outdoor_active', label: '실외기', activeColor: 'bg-green-500', activeBg: 'bg-green-50 dark:bg-green-900/20' },
  { key: 'refrigerant_on', label: '냉매', activeColor: 'bg-blue-500', activeBg: 'bg-blue-50 dark:bg-blue-900/20' },
  { key: 'heat_demand', label: '난방 요구', activeColor: 'bg-orange-500', activeBg: 'bg-orange-50 dark:bg-orange-900/20' },
];

const COMPRESSOR_CAP_MAX = 15;

/** 실외기 모니터링 패널 */
export default function OutdoorControlPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: OutdoorControlPanelProps) {
  const deviceId = config.deviceId as string | undefined;
  // 현재 값 (압축기 주파수 / 토출 온도) 표시 색상 — 패널 설정에서 지정 가능.
  // 미지정 시 text-primary (라이트/다크모드 자동 대응).
  const currentValueColor = config.currentValueColor as string | undefined;
  const { data: device, isLoading } = useDeviceRealtime(deviceId ?? '');

  // ---- 디바이스 미설정 ----
  if (!deviceId) {
    return (
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center rounded-2xl bg-(--color-bg-surface) p-3 ring-1 ring-(--color-border-default)">
        <HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />
        <p className="text-xs text-(--color-text-muted)">디바이스가 설정되지 않았습니다.</p>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-3 ring-1 ring-(--color-border-default)">
        <div className="mb-2 flex shrink-0 items-center gap-2">
          <Cpu className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
          <span className="truncate text-sm font-semibold text-(--color-text-primary)">{title}</span>
        </div>
        <div className="flex flex-1 items-center justify-center">
          <div className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-default) border-t-blue-600" />
        </div>
      </div>
    );
  }

  if (!device) {
    return (
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center rounded-2xl bg-(--color-bg-surface) p-3 ring-1 ring-(--color-border-default)">
        <HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />
        <p className="text-xs text-(--color-text-muted)">디바이스를 찾을 수 없습니다.</p>
      </div>
    );
  }

  const rawProps = device?.state?.properties ?? {};
  const online = device?.online ?? false;
  const protocol = device?.protocol ?? '';

  // LG ICP-01 ODU 전용 레이아웃
  if (protocol === 'lg_icp01') {
    return <LgIcp01OutdoorLayout title={title} online={online} rawProps={rawProps} currentValueColor={currentValueColor} />;
  }

  // 기본 (LGCP 등): 압축기 주파수 + 상태 인디케이터
  const compressorHz = typeof rawProps['compressor_hz'] === 'number' ? rawProps['compressor_hz'] : 0;
  const compressorCap = typeof rawProps['compressor_cap'] === 'number' ? rawProps['compressor_cap'] : 0;
  const opMode = (rawProps['op_mode'] as OpMode) ?? 'auto';
  const capPercent = Math.round((compressorCap / COMPRESSOR_CAP_MAX) * 100);

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4 rounded-2xl bg-(--color-bg-surface) p-5 ring-1 ring-(--color-border-default)">
      {/* ---- 헤더: 아이콘+타이틀 | 상태배지+모드 ---- */}
      <div className="flex shrink-0 items-center justify-between">
        <div className="flex items-center gap-2.5">
          <Gauge className="h-5 w-5 text-blue-500" />
          <span className="truncate text-base font-bold text-(--color-text-primary)">{title}</span>
        </div>
        <div className="flex items-center gap-2">
          <span title="모니터링 전용"><Eye className="h-4 w-4 text-amber-500 dark:text-amber-400" aria-label="모니터링 전용" /></span>
          <span className={cn(
            'inline-flex items-center gap-1 rounded-full px-2 py-1',
            online
              ? 'bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400'
              : 'bg-slate-100 text-slate-400 dark:bg-slate-800 dark:text-slate-500',
          )}>
            {online
              ? <span title="가동 중"><Activity className="h-3.5 w-3.5" aria-label="가동 중" /></span>
              : <span title="대기"><Moon className="h-3.5 w-3.5" aria-label="대기" /></span>}
          </span>
          <span className={cn(
            'inline-flex items-center rounded-full px-2.5 py-0.5 text-[10px] font-medium ring-1',
            MODE_COLORS[opMode] ?? MODE_COLORS.auto,
          )}>
            {MODE_LABELS[opMode] ?? opMode}
          </span>
        </div>
      </div>

      {/* ---- 중앙: 압축기 주파수 (크게) ---- */}
      <div className="flex shrink-0 flex-col items-center gap-0.5 py-3">
        <div className="flex items-end">
          <span
            className="text-5xl font-light text-(--color-text-primary)"
            style={currentValueColor ? { color: currentValueColor } : undefined}
          >
            {compressorHz}
          </span>
          <span
            className="ml-1 text-xl text-(--color-text-primary)"
            style={currentValueColor ? { color: currentValueColor } : undefined}
          >
            Hz
          </span>
        </div>
        <span className="text-xs font-medium text-(--color-text-muted)">압축기 주파수</span>
      </div>

      {/* ---- 구분선 ---- */}
      <div className="border-t border-(--color-border-default)" />

      {/* ---- 상태 인디케이터 (2x2 그리드) ---- */}
      <div className="grid shrink-0 grid-cols-2 gap-2">
        {STATUS_INDICATORS.map(({ key, label, activeColor, activeBg }) => {
          const active = !!rawProps[key];
          return (
            <div
              key={key}
              className={cn(
                'flex items-center gap-2 rounded-lg px-3 py-2',
                active ? activeBg : 'bg-(--color-bg-elevated)',
              )}
            >
              <span className={cn(
                'h-2 w-2 shrink-0 rounded-full',
                active ? activeColor : 'bg-gray-300 dark:bg-gray-600',
              )} />
              <span className={cn(
                'text-xs font-medium',
                active ? 'text-(--color-text-primary)' : 'text-(--color-text-muted)',
              )}>
                {label}
              </span>
            </div>
          );
        })}
      </div>

      {/* ---- 구분선 ---- */}
      <div className="border-t border-(--color-border-default)" />

      {/* ---- 하단: 압축기 용량 바 ---- */}
      <div className="flex shrink-0 flex-col gap-1.5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1.5">
            <Cpu className="h-3.5 w-3.5 text-(--color-text-muted)" />
            <span className="text-xs font-medium text-(--color-text-secondary)">압축기 용량</span>
          </div>
          <span className="text-xs font-semibold text-(--color-text-primary)">
            {compressorCap} / {COMPRESSOR_CAP_MAX}
          </span>
        </div>
        <div className="h-2 w-full overflow-hidden rounded-full bg-(--color-bg-elevated)">
          <div
            className="h-full rounded-full bg-blue-500 transition-all duration-300"
            style={{ width: `${capPercent}%` }}
          />
        </div>
      </div>
    </div>
  );
}

// ---- LG ICP-01 ODU 전용 레이아웃 ----

/** LG ICP-01 ODU 온도 항목 정의 */
const LG_ICP01_ODU_TEMPS: { key: string; label: string; icon: string }[] = [
  { key: 'outdoor_temperature', label: '외기 온도', icon: '🌡' },
  { key: 'compressor_suction_temperature', label: '압축기 흡입', icon: '❄' },
  { key: 'compressor_discharge_temperature', label: '압축기 토출', icon: '🔥' },
  { key: 'condenser_temperature_a', label: '응축기 A', icon: '💧' },
  { key: 'condenser_temperature_b', label: '응축기 B', icon: '💧' },
  { key: 'avg_temperature', label: '운전 평균', icon: '📊' },
];

function LgIcp01OutdoorLayout({
  title,
  online,
  rawProps,
  currentValueColor,
}: {
  title: string;
  online: boolean;
  rawProps: Record<string, unknown>;
  currentValueColor?: string;
}) {
  const outdoorTemp = typeof rawProps['outdoor_temperature'] === 'number' ? rawProps['outdoor_temperature'] : null;

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4 rounded-2xl bg-(--color-bg-surface) p-5 ring-1 ring-(--color-border-default)">
      {/* 헤더 */}
      <div className="flex shrink-0 items-center justify-between">
        <div className="flex items-center gap-2.5">
          <Gauge className="h-5 w-5 text-blue-500" />
          <span className="truncate text-base font-bold text-(--color-text-primary)">{title}</span>
        </div>
        <div className="flex items-center gap-2">
          <span title="모니터링 전용"><Eye className="h-4 w-4 text-amber-500 dark:text-amber-400" aria-label="모니터링 전용" /></span>
          <span className={cn(
            'inline-flex items-center gap-1 rounded-full px-2 py-1',
            online
              ? 'bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400'
              : 'bg-slate-100 text-slate-400 dark:bg-slate-800 dark:text-slate-500',
          )}>
            {online
              ? <span title="가동 중"><Activity className="h-3.5 w-3.5" aria-label="가동 중" /></span>
              : <span title="대기"><Moon className="h-3.5 w-3.5" aria-label="대기" /></span>}
          </span>
        </div>
      </div>

      {/* 중앙: 외기 온도 (크게) */}
      <div className="flex shrink-0 flex-col items-center gap-0.5 py-3">
        <div className="flex items-end">
          <span
            className="text-5xl font-light text-(--color-text-primary)"
            style={currentValueColor ? { color: currentValueColor } : undefined}
          >
            {outdoorTemp !== null ? outdoorTemp.toFixed(1) : '--'}
          </span>
          <span
            className="ml-1 text-xl text-(--color-text-primary)"
            style={currentValueColor ? { color: currentValueColor } : undefined}
          >
            °C
          </span>
        </div>
        <span className="text-xs font-medium text-(--color-text-muted)">외기 온도</span>
      </div>

      <div className="border-t border-(--color-border-default)" />

      {/* 냉동 사이클 온도 그리드 */}
      <div className="grid shrink-0 grid-cols-2 gap-2">
        {LG_ICP01_ODU_TEMPS.filter(t => t.key !== 'outdoor_temperature').map(({ key, label, icon }) => {
          const val = typeof rawProps[key] === 'number' ? rawProps[key] as number : null;
          const available = val !== null;
          return (
            <div
              key={key}
              className={cn(
                'flex items-center justify-between rounded-lg px-3 py-2.5',
                available ? 'bg-(--color-bg-elevated)' : 'bg-(--color-bg-elevated) opacity-40',
              )}
            >
              <div className="flex items-center gap-2">
                <span className="text-xs">{icon}</span>
                <span className={cn(
                  'text-xs font-medium',
                  available ? 'text-(--color-text-secondary)' : 'text-(--color-text-muted)',
                )}>
                  {label}
                </span>
              </div>
              <span className={cn(
                'text-sm font-semibold',
                available ? 'text-(--color-text-primary)' : 'text-(--color-text-muted)',
              )}>
                {available ? `${val.toFixed(1)}°C` : '--'}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
