// Facility 역사 패널 (SPEC-FACILITY-DASHBOARD-001 M3, REQ-FACDASH-001-02-*, 06-*).
//
// config 의 agentId + station 으로 로스터를 조회해 한 역사의 (1) 통계, (2) 기기별 상태 목록,
// (3) 역사 일괄 제어(station 셀렉터 fan-out)를 렌더한다. 집계는 stationSummary(롤업만, UB-001),
// fan-out 은 useXsfmControl(FacilityBulkControl)을 호출만 한다. 미등록 역사/기기 없음은
// 안내 + 일괄 제어 비활성(REQ-02-05, 06-03).

import { useState } from 'react';

import { Activity, HardDrive, Moon } from 'lucide-react';

import {
  isFanOutResponse,
  useXsfmControl,
  useFacilityRoster,
  type ControlResponse,
  type SingleDeviceResult,
} from '@/hooks/useXsfmControl';
import type { AirDevice, AirStation } from '@/hooks/useStation';
import { stationSummary } from '@/lib/facilityAggregation';
import { useTranslation } from '@/lib/i18n';
import {
  FacilityBulkControl,
  FacilityControlButtons,
  StatTiles,
  type FanActive,
} from './facilityShared';

/** 개별 기기 목록 라벨 표시 방식(config-only, UB-003). */
export type DeviceLabelMode = 'placeIndex' | 'name';

/**
 * 기기 목록/개별 제어 헤더의 라벨을 config 옵션에 따라 해석한다(표시 전용).
 *   - 'placeIndex'(기본): place 코드 → 레지스트리 display_name 해석 + '-' + index
 *     (예: "상행 1번 승강장-1"). place 미해석 시 place 코드 → name → device_id 순 폴백.
 *   - 'name': 기기 name 필드(list_devices 조합 이름). 없으면 place 표시명 → device_id 폴백.
 */
function resolveDeviceLabel(
  device: AirDevice,
  entry: AirStation | undefined,
  mode: DeviceLabelMode,
): string {
  const placeDisplay =
    entry?.places.find((p) => p.place === device.place)?.display_name || device.place;
  if (mode === 'name') {
    return device.name || placeDisplay || device.device_id;
  }
  const base = placeDisplay || device.name || device.device_id;
  return `${base}-${device.index}`;
}

/**
 * 기기의 현재 (전원, 풍량) 상태를 제어 버튼 강조 대상으로 변환한다.
 * 오프라인 기기는 실제 상태를 신뢰할 수 없으므로 기본적으로 강조하지 않지만(중립), offlineAsOff
 * 옵션이 켜지면 오프라인을 꺼짐으로 간주해 OFF 버튼을 강조한다(item 2). 전원 OFF → 'off',
 * 전원 ON + fan_speed 1/2/3 → 해당 레벨. 그 외(방어값) → 강조 없음.
 */
function deviceActive(device: AirDevice, offlineAsOff: boolean): FanActive {
  if (!device.online) return offlineAsOff ? 'off' : null;
  if (!device.power) return 'off';
  if (device.fan_speed === 1) return 1;
  if (device.fan_speed === 2) return 2;
  if (device.fan_speed === 3) return 3;
  return null;
}

/** setPower/setFanSpeed 뮤테이션의 진행 상태 관찰(스피너 대상 버튼 도출용). */
type PendingView = {
  isPending: boolean;
  variables?: { power?: boolean; fan_speed?: number } | undefined;
};

/**
 * 현재 응답 대기(로딩) 중인 제어 버튼을 뮤테이션 상태에서 도출한다(item 3). 클릭한 버튼 = 진행 중인
 * 뮤테이션의 variables(전원 OFF → 'off', 풍량 N → N)로 식별한다. 진행 중이 아니면 null(스피너 없음).
 */
function pendingButton(setPower: PendingView, setFanSpeed: PendingView): FanActive {
  if (setPower.isPending) return setPower.variables?.power === false ? 'off' : null;
  if (setFanSpeed.isPending) {
    const fs = setFanSpeed.variables?.fan_speed;
    return fs === 1 || fs === 2 || fs === 3 ? fs : null;
  }
  return null;
}

/**
 * 제어 실패(에러/타임아웃) 메시지를 도출한다(UB-002 — 무음 금지, item 4). 뮤테이션 예외(activeError)가
 * 우선하고, 단일 기기 응답(SingleDeviceResult)의 status 가 timeout/error 이면 그 사유를 문구로 만든다.
 * 성공(ok)/미실행/fan-out 응답이면 null(문구 없음 — 성공은 조용히 통과).
 */
function failureMessage(
  activeError: unknown,
  result: ControlResponse | undefined,
  t: (k: string) => string,
): string | null {
  if (activeError) return String((activeError as Error)?.message ?? activeError);
  if (!result || isFanOutResponse(result)) return null;
  const single = result as SingleDeviceResult;
  const status = String(single.status);
  if (status === 'timeout') return t('dashboard.facility.device.result.timeout');
  if (status !== 'ok') {
    const detail = typeof single.error === 'string' ? single.error : status;
    return `${t('dashboard.facility.device.result.error')} ${detail}`;
  }
  return null;
}

interface FacilityStationPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

/** Facility 역사 패널. */
export default function FacilityStationPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: FacilityStationPanelProps) {
  const { t } = useTranslation();
  const agentId = (config.agentId as string | undefined) ?? '';
  const station = config.station as string | undefined;
  const refreshMs = config.refreshMs as number | undefined;
  // C4: 역사 통계(StatTiles) 표시 여부(config, 영속, 기본 true).
  const showStats = (config.showStats as boolean | undefined) ?? true;
  // 개별 기기 라벨 표시 방식(config, 영속, 기본 placeIndex).
  const deviceLabelMode = (config.deviceLabelMode as DeviceLabelMode | undefined) ?? 'placeIndex';
  // 오프라인을 꺼짐으로 표시(config, 영속, 기본 false — item 2). 라인 패널과 동일한 옵션.
  const offlineAsOff = (config.offlineAsOff as boolean | undefined) ?? false;

  const { devices, stations, isLoading, isError } = useFacilityRoster(agentId, refreshMs);

  if (!station || !agentId) {
    return <Shell title={title}>{notice(t('dashboard.facility.notConfigured'))}</Shell>;
  }
  if (isLoading) {
    return (
      <Shell title={title}>
        <div className="flex flex-1 items-center justify-center">
          <div className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-default) border-t-blue-600" />
        </div>
      </Shell>
    );
  }
  if (isError) {
    return <Shell title={title}>{notice(t('dashboard.facility.loadError'))}</Shell>;
  }

  const entry = stations.find((s) => s.station === station);
  const stationDevices = devices.filter((d) => d.station === station);
  const summary = stationSummary(station, stationDevices, entry);
  const unregistered = !entry;
  const empty = stationDevices.length === 0;

  return (
    // C1: 우상단 식별자는 호선(line)만 표시한다(레지스트리 해석값).
    <Shell title={title} line={summary.line}>
      {/* 미등록/기기 없음 안내(REQ-02-05) */}
      {unregistered && (
        <p className="rounded-lg bg-amber-50 px-2.5 py-1.5 text-[11px] text-amber-600 dark:bg-amber-900/20 dark:text-amber-400">
          {t('dashboard.facility.unregisteredStation')}
        </p>
      )}
      {empty && (
        <p className="rounded-lg bg-(--color-bg-elevated) px-2.5 py-1.5 text-[11px] text-(--color-text-muted)">
          {t('dashboard.facility.noDevices')}
        </p>
      )}

      {/* (1) 역사 통계(C4: showStats 로 토글). */}
      {showStats && <StatTiles stats={summary.stats} />}

      {/* (2) 기기별 상태 목록 + 목록 제목 우측 일괄 제어(C5). */}
      <div className="space-y-1">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-semibold text-(--color-text-secondary)">
            {t('dashboard.facility.station.devices')}
          </span>
          {/* C5: 역사 일괄 제어(station 셀렉터)를 "기기 목록" 제목 우측에 인라인 배치. 미등록 비활성. */}
          <FacilityBulkControl
            agentId={agentId}
            selector={{ station }}
            memberCount={stationDevices.length}
            disabled={unregistered}
          />
        </div>
        {stationDevices.length > 0 ? (
          <ul className="space-y-1" data-testid="station-device-list">
            {stationDevices.map((d) => (
              <StationDeviceRow
                key={d.device_id}
                agentId={agentId}
                device={d}
                entry={entry}
                labelMode={deviceLabelMode}
                offlineAsOff={offlineAsOff}
              />
            ))}
          </ul>
        ) : (
          <p className="text-[11px] text-(--color-text-muted)">{t('dashboard.facility.noDevices')}</p>
        )}
      </div>
    </Shell>
  );
}

// ---- 기기 행(이름 표시 + 개별 제어) ----

/** 마지막 실행 뮤테이션 응답을 고른다(이 행의 버튼을 눌렀을 때만 결과를 노출). */
function pickRowResult(
  lastAction: 'power' | 'fan' | null,
  power: { data?: ControlResponse },
  fan: { data?: ControlResponse },
): ControlResponse | undefined {
  if (lastAction === 'power') return power.data;
  if (lastAction === 'fan') return fan.data;
  return undefined;
}

/**
 * 역사 기기 목록의 한 행(C2/C3). 기기 이름(name = station:place:index 조합, 없으면 위치 표시명
 * → device_id 폴백)을 주 라벨로 크게 보여주고, 각 행에 개별 제어 버튼(OFF + 풍량 1/2/3)을 둔다.
 * 제어는 useXsfmControl 을 {device_id} 셀렉터로 호출하며(단일 응답), 결과/진행을 인라인
 * 표시한다(UB-002). 전원 OFF 상태에서는 풍량 버튼을 비활성화한다(UB-005 — 위장 없음).
 * 전원 ON 버튼은 제공하지 않는다(A1 정책 일치 — OFF + 풍량만).
 */
function StationDeviceRow({
  agentId,
  device,
  entry,
  labelMode,
  offlineAsOff,
}: {
  agentId: string;
  device: AirDevice;
  entry?: AirStation;
  labelMode: DeviceLabelMode;
  offlineAsOff: boolean;
}) {
  const { t } = useTranslation();
  const { setPower, setFanSpeed } = useXsfmControl(agentId);
  const [lastAction, setLastAction] = useState<'power' | 'fan' | null>(null);

  const primaryLabel = resolveDeviceLabel(device, entry, labelMode);
  const fanDisabled = !device.power; // 전원 OFF → 풍량 제어 비활성(UB-005)
  const isPending = setPower.isPending || setFanSpeed.isPending;
  // 진행 중인(클릭한) 버튼 — 그 버튼 위에 스피너 표시(item 3).
  const pending = pendingButton(setPower, setFanSpeed);
  const result = pickRowResult(lastAction, setPower, setFanSpeed);
  const activeError = lastAction === 'fan' ? setFanSpeed.error : lastAction === 'power' ? setPower.error : null;
  // 실패(에러/타임아웃) 문구 — 성공/미실행이면 null(item 4). 상태 정보 텍스트 뒤에 인라인 표시한다.
  const failure = failureMessage(activeError, result, t);

  // 오프라인을 꺼짐으로 표시(item 2): showAsOff 이면 상태 텍스트를 꺼짐으로 바꾼다.
  const showAsOff = offlineAsOff && !device.online;
  const statusText = showAsOff
    ? t('dashboard.facility.device.off')
    : device.power
      ? `${t('dashboard.facility.device.on')} · ${t('dashboard.facility.device.fanSpeed')} ${device.fan_speed}`
      : t('dashboard.facility.device.off');

  const runPower = (power: boolean) => {
    setLastAction('power');
    setPower.mutate({ device_id: device.device_id, power });
  };
  const runFan = (fan_speed: number) => {
    setLastAction('fan');
    setFanSpeed.mutate({ device_id: device.device_id, fan_speed });
  };

  return (
    <li className="space-y-1.5 rounded-lg border border-(--color-border-default) px-2.5 py-2">
      <div className="flex items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-(--color-text-primary)" title={primaryLabel}>
            {primaryLabel}
          </p>
          {/* 상태 정보 텍스트(온라인/전원/풍량). showAsOff 이면 오프라인을 꺼짐으로 표기. */}
          <p
            data-testid={`facility-device-status-${device.device_id}`}
            className="mt-0.5 flex items-center gap-1 text-[11px] text-(--color-text-muted)"
          >
            {device.online ? (
              <Activity className="h-3 w-3" aria-label={t('dashboard.facility.device.online')} />
            ) : (
              <Moon
                className="h-3 w-3"
                aria-label={
                  showAsOff ? t('dashboard.facility.device.off') : t('dashboard.facility.device.offline')
                }
              />
            )}
            <span>{statusText}</span>
          </p>
          {/* 실패 메시지(item 4): 상태 정보 텍스트 바로 뒤에 인라인 표시(UB-002 — 무음 금지). */}
          {failure ? (
            <p
              data-testid={`facility-device-failure-${device.device_id}`}
              className="mt-0.5 text-[11px] text-red-600 dark:text-red-400"
            >
              {failure}
            </p>
          ) : null}
        </div>

        {/* 개별 제어: 일괄 제어와 동일한 공용 버튼 행(OFF + 풍량 1/2/3). 현재 상태를 레벨 색상으로
            강조하고, 진행 중인 버튼에는 스피너를 얹는다(item 3). */}
        <div className="shrink-0">
          <FacilityControlButtons
            onPowerOff={() => runPower(false)}
            onFan={runFan}
            disabled={isPending}
            offDisabled={!device.power}
            fanDisabled={fanDisabled}
            fanTitle={fanDisabled ? t('dashboard.facility.device.fanDisabledHint') : undefined}
            active={deviceActive(device, offlineAsOff)}
            pending={pending}
            powerOffTestId={`facility-device-power-off-${device.device_id}`}
            fanTestId={(n) => `facility-device-fan-${n}-${device.device_id}`}
          />
        </div>
      </div>
    </li>
  );
}

// ---- 로컬 셸 ----

function Shell({
  title,
  line,
  children,
}: {
  title: string;
  line?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 rounded-lg bg-(--color-bg-surface) p-4 shadow">
      <div className="flex shrink-0 items-baseline justify-between gap-2">
        <span className="truncate text-sm font-medium text-(--color-text-primary)">{title}</span>
        {/* C1: 우상단 식별자는 호선만 표시. */}
        {line && <span className="shrink-0 truncate text-xs text-(--color-text-muted)">{line}</span>}
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto">{children}</div>
    </div>
  );
}

function notice(text: string) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center">
      <HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />
      <p className="text-xs text-(--color-text-muted)">{text}</p>
    </div>
  );
}
