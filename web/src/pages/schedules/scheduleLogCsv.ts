// SPEC-SCHEDULE-VIEW-001 M6: 실행 로그 CSV 내보내기.
//
// 병합된 논리 로그 행(MergedScheduleLog)을 CSV 텍스트로 직렬화한다. 필드 이스케이프
// (따옴표/쉼표/개행)와 Excel 한글 인식을 위한 UTF-8 BOM 을 처리한다. 순수 빌더
// (buildScheduleLogCsv)와 브라우저 다운로드 부수효과(downloadCsv)를 분리해
// 빌더를 단위 테스트할 수 있게 하고 fast-refresh 경계를 지킨다.

import { formatDate } from '@/lib/utils/format';

import type { MergedScheduleLog } from './scheduleLogMerge';

/** CSV 헤더(한국어). 테이블 컬럼과 대응한다. */
const CSV_HEADERS = ['실행 시각', '규칙 이름', '에이전트', '대상', '동작', '결과'] as const;

/** UTF-8 BOM(U+FEFF). Excel 이 CSV 를 UTF-8 로 해석하도록 강제한다. */
const UTF8_BOM = String.fromCharCode(0xfeff);

/** epoch ms → "YYYY-MM-DD HH:mm:ss". 0/무효 값은 '-'. */
function formatTriggerTime(ms: number): string {
  if (!ms || Number.isNaN(ms)) return '-';
  return formatDate(new Date(ms), 'long');
}

/** CSV 필드 이스케이프: 따옴표/쉼표/개행이 있으면 큰따옴표로 감싸고 내부 따옴표를 이중화. */
function escapeCsvField(value: string): string {
  if (/["\n\r,]/.test(value)) {
    return `"${value.replace(/"/g, '""')}"`;
  }
  return value;
}

/** 에이전트 셀: 실제 실행자가 선언과 다르면 양쪽을 표기. */
function agentCell(log: MergedScheduleLog): string {
  if (log.hasResult && log.actorAgentId !== '' && log.actorAgentId !== log.declaredAgentId) {
    return `${log.declaredAgentId} (실행: ${log.actorAgentId})`;
  }
  return log.declaredAgentId || '-';
}

/** 결과 셀: 집계 결과 + 대상별 성공 요약(ok=N/M). fire-only 는 "결과 없음". */
function resultCell(log: MergedScheduleLog): string {
  if (!log.hasResult) return '결과 없음';
  const base = log.result || '-';
  if (log.targets.length > 0) {
    const ok = log.targets.filter((t) => t.result === 'ok').length;
    return `${base} (ok=${ok}/${log.targets.length})`;
  }
  return base;
}

/**
 * 병합된 로그 행을 CSV 텍스트로 직렬화한다(BOM 없음, CRLF 구분).
 *
 * 병합 논리 행 하나당 CSV 한 줄. 대상별 상세는 결과 셀에 요약(ok=N/M)으로 압축한다.
 */
export function buildScheduleLogCsv(rows: MergedScheduleLog[]): string {
  const lines: string[] = [CSV_HEADERS.join(',')];
  for (const log of rows) {
    const cells = [
      formatTriggerTime(log.triggerTime),
      log.ruleName || '-',
      agentCell(log),
      log.hasResult ? log.target || '-' : '-',
      log.hasResult ? log.action || '-' : '-',
      resultCell(log),
    ];
    lines.push(cells.map(escapeCsvField).join(','));
  }
  return lines.join('\r\n');
}

/**
 * CSV 텍스트를 파일로 다운로드한다(UTF-8 BOM 부착 → Excel 한글 인식).
 */
export function downloadCsv(csv: string, filename: string): void {
  const blob = new Blob([`${UTF8_BOM}${csv}`], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}
