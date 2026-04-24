// 시리즈 매트릭스 → CSV 변환 + 브라우저 다운로드 유틸.
// LineChartPanel 의 csvExport 와 달리 행 구조가 `SeriesMatrix` (columns + rows) 이고,
// 타임스탬프는 ISO-8601 로컬 타임존(오프셋 포함) 단일 컬럼만 사용한다.
//
// 이 모듈은 외부 라이브러리 없이 순수 Blob + <a download> 로 다운로드를 트리거한다.
//
// @spec SPEC-WEB-005

import type { SeriesMatrix } from '@/services/api/seriesDataSource';

/**
 * epoch ms 를 ISO-8601 로컬 타임존 오프셋 포함 형식으로 포맷한다.
 * 예: `2026-04-23T12:34:56+09:00`.
 *
 * `toISOString()` 은 UTC (`...Z`) 로만 출력하므로, 로컬 연/월/일/시/분/초를
 * 직접 조립하고 타임존 오프셋을 수동으로 붙인다.
 */
export function formatLocalIsoWithOffset(ms: number): string {
  const d = new Date(ms);
  const pad = (n: number): string => String(n).padStart(2, '0');

  const y = d.getFullYear();
  const mo = pad(d.getMonth() + 1);
  const day = pad(d.getDate());
  const h = pad(d.getHours());
  const mi = pad(d.getMinutes());
  const s = pad(d.getSeconds());

  // getTimezoneOffset 은 "UTC - local" 을 분 단위로 반환한다 (로컬이 UTC+9 면 -540).
  // ISO-8601 에서는 로컬 기준 오프셋 부호가 반대이므로 부호를 뒤집는다.
  const offsetMin = -d.getTimezoneOffset();
  const offsetSign = offsetMin >= 0 ? '+' : '-';
  const absOffset = Math.abs(offsetMin);
  const offsetH = pad(Math.floor(absOffset / 60));
  const offsetM = pad(absOffset % 60);

  return `${y}-${mo}-${day}T${h}:${mi}:${s}${offsetSign}${offsetH}:${offsetM}`;
}

/**
 * 숫자 값을 CSV 셀 문자열로 변환한다.
 * - null / NaN / Infinity → 빈 문자열
 * - 정수 → 소수점 없이
 * - 실수 → `toString()` (JS 기본 표현)
 */
function formatCsvValue(v: number | null): string {
  if (v === null || v === undefined) return '';
  if (!Number.isFinite(v)) return '';
  return String(v);
}

/** CSV 필드 이스케이프 (콤마/따옴표/개행 포함 시 따옴표로 감싸고 내부 따옴표는 이중화). */
function escapeCsvCell(s: string): string {
  if (s.includes(',') || s.includes('"') || s.includes('\n') || s.includes('\r')) {
    return `"${s.replace(/"/g, '""')}"`;
  }
  return s;
}

/**
 * 시리즈 매트릭스를 CSV 문자열로 직렬화한다.
 *
 * - 헤더: `timestamp,<key1>,<key2>,...` (키에 콤마/따옴표가 포함되면 따옴표로 감싼다)
 * - 각 행: 로컬 ISO-8601 타임스탬프 + 컬럼 순서대로 값 (null → 빈 문자열)
 * - 라인 구분자: `\n`, 전체 끝에 trailing newline 을 추가한다.
 */
export function seriesMatrixToCsv(matrix: SeriesMatrix): string {
  const headerCells = ['timestamp', ...matrix.columns].map(escapeCsvCell);
  const lines: string[] = [headerCells.join(',')];

  for (const row of matrix.rows) {
    const cells: string[] = [formatLocalIsoWithOffset(row.bucketStartMs)];
    for (const v of row.values) {
      cells.push(formatCsvValue(v));
    }
    lines.push(cells.map(escapeCsvCell).join(','));
  }

  return lines.join('\n') + '\n';
}

/**
 * 파일 이름에 사용할 수 없는/부적절한 문자를 하이픈으로 대체한다.
 *
 * - `:` 는 Windows 에서 불법이므로 `-` 로 변환 (타임스탬프 포맷에서 자주 등장).
 * - 공백/콤마/슬래시/백슬래시/따옴표 등도 하이픈으로 치환.
 * - 연속 하이픈은 하나로 축약.
 */
export function sanitizeFilenamePart(s: string): string {
  return s
    .replace(/[\s,:/\\"'<>|?*]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '');
}

/**
 * 시리즈 매트릭스 CSV 다운로드를 트리거한다.
 *
 * - UTF-8 BOM 을 prefix 로 붙여 Excel 이 UTF-8 로 인식하도록 한다.
 * - `URL.createObjectURL` + 임시 `<a download>` 엘리먼트 클릭 패턴 사용.
 * - 파일명: `series-{agentName}-{startISO}-to-{endISO}.csv` (특수 문자 이스케이프).
 *
 * SSR 환경(`window`/`document` 미정의)에서는 no-op.
 */
export function downloadSeriesMatrixCsv(
  matrix: SeriesMatrix,
  options: {
    agentName: string;
    startMs: number;
    endMs: number;
  },
): void {
  if (typeof window === 'undefined' || typeof document === 'undefined') return;
  if (matrix.rows.length === 0) return;

  const csv = seriesMatrixToCsv(matrix);
  // UTF-8 BOM + CSV 본문.
  const bom = '﻿';
  const blob = new Blob([bom + csv], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);

  const startPart = sanitizeFilenamePart(formatLocalIsoWithOffset(options.startMs));
  const endPart = sanitizeFilenamePart(formatLocalIsoWithOffset(options.endMs));
  const agentPart = sanitizeFilenamePart(options.agentName) || 'series';
  const filename = `series-${agentPart}-${startPart}-to-${endPart}.csv`;

  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}
