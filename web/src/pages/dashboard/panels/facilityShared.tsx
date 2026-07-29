// Facility 대시보드 패널 공용 서브컴포넌트 (SPEC-FACILITY-DASHBOARD-001 B2).
//
// 3종 패널(라인/역사/기기)이 공유하는 표시·제어 UI 를 모아 중복을 제거한다(단순성 원칙).
//   - StatTiles: StatCounts(총/온라인/오프라인/전원/풍량 분포)를 라벨/값 타일 그리드로 표시.
//   - ControlResultView: 제어 응답을 렌더한다. fan-out 응답은 멤버별 ok/timeout/error 요약 +
//     실패·타임아웃 상세를(REQ-06-02), 단일 device 응답은 ok/timeout/error 를 명시 표시(REQ-03-02).
//     UB-002: 결과를 소리 없이 누락하지 않는다.
//   - FacilityBulkControl: 셀렉터(station/line) fan-out 일괄 제어. fan-out 로직은 재구현하지
//     않고(UB-001) useAirpurifierControl 을 호출만 한다. 빈 대상/미등록 시 비활성(REQ-06-03).

import { useState } from 'react';

import {
  isFanOutResponse,
  useAirpurifierControl,
  type ControlResponse,
  type SetFanSpeedVariables,
  type SetPowerVariables,
  type SingleDeviceResult,
} from '@/hooks/useAirpurifierControl';
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

/** FacilityBulkControl 대상 셀렉터: 역사(station) 또는 라인(line). */
export type BulkSelector = { station: string } | { line: string };

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
 * 셀렉터 fan-out 일괄 제어(set_power off + set_fan_speed 1/2/3). useAirpurifierControl 을
 * 호출만 하고 fan-out 을 재구현하지 않는다(UB-001). 진행 중 로딩 표시(REQ-06-04) + 멤버별 결과
 * 렌더(REQ-06-02, UB-002). 빈 대상(memberCount=0)/미등록(disabled) 시 컨트롤을 비활성화한다.
 *
 * 버튼 구성: OFF(전원 끄기) + 풍량 1/2/3. 전원 켜기(ON) 버튼은 제공하지 않는다(패널 정책).
 * 컴팩트 인라인 레이아웃(헤더/목록 제목 옆 배치용)이며, 결과/진행/에러는 버튼 아래
 * 팝오버로 표시해 UB-002(결과 무음 누락 금지)를 지킨다.
 */
export function FacilityBulkControl({ agentId, selector, memberCount, disabled }: FacilityBulkControlProps) {
  const { t } = useTranslation();
  const { setPower, setFanSpeed } = useAirpurifierControl(agentId);
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
