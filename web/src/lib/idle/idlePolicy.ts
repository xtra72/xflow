// 유휴 로그아웃의 시간 판정 (@SPEC:SPEC-AUTH-IDLE-001).
//
// 타이머·이벤트·스토리지에서 떼어 낸 순수 층이다. "지금 몇 시이고 마지막 활동이
// 언제였는가" 만 받아 국면을 돌려준다 — 시험이 시계를 붙잡을 필요가 없다.

/** 유휴 설정. 서버 전역 1벌로 저장되며 value 는 이 모양의 JSON 이다. */
export interface IdleLogoutSetting {
  /** 기능 사용 여부. 기본 켜짐. */
  enabled: boolean;
  /** 유휴 한도(분). 이 시간 동안 사용자 입력이 없으면 로그아웃한다. */
  timeoutMinutes: number;
}

/** 저장된 값이 없거나 읽을 수 없을 때 쓰는 기본값. */
export const DEFAULT_IDLE_SETTING: IdleLogoutSetting = {
  enabled: true,
  timeoutMinutes: 10,
};

/** 설정 가능한 유휴 한도의 하한/상한(분). */
export const MIN_TIMEOUT_MINUTES = 1;
export const MAX_TIMEOUT_MINUTES = 480; // 8시간

/** 경고를 띄우는 기본 선행 시간(ms). */
const DEFAULT_WARN_LEAD_MS = 60_000;

/** 유휴 국면. */
export type IdlePhase = 'active' | 'warning' | 'expired';

export interface IdleEvaluation {
  phase: IdlePhase;
  /** 로그아웃까지 남은 시간(ms). 만료 시 0. */
  remainingMs: number;
}

/**
 * 알 수 없는 JSON 을 설정으로 좁힌다. 서버는 스키마를 검증하지 않으므로
 * (불투명 JSON 저장소) 읽는 쪽이 전적으로 방어한다.
 *
 * 부분적으로 망가진 값도 **필드 단위로** 살린다 — enabled 만 멀쩡하면 그것은 쓰고
 * timeoutMinutes 만 기본값으로 돌린다. 한 필드가 깨졌다고 다른 필드까지 버리면,
 * 관리자가 저장한 정책이 통째로 사라져 놓고 화면은 아무 말도 하지 않는다.
 */
export function normalizeIdleSetting(raw: unknown): IdleLogoutSetting {
  if (!raw || typeof raw !== 'object') return { ...DEFAULT_IDLE_SETTING };
  const v = raw as Partial<Record<keyof IdleLogoutSetting, unknown>>;

  const enabled = typeof v.enabled === 'boolean' ? v.enabled : DEFAULT_IDLE_SETTING.enabled;

  const minutes =
    typeof v.timeoutMinutes === 'number' && Number.isFinite(v.timeoutMinutes)
      ? clampTimeoutMinutes(v.timeoutMinutes)
      : DEFAULT_IDLE_SETTING.timeoutMinutes;

  return { enabled, timeoutMinutes: minutes };
}

/**
 * 유휴 한도를 허용 범위로 자른다(소수점은 버린다).
 *
 * NaN/Infinity 는 범위 비교가 **둘 다 거짓**이라 그냥 통과해 버린다. 그렇게 새어
 * 나간 NaN 은 한도 ms 를 NaN 으로 만들고, 그러면 남은 시간도 NaN 이라 어떤 비교도
 * 참이 되지 않아 **기능이 조용히 꺼진다** — 잠긴다고 믿는 화면이 영원히 열려 있는
 * 것이 이 결함의 모습이다. 그래서 수가 아닌 것은 기본 정책으로 돌린다. 하한(1분)이
 * 아니라 기본값인 까닭은, 읽을 수 없는 값 때문에 관리자가 정하지 않은 가장 공격적인
 * 정책이 적용되면 그것대로 사고이기 때문이다.
 */
export function clampTimeoutMinutes(minutes: number): number {
  if (!Number.isFinite(minutes)) return DEFAULT_IDLE_SETTING.timeoutMinutes;
  const floored = Math.floor(minutes);
  if (floored < MIN_TIMEOUT_MINUTES) return MIN_TIMEOUT_MINUTES;
  if (floored > MAX_TIMEOUT_MINUTES) return MAX_TIMEOUT_MINUTES;
  return floored;
}

/**
 * 경고 선행 시간(ms)을 정한다.
 *
 * 기본은 1분이지만, 한도가 짧으면 경고가 한도를 집어삼킨다 — 1분 설정에 1분 경고면
 * 창이 뜨는 순간이 곧 만료다. 그래서 한도의 절반을 넘지 않게 한다(1분 → 30초).
 */
export function warnLeadMs(timeoutMinutes: number): number {
  const timeoutMs = clampTimeoutMinutes(timeoutMinutes) * 60_000;
  return Math.min(DEFAULT_WARN_LEAD_MS, Math.floor(timeoutMs / 2));
}

/**
 * 지금이 어느 국면인지 판정한다.
 *
 * `lastActivityAt` 이 미래면(시계 역행·다른 탭의 시계 차이) 음수 경과가 나오는데,
 * 그것은 "방금 활동함" 과 같은 뜻이므로 active 로 읽힌다 — 패닉도, 즉시 로그아웃도
 * 아니다. 절전에서 깨어나 몇 시간이 지난 경우는 경과가 한도를 훌쩍 넘으므로
 * 곧바로 expired 다(의도한 동작 — 자리를 비운 것이 맞다).
 */
export function evaluateIdle(params: {
  now: number;
  lastActivityAt: number;
  timeoutMinutes: number;
}): IdleEvaluation {
  const timeoutMs = clampTimeoutMinutes(params.timeoutMinutes) * 60_000;
  const elapsed = params.now - params.lastActivityAt;
  const remainingMs = timeoutMs - elapsed;

  if (remainingMs <= 0) return { phase: 'expired', remainingMs: 0 };
  if (remainingMs <= warnLeadMs(params.timeoutMinutes)) return { phase: 'warning', remainingMs };
  return { phase: 'active', remainingMs };
}

/**
 * 남은 ms 를 `m:ss` 로 적는다. 음수는 0으로 본다 — 카운트다운이 음수를 보이면
 * 이미 로그아웃되었어야 할 화면이 아직 떠 있다는 뜻이라 사용자를 혼란스럽게 한다.
 */
export function formatRemaining(remainingMs: number): string {
  const totalSeconds = Math.max(0, Math.ceil(remainingMs / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${String(seconds).padStart(2, '0')}`;
}
