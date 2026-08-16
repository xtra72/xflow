// Formatting utilities for dates, numbers, bytes, and durations.

const RELATIVE_UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 60 * 60 * 24 * 365],
  ['month', 60 * 60 * 24 * 30],
  ['week', 60 * 60 * 24 * 7],
  ['day', 60 * 60 * 24],
  ['hour', 60 * 60],
  ['minute', 60],
  ['second', 1],
];

/**
 * Format a date value.
 * - short:    "2024-01-15"
 * - long:     "2024-01-15 14:30:00"
 * - relative: "5 minutes ago"
 */
export function formatDate(
  date: string | Date,
  format: 'short' | 'long' | 'relative' = 'short',
): string {
  const d = date instanceof Date ? date : new Date(date);

  if (format === 'short') {
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${y}-${m}-${day}`;
  }

  if (format === 'long') {
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    const h = String(d.getHours()).padStart(2, '0');
    const min = String(d.getMinutes()).padStart(2, '0');
    const sec = String(d.getSeconds()).padStart(2, '0');
    return `${y}-${m}-${day} ${h}:${min}:${sec}`;
  }

  // relative
  const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });
  const diffSec = Math.round((d.getTime() - Date.now()) / 1000);

  for (const [unit, seconds] of RELATIVE_UNITS) {
    if (Math.abs(diffSec) >= seconds) {
      const value = Math.round(diffSec / seconds);
      return rtf.format(value, unit);
    }
  }
  return rtf.format(0, 'second');
}

/**
 * epoch milliseconds → 사람이 읽는 로컬 시간 문자열 (예: "2026. 08. 13. 14:32:01").
 * 유효하지 않은 값(0/음수/NaN)은 '-' 를 반환한다.
 *
 * 정확한 시각이 필요한 곳(이력 테이블 등)에서 사용한다. 신선도 표시처럼 "지금으로부터
 * 얼마나 지났는가" 가 중요한 곳은 formatRelativeEpochMs 를 사용한다.
 */
export function formatEpochMs(ms: number): string {
  if (!ms || ms <= 0) return '-';
  const d = new Date(ms);
  if (isNaN(d.getTime())) return '-';
  return d.toLocaleString('ko-KR', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

/**
 * epoch milliseconds → 상대 시간 문자열 (예: "3분 전", "방금").
 * 유효하지 않은 값은 '-' 를 반환한다.
 *
 * 측정치 카드처럼 좁은 공간에서 신선도를 나타낼 때 사용한다. 절대 시각은
 * formatEpochMs 로 title 속성에 함께 제공하는 것을 권장한다.
 */
export function formatRelativeEpochMs(ms: number, now: number = Date.now()): string {
  if (!ms || ms <= 0) return '-';
  const d = new Date(ms);
  if (isNaN(d.getTime())) return '-';

  const rtf = new Intl.RelativeTimeFormat('ko', { numeric: 'auto' });
  const diffSec = Math.round((ms - now) / 1000);

  for (const [unit, seconds] of RELATIVE_UNITS) {
    if (Math.abs(diffSec) >= seconds) {
      return rtf.format(Math.round(diffSec / seconds), unit);
    }
  }
  // 1초 미만은 '방금' (Intl 의 "0초 전" 보다 자연스럽다).
  return '방금';
}

// ---- LoRaWAN 링크 품질 표시 포맷터 ----
//
// 에이전트 게이트웨이 탭(ChirpstackGatewaysTab)과 디바이스 상세의 게이트웨이 섹션이
// 같은 와이어 값을 서로 다르게 표기하는 일이 없도록 두 화면이 공유한다.

/** Hz → 사람이 읽는 주파수 문자열. 922100000 → "922.1 MHz". 0/음수/NaN 은 '-'. */
export function formatFrequencyHz(hz: number): string {
  if (!hz || hz <= 0 || !Number.isFinite(hz)) return '-';
  // 소수 셋째 자리까지 유지하되 후행 0 은 제거한다(922.1 / 868.325).
  return `${Number((hz / 1_000_000).toFixed(3))} MHz`;
}

/** 대역폭 Hz → kHz 문자열. 125000 → "125 kHz". */
export function formatBandwidthHz(hz: number): string {
  if (!hz || hz <= 0 || !Number.isFinite(hz)) return '-';
  return `${Number((hz / 1000).toFixed(1))} kHz`;
}

/** SNR 표시. 와이어 값은 고정 소수가 아니므로(9.25 / 12) 최대 2자리로 다듬고 후행 0 을 제거한다. */
export function formatSnr(snr: number): string {
  if (!Number.isFinite(snr)) return '-';
  return String(Number(snr.toFixed(2)));
}

/**
 * LoRaWAN 변조 표기. `SF7 / 125 kHz` 형태로 확산 계수와 대역폭을 함께 보여준다.
 *
 * SF 와 BW 는 항상 쌍으로 읽어야 의미가 있다(같은 SF 라도 대역폭이 다르면 데이터
 * 레이트가 다르다). 두 값을 따로 두면 화면마다 조합 표기가 갈리므로 포맷터로 고정한다.
 *
 * 두 값 모두 없으면(0/음수/NaN) '-' 를 반환한다. 한쪽만 있으면 있는 쪽만 표기한다 —
 * 업링크에 txInfo 가 없어 대역폭이 0 으로 내려오는 경우가 실제로 있기 때문이다.
 */
export function formatModulation(spreadingFactor: number, bandwidthHz: number): string {
  const hasSf = Number.isFinite(spreadingFactor) && spreadingFactor > 0;
  const bw = formatBandwidthHz(bandwidthHz);
  if (!hasSf) return bw;
  if (bw === '-') return `SF${spreadingFactor}`;
  return `SF${spreadingFactor} / ${bw}`;
}

/**
 * Format a byte count into a human-readable string (e.g. "1.5 KB", "2.3 MB").
 */
export function formatBytes(bytes: number, decimals = 1): string {
  if (bytes === 0) return '0 B';

  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  const value = bytes / Math.pow(k, i);

  return `${value.toFixed(decimals)} ${sizes[i]}`;
}

/**
 * Format a number using the user's locale (e.g. 1,234,567).
 */
export function formatNumber(num: number): string {
  return num.toLocaleString();
}

/**
 * Format a ratio as a percentage string (e.g. 0.85 -> "85.0%").
 */
export function formatPercent(value: number, decimals = 1): string {
  return `${(value * 100).toFixed(decimals)}%`;
}

/**
 * Format a duration in seconds into a human-readable string (e.g. "1h 30m 5s").
 */
export function formatDuration(seconds: number): string {
  if (seconds < 0) seconds = 0;

  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);

  const parts: string[] = [];
  if (h > 0) parts.push(`${h}h`);
  if (m > 0) parts.push(`${m}m`);
  if (s > 0 || parts.length === 0) parts.push(`${s}s`);

  return parts.join(' ');
}
