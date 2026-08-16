// 디바이스 이력의 전용 테이블(게이트웨이 수신 이력 / 측정치 이력) 변환 + CSV 직렬화.
//
// DeviceDetailPanel 의 이력 섹션은 원래 top-level properties 키마다 셀 하나를 그렸다.
// chirpstack 디바이스는 그 키가 `gateways`(배열)와 `measurements`(중첩 맵)뿐이라
// 두 셀이 80자에서 잘린 compact JSON 덩어리로 렌더되어 사실상 읽을 수 없었다.
// 이 모듈은 두 구조를 각각 제대로 된 표 모델로 펼치는 순수 변환을 담당한다.
//
// 렌더/i18n 은 포함하지 않는다 — 라벨은 호출부가 주입한다(테스트 가능성 + 로케일 분리).

import {
  expandMeasurementEntries,
  extractGatewayLinks,
  type DeviceGatewayLink,
} from '@/lib/utils/deviceLabels';
import { formatLocalIsoWithOffset } from '@/pages/agents/tsdbCsvExport';
import type { DeviceHistoryEntry } from '@/types/device';

// ---- 게이트웨이 수신 이력 ----

/**
 * 이력 1건 × 게이트웨이 1건으로 평탄화한 행.
 *
 * 하나의 이력 엔트리는 그 시점에 디바이스를 수신한 게이트웨이 **배열**을 담는다.
 * 시계열은 확장/접기보다 평탄한 표가 읽기 쉬우므로 (엔트리, 게이트웨이) 쌍마다
 * 한 행으로 펼친다.
 */
export interface GatewayHistoryRow {
  /** 이력 엔트리 시각 (epoch ms). */
  timestamp: number;
  /** 같은 시각에 여러 게이트웨이가 있을 때 React key 를 안정적으로 만들기 위한 엔트리 인덱스. */
  entryIndex: number;
  link: DeviceGatewayLink;
}

/**
 * 이력 엔트리 목록을 (엔트리, 게이트웨이) 행 목록으로 평탄화한다.
 *
 * 순서: 엔트리 순서(백엔드가 최신순으로 내려준다)를 유지하고, 엔트리 안에서는
 * `extractGatewayLinks` 가 보존하는 백엔드의 `gateway_id` 오름차순을 유지한다.
 * 신호 세기로 재정렬하지 않는다 — RSSI 는 업링크마다 요동쳐 폴링마다 행이 뒤바뀌면
 * 특정 게이트웨이를 눈으로 추적할 수 없다.
 *
 * 게이트웨이 정보가 하나도 없으면 빈 배열을 반환한다(호출부가 빈 표를 만들지 않도록).
 */
export function flattenGatewayHistory(entries: DeviceHistoryEntry[]): GatewayHistoryRow[] {
  const rows: GatewayHistoryRow[] = [];
  entries.forEach((entry, entryIndex) => {
    const links = extractGatewayLinks(entry.properties ?? {});
    if (!links) return;
    for (const link of links) {
      rows.push({ timestamp: entry.timestamp, entryIndex, link });
    }
  });
  return rows;
}

// ---- 측정치 이력 ----

/** 측정치 이력 표의 셀 1개. */
export interface MeasurementCell {
  value: unknown;
  /** 이 측정치가 마지막으로 갱신된 시각 (epoch ms). 형식이 다르면 undefined. */
  timeMs?: number;
  /**
   * 이번 엔트리에서 실제로 갱신된 값이 아니라 이전 업링크에서 이월된 값인지 여부.
   * 판단 근거는 `isCarriedOver` 참고.
   */
  carriedOver: boolean;
}

/** 측정치 이력 표의 행 1개. */
export interface MeasurementHistoryRow {
  timestamp: number;
  /** 측정치 이름 → 셀. 해당 엔트리에 없는 키는 아예 없다(호출부가 '-' 로 렌더). */
  cells: Record<string, MeasurementCell>;
}

export interface MeasurementHistoryTable {
  /** 전체 엔트리에 걸친 측정치 키의 합집합 (최초 등장 순서). */
  columns: string[];
  rows: MeasurementHistoryRow[];
}

/**
 * 측정치가 이번 엔트리에서 갱신된 값인지, 이전 값이 이월된 것인지 판정한다.
 *
 * 배경: 백엔드의 measurements 는 **병합 캐시**다. 매 업링크마다 모든 키를 다시 싣는
 * 것이 아니라, 이번에 실려온 키만 갱신하고 나머지 키는 값과 `time_ms` 를 **그대로
 * 유지**한다 (chirpstack provider 의 mergeMeasurements). 따라서 아무 표시 없이
 * 표를 그리면 갱신되지 않은 컬럼에 같은 값이 계속 반복되어, 마치 매 시점 그 값을
 * 수신한 것처럼 보인다 — 이 화면을 갈아엎는 이유가 된 바로 그 오해다.
 *
 * 구분이 가능한 이유: 엔트리의 `timestamp` 는 (chirpstack 의 경우) 모든 측정치 시각과
 * 게이트웨이 last_seen 의 **최댓값**에서 파생된 업링크 시각이다. 그러므로 자기
 * `time_ms` 가 엔트리 시각보다 이전인 셀은 이번 업링크가 실어오지 않은 이월 값이다.
 *
 * 시각을 모르면(undefined/0) 이월 여부를 알 수 없으므로 이월로 단정하지 않는다 —
 * 확인되지 않은 값을 흐리게 처리하면 없는 사실을 주장하게 된다.
 */
export function isCarriedOver(timeMs: number | undefined, entryTimestamp: number): boolean {
  if (timeMs === undefined || timeMs <= 0) return false;
  if (!entryTimestamp || entryTimestamp <= 0) return false;
  return timeMs < entryTimestamp;
}

/**
 * `properties.measurements` 를 측정치별 항목으로 펼친다.
 *
 * `expandMeasurementEntries` 를 그대로 재사용하되, 그 함수가 measurements 가 아닌
 * 값을 원본 그대로 통과시키므로 호출 전에 "중첩 객체인 measurements" 임을 확인한다.
 */
function measurementEntriesOf(properties: Record<string, unknown>) {
  const raw = properties['measurements'];
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) return [];
  return expandMeasurementEntries([['measurements', raw]]);
}

/**
 * 이력 엔트리 목록에서 측정치 이력 표를 만든다.
 *
 * 컬럼 집합은 **전체 엔트리의 합집합**이다. measurements 는 병합 캐시라 옛 엔트리가
 * 더 적은 키를 갖는 것이 정상이므로, 한 엔트리만 보고 컬럼을 정하면 키가 누락된다.
 * (기존 이력 표의 top-level 키 합집합 로직과 같은 발상을 한 단계 안쪽에 적용한다.)
 *
 * 측정치가 하나도 없으면 columns/rows 모두 빈 배열을 반환한다.
 */
export function buildMeasurementHistory(entries: DeviceHistoryEntry[]): MeasurementHistoryTable {
  const columns: string[] = [];
  const seen = new Set<string>();
  const rows: MeasurementHistoryRow[] = [];

  for (const entry of entries) {
    const items = measurementEntriesOf(entry.properties ?? {});
    if (items.length === 0) continue;

    const cells: Record<string, MeasurementCell> = {};
    for (const item of items) {
      if (!seen.has(item.key)) {
        seen.add(item.key);
        columns.push(item.key);
      }
      cells[item.key] = {
        value: item.value,
        timeMs: item.timeMs,
        carriedOver: isCarriedOver(item.timeMs, entry.timestamp),
      };
    }
    rows.push({ timestamp: entry.timestamp, cells });
  }

  return { columns, rows };
}

// ---- CSV ----

/**
 * CSV 필드 이스케이프 (RFC 4180).
 * 콤마/따옴표/개행(CR·LF)이 있으면 따옴표로 감싸고 내부 따옴표는 이중화한다.
 *
 * csvExport.ts / tsdbCsvExport.ts 의 동명 헬퍼는 모두 모듈 private 이라 재사용할 수
 * 없어 동일 규칙을 여기서 다시 구현한다.
 */
export function escapeCsvCell(s: string): string {
  if (s.includes(',') || s.includes('"') || s.includes('\n') || s.includes('\r')) {
    return `"${s.replace(/"/g, '""')}"`;
  }
  return s;
}

/**
 * 값을 CSV 셀 문자열로 변환한다. **표시용 축약이 아니라 원본 정밀도**를 내보낸다
 * (정밀도를 잃은 내보내기는 없느니만 못하다).
 *
 * - null/undefined → 빈 문자열
 * - 비유한 숫자(NaN/Infinity) → 빈 문자열
 * - boolean → true/false
 * - 객체/배열 → compact JSON (말줄임 없음)
 */
export function toCsvValue(v: unknown): string {
  if (v === null || v === undefined) return '';
  if (typeof v === 'number') return Number.isFinite(v) ? String(v) : '';
  if (typeof v === 'boolean') return String(v);
  if (typeof v === 'object') {
    try {
      return JSON.stringify(v) ?? '';
    } catch {
      return '';
    }
  }
  return String(v);
}

/** 행 배열 → CSV 문자열 (헤더 + 본문, 각 줄 `\n`, 끝에 trailing newline). */
function rowsToCsv(header: string[], body: string[][]): string {
  const lines = [header.map(escapeCsvCell).join(',')];
  for (const row of body) {
    lines.push(row.map(escapeCsvCell).join(','));
  }
  return lines.join('\n') + '\n';
}

/** 게이트웨이 이력 CSV 의 컬럼 헤더 (호출부가 로케일에 맞춰 주입). */
export interface GatewayCsvHeaders {
  time: string;
  gatewayId: string;
  rssi: string;
  snr: string;
  channel: string;
  frequencyHz: string;
  spreadingFactor: string;
  bandwidthHz: string;
  linkLastSeen: string;
}

/**
 * 게이트웨이 수신 이력을 CSV 로 직렬화한다.
 *
 * 시각 컬럼만 사람이 읽는 로컬 ISO-8601(오프셋 포함)이고, 나머지는 전부 와이어 원본
 * 값이다 — 주파수는 Hz 그대로(922.1 MHz 아님), 변조는 SF 와 대역폭(Hz) 두 컬럼으로
 * 분해한다. 화면 표기는 사람을 위한 것이고 CSV 는 분석을 위한 것이다.
 *
 * `stale` 컬럼은 화면과 마찬가지로 내보내지 않는다(조회 시각 파생값). 대신 그 값이
 * 파생되어 나온 원본인 게이트웨이 last_seen 을 실어 사용자가 직접 판단하게 한다.
 */
export function gatewayHistoryToCsv(
  rows: GatewayHistoryRow[],
  headers: GatewayCsvHeaders,
): string {
  const header = [
    headers.time,
    headers.gatewayId,
    headers.rssi,
    headers.snr,
    headers.channel,
    headers.frequencyHz,
    headers.spreadingFactor,
    headers.bandwidthHz,
    headers.linkLastSeen,
  ];
  const body = rows.map(({ timestamp, link }) => [
    formatLocalIsoWithOffset(timestamp),
    link.gateway_id,
    toCsvValue(link.rssi),
    toCsvValue(link.snr),
    toCsvValue(link.channel),
    toCsvValue(link.frequency_hz),
    toCsvValue(link.spreading_factor),
    toCsvValue(link.bandwidth),
    link.last_seen_ms > 0 ? formatLocalIsoWithOffset(link.last_seen_ms) : '',
  ]);
  return rowsToCsv(header, body);
}

/**
 * 측정치 이력을 CSV 로 직렬화한다.
 *
 * 측정치마다 값 컬럼과 **수신 시각 컬럼**을 쌍으로 내보낸다. 이월 여부(화면의 흐림
 * 처리)는 "이 셀의 시각 < 행의 시각" 에서 나오는 파생 정보이므로, 파생 불리언 대신
 * 원본 시각을 실으면 사용자가 스프레드시트에서 같은 판정을 재현할 수 있다.
 *
 * 컬럼 헤더는 화면과 같은 라벨을 쓰되, 라벨이 원본 키와 다르면 `라벨 (key)` 로
 * 원본 키를 병기한다 — 서로 다른 키가 같은 라벨로 humanize 되어도 구분되게 하기 위함이다.
 */
export function measurementHistoryToCsv(
  table: MeasurementHistoryTable,
  headers: { time: string; measuredAtSuffix: string },
  labelOf: (key: string) => string,
): string {
  const header = [headers.time];
  for (const key of table.columns) {
    const label = labelOf(key);
    const name = label && label !== key ? `${label} (${key})` : key;
    header.push(name, `${name} ${headers.measuredAtSuffix}`);
  }

  const body = table.rows.map((row) => {
    const cells = [formatLocalIsoWithOffset(row.timestamp)];
    for (const key of table.columns) {
      const cell = row.cells[key];
      if (!cell) {
        cells.push('', '');
        continue;
      }
      cells.push(
        toCsvValue(cell.value),
        cell.timeMs !== undefined && cell.timeMs > 0
          ? formatLocalIsoWithOffset(cell.timeMs)
          : '',
      );
    }
    return cells;
  });

  return rowsToCsv(header, body);
}

/**
 * UTF-8 BOM.
 *
 * 이 CSV 들의 헤더는 로케일 라벨(한국어)이고 주 소비처는 Excel 이다. Excel 은 BOM 이
 * 없으면 UTF-8 을 시스템 ANSI 코드페이지로 잘못 읽어 한글 헤더가 깨진다.
 * (BOM 을 붙이지 않는 charts/csvExport.ts 와는 헤더가 ASCII 키라는 점이 다르다.)
 */
export const UTF8_BOM = '﻿';
