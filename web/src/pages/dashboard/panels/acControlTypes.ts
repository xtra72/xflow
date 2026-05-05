// 에어컨 제어 패널 공유 타입 (AcControlPanel.tsx 와 acControlColors.ts 가 공유)

/** 운전 모드 — 백엔드 통일 컨벤션 (NASA/LGCP/LGAP/LGCNP 공통, SPEC §3 REQ-M3-04) */
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
