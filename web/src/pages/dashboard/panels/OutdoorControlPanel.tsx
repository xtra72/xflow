// 실외기/제어기 모니터링 패널 컴포넌트.
// 압축기 주파수, 압축기 용량, 운전 모드, 상태 플래그를 표시한다.
// 읽기 전용(passive-monitor) 패널이므로 제어 버튼이 없다.

import { Activity, Cpu, Eye, Gauge, HardDrive, Moon } from 'lucide-react';

import { useDeviceDetailTarget } from '@/hooks/useDetailTargets';
import { useDeviceRealtime } from '@/hooks/useDevice';
import { useTranslation, type TranslationFn } from '@/lib/i18n';
import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { cn } from '@/lib/utils/cn';
import { usePanelTitleStyle, usePanelTitleVisible } from '../panelChromeContext';

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
  fan: 'bg-(--color-bg-secondary) text-(--color-text-muted) ring-(--color-border-default)',
};

/** 모드 라벨 i18n 키 (렌더 시 t(key) 로 변환) */
const MODE_LABEL_KEYS: Record<OpMode, string> = {
  cool: 'dashboard.acPanel.cooling',
  heat: 'dashboard.acPanel.heating',
  auto: 'dashboard.acPanel.auto',
  dry: 'dashboard.acPanel.dehumidify',
  fan: 'dashboard.acControl.fan',
};

// ---- 상태 인디케이터 설정 ----

interface StatusIndicator {
  key: string;
  labelKey: string;
  activeColor: string;
  activeBg: string;
}

const STATUS_INDICATORS: StatusIndicator[] = [
  { key: 'compressor_run', labelKey: 'dashboard.outdoor.compressor', activeColor: 'bg-green-500', activeBg: 'bg-green-50 dark:bg-green-900/20' },
  { key: 'outdoor_active', labelKey: 'dashboard.outdoor.outdoorUnit', activeColor: 'bg-green-500', activeBg: 'bg-green-50 dark:bg-green-900/20' },
  { key: 'refrigerant_on', labelKey: 'dashboard.outdoor.refrigerant', activeColor: 'bg-blue-500', activeBg: 'bg-blue-50 dark:bg-blue-900/20' },
  { key: 'heat_demand', labelKey: 'dashboard.outdoor.heatDemand', activeColor: 'bg-orange-500', activeBg: 'bg-orange-50 dark:bg-orange-900/20' },
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
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();
  const { t } = useTranslation();
  const deviceId = config.deviceId as string | undefined;
  // 현재 값 (압축기 주파수 / 토출 온도) 표시 색상 — 패널 설정에서 지정 가능.
  // 미지정 시 text-primary (라이트/다크모드 자동 대응).
  const currentValueColor = config.currentValueColor as string | undefined;

  // 원격 대시보드 target(SPEC-REMOTE-001 M10, REQ-L08): 모니터링 전용 패널이므로
  // device.state read 만 target-aware 하다(명령 쓰기 없음). 로컬은 불변.
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const localDevice = useDeviceRealtime(remote ? '' : deviceId ?? '');
  const remoteDevice = useDeviceDetailTarget(target, deviceId ?? '');
  const device = remote ? remoteDevice.data : localDevice.data;
  const isLoading = remote ? remoteDevice.isLoading : localDevice.isLoading;

  // ---- 디바이스 미설정 ----
  if (!deviceId) {
    return (
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center rounded-2xl bg-(--color-bg-surface) p-3 ring-1 ring-(--color-border-default)">
        <HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />
        <p className="text-xs text-(--color-text-muted)">{t('dashboard.panel.deviceNotConfigured')}</p>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-3 ring-1 ring-(--color-border-default)">
        {showTitle && (
          <div className="mb-2 flex shrink-0 items-center gap-2">
            <Cpu className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
            <span className="truncate text-sm font-semibold text-(--color-text-primary)" style={titleStyle}>{title}</span>
          </div>
        )}
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
        <p className="text-xs text-(--color-text-muted)">{t('dashboard.panel.deviceNotFound')}</p>
      </div>
    );
  }

  const rawProps = device?.state?.properties ?? {};
  const online = device?.online ?? false;
  const protocol = device?.protocol ?? '';

  // LG ICP-01 ODU 전용 레이아웃
  if (protocol === 'lg_icp01') {
    return <LgIcp01OutdoorLayout title={title} online={online} rawProps={rawProps} currentValueColor={currentValueColor} t={t} />;
  }

  // 기본 (LG ICP-02 등): 압축기 주파수 + 상태 인디케이터
  const compressorHz = typeof rawProps['compressor_hz'] === 'number' ? rawProps['compressor_hz'] : 0;
  const compressorCap = typeof rawProps['compressor_cap'] === 'number' ? rawProps['compressor_cap'] : 0;
  const opMode = (rawProps['op_mode'] as OpMode) ?? 'auto';
  const capPercent = Math.round((compressorCap / COMPRESSOR_CAP_MAX) * 100);

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4 rounded-2xl bg-(--color-bg-surface) p-5 ring-1 ring-(--color-border-default)">
      {/* ---- 헤더: 아이콘+타이틀 | 상태배지+모드 ---- */}
      <div className="flex shrink-0 items-center justify-between">
        {showTitle && (
          <div className="flex items-center gap-2.5">
            <Gauge className="h-5 w-5 text-blue-500" />
            <span className="truncate text-base font-bold text-(--color-text-primary)" style={titleStyle}>{title}</span>
          </div>
        )}
        <div className="flex items-center gap-2">
          <span title={t('dashboard.panel.monitorOnly')}><Eye className="h-4 w-4 text-amber-500 dark:text-amber-400" aria-label={t('dashboard.panel.monitorOnly')} /></span>
          <span className={cn(
            'inline-flex items-center gap-1 rounded-full px-2 py-1',
            online
              ? 'bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400'
              : 'bg-(--color-bg-sunken) text-(--color-text-muted)',
          )}>
            {online
              ? <span title={t('dashboard.acPanel.operating')}><Activity className="h-3.5 w-3.5" aria-label={t('dashboard.acPanel.operating')} /></span>
              : <span title={t('dashboard.acPanel.standby')}><Moon className="h-3.5 w-3.5" aria-label={t('dashboard.acPanel.standby')} /></span>}
          </span>
          <span className={cn(
            'inline-flex items-center rounded-full px-2.5 py-0.5 text-[10px] font-medium ring-1',
            MODE_COLORS[opMode] ?? MODE_COLORS.auto,
          )}>
            {MODE_LABEL_KEYS[opMode] ? t(MODE_LABEL_KEYS[opMode]) : opMode}
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
        <span className="text-xs font-medium text-(--color-text-muted)">{t('dashboard.outdoor.compressorFreq')}</span>
      </div>

      {/* ---- 구분선 ---- */}
      <div className="border-t border-(--color-border-default)" />

      {/* ---- 상태 인디케이터 (2x2 그리드) ---- */}
      <div className="grid shrink-0 grid-cols-2 gap-2">
        {STATUS_INDICATORS.map(({ key, labelKey, activeColor, activeBg }) => {
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
                active ? activeColor : 'bg-(--color-border-strong)',
              )} />
              <span className={cn(
                'text-xs font-medium',
                active ? 'text-(--color-text-primary)' : 'text-(--color-text-muted)',
              )}>
                {t(labelKey)}
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
            <span className="text-xs font-medium text-(--color-text-secondary)">{t('dashboard.outdoor.compressorCap')}</span>
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

/** LG ICP-01 ODU 온도 항목 정의 (label 은 i18n 키, 렌더 시 t(key) 로 변환) */
const LG_ICP01_ODU_TEMPS: { key: string; labelKey: string; icon: string }[] = [
  { key: 'outdoor_temperature', labelKey: 'dashboard.outdoor.outdoorTemp', icon: '🌡' },
  { key: 'compressor_suction_temperature', labelKey: 'dashboard.outdoor.compressorSuction', icon: '❄' },
  { key: 'compressor_discharge_temperature', labelKey: 'dashboard.outdoor.compressorDischarge', icon: '🔥' },
  { key: 'condenser_temperature_a', labelKey: 'dashboard.outdoor.condenserA', icon: '💧' },
  { key: 'condenser_temperature_b', labelKey: 'dashboard.outdoor.condenserB', icon: '💧' },
  { key: 'avg_temperature', labelKey: 'dashboard.outdoor.avgTemp', icon: '📊' },
];

function LgIcp01OutdoorLayout({
  title,
  online,
  rawProps,
  currentValueColor,
  t,
}: {
  title: string;
  online: boolean;
  rawProps: Record<string, unknown>;
  currentValueColor?: string;
  t: TranslationFn;
}) {
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();
  const outdoorTemp = typeof rawProps['outdoor_temperature'] === 'number' ? rawProps['outdoor_temperature'] : null;

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4 rounded-2xl bg-(--color-bg-surface) p-5 ring-1 ring-(--color-border-default)">
      {/* 헤더 */}
      <div className="flex shrink-0 items-center justify-between">
        {showTitle && (
          <div className="flex items-center gap-2.5">
            <Gauge className="h-5 w-5 text-blue-500" />
            <span className="truncate text-base font-bold text-(--color-text-primary)" style={titleStyle}>{title}</span>
          </div>
        )}
        <div className="flex items-center gap-2">
          <span title={t('dashboard.panel.monitorOnly')}><Eye className="h-4 w-4 text-amber-500 dark:text-amber-400" aria-label={t('dashboard.panel.monitorOnly')} /></span>
          <span className={cn(
            'inline-flex items-center gap-1 rounded-full px-2 py-1',
            online
              ? 'bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400'
              : 'bg-(--color-bg-sunken) text-(--color-text-muted)',
          )}>
            {online
              ? <span title={t('dashboard.acPanel.operating')}><Activity className="h-3.5 w-3.5" aria-label={t('dashboard.acPanel.operating')} /></span>
              : <span title={t('dashboard.acPanel.standby')}><Moon className="h-3.5 w-3.5" aria-label={t('dashboard.acPanel.standby')} /></span>}
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
        <span className="text-xs font-medium text-(--color-text-muted)">{t('dashboard.outdoor.outdoorTemp')}</span>
      </div>

      <div className="border-t border-(--color-border-default)" />

      {/* 냉동 사이클 온도 그리드 */}
      <div className="grid shrink-0 grid-cols-2 gap-2">
        {LG_ICP01_ODU_TEMPS.filter(item => item.key !== 'outdoor_temperature').map(({ key, labelKey, icon }) => {
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
                  {t(labelKey)}
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
