// 테이블 패널의 순수 열 계산 — 셀 포맷 · 폭 비율 · 열 필터.
//
// 컴포넌트를 export 하지 않는 순수 모듈이라 UI 없이 전수 테스트할 수 있고,
// TablePanel.tsx 는 컴포넌트만 export 하는 상태를 유지한다.

import { isColumnFilterActive, type ColumnFilter } from '@/pages/agents/storeEntrySort';

import { getByPath, type ChartEntry, type TableColumn } from './chartChannelTypes';
import { formatTimestamp } from './chartChannelUtils';
import { DEFAULT_DECIMAL_PLACES } from './decimalPlaces';
import { TAG_FIELD_PREFIX } from './panelTagKeys';
import { resolveSeriesValue, seriesNameOfField } from './tablePivot';
import { formatValueWithUnit } from './unitOptions';

/**
 * 셀 표시 문자열.
 *
 * `decimals` 와 `unit` 은 `format: 'number'` 열에만 쓰인다. 종전에는 `String(n)` 이라
 * `21.533333333333335` 가 그대로 나왔다.
 *
 * **필터·목록도 이 함수를 쓴다.** "보이는 값 = 고르는 값 = 걸러지는 값" 이 표의 규약이라,
 * 자릿수를 표시에만 걸면 필터 드롭다운의 항목과 셀 글자가 어긋나 아무것도 걸리지 않는다.
 * 그래서 `uniqueTableColumnValues` · `applyTableColumnFilters` 도 같은 값을 받는다.
 */
export function formatCell(
  value: unknown,
  format: TableColumn['format'],
  decimals: number = DEFAULT_DECIMAL_PLACES,
  unit?: string,
): string {
  if (value == null) return '';
  if (format === 'datetime') {
    const n = typeof value === 'number' ? value : Number(value);
    if (!Number.isFinite(n)) return String(value);
    return formatTimestamp(n);
  }
  if (format === 'number') {
    const n = typeof value === 'number' ? value : Number(value);
    // 수로 볼 수 없는 값에는 단위를 붙이지 않는다 — `abckW` 는 값도 단위도 아니다.
    return Number.isFinite(n) ? formatValueWithUnit(n, decimals, unit) : String(value);
  }
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

/**
 * 열 필터 상태. 데이터 소스 시리즈 리스트(Store 엔트리 표)의 `ColumnFilter`
 * (`text` + `values`)를 그대로 이어받고, 수치/시각 열을 위한 범위 항(`min`/`max`)을
 * 더한다. 두 표가 서로 다른 필터 어휘를 쓰면 사용자는 같은 화면 안에서 두 가지
 * 조작법을 익혀야 하므로 공통 항은 공유한다.
 *
 * 범위와 값 선택은 **배타가 아니다** — 열 형식이 어느 쪽 UI 를 보여줄지 정할 뿐,
 * 판정은 존재하는 항을 모두 AND 로 결합한다.
 */
export interface TableColumnFilter extends ColumnFilter {
  /** 범위 하한(포함). 수치 열은 값 그대로, 시각 열은 epoch ms. */
  min?: number;
  /** 범위 상한(포함). */
  max?: number;
}

/** 열별 필터 상태. 키 = 열 field. */
export type ColumnFilters = Record<string, TableColumnFilter>;

/** 빈 필터(어느 항도 걸리지 않은 상태). */
export function emptyTableColumnFilter(): TableColumnFilter {
  return { text: '', values: new Set<string>() };
}

/** 하나라도 걸린 항이 있는가. 없으면 그 열은 전체 통과다. */
export function isTableColumnFilterActive(filter: TableColumnFilter | undefined): boolean {
  if (!filter) return false;
  if (isColumnFilterActive(filter)) return true;
  return filter.min !== undefined || filter.max !== undefined;
}

/**
 * 범위 UI 로 거르는 열 형식.
 *
 * `value`(number) 와 `timestamp`(datetime) 는 값 하나하나가 거의 다 달라서 값 목록으로는
 * 고를 수 없다 — 행 수만큼 선택지가 생긴다. 이 두 형식은 하한·상한으로 거른다.
 */
export function isRangeFilterColumn(column: TableColumn): boolean {
  return column.format === 'number' || column.format === 'datetime';
}

/**
 * 열 `field` 로 엔트리에서 원시 값을 뽑는다.
 *
 * 세 가지 표기를 지원한다:
 *   - `$.tags.<키>` — 시리즈 태그. 저장 위치는 `entry.labels` 이지만 사용자에게는
 *     "태그" 라는 어휘가 익숙하므로(데이터 소스 화면·store 엔트리 표가 모두 태그라 부른다)
 *     표기는 태그로 두고 여기서 매핑한다.
 *   - `$.series.<이름>` — 시각 기준 행(피벗)에서의 시리즈 값. 이름에 점·공백이 들어갈
 *     수 있어 점 경로로는 읽을 수 없다(`tablePivot.ts` 머리말).
 *   - 그 밖 — 기존 점 경로(`timestamp` / `value` / `labels.name` / `meta.x` …).
 */
export function resolveCellValue(entry: ChartEntry, field: string): unknown {
  if (field.startsWith(TAG_FIELD_PREFIX)) {
    const key = field.slice(TAG_FIELD_PREFIX.length);
    if (key === '') return undefined;
    return entry.labels?.[key];
  }
  const seriesName = seriesNameOfField(field);
  if (seriesName !== undefined) return resolveSeriesValue(entry, seriesName);
  return getByPath(entry, field);
}

/**
 * 범위 비교에 쓸 수치를 뽑는다. datetime 열의 원시 값은 epoch ms 이므로 그대로 쓴다.
 * 수치로 볼 수 없으면 `undefined` — 그런 행은 범위 필터에서 탈락한다(값을 모르므로
 * 통과시키면 "범위를 걸었는데 엉뚱한 행이 남는다").
 */
export function numericCellValue(entry: ChartEntry, column: TableColumn): number | undefined {
  const raw = resolveCellValue(entry, column.field);
  if (typeof raw === 'number') return Number.isFinite(raw) ? raw : undefined;
  if (typeof raw === 'string' && raw.trim() !== '') {
    const n = Number(raw);
    return Number.isFinite(n) ? n : undefined;
  }
  return undefined;
}

/**
 * 한 열이 실제로 갖는 **고유 표시값** 목록(정렬됨).
 *
 * 필터 드롭다운의 체크박스 목록이 된다 — 사용자가 값을 타이핑하는 대신 나열된 것 중에서
 * 고른다. 표시 문자열(formatCell 결과)을 쓰는 이유는 필터 매칭도 같은 문자열로 하기
 * 때문이다(보이는 값 = 고르는 값 = 걸러지는 값).
 */
export function uniqueTableColumnValues(
  entries: readonly ChartEntry[],
  column: TableColumn,
  decimals: number = DEFAULT_DECIMAL_PLACES,
): string[] {
  const set = new Set<string>();
  for (const e of entries) {
    set.add(formatCell(resolveCellValue(e, column.field), column.format, decimals, column.unit));
  }
  return Array.from(set).sort((a, b) => a.localeCompare(b));
}

/**
 * 열 폭 비율을 CSS `<col width>` 백분율로 환산한다.
 *
 * 지정된 열이 하나도 없으면 `undefined` — 브라우저 기본 테이블 레이아웃을 그대로 둔다
 * (전 열 자동일 때 굳이 백분율을 박으면 내용에 맞춘 자동 폭 조절을 잃는다).
 *
 * 일부만 지정된 경우: 지정 열은 자기 비율만큼, 미지정 열은 자동(`undefined`)으로 남겨
 * 남은 폭을 나눠 갖는다. 지정 비율 합이 100%를 넘지 않도록 정규화하지 않는다 — 비율은
 * 사용자가 준 가중치 그대로가 예측 가능하고, 넘치면 브라우저가 비례 축소한다.
 */
export function columnWidthPercents(columns: TableColumn[]): (string | undefined)[] | undefined {
  const total = columns.reduce((sum, c) => sum + (c.width && c.width > 0 ? c.width : 0), 0);
  if (total <= 0) return undefined;
  return columns.map((c) =>
    c.width && c.width > 0 ? `${((c.width / total) * 100).toFixed(4)}%` : undefined,
  );
}

/**
 * 열 필터를 적용한다. 판정 규칙은 Store 엔트리 표(`entryPassesColumnFilter`)와 같다:
 *
 *   - `text`: 표시 문자열에 부분 일치(대소문자 무시). 비어 있으면 통과.
 *   - `values`: 선택된 값 집합에 표시 문자열이 있어야 통과. 비어 있으면 통과(= 전체).
 *   - 둘 다 있으면 AND, 여러 열도 AND.
 *
 * 표시 문자열(formatCell 결과)로 판정하는 이유는 사용자가 보는 값과 고르는 값과 걸러지는
 * 값이 같아야 예측 가능하기 때문이다(예: datetime 열을 포맷된 "2026-08-26 …" 로 고른다).
 */
export function applyTableColumnFilters(
  entries: ChartEntry[],
  columns: TableColumn[],
  filters: ColumnFilters,
  decimals: number = DEFAULT_DECIMAL_PLACES,
): ChartEntry[] {
  const active = columns
    .map((col) => ({ col, filter: filters[col.field] }))
    .filter((x): x is { col: TableColumn; filter: TableColumnFilter } =>
      isTableColumnFilterActive(x.filter),
    );
  if (active.length === 0) return entries;
  return entries.filter((entry) =>
    active.every(({ col, filter }) => {
      const shown = formatCell(resolveCellValue(entry, col.field), col.format, decimals, col.unit);
      const text = filter.text.trim().toLowerCase();
      if (text !== '' && !shown.toLowerCase().includes(text)) return false;
      if (filter.values.size > 0 && !filter.values.has(shown)) return false;
      if (filter.min !== undefined || filter.max !== undefined) {
        const n = numericCellValue(entry, col);
        if (n === undefined) return false;
        if (filter.min !== undefined && n < filter.min) return false;
        if (filter.max !== undefined && n > filter.max) return false;
      }
      return true;
    }),
  );
}

// ---- 범위 필터 입력 변환 ----

/** epoch ms → `datetime-local` 입력이 받는 `YYYY-MM-DDTHH:mm` (로컬 시간). */
export function epochToLocalInput(ms: number | undefined): string {
  if (ms === undefined || !Number.isFinite(ms)) return '';
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** `datetime-local` 입력값 → epoch ms. 빈 값/파싱 실패는 undefined(= 경계 없음). */
export function localInputToEpoch(v: string): number | undefined {
  if (v.trim() === '') return undefined;
  const ms = new Date(v).getTime();
  return Number.isFinite(ms) ? ms : undefined;
}

// ---- 열 경계 드래그 리사이즈 ----

/** 드래그로 줄일 수 있는 열의 최소 폭(px). 이보다 좁아지면 헤더 글자가 사라진다. */
export const MIN_COLUMN_PX = 32;

/**
 * 경계 `index`(= index 번 열과 index+1 번 열 사이)를 `dx` px 만큼 끌었을 때의 새 폭 배열.
 *
 * **인접 두 열만 주고받는다.** 표 전체 폭은 그대로 두므로 한 경계를 끌 때 멀리 있는
 * 열까지 출렁이지 않는다(브라우저 표·스프레드시트가 쓰는 방식과 같다).
 *
 * `start` 는 드래그 시작 시점의 **실제 렌더 픽셀 폭**이다. 저장되는 `width` 는 비율
 * (가중치)이므로 픽셀 값을 그대로 가중치로 써도 뜻이 같다 — 비율의 합이 얼마든
 * `columnWidthPercents` 가 백분율로 정규화한다.
 *
 * 최소 폭(`MIN_COLUMN_PX`) 때문에 `dx` 는 양쪽으로 클램프된다. 범위 밖으로 끌어도
 * 열이 사라지거나 음수 폭이 되지 않는다.
 */
export function resizeColumnWidths(
  start: readonly number[],
  index: number,
  dx: number,
  minPx: number = MIN_COLUMN_PX,
): number[] {
  const next = start.slice();
  const a = start[index];
  const b = start[index + 1];
  if (a === undefined || b === undefined) return next;
  // 왼쪽 열이 최소가 되는 지점과 오른쪽 열이 최소가 되는 지점 사이로 제한한다.
  const clamped = Math.max(minPx - a, Math.min(dx, b - minPx));
  next[index] = a + clamped;
  next[index + 1] = b - clamped;
  return next;
}

/**
 * 드래그 결과 폭 배열을 열 설정에 반영한다.
 *
 * **모든 열에 `width` 를 적는다.** 끈 두 열만 적으면 나머지 자동 폭 열이 남은 공간을
 * 다시 나눠 갖느라 화면이 튄다 — 사용자가 방금 본 배치와 저장 결과가 달라진다.
 */
export function applyColumnWidths(
  columns: readonly TableColumn[],
  widths: readonly number[],
): TableColumn[] {
  return columns.map((c, i) => {
    const w = widths[i];
    return w !== undefined && w > 0 ? { ...c, width: Math.round(w) } : c;
  });
}
