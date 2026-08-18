// 에어컨 제어 패널 컴포넌트.
// 전원, 현재 온도, 설정 온도, 운전 모드, 풍량, 스윙, 필터 상태를 표시한다.

import { useState } from 'react';
import {
  Power,
  Snowflake,
  Flame,
  Droplets,
  RefreshCw,
  Fan,
  Thermometer,
  ArrowUpDown,
  Minus,
  Plus,
  AlertTriangle,
  Eye,
  HardDrive,
  Activity,
  Moon,
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
import { usePanelTitleVisible } from '../panelChromeContext';
import {
  readControlButtonColorConfig,
  readFanLevelColorConfig,
  readValueColorConfig,
  resolveControlButtonColor,
  resolveFanLevelColor,
  resolveValueColor,
} from './acControlColors';
import {
  normalizeAcMode,
  normalizeFanSpeed,
  type AcMode,
  type FanSpeed,
} from './acControlTypes';

// ---- 디바이스 속성 읽기 ----
// 백엔드에서 속성명이 통일되어 있으므로 (power, current_temperature, target_temperature, mode)
// 프론트엔드는 단순 읽기만 수행한다. mode/fan_speed 는 hvac 통일 ID (int) 로
// emit 되므로 acControlTypes 의 normalize 헬퍼로 문자열로 변환한다.

function readAcProps(props: Record<string, unknown>, capabilities?: string[]) {
  const hasControl = capabilities?.some(c => c.startsWith('set_')) ?? false;
  const isPassive = !hasControl;
  const power = typeof props['power'] === 'boolean' ? props['power'] : undefined;
  const currentTemp = props['current_temperature'] as number | undefined;
  // 현재 습도(%) — 삼성 NASA V1.1 등 지원 기기만 emit 한다(미지원 시 undefined → 표시 생략).
  const currentHumidity = props['current_humidity'] as number | undefined;
  const targetTemp = (props['target_temperature'] as number) ?? 24;
  const mode: AcMode = normalizeAcMode(props['mode']);
  const fanSpeed: FanSpeed = normalizeFanSpeed(props['fan_speed']);
  return { power, mode, currentTemp, currentHumidity, targetTemp, fanSpeed, isPassive };
}

// ---- 타입 정의 ----

interface AcControlPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

// AcMode, FanSpeed 는 ./acControlTypes 에서 import (공유 타입)

// ---- 모드/풍량 설정 ----

// 모듈 스코프 상수에는 i18n 키만 저장하고, 렌더 시 t(labelKey) 로 변환한다.
const MODE_CONFIG: { key: AcMode; labelKey: string; icon: React.ReactNode }[] = [
  { key: 'cool', labelKey: 'dashboard.acPanel.cooling', icon: <Snowflake className="h-4 w-4" /> },
  { key: 'heat', labelKey: 'dashboard.acPanel.heating', icon: <Flame className="h-4 w-4" /> },
  { key: 'auto', labelKey: 'dashboard.acPanel.auto', icon: <RefreshCw className="h-4 w-4" /> },
  { key: 'dry',  labelKey: 'dashboard.acPanel.dehumidify', icon: <Droplets className="h-4 w-4" /> },
  { key: 'fan',  labelKey: 'dashboard.acControl.fan',   icon: <Fan className="h-4 w-4" /> },
];

const ALL_FAN_SPEEDS: { key: FanSpeed; labelKey: string }[] = [
  { key: 'auto', labelKey: 'dashboard.acPanel.auto' },
  { key: 'quiet', labelKey: 'dashboard.acControl.fanQuiet' },
  { key: 'low', labelKey: 'dashboard.acPanel.low' },
  { key: 'medium', labelKey: 'dashboard.acPanel.medium' },
  { key: 'high', labelKey: 'dashboard.acPanel.high' },
  { key: 'turbo', labelKey: 'dashboard.acControl.fanTurbo' },
];

/** 프로토콜별 지원 풍량 */
const FAN_SPEEDS_BY_PROTOCOL: Record<string, Set<FanSpeed>> = {
  samsung_nasa:   new Set(['auto', 'low', 'medium', 'high']),
  lg_icp02:       new Set(['auto', 'low', 'medium', 'high', 'turbo']),
  lgap:           new Set(['auto', 'quiet', 'low', 'medium', 'high']),
  lg_icp01:       new Set(['auto', 'quiet', 'low', 'medium', 'high']),
};

const DEFAULT_FAN_SPEEDS = new Set<FanSpeed>(['auto', 'low', 'medium', 'high']);

function getFanSpeedConfig(protocol?: string): { key: FanSpeed; labelKey: string }[] {
  const allowed = (protocol && FAN_SPEEDS_BY_PROTOCOL[protocol]) || DEFAULT_FAN_SPEEDS;
  return ALL_FAN_SPEEDS.filter(({ key }) => allowed.has(key));
}

const TEMP_MIN = 16;
const TEMP_MAX = 30;

/** 에어컨 제어 패널 */
export default function AcControlPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: AcControlPanelProps) {
  const showTitle = usePanelTitleVisible();
  const { t } = useTranslation();
  const deviceId = config.deviceId as string | undefined;
  // 레거시: 단일 currentValueColor 만 지정하던 시절의 호환 경로.
  // 신규: valueColor (default + ranges) 로 값 범위별 컬러 지정.
  // 두 설정이 모두 있는 경우, valueColor 가 우선한다.
  const legacyCurrentValueColor = config.currentValueColor as string | undefined;
  const valueColorConfig = readValueColorConfig(config);
  const controlButtonColorConfig = readControlButtonColorConfig(config);
  const fanLevelColorConfig = readFanLevelColorConfig(config);

  // 원격 대시보드 target(SPEC-REMOTE-001 M10, REQ-L08): 원격이면 device.state(그룹 J)
  // 로 실시간 상태를 읽고, 명령 쓰기는 그룹 D(execute)로 라우팅한다(REQ-D04/J03).
  // config 의 bare deviceId 는 그 노드 디바이스로 해석된다(REQ-L03). 로컬은 불변.
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const gating = useTargetGating(target);
  const localDevice = useDeviceRealtime(remote ? '' : deviceId ?? '');
  const remoteDevice = useDeviceDetailTarget(target, deviceId ?? '');
  const device = remote ? remoteDevice.data : localDevice.data;
  const isLoading = remote ? remoteDevice.isLoading : localDevice.isLoading;

  // 디바이스 제어 명령 실행(로컬: /execute, 원격: 그룹 D execute).
  const commandTarget = useDeviceCommandTarget(target);

  const execute = (command: string, params: Record<string, unknown>) => {
    if (!deviceId) return;
    // 원격: 노드 미승인/오프라인이면 명령을 막는다(게이팅 — REQ-L11).
    if (remote && !gating.canControl()) return;
    commandTarget.execute(deviceId, command, params);
  };

  // 디바이스 상태에서 읽기 (백엔드에서 속성명 통일됨)
  const rawProps = device?.state?.properties ?? {};
  const capabilities = device?.capabilities as string[] | undefined;
  const { power: serverPower, mode, currentTemp, currentHumidity, targetTemp, fanSpeed, isPassive } =
    readAcProps(rawProps, capabilities);

  // 낙관적 전원 토글: 즉시 UI 반영 → 서버 확인 후 동기화 / 타임아웃 시 복원
  const { displayValue: power, setOptimistic: setOptimisticPower, isPendingConfirmation } =
    useOptimisticToggle(serverPower);

  const isPending = commandTarget.isPending || isPendingConfirmation;

  // passive-monitor 디바이스는 제어 불가. 원격은 노드 ready(승인∧온라인) 아닐 때도 비활성.
  const remoteBlocked = remote && !gating.canControl();
  const controlDisabled = isPassive || !power || isPending || remoteBlocked;

  // TODO: swing 속성이 디바이스에 없을 경우 로컬 상태로 유지
  const [swing, setSwing] = useState(false);

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
            <Snowflake className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
            <span className="truncate text-sm font-semibold text-(--color-text-primary)">{title}</span>
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

  const displayTemp = currentTemp ?? '--';
  // 현재 값 컬러: valueColor 가 있으면 우선, 없으면 legacyCurrentValueColor 폴백
  const resolvedValueColor =
    resolveValueColor(currentTemp, valueColorConfig) ?? legacyCurrentValueColor;

  const handleTempUp = () => execute('target_temperature', { target_temperature: Math.min(targetTemp + 1, TEMP_MAX) });
  const handleTempDown = () => execute('target_temperature', { target_temperature: Math.max(targetTemp - 1, TEMP_MIN) });

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4 rounded-2xl bg-(--color-bg-surface) p-5 ring-1 ring-(--color-border-default)">
      {/* ---- 헤더: 아이콘+타이틀 | 상태뱃지+전원버튼 ---- */}
      <div className="flex shrink-0 items-center justify-between">
        {/* 우측에 전원 버튼이 함께 있으므로 타이틀 묶음만 숨긴다(제어 유실 방지). */}
        {showTitle && (
          <div className="flex items-center gap-2.5">
            <Snowflake className="h-5 w-5 text-blue-500" />
            <span className="truncate text-base font-bold text-(--color-text-primary)">{title}</span>
          </div>
        )}
        <div className="flex items-center gap-2">
          {isPassive && (
            <span title={t('dashboard.panel.monitorOnly')}><Eye className="h-4 w-4 text-amber-500 dark:text-amber-400" aria-label={t('dashboard.panel.monitorOnly')} /></span>
          )}
          <span className={cn(
            'inline-flex items-center gap-1 rounded-full px-2 py-1',
            power
              ? 'bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400'
              : 'bg-slate-100 text-slate-400 dark:bg-slate-800 dark:text-slate-500',
          )}>
            {power
              ? <span title={t('dashboard.acPanel.operating')}><Activity className="h-3.5 w-3.5" aria-label={t('dashboard.acPanel.operating')} /></span>
              : <span title={t('dashboard.acPanel.standby')}><Moon className="h-3.5 w-3.5" aria-label={t('dashboard.acPanel.standby')} /></span>}
          </span>
          {!isPassive && (
            <button
              type="button"
              onClick={() => {
                const nextPower = !power;
                setOptimisticPower(nextPower);
                execute('set_power', { power: nextPower });
              }}
              disabled={isPending || remoteBlocked}
              className={cn(
                'flex h-8 w-8 items-center justify-center rounded-lg transition-colors',
                power
                  ? 'bg-blue-500 text-white hover:bg-blue-600'
                  : 'bg-slate-200 text-slate-500 hover:bg-slate-300 dark:bg-slate-700 dark:text-slate-400 dark:hover:bg-slate-600',
              )}
              aria-label={power ? t('dashboard.acControl.powerOffAria') : t('dashboard.acControl.powerOnAria')}
            >
              <Power className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>

      {/* ---- 전원 OFF: 중앙 OFF 표시 ---- */}
      {power === false && (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 py-6">
          <Power className="h-10 w-10 text-slate-300 dark:text-slate-600" />
          <span className="text-sm font-medium text-slate-400 dark:text-slate-500">{t('dashboard.acControl.powerOff')}</span>
        </div>
      )}

      {/* ---- 전원 ON: 현재 온도 (중앙, 크게) ---- */}
      {/*
        currentValueColor 가 지정되면 inline style 로 적용.
        미지정 시 text-(--color-text-primary) — 라이트/다크모드 자동 대응.
      */}
      {power !== false && (
      <div className="flex shrink-0 flex-col items-center gap-0.5 py-3">
        <div className="flex items-end">
          <span
            className="text-5xl font-light text-(--color-text-primary)"
            style={resolvedValueColor ? { color: resolvedValueColor } : undefined}
          >
            {displayTemp}
          </span>
          <span
            className="text-xl text-(--color-text-primary)"
            style={resolvedValueColor ? { color: resolvedValueColor } : undefined}
          >
            °C
          </span>
        </div>
        <span className="text-xs font-medium text-(--color-text-muted)">{t('dashboard.acControl.currentTemp')}</span>
        {/* ---- 현재 습도(%) — 지원 기기만(current_humidity emit 시). 미지원 시 생략 ---- */}
        {currentHumidity !== undefined && (
          <div className="mt-1 flex items-center gap-1" title={t('dashboard.acControl.currentHumidity')}>
            <Droplets className="h-3.5 w-3.5 text-(--color-text-muted)" />
            <span className="text-sm font-medium text-(--color-text-secondary)">{currentHumidity}%</span>
          </div>
        )}
      </div>
      )}

      {/* ---- 설정 온도 ~ 구분선 (ON 시에만) ---- */}
      {power !== false && (
      <>
      <div className="flex shrink-0 items-center justify-center gap-2.5">
        <Thermometer className="h-4 w-4 text-(--color-text-muted)" />
        <button
          type="button"
          onClick={handleTempDown}
          disabled={controlDisabled || targetTemp <= TEMP_MIN}
          className="flex h-7 w-7 items-center justify-center rounded-md bg-(--color-bg-elevated) text-(--color-text-secondary) transition-colors hover:bg-(--color-border-default) disabled:opacity-40"
          aria-label={t('dashboard.acControl.tempDown')}
        >
          <Minus className="h-3.5 w-3.5" />
        </button>
        <span className="text-sm font-semibold text-(--color-text-primary)">
          {t('dashboard.acControl.setLabel')} {targetTemp}°C
        </span>
        <button
          type="button"
          onClick={handleTempUp}
          disabled={controlDisabled || targetTemp >= TEMP_MAX}
          className="flex h-7 w-7 items-center justify-center rounded-md bg-(--color-bg-elevated) text-(--color-text-secondary) transition-colors hover:bg-(--color-border-default) disabled:opacity-40"
          aria-label={t('dashboard.acControl.tempUp')}
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>
      <div className="border-t border-(--color-border-default)" />
      </>
      )}

      {/* ---- 모드 + 풍량 (ON 시에만) ---- */}
      {power !== false && (
      <>
      <div className="flex shrink-0 gap-1.5">
        {MODE_CONFIG.map(({ key, labelKey, icon }) => {
          const label = t(labelKey);
          // 사용자 지정 컬러 해석. selected 일 때만 배경색을 inline 으로 적용한다.
          // unselected 컬러는 ring 으로 적용 (배경은 surface 토큰 유지).
          const { active, color } = resolveControlButtonColor(mode, key, controlButtonColorConfig);
          const styleOverride: React.CSSProperties = {};
          if (active && color) {
            styleOverride.backgroundColor = color;
            styleOverride.color = '#ffffff';
          } else if (!active && controlButtonColorConfig?.unselected) {
            styleOverride.boxShadow = `inset 0 0 0 1px ${controlButtonColorConfig.unselected}`;
          }
          return (
            <button
              key={key}
              type="button"
              onClick={() => execute('set_mode', { mode: key })}
              disabled={controlDisabled}
              className={cn(
                'flex h-[50px] flex-1 flex-col items-center justify-center gap-1 rounded-[10px] text-[9px] font-medium transition-colors disabled:opacity-40',
                active
                  ? 'bg-blue-600 font-semibold text-white'
                  : 'bg-(--color-bg-surface) text-(--color-text-secondary) ring-1 ring-(--color-border-default) hover:bg-(--color-bg-elevated)',
              )}
              style={styleOverride}
              aria-label={t('dashboard.acControl.modeAria').replace('{label}', label)}
              aria-pressed={active}
            >
              {icon}
              <span>{label}</span>
            </button>
          );
        })}
      </div>

      <div className="flex shrink-0 items-center gap-2">
        <div className="flex items-center gap-1">
          <Fan className="h-3.5 w-3.5 text-blue-600" />
          <span className="text-xs font-semibold text-blue-600">{t('dashboard.acControl.fanSpeed')}</span>
        </div>
        {getFanSpeedConfig(device?.protocol).map(({ key, labelKey }) => {
          const label = t(labelKey);
          const { active, color } = resolveFanLevelColor(fanSpeed, key, fanLevelColorConfig);
          const styleOverride: React.CSSProperties = {};
          if (active && color) {
            // 활성 단계: 배경 = 사용자 지정 색상의 라이트 톤, 글자/링 = 사용자 색상.
            // CSS color-mix 폴백 대신 단순히 배경/링/글자 모두 동일 색상으로 적용한다.
            styleOverride.backgroundColor = color;
            styleOverride.color = '#ffffff';
            styleOverride.boxShadow = `inset 0 0 0 1px ${color}`;
          } else if (!active && fanLevelColorConfig?.unselected) {
            styleOverride.boxShadow = `inset 0 0 0 1px ${fanLevelColorConfig.unselected}`;
          }
          return (
            <button
              key={key}
              type="button"
              onClick={() => execute('set_fan_speed', { fan_speed: key })}
              disabled={controlDisabled}
              className={cn(
                'flex h-8 flex-1 items-center justify-center rounded-lg text-[11px] font-medium transition-colors disabled:opacity-40',
                active
                  ? 'bg-blue-50 text-blue-600 ring-1 ring-blue-500 dark:bg-blue-900/30 dark:text-blue-300'
                  : 'bg-(--color-bg-surface) text-(--color-text-secondary) ring-1 ring-(--color-border-default) hover:bg-(--color-bg-elevated)',
              )}
              style={styleOverride}
              aria-label={t('dashboard.acControl.fanSpeedAria').replace('{label}', label)}
              aria-pressed={active}
            >
              {label}
            </button>
          );
        })}
      </div>
      </>
      )}

      {/* ---- 에러 표시 ---- 원격은 503/504/502/404 를 editError 로 매핑(REQ-L11). ---- */}
      {commandTarget.error ? (
        <div className="shrink-0 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
          {remote
            ? remoteEditErrorMessage(commandTarget.error, t)
            : String((commandTarget.error as Error)?.message ?? commandTarget.error)}
        </div>
      ) : null}

      {/* ---- 하단: 스윙 + 필터 (ON 시에만) ---- */}
      {power !== false && (
      <>
      <div className="border-t border-(--color-border-default)" />
      <div className="flex shrink-0 items-center gap-4 text-xs">
        <button
          type="button"
          onClick={() => setSwing((prev) => !prev)}
          disabled={controlDisabled}
          className="flex items-center gap-1 text-(--color-text-muted) transition-colors hover:text-(--color-text-secondary) disabled:opacity-40"
        >
          <ArrowUpDown className="h-3.5 w-3.5" />
          <span className="font-medium">{t('dashboard.acControl.swing')} {swing ? 'ON' : 'OFF'}</span>
        </button>
        <div className="flex items-center gap-1 text-amber-500">
          <AlertTriangle className="h-3.5 w-3.5" />
          <span className="font-medium">{t('dashboard.acControl.filterReplace')}</span>
        </div>
      </div>
      </>
      )}
    </div>
  );
}
