// 에어컨 제어 패널 공유 타입 (AcControlPanel.tsx 와 acControlColors.ts 가 공유)

/** 운전 모드 — 백엔드 통일 컨벤션 (NASA/LGCP/LGAP/LG ICP-01 공통, SPEC §3 REQ-M3-04) */
export type AcMode = 'cool' | 'heat' | 'auto' | 'dry' | 'fan';

/** 풍량 */
export type FanSpeed = 'auto' | 'low' | 'medium' | 'high' | 'quiet' | 'turbo';

export const AC_MODE_KEYS: AcMode[] = ['cool', 'heat', 'auto', 'dry', 'fan'];

export const FAN_SPEED_KEYS: FanSpeed[] = ['auto', 'quiet', 'low', 'medium', 'high', 'turbo'];

export const AC_MODE_LABELS: Record<AcMode, string> = {
  cool: '냉방',
  heat: '난방',
  auto: '자동',
  dry: '제습',
  fan: '팬',
};

export const FAN_SPEED_LABELS: Record<FanSpeed, string> = {
  auto: '자동',
  quiet: '미풍',
  low: '약',
  medium: '중',
  high: '강',
  turbo: '터보',
};

// v0.7.5+ 백엔드는 mode/fan_speed 를 hvac 통일 ID (int) 로 emit 한다
// (internal/agent/hvac/codes.go). 옛 string ("cool", "auto" 등) 도 지원해
// 호환성을 유지한다.

/** hvac 통일 mode ID → AcMode 문자열. */
const AC_MODE_ID_TO_STRING: Record<number, AcMode> = {
  0: 'auto', // off/auto
  1: 'cool',
  2: 'heat',
  3: 'dry',
  4: 'fan',
};

/** hvac 통일 fan_speed ID → FanSpeed 문자열. */
const FAN_SPEED_ID_TO_STRING: Record<number, FanSpeed> = {
  // 0 (off) 는 표시상 'auto' 로 매핑 — UI 에 "off" 상태는 power 로 표현.
  0: 'auto',
  1: 'auto',
  2: 'quiet',
  3: 'low',
  4: 'medium',
  5: 'high',
  6: 'turbo',
};

/** mode raw 값 (int / string) 을 표준 AcMode 문자열로 변환. */
export function normalizeAcMode(raw: unknown, fallback: AcMode = 'cool'): AcMode {
  if (typeof raw === 'number') return AC_MODE_ID_TO_STRING[raw] ?? fallback;
  if (typeof raw === 'string') {
    if (raw === 'cooling') return 'cool';
    if (raw === 'heating') return 'heat';
    if (raw === 'dehumidify') return 'dry';
    if (raw === 'off') return 'auto';
    return (raw as AcMode);
  }
  return fallback;
}

/** fan_speed raw 값 (int / string) 을 표준 FanSpeed 문자열로 변환. */
export function normalizeFanSpeed(raw: unknown, fallback: FanSpeed = 'auto'): FanSpeed {
  if (typeof raw === 'number') return FAN_SPEED_ID_TO_STRING[raw] ?? fallback;
  if (typeof raw === 'string') {
    if (raw === 'slow') return 'low';
    if (raw === 'off' || raw === '') return 'auto';
    return (raw as FanSpeed);
  }
  return fallback;
}
