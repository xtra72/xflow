// 시리즈 쿼리 결과 매트릭스 렌더러.
// 시리즈 키를 컬럼으로, 시간 버킷을 행으로 표시한다.
// 누락된 버킷×시리즈 교차점은 em-dash(—) 로 표시한다.
//
// SPEC-WEB-005 v0.2.0 에서 `SeriesMatrix` (columns + rows 형태) 를 직접
// 받도록 리팩터되었다. 기존 파일명/디폴트 export 는 최소 변경 원칙으로 유지.
//
// SPEC-WEB-005 v0.3.0 Wave 2 UI/UX 개선:
//   - 헤더 우측에 "CSV 내보내기" 버튼 추가 (rows.length > 0 일 때만 노출).
//   - 행 수가 500 개를 넘으면 react-window List 기반 가상 스크롤로 전환.
//     일반 HTML 테이블은 500 행 미만에서 기존 동작 그대로 유지된다.
//
// @spec SPEC-WEB-005

import { useCallback, useMemo, type CSSProperties } from 'react';
import { Download } from 'lucide-react';
import { List, type RowComponentProps } from 'react-window';

import { formatLocalTimestamp } from '@/services/api/tsdb';
import type { SeriesMatrix } from '@/services/api/seriesDataSource';

import { downloadSeriesMatrixCsv } from './tsdbCsvExport';

/** 가상 스크롤로 전환되는 행 수 임계치. */
const VIRTUAL_ROW_THRESHOLD = 500;

/** 가상 스크롤 행 높이 (px). 일반 테이블 `py-1.5` + text-xs 에 맞춘 값. */
const VIRTUAL_ROW_HEIGHT = 32;

/** 타임스탬프 컬럼 너비 (px). */
const TIMESTAMP_COL_WIDTH = 180;

/** 데이터 컬럼 기본 너비 (px). */
const DATA_COL_WIDTH = 140;

interface SeriesResultMatrixProps {
  /** 쿼리 응답 — 이미 컬럼/행으로 pivot 된 매트릭스. */
  matrix: SeriesMatrix;
  /** 누락 셀 대체 문자. 기본 "—". */
  emptyCellPlaceholder?: string;
  /**
   * CSV 파일명 구성에 사용할 에이전트 이름. 미지정 시 `series` 로 대체된다.
   * 모달 상위 컴포넌트가 TSDB/Store 에이전트 이름을 주입한다.
   */
  agentName?: string;
  /** CSV 파일명 구성에 사용할 범위 시작 (epoch ms). */
  exportStartMs?: number;
  /** CSV 파일명 구성에 사용할 범위 종료 (epoch ms). */
  exportEndMs?: number;
}

/** 숫자를 표시용으로 포맷 — 정수는 그대로, 소수는 최대 4자리까지 표기. */
function formatValue(value: number): string {
  if (Number.isInteger(value)) {
    return value.toLocaleString();
  }
  return value.toLocaleString(undefined, { maximumFractionDigits: 4 });
}

/**
 * 가상 스크롤 경로에서 한 행을 렌더링하는 row 컴포넌트.
 * react-window v2 의 `List` 가 주입하는 `ariaAttributes`, `index`, `style` 에
 * 행 데이터 배열(`rows`)과 표시용 prop(`columns`, `emptyCellPlaceholder`) 을
 * `rowProps` 로 받아 렌더링한다.
 */
interface VirtualRowProps {
  rows: SeriesMatrix['rows'];
  columns: string[];
  emptyCellPlaceholder: string;
}

function VirtualRow({
  ariaAttributes,
  index,
  style,
  rows,
  columns,
  emptyCellPlaceholder,
}: RowComponentProps<VirtualRowProps>) {
  const row = rows[index];
  if (!row) return null;

  // 타임스탬프 + 데이터 셀을 flex row 로 배치. 컬럼 너비는 고정값 사용.
  return (
    <div
      {...ariaAttributes}
      style={style}
      className="flex items-center border-b border-(--color-border-default) bg-(--color-bg-surface) hover:bg-(--color-bg-elevated)"
    >
      {/* 타임스탬프 셀 (sticky-like: 리스트 자체가 좌측에서 시작하므로 고정 너비만 보장) */}
      <div
        className="flex-shrink-0 whitespace-nowrap border-r border-(--color-border-default) px-3 py-1.5 text-left font-mono text-xs text-(--color-text-secondary)"
        style={{ width: TIMESTAMP_COL_WIDTH }}
        role="rowheader"
      >
        {formatLocalTimestamp(row.bucketStartMs)}
      </div>
      {/* 데이터 셀들 */}
      {row.values.map((value, colIdx) => {
        const columnKey = columns[colIdx] ?? `col-${colIdx}`;
        return (
          <div
            key={columnKey}
            className="flex-shrink-0 whitespace-nowrap px-3 py-1.5 text-right font-mono text-xs text-(--color-text-primary)"
            style={{ width: DATA_COL_WIDTH }}
            role="cell"
          >
            {value === null ? emptyCellPlaceholder : formatValue(value)}
          </div>
        );
      })}
    </div>
  );
}

/**
 * 매트릭스 렌더러 본체.
 * 컬럼이 0개이거나 행이 0개이면 안내 메시지를 렌더링한다.
 */
function SeriesResultMatrixImpl({
  matrix,
  emptyCellPlaceholder = '—',
  agentName,
  exportStartMs,
  exportEndMs,
}: SeriesResultMatrixProps) {
  const { columns, rows } = matrix;

  const handleExportClick = useCallback(() => {
    // CSV 파일명 필드가 일부 빠져도 안전한 기본값으로 대체한다.
    downloadSeriesMatrixCsv(matrix, {
      agentName: agentName ?? 'series',
      startMs: exportStartMs ?? matrix.rows[0]?.bucketStartMs ?? Date.now(),
      endMs:
        exportEndMs ?? matrix.rows[matrix.rows.length - 1]?.bucketStartMs ?? Date.now(),
    });
  }, [matrix, agentName, exportStartMs, exportEndMs]);

  // 가상 스크롤 영역에 전달할 rowProps. 매 렌더마다 새 객체를 만들지 않도록 메모.
  const virtualRowProps = useMemo<VirtualRowProps>(
    () => ({ rows, columns, emptyCellPlaceholder }),
    [rows, columns, emptyCellPlaceholder],
  );

  // 전체 너비: 타임스탬프 + 데이터 컬럼 × n.
  const totalInnerWidth = useMemo(
    () => TIMESTAMP_COL_WIDTH + columns.length * DATA_COL_WIDTH,
    [columns.length],
  );

  if (columns.length === 0) {
    return (
      <div className="p-4 text-center text-sm text-(--color-text-muted)">
        선택된 시리즈가 없습니다.
      </div>
    );
  }

  const hasRows = rows.length > 0;
  const useVirtual = rows.length >= VIRTUAL_ROW_THRESHOLD;

  // 헤더 (CSV 내보내기 버튼 포함) — rows 0 일 때도 헤더는 렌더링하되 버튼은 숨긴다.
  const headerBar = (
    <div className="mb-2 flex items-center justify-between">
      <h4 className="text-xs font-medium text-(--color-text-muted)">
        {hasRows
          ? `행 ${rows.length.toLocaleString()}개${useVirtual ? ' (가상 스크롤)' : ''}`
          : '결과 없음'}
      </h4>
      {hasRows && (
        <button
          type="button"
          onClick={handleExportClick}
          className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1 text-xs font-medium text-(--color-text-primary) transition-colors hover:bg-(--color-bg-elevated)"
          data-testid="tsdb-result-csv-export"
        >
          <Download className="h-3.5 w-3.5" aria-hidden="true" />
          CSV 내보내기
        </button>
      )}
    </div>
  );

  if (!hasRows) {
    return (
      <div>
        {headerBar}
        <div className="p-4 text-center text-sm text-(--color-text-muted)">
          쿼리 결과가 비어 있습니다.
        </div>
      </div>
    );
  }

  if (!useVirtual) {
    // 기존 HTML 테이블 경로 (500 행 미만).
    return (
      <div>
        {headerBar}
        <div className="max-h-[60vh] overflow-auto rounded-md border border-(--color-border-default)">
          <table className="min-w-full border-collapse text-xs" role="table">
            <thead className="sticky top-0 bg-(--color-bg-primary)">
              <tr>
                <th
                  scope="col"
                  className="sticky left-0 z-10 border-b border-r border-(--color-border-default) bg-(--color-bg-primary) px-3 py-2 text-left font-medium text-(--color-text-muted)"
                >
                  타임스탬프 (Local)
                </th>
                {columns.map((k) => (
                  <th
                    key={k}
                    scope="col"
                    className="whitespace-nowrap border-b border-(--color-border-default) px-3 py-2 text-right font-mono font-medium text-(--color-text-muted)"
                  >
                    {k}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.bucketStartMs} className="hover:bg-(--color-bg-elevated)">
                  <th
                    scope="row"
                    className="sticky left-0 whitespace-nowrap border-b border-r border-(--color-border-default) bg-(--color-bg-surface) px-3 py-1.5 text-left font-mono text-(--color-text-secondary)"
                  >
                    {formatLocalTimestamp(row.bucketStartMs)}
                  </th>
                  {row.values.map((value, colIdx) => {
                    // 동일 버킷의 컬럼은 columns 배열 인덱스로 안정적인 key 를 만든다.
                    // 컬럼 순서는 SeriesMatrix 계약에 의해 렌더 간 고정되므로 index key 안전.
                    const columnKey = columns[colIdx] ?? `col-${colIdx}`;
                    return (
                      <td
                        key={columnKey}
                        className="whitespace-nowrap border-b border-(--color-border-default) px-3 py-1.5 text-right font-mono text-(--color-text-primary)"
                      >
                        {value === null ? emptyCellPlaceholder : formatValue(value)}
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    );
  }

  // 가상 스크롤 경로 (500 행 이상).
  // 외곽: role="table" div — 시맨틱 테이블은 유지하되 tbody 는 가상 리스트로 대체.
  // 헤더는 flex-row 로 sticky 위치에 고정한다.
  const headerStickyStyle: CSSProperties = {
    minWidth: totalInnerWidth,
    width: totalInnerWidth,
  };

  return (
    <div>
      {headerBar}
      <div
        className="max-h-[60vh] overflow-auto rounded-md border border-(--color-border-default)"
        role="table"
        data-testid="tsdb-result-virtual-wrapper"
      >
        {/* 스티키 헤더 행 */}
        <div
          className="sticky top-0 z-10 flex border-b border-(--color-border-default) bg-(--color-bg-primary)"
          style={headerStickyStyle}
          role="row"
        >
          <div
            className="flex-shrink-0 border-r border-(--color-border-default) px-3 py-2 text-left text-xs font-medium text-(--color-text-muted)"
            style={{ width: TIMESTAMP_COL_WIDTH }}
            role="columnheader"
          >
            타임스탬프 (Local)
          </div>
          {columns.map((k) => (
            <div
              key={k}
              className="flex-shrink-0 whitespace-nowrap px-3 py-2 text-right font-mono text-xs font-medium text-(--color-text-muted)"
              style={{ width: DATA_COL_WIDTH }}
              role="columnheader"
            >
              {k}
            </div>
          ))}
        </div>
        {/* 가상 스크롤 바디 — 고정 높이로 렌더, 외곽 max-h 와 겹치지 않게 명시 */}
        <div
          style={{ minWidth: totalInnerWidth, width: totalInnerWidth }}
          role="rowgroup"
        >
          <List
            rowComponent={VirtualRow}
            rowCount={rows.length}
            rowHeight={VIRTUAL_ROW_HEIGHT}
            rowProps={virtualRowProps}
            // 외곽 컨테이너가 max-h 로 높이를 제한하므로 List 는 자연 높이 계산.
            // defaultHeight 는 초기 SSR 렌더용 fallback.
            defaultHeight={Math.min(rows.length * VIRTUAL_ROW_HEIGHT, 500)}
            overscanCount={5}
            data-testid="tsdb-result-virtual-list"
          />
        </div>
      </div>
    </div>
  );
}

// ---- Public exports ----

/** TSDB/Store 공용 매트릭스 렌더러 (명시적 명칭). */
export const SeriesResultMatrix = SeriesResultMatrixImpl;

/**
 * 기존 콜사이트 호환을 위한 default export.
 * SPEC-WEB-005 v0.2.0 이전에는 `keys` + `TsdbQueryResponse` 를 받았으나,
 * 현재는 pivot 된 `SeriesMatrix` 를 받는다.
 */
export default SeriesResultMatrixImpl;
