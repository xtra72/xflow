// 시리즈 쿼리 결과 매트릭스 렌더러.
// 시리즈 키를 컬럼으로, 시간 버킷을 행으로 표시한다.
// 누락된 버킷×시리즈 교차점은 em-dash(—) 로 표시한다.
//
// SPEC-WEB-005 v0.2.0 에서 `SeriesMatrix` (columns + rows 형태) 를 직접
// 받도록 리팩터되었다. 기존 파일명/디폴트 export 는 최소 변경 원칙으로 유지.
//
// @spec SPEC-WEB-005

import { formatLocalTimestamp } from '@/services/api/tsdb';
import type { SeriesMatrix } from '@/services/api/seriesDataSource';

interface SeriesResultMatrixProps {
  /** 쿼리 응답 — 이미 컬럼/행으로 pivot 된 매트릭스. */
  matrix: SeriesMatrix;
  /** 누락 셀 대체 문자. 기본 "—". */
  emptyCellPlaceholder?: string;
}

/** 숫자를 표시용으로 포맷 — 정수는 그대로, 소수는 최대 4자리까지 표기. */
function formatValue(value: number): string {
  if (Number.isInteger(value)) {
    return value.toLocaleString();
  }
  return value.toLocaleString(undefined, { maximumFractionDigits: 4 });
}

/**
 * 매트릭스 렌더러 본체.
 * 컬럼이 0개이거나 행이 0개이면 안내 메시지를 렌더링한다.
 */
function SeriesResultMatrixImpl({
  matrix,
  emptyCellPlaceholder = '—',
}: SeriesResultMatrixProps) {
  const { columns, rows } = matrix;

  if (columns.length === 0) {
    return (
      <div className="p-4 text-center text-sm text-(--color-text-muted)">
        선택된 시리즈가 없습니다.
      </div>
    );
  }

  if (rows.length === 0) {
    return (
      <div className="p-4 text-center text-sm text-(--color-text-muted)">
        쿼리 결과가 비어 있습니다.
      </div>
    );
  }

  return (
    <div className="max-h-[60vh] overflow-auto rounded-md border border-(--color-border-default)">
      <table className="min-w-full border-collapse text-xs">
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
