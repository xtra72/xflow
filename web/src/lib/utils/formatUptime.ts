// SPEC-WEB-007 v0.1.0 (M9) — uptime 가독 형식 유틸.
//
// 프로세스 uptime(초) 을 "3d 4h 12m" 같은 사람이 읽기 좋은 형식으로 변환한다.
//
// 규칙:
//   - 일(d) 이 있으면 "Nd Nh Nm" (시/분이 0이어도 자릿수 안정성을 위해 모두 표기)
//   - 시(h) 만 있으면 "Nh Nm"
//   - 분(m) 만 있으면 "Nm"
//   - 1분 미만이면 "<1m" (0초 포함 — 막 시작한 프로세스도 "측정 안 됨"이 아님을 명시)
//   - 음수/NaN 등 비정상 입력은 "-" 로 안전 처리 (백엔드 이상값 방어)
//   - 소수점 초는 내림(Math.floor)하여 처리
//
// @spec SPEC-WEB-007 v0.1.0 (M9, AC-6)

/**
 * uptime_seconds(초) 를 "3d 4h 12m" 가독 형식 문자열로 변환한다.
 *
 * @param seconds 프로세스 uptime(초). 음수/NaN 은 "-" 반환.
 * @returns 가독 형식 문자열 (예: "3d 4h 12m", "4h 12m", "12m", "<1m", "-")
 */
export function formatUptime(seconds: number): string {
  // 비정상 입력 방어: NaN, 음수.
  if (!Number.isFinite(seconds) || seconds < 0) {
    return '-';
  }

  const total = Math.floor(seconds);
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);

  if (days > 0) {
    return `${days}d ${hours}h ${minutes}m`;
  }
  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  if (minutes > 0) {
    return `${minutes}m`;
  }
  // 1분 미만: 0초 포함. "측정 안 됨" 으로 오인되지 않도록 "<1m" 명시.
  return '<1m';
}
