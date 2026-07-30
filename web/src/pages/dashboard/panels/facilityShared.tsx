// Facility 대시보드 패널 공용 서브컴포넌트 (SPEC-FACILITY-DASHBOARD-001 B2).
//
// 3종 패널(라인/역사/기기)이 공유하는 표시·제어 UI 를 모아 중복을 제거한다(단순성 원칙).
//   - StatTiles: StatCounts(총/온라인/오프라인/전원/풍량 분포)를 라벨/값 타일 그리드로 표시.
//   - ControlResultView: 제어 응답을 렌더한다. fan-out 응답은 멤버별 ok/timeout/error 요약 +
//     실패·타임아웃 상세를(REQ-06-02), 단일 device 응답은 ok/timeout/error 를 명시 표시(REQ-03-02).
//     UB-002: 결과를 소리 없이 누락하지 않는다.
//   - FacilityBulkControl: 셀렉터(station/line) fan-out 일괄 제어. fan-out 로직은 재구현하지
//     않고(UB-001) useXsfmControl 을 호출만 한다. 빈 대상/미등록 시 비활성(REQ-06-03).

import { useState } from 'react';

import { Activity, Moon } from 'lucide-react';

import {
  isFanOutResponse,
  useXsfmControl,
  type ControlResponse,
  type SetFanSpeedVariables,
  type SetPowerVariables,
  type SingleDeviceResult,
} from '@/hooks/useXsfmControl';
import type { AirDevice, AirStation } from '@/hooks/useStation';
import type { StatCounts } from '@/lib/facilityAggregation';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

// ---- StatTiles ----

/** StatCounts 를 8개 label/value 타일로 표시(총/온라인/오프라인/가동/정지/풍량1-3). */
export function StatTiles({ stats }: { stats: StatCounts }) {
  const { t } = useTranslation();
  const tiles: { key: string; label: string; value: number }[] = [
    { key: 'total', label: t('dashboard.facility.stat.total'), value: stats.total },
    { key: 'online', label: t('dashboard.facility.stat.online'), value: stats.online },
    { key: 'offline', label: t('dashboard.facility.stat.offline'), value: stats.offline },
    { key: 'powerOn', label: t('dashboard.facility.stat.powerOn'), value: stats.powerOn },
    { key: 'powerOff', label: t('dashboard.facility.stat.powerOff'), value: stats.powerOff },
    { key: 'fan1', label: t('dashboard.facility.stat.fan1'), value: stats.fan1 },
    { key: 'fan2', label: t('dashboard.facility.stat.fan2'), value: stats.fan2 },
    { key: 'fan3', label: t('dashboard.facility.stat.fan3'), value: stats.fan3 },
  ];
  return (
    <div className="grid grid-cols-4 gap-2">
      {tiles.map((tile) => (
        <div
          key={tile.key}
          data-testid={`stat-tile-${tile.key}`}
          className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1.5 text-center"
        >
          <p className="truncate text-[10px] text-(--color-text-muted)">{tile.label}</p>
          <p className="text-sm font-semibold tabular-nums text-(--color-text-primary)">{tile.value}</p>
        </div>
      ))}
    </div>
  );
}

// ---- ControlResultView ----

/** 제어 응답 상태 종류로 정규화(단일/멤버 공용). */
function statusKind(status: string): 'ok' | 'timeout' | 'error' {
  if (status === 'ok') return 'ok';
  if (status === 'timeout') return 'timeout';
  return 'error';
}

/**
 * 제어 응답 렌더러(UB-002 — 항상 표시). fan-out 이면 멤버별 ok/timeout/error 수 요약 +
 * 실패·타임아웃 멤버 상세(device_id + 상태)를(REQ-06-02), 단일 device 응답이면 ok(성공, 에코
 * 반영)/timeout(응답 시간 초과)/error 를 명시한다(REQ-03-02). response 미지정 시 렌더 없음.
 */
export function ControlResultView({ response }: { response?: ControlResponse }) {
  const { t } = useTranslation();
  if (!response) return null;

  if (isFanOutResponse(response)) {
    const ok = response.results.filter((r) => r.status === 'ok').length;
    const timeout = response.results.filter((r) => r.status === 'timeout').length;
    const error = response.results.filter((r) => r.status === 'error').length;
    const failed = response.results.filter((r) => r.status !== 'ok');
    return (
      <div
        role="status"
        data-testid="control-result"
        className="space-y-1 rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) px-2.5 py-2"
      >
        <div className="flex flex-wrap gap-x-3 gap-y-0.5 text-[11px]">
          <span className="text-green-600 dark:text-green-400">
            {t('dashboard.facility.result.memberOk')}{' '}
            <b className="tabular-nums" data-testid="result-ok">{ok}</b>
          </span>
          <span className="text-amber-600 dark:text-amber-400">
            {t('dashboard.facility.result.memberTimeout')}{' '}
            <b className="tabular-nums" data-testid="result-timeout">{timeout}</b>
          </span>
          <span className="text-red-600 dark:text-red-400">
            {t('dashboard.facility.result.memberError')}{' '}
            <b className="tabular-nums" data-testid="result-error">{error}</b>
          </span>
          {response.excluded && response.excluded.length > 0 ? (
            <span className="text-(--color-text-muted)">
              {t('dashboard.facility.result.excluded')}{' '}
              <b className="tabular-nums">{response.excluded.length}</b>
            </span>
          ) : null}
        </div>
        {failed.length > 0 && (
          <ul className="space-y-0.5 text-[10px]" data-testid="result-failed-list">
            {failed.map((m) => (
              <li key={m.device_id} className="flex items-center justify-between gap-2">
                <span className="truncate font-mono text-(--color-text-secondary)" title={m.device_id}>
                  {m.device_id}
                </span>
                <span
                  className={cn(
                    'shrink-0',
                    m.status === 'timeout'
                      ? 'text-amber-600 dark:text-amber-400'
                      : 'text-red-600 dark:text-red-400',
                  )}
                >
                  {m.status === 'timeout'
                    ? t('dashboard.facility.result.memberTimeout')
                    : t('dashboard.facility.result.memberError')}
                  {m.error ? ` (${m.error})` : ''}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    );
  }

  // 단일 device 응답: ok / timeout / error 명시(REQ-03-02, UB-005 — 위장 없음).
  const single = response as SingleDeviceResult;
  const kind = statusKind(String(single.status));
  const errText = typeof single.error === 'string' ? single.error : String(single.status);
  return (
    <div
      role="status"
      data-testid="control-result"
      className={cn(
        'rounded-lg px-2.5 py-2 text-xs',
        kind === 'ok' && 'bg-green-50 text-green-600 dark:bg-green-900/20 dark:text-green-400',
        kind === 'timeout' && 'bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-400',
        kind === 'error' && 'bg-red-50 text-red-600 dark:bg-red-900/20 dark:text-red-400',
      )}
    >
      {kind === 'ok' && t('dashboard.facility.device.result.ok')}
      {kind === 'timeout' && t('dashboard.facility.device.result.timeout')}
      {kind === 'error' && (
        <span>
          {t('dashboard.facility.device.result.error')} <span className="font-mono">{errText}</span>
        </span>
      )}
    </div>
  );
}

// ---- FacilityControlButtons (공용 제어 버튼 행) ----

/**
 * 제어 버튼 행에서 현재 상태로 강조할 대상.
 *   - 'off': 전원 OFF 버튼 강조
 *   - 1|2|3: 해당 풍량 버튼을 레벨 색상으로 강조(1단 노랑·2단 초록·3단 파랑)
 *   - null: 강조 없음(일괄 제어 또는 오프라인 기기)
 */
export type FanActive = 'off' | 1 | 2 | 3 | null;

/** 공용 버튼 base(크기/모양). 일괄·개별 제어가 동일 스타일을 쓰도록 단일 출처로 둔다. */
const CONTROL_BTN_BASE =
  'rounded-md px-2.5 py-1 text-xs font-medium ring-1 transition-colors disabled:opacity-40';

/** OFF 버튼 색상(비활성 base / 현재 상태 활성). */
const OFF_BASE =
  'bg-slate-200 text-slate-600 ring-transparent hover:bg-slate-300 dark:bg-slate-700 dark:text-slate-300';
const OFF_ACTIVE =
  'bg-slate-300 text-slate-800 ring-slate-500 dark:bg-slate-600 dark:text-slate-100 dark:ring-slate-400';

/** 풍량 버튼 비활성 base 색상. */
const FAN_BASE =
  'bg-(--color-bg-elevated) text-(--color-text-secondary) ring-(--color-border-default) hover:bg-(--color-border-default)';

/** 풍량 단계별 활성 색상(배지 팔레트와 일치: 1단 노랑·2단 초록·3단 파랑). */
const FAN_ACTIVE: Record<1 | 2 | 3, string> = {
  1: 'bg-yellow-50 text-yellow-700 ring-yellow-500 dark:bg-yellow-900/30 dark:text-yellow-300',
  2: 'bg-green-50 text-green-700 ring-green-500 dark:bg-green-900/30 dark:text-green-300',
  3: 'bg-blue-50 text-blue-600 ring-blue-500 dark:bg-blue-900/30 dark:text-blue-300',
};

interface FacilityControlButtonsProps {
  /** OFF(전원 끄기) 클릭. */
  onPowerOff: () => void;
  /** 풍량 N 클릭. */
  onFan: (n: number) => void;
  /** 전체 비활성(진행 중/빈 대상/미등록). */
  disabled?: boolean;
  /** OFF 버튼 추가 비활성(이미 꺼짐). */
  offDisabled?: boolean;
  /** 풍량 버튼 추가 비활성(전원 OFF — UB-005). */
  fanDisabled?: boolean;
  /** OFF 버튼 title(hint). */
  offTitle?: string;
  /** 풍량 버튼 title(hint). */
  fanTitle?: string;
  /** 현재 상태로 강조할 버튼(OFF 또는 풍량 N). 일괄 제어는 null. */
  active?: FanActive;
  /**
   * 응답 대기 중(로딩)인 버튼(클릭한 버튼). 해당 버튼 위에 스피너를 얹어 어떤 제어가 진행
   * 중인지 버튼 자체로 표시한다(C item 3). null 이면 스피너 없음(일괄 제어는 미사용).
   */
  pending?: FanActive;
  /** testid: OFF 버튼. */
  powerOffTestId: string;
  /** testid: 풍량 N 버튼(1/2/3). */
  fanTestId: (n: number) => string;
}

/** 제어 버튼 내부 로딩 스피너(진행 중 버튼에 얹는다). 버튼 텍스트를 대체한다. */
function ControlSpinner() {
  return (
    <span
      data-testid="control-btn-spinner"
      aria-hidden="true"
      className="inline-block h-3 w-3 animate-spin rounded-full border-2 border-current border-t-transparent align-middle"
    />
  );
}

/**
 * 공용 제어 버튼 행(OFF + 풍량 1/2/3). 일괄 제어(FacilityBulkControl)와 개별 기기 행이 동일한
 * 시각을 쓰도록 버튼 스타일을 한곳으로 모은다(단순성 원칙). 전원 켜기(ON) 버튼은 제공하지 않는다
 * (패널 정책 A1). `active` 로 현재 상태 버튼을 레벨 색상(1단 노랑·2단 초록·3단 파랑)/OFF 로 강조한다.
 */
export function FacilityControlButtons({
  onPowerOff,
  onFan,
  disabled,
  offDisabled,
  fanDisabled,
  offTitle,
  fanTitle,
  active = null,
  pending = null,
  powerOffTestId,
  fanTestId,
}: FacilityControlButtonsProps) {
  const { t } = useTranslation();
  const offActive = active === 'off';
  const offPending = pending === 'off';
  return (
    <div className="flex flex-wrap items-center gap-1">
      <button
        type="button"
        data-testid={powerOffTestId}
        onClick={onPowerOff}
        disabled={Boolean(disabled) || Boolean(offDisabled)}
        title={offTitle}
        aria-pressed={offActive}
        aria-busy={offPending}
        className={cn(CONTROL_BTN_BASE, offActive ? OFF_ACTIVE : OFF_BASE)}
        aria-label={t('dashboard.facility.control.off')}
      >
        {offPending ? <ControlSpinner /> : t('dashboard.facility.control.off')}
      </button>
      {[1, 2, 3].map((n) => {
        const isActive = active === n;
        const isPending = pending === n;
        return (
          <button
            key={n}
            type="button"
            data-testid={fanTestId(n)}
            onClick={() => onFan(n)}
            disabled={Boolean(disabled) || Boolean(fanDisabled)}
            title={fanTitle}
            aria-pressed={isActive}
            aria-busy={isPending}
            className={cn(CONTROL_BTN_BASE, isActive ? FAN_ACTIVE[n as 1 | 2 | 3] : FAN_BASE)}
            aria-label={`${t('dashboard.facility.control.fan')} ${n}`}
          >
            {isPending ? <ControlSpinner /> : `${t('dashboard.facility.control.fan')} ${n}`}
          </button>
        );
      })}
    </div>
  );
}

// ---- FacilityBulkControl ----

/** setPower / setFanSpeed 뮤테이션의 공통 관찰 필드(변수 타입이 달라 구조적 뷰로 좁힌다). */
type MutationView = { data?: ControlResponse; isPending: boolean; error: unknown };

/**
 * 마지막으로 실행한 뮤테이션을 고른다. 명시적 클릭(lastAction)이 우선하고, 초기 상태(액션 전)에는
 * 응답/진행/에러를 보유한 쪽으로 폴백한다(테스트에서 .data 만 세팅한 경우 렌더되도록).
 */
function pickActive(
  lastAction: 'power' | 'fan' | null,
  setPower: MutationView,
  setFanSpeed: MutationView,
): MutationView | undefined {
  if (lastAction === 'power') return setPower;
  if (lastAction === 'fan') return setFanSpeed;
  if (setPower.data || setPower.isPending || setPower.error) return setPower;
  if (setFanSpeed.data || setFanSpeed.isPending || setFanSpeed.error) return setFanSpeed;
  return undefined;
}

/** FacilityBulkControl 대상 셀렉터: 역사(station) · 라인(line) · 그룹(group_id).
 *  group_id 는 SPEC-XSFM-GROUP-001 M6/M7 에서 추가(그룹 탭·설비 그룹 패널 일괄 제어). */
export type BulkSelector = { station: string } | { line: string } | { group_id: string };

interface FacilityBulkControlProps {
  agentId: string;
  /** fan-out 대상 셀렉터(역사=station, 라인=line). fan-out 로직은 백엔드가 수행(UB-001). */
  selector: BulkSelector;
  /** 대상 멤버 기기 수. 0 이면 일괄 제어를 비활성화한다(REQ-06-03, 빈 대상 방지). */
  memberCount: number;
  /** 외부 사유(미등록 등)로 강제 비활성화(REQ-02-05 / 01-06). */
  disabled?: boolean;
}

/**
 * 셀렉터 fan-out 일괄 제어(set_power off + set_fan_speed 1/2/3). useXsfmControl 을
 * 호출만 하고 fan-out 을 재구현하지 않는다(UB-001). 진행 중 로딩 표시(REQ-06-04) + 멤버별 결과
 * 렌더(REQ-06-02, UB-002). 빈 대상(memberCount=0)/미등록(disabled) 시 컨트롤을 비활성화한다.
 *
 * 버튼 구성: OFF(전원 끄기) + 풍량 1/2/3. 전원 켜기(ON) 버튼은 제공하지 않는다(패널 정책).
 * 컴팩트 인라인 레이아웃(헤더/목록 제목 옆 배치용)이며, 결과/진행/에러는 버튼 아래
 * 팝오버로 표시해 UB-002(결과 무음 누락 금지)를 지킨다.
 */
export function FacilityBulkControl({ agentId, selector, memberCount, disabled }: FacilityBulkControlProps) {
  const { t } = useTranslation();
  const { setPower, setFanSpeed } = useXsfmControl(agentId);
  const [lastAction, setLastAction] = useState<'power' | 'fan' | null>(null);

  const isPending = setPower.isPending || setFanSpeed.isPending;
  const emptyTarget = memberCount === 0;
  const controlDisabled = Boolean(disabled) || emptyTarget || isPending;
  const active = pickActive(lastAction, setPower, setFanSpeed);
  const hasResult = isPending || Boolean(active?.data) || Boolean(active?.error);
  const emptyHint = emptyTarget && !disabled ? t('dashboard.facility.control.noTarget') : undefined;

  const runPower = (power: boolean) => {
    setLastAction('power');
    setPower.mutate({ ...selector, power } as SetPowerVariables);
  };
  const runFan = (fan_speed: number) => {
    setLastAction('fan');
    setFanSpeed.mutate({ ...selector, fan_speed } as SetFanSpeedVariables);
  };

  return (
    <div className="relative" data-testid="facility-bulk">
      {/* 공용 버튼 행: 일괄 제어는 단일 현재 상태가 없어 강조(active) 없이 표시. */}
      <FacilityControlButtons
        onPowerOff={() => runPower(false)}
        onFan={runFan}
        disabled={controlDisabled}
        offTitle={emptyHint}
        fanTitle={emptyHint}
        active={null}
        powerOffTestId="facility-bulk-power-off"
        fanTestId={(n) => `facility-bulk-fan-${n}`}
      />

      {hasResult && (
        <div
          data-testid="facility-bulk-result"
          className="absolute right-0 top-full z-20 mt-1 w-64 max-w-[80vw] space-y-1 rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-2 shadow-lg"
        >
          {isPending && (
            <p role="status" className="text-[11px] text-(--color-text-muted)">
              {t('dashboard.facility.control.running')}
            </p>
          )}
          <ControlResultView response={active?.data} />
          {active?.error ? (
            <p className="rounded-lg bg-red-50 px-2.5 py-1.5 text-[11px] text-red-600 dark:bg-red-900/20 dark:text-red-400">
              {String((active.error as Error)?.message ?? active.error)}
            </p>
          ) : null}
        </div>
      )}
    </div>
  );
}

// ---- FacilityDeviceRow (공용 기기별 개별 제어 행) ----
//
// 역사 패널·그룹 패널이 공유하는 "한 기기의 상태 + 개별 제어(OFF + 풍량 1/2/3)" 행이다.
// 제어는 useXsfmControl 을 {device_id} 셀렉터로 호출하며(단일 응답), 결과/진행/실패를 인라인으로
// 표시한다(UB-002). 전원 OFF 상태에서는 풍량 버튼을 비활성화한다(UB-005 — 위장 없음). 전원 켜기(ON)
// 버튼은 제공하지 않는다(패널 정책 A1 — OFF + 풍량만). 라벨은 config 옵션(placeIndex|name)으로 해석한다.

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
 * 공용 기기별 개별 제어 행(역사/그룹 패널 공유). 기기 이름(name = station:place:index 조합, 없으면
 * 위치 표시명 → device_id 폴백)을 주 라벨로 크게 보여주고, 각 행에 개별 제어 버튼(OFF + 풍량 1/2/3)을
 * 둔다. 제어는 useXsfmControl 을 {device_id} 셀렉터로 호출하며(단일 응답), 결과/진행을 인라인
 * 표시한다(UB-002). 전원 OFF 상태에서는 풍량 버튼을 비활성화한다(UB-005 — 위장 없음).
 * 전원 ON 버튼은 제공하지 않는다(A1 정책 일치 — OFF + 풍량만).
 */
export function FacilityDeviceRow({
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
