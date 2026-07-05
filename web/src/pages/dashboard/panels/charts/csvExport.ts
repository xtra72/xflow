// 차트 데이터 → CSV 변환 + 브라우저 다운로드 트리거.
// LineChartPanel 의 다중 시리즈 row 구조와 호환된다.

export interface ChartCsvRow {
  timestamp: number;
  [seriesKey: string]: number | null | undefined | unknown;
}

/**
 * RFC 4180 부분 호환 CSV 생성.
 * 컬럼: timestamp, iso, ...seriesKeys
 * 누락/비유한 숫자는 빈 문자열.
 *
 * booleanKeys 에 포함된 시리즈는 값(0/1)을 true/false 문자열로 내보낸다.
 */
export function chartDataToCsv(
  rows: ChartCsvRow[],
  seriesKeys: string[],
  booleanKeys?: Set<string>,
): string {
  const header = ['timestamp', 'iso', ...seriesKeys].map(escapeCell).join(',');
  const body = rows.map((row) => {
    const cells: string[] = [
      String(row.timestamp),
      new Date(row.timestamp).toISOString(),
    ];
    for (const key of seriesKeys) {
      cells.push(formatValue(row[key], booleanKeys?.has(key) ?? false));
    }
    return cells.map(escapeCell).join(',');
  });
  return [header, ...body].join('\n') + '\n';
}

function formatValue(v: unknown, isBoolean = false): string {
  if (v == null) return '';
  if (isBoolean) {
    if (v === 1 || v === true) return 'true';
    if (v === 0 || v === false) return 'false';
  }
  if (typeof v === 'number') {
    return Number.isFinite(v) ? String(v) : '';
  }
  return String(v);
}

function escapeCell(s: string): string {
  if (s.includes(',') || s.includes('"') || s.includes('\n')) {
    return `"${s.replace(/"/g, '""')}"`;
  }
  return s;
}

/** 브라우저에서 CSV 문자열을 파일로 다운로드. */
export function downloadCsv(csv: string, filename: string): void {
  if (typeof window === 'undefined' || typeof document === 'undefined') return;
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}
