// TSDB 쿼리 결과 매트릭스 렌더러.
// 시리즈 키를 컬럼으로, 시간 버킷을 행으로 표시한다.
// 누락된 버킷×시리즈 교차점은 em-dash(—) 로 표시한다.
//
// @spec SPEC-WEB-005

import { useMemo } from 'react';

import { formatLocalTimestamp, type TsdbQueryResponse } from '@/services/api/tsdb';

interface TsdbResultMatrixProps {
  /** 사용자가 요청 시점에 선택한 시리즈 키 순서. 컬럼 순서를 결정한다. */
  keys: string[];
  /** 쿼리 응답 (fan-out 병합 결과). */
  response: TsdbQueryResponse;
  /** 누락 셀 대체 문자. 기본 "—". */
  emptyCellPlaceholder?: string;
}

/** 숫자를 표시용으로 포맷 — 소수점 과다 자르고 thousands separator 적용. */
function formatValue(value: number): string {
  // 정수는 그대로, 소수는 최대 4자리까지 표기.
  if (Number.isInteger(value)) {
    return value.toLocaleString();
  }
  return value.toLocaleString(undefined, { maximumFractionDigits: 4 });
}

export default function TsdbResultMatrix({
  keys,
  response,
  emptyCellPlaceholder = '—',
}: TsdbResultMatrixProps) {
  /**
   * 버킷(ms) -> (key -> value) 맵을 구성한다.
   * 추가로 모든 타임스탬프의 정렬된 오름차순 배열을 산출한다.
   */
  const { sortedTimestamps, cellMap } = useMemo(() => {
    const map = new Map<number, Map<string, number>>();
    for (const series of response.results) {
      for (const p of series.points) {
        if (p.value === null || !Number.isFinite(p.timestampMs)) continue;
        let row = map.get(p.timestampMs);
        if (!row) {
          row = new Map<string, number>();
          map.set(p.timestampMs, row);
        }
        row.set(series.key, p.value);
      }
    }
    const stamps = Array.from(map.keys()).sort((a, b) => a - b);
    return { sortedTimestamps: stamps, cellMap: map };
  }, [response]);

  if (keys.length === 0) {
    return (
      <div className="p-4 text-center text-sm text-(--color-text-muted)">
        선택된 시리즈가 없습니다.
      </div>
    );
  }

  if (sortedTimestamps.length === 0) {
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
            {keys.map((k) => (
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
          {sortedTimestamps.map((ts) => {
            const row = cellMap.get(ts);
            return (
              <tr key={ts} className="hover:bg-(--color-bg-elevated)">
                <th
                  scope="row"
                  className="sticky left-0 whitespace-nowrap border-b border-r border-(--color-border-default) bg-(--color-bg-surface) px-3 py-1.5 text-left font-mono text-(--color-text-secondary)"
                >
                  {formatLocalTimestamp(ts)}
                </th>
                {keys.map((k) => {
                  const value = row?.get(k);
                  return (
                    <td
                      key={k}
                      className="whitespace-nowrap border-b border-(--color-border-default) px-3 py-1.5 text-right font-mono text-(--color-text-primary)"
                    >
                      {value === undefined ? emptyCellPlaceholder : formatValue(value)}
                    </td>
                  );
                })}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
