// Facility 기기 패널 (SPEC-FACILITY-DASHBOARD-001 M2, REQ-FACDASH-001-03-*).
//
// config 의 agentId + deviceId 로 로스터를 조회해 단일 xsfm 기기의 상태(power,
// fan_speed, online, station/place/index)를 표시하고, set_power/set_fan_speed 제어를 제공한다.
// 제어 응답은 성공(ok, 에코 반영) vs 타임아웃(timeout/ErrControlTimeout)을 명시한다(REQ-03-02,
// UB-002). 전원 OFF 시 풍량 컨트롤을 비활성화한다(REQ-03-03, UB-005 — 위장 없음).

import { useState } from 'react';
import { Activity, Fan, HardDrive, Moon, Power } from 'lucide-react';

import {
  useXsfmControl,
  useFacilityRoster,
  type ControlResponse,
} from '@/hooks/useXsfmControl';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { ControlResultView } from './facilityShared';
import { usePanelTitleVisible } from '../panelChromeContext';

interface FacilityDevicePanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

/** 마지막 실행 뮤테이션 응답을 고른다(테스트에서 .data 만 세팅해도 렌더되도록 폴백). */
function pickResult(
  lastAction: 'power' | 'fan' | null,
  power: { data?: ControlResponse; error: unknown },
  fan: { data?: ControlResponse; error: unknown },
): ControlResponse | undefined {
  if (lastAction === 'power') return power.data;
  if (lastAction === 'fan') return fan.data;
  return power.data ?? fan.data;
}

/** Facility 기기 제어 패널. */
export default function FacilityDevicePanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: FacilityDevicePanelProps) {
  const { t } = useTranslation();
  const agentId = (config.agentId as string | undefined) ?? '';
  const deviceId = config.deviceId as string | undefined;
  const refreshMs = config.refreshMs as number | undefined;

  const { devices, stations, isLoading, isError } = useFacilityRoster(agentId, refreshMs);
  const { setPower, setFanSpeed } = useXsfmControl(agentId);
  const [lastAction, setLastAction] = useState<'power' | 'fan' | null>(null);

  const device = deviceId ? devices.find((d) => d.device_id === deviceId) : undefined;
  const entry = device ? stations.find((s) => s.station === device.station) : undefined;

  // ---- 가드: 미설정 / 로딩 / 에러 / 미발견 ----
  if (!deviceId || !agentId) {
    return (
      <PanelShell title={title}>
        <Centered icon={<HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />}>
          {t('dashboard.facility.notConfigured')}
        </Centered>
      </PanelShell>
    );
  }
  if (isLoading) {
    return (
      <PanelShell title={title}>
        <div className="flex flex-1 items-center justify-center">
          <div className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-default) border-t-blue-600" />
        </div>
      </PanelShell>
    );
  }
  if (isError) {
    return (
      <PanelShell title={title}>
        <Centered icon={<HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />}>
          {t('dashboard.facility.loadError')}
        </Centered>
      </PanelShell>
    );
  }
  if (!device) {
    return (
      <PanelShell title={title}>
        <Centered icon={<HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />}>
          {t('dashboard.facility.deviceNotFound')}
        </Centered>
      </PanelShell>
    );
  }

  const stationName = entry?.display_name || device.station || '-';
  const placeName =
    entry?.places.find((p) => p.place === device.place)?.display_name || device.place || '-';
  const fanDisabled = !device.power; // 전원 OFF → 풍량 제어 비활성(REQ-03-03, UB-005)
  const isPending = setPower.isPending || setFanSpeed.isPending;
  const result = pickResult(lastAction, setPower, setFanSpeed);
  const activeError = lastAction === 'fan' ? setFanSpeed.error : setPower.error;

  const runPower = (nextPower: boolean) => {
    setLastAction('power');
    setPower.mutate({ device_id: device.device_id, power: nextPower });
  };
  const runFan = (fan_speed: number) => {
    setLastAction('fan');
    setFanSpeed.mutate({ device_id: device.device_id, fan_speed });
  };

  return (
    <PanelShell title={title} online={device.online}>
      {/* 상태 정보 */}
      <div className="grid grid-cols-2 gap-2 text-xs">
        <Field label={t('dashboard.facility.device.power')}>
          {device.power ? t('dashboard.facility.device.on') : t('dashboard.facility.device.off')}
        </Field>
        <Field label={t('dashboard.facility.device.fanSpeed')}>
          {device.power ? String(device.fan_speed) : '-'}
        </Field>
        <Field label={t('dashboard.facility.device.station')}>{stationName}</Field>
        <Field label={t('dashboard.facility.device.place')}>{placeName}</Field>
        <Field label={t('dashboard.facility.device.index')}>{String(device.index)}</Field>
        <Field label={t('dashboard.facility.device.online')}>
          {device.online ? t('dashboard.facility.device.online') : t('dashboard.facility.device.offline')}
        </Field>
      </div>

      {/* 제어 */}
      <div className="space-y-2">
        <span className="text-xs font-semibold text-(--color-text-secondary)">
          {t('dashboard.facility.device.control')}
        </span>
        <div className="flex flex-wrap items-center gap-1.5">
          <button
            type="button"
            data-testid="facility-device-power-on"
            onClick={() => runPower(true)}
            disabled={isPending || device.power}
            className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-40 dark:bg-blue-500"
            aria-label={t('dashboard.facility.control.powerOn')}
          >
            <Power className="h-3.5 w-3.5" />
            {t('dashboard.facility.control.powerOn')}
          </button>
          <button
            type="button"
            data-testid="facility-device-power-off"
            onClick={() => runPower(false)}
            disabled={isPending || !device.power}
            className="inline-flex items-center gap-1 rounded-md bg-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-300 disabled:opacity-40 dark:bg-slate-700 dark:text-slate-300"
            aria-label={t('dashboard.facility.control.powerOff')}
          >
            <Power className="h-3.5 w-3.5" />
            {t('dashboard.facility.control.powerOff')}
          </button>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <Fan className="h-3.5 w-3.5 text-(--color-text-muted)" />
          {[1, 2, 3].map((n) => (
            <button
              key={n}
              type="button"
              data-testid={`facility-device-fan-${n}`}
              onClick={() => runFan(n)}
              disabled={isPending || fanDisabled}
              title={fanDisabled ? t('dashboard.facility.device.fanDisabledHint') : undefined}
              className={cn(
                'rounded-md px-3 py-1.5 text-xs font-medium ring-1 transition-colors disabled:opacity-40',
                device.fan_speed === n && device.power
                  ? 'bg-blue-50 text-blue-600 ring-blue-500 dark:bg-blue-900/30 dark:text-blue-300'
                  : 'bg-(--color-bg-surface) text-(--color-text-secondary) ring-(--color-border-default) hover:bg-(--color-bg-elevated)',
              )}
              aria-label={`${t('dashboard.facility.device.fanSpeed')} ${n}`}
            >
              {n}
            </button>
          ))}
        </div>
        {fanDisabled && (
          <p className="text-[11px] text-(--color-text-muted)">
            {t('dashboard.facility.device.fanDisabledHint')}
          </p>
        )}
        {isPending && (
          <p role="status" className="text-[11px] text-(--color-text-muted)">
            {t('dashboard.facility.control.running')}
          </p>
        )}
        <ControlResultView response={result} />
        {activeError ? (
          <p className="rounded-lg bg-red-50 px-2.5 py-2 text-[11px] text-red-600 dark:bg-red-900/20 dark:text-red-400">
            {String((activeError as Error)?.message ?? activeError)}
          </p>
        ) : null}
      </div>
    </PanelShell>
  );
}

// ---- 로컬 프리미티브(패널 셸/필드/센터) ----

function PanelShell({
  title,
  online,
  children,
}: {
  title: string;
  online?: boolean;
  children: React.ReactNode;
}) {
  const { t } = useTranslation();
  const showTitle = usePanelTitleVisible();
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 rounded-lg bg-(--color-bg-surface) p-4 shadow">
      {showTitle && (
      <div className="flex shrink-0 items-center justify-between">
        <span className="truncate text-sm font-medium text-(--color-text-primary)">{title}</span>
        {online !== undefined && (
          <span
            className={cn(
              'inline-flex items-center gap-1 rounded-full px-2 py-1',
              online
                ? 'bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400'
                : 'bg-slate-100 text-slate-400 dark:bg-slate-800 dark:text-slate-500',
            )}
          >
            {online ? (
              <Activity className="h-3.5 w-3.5" aria-label={t('dashboard.facility.device.online')} />
            ) : (
              <Moon className="h-3.5 w-3.5" aria-label={t('dashboard.facility.device.offline')} />
            )}
          </span>
        )}
      </div>
      )}
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto">{children}</div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-(--color-border-default) px-2.5 py-1.5">
      <p className="text-[10px] text-(--color-text-muted)">{label}</p>
      <p className="mt-0.5 text-sm font-medium text-(--color-text-primary)">{children}</p>
    </div>
  );
}

function Centered({ icon, children }: { icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center">
      {icon}
      <p className="text-xs text-(--color-text-muted)">{children}</p>
    </div>
  );
}
