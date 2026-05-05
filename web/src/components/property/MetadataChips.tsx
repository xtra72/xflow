// 키 메타데이터 칩 그룹.
//
// SPEC-STORE-003 v0.3.0 응답에서 키별로 제공되는 `data_type`, `metric_type`,
// `registration` 메타데이터를 작은 칩 형태로 시각화한다.
//
// 사용처:
//   - TsdbDataViewerModal (Store): 시리즈 멀티셀렉트 행에서 키별 메타데이터 표시
//   - StoreKeysEditor (Phase F): 키 목록에서 자동/수동 등록 배지 표시 예정
//
// 설계 원칙:
//   - undefined 인 prop 은 렌더링하지 않는다 (선택적 노출).
//   - data_type 별 색상은 시각적 구분만 제공하며, 의미는 텍스트로 전달된다.
//   - metric_type === 'unknown' 은 별도 muted 스타일로 표시해 자동 등록 키임을 시사한다.
//   - showAutoBadge=true 일 때만 registration 배지가 노출된다 (필요한 컨텍스트에서만).
//
// @spec SPEC-WEB-005 v0.7.0 (M16)
// @spec SPEC-STORE-003 v0.3.0

import type { DataType, RegistrationSource } from '@/services/api/store';

interface MetadataChipsProps {
  /** 데이터 타입 (int/float/string/boolean/bytes/json). undefined 시 칩 미표시. */
  dataType?: DataType;
  /** 메트릭 타입 (예: gauge/counter/temperature). 'unknown' 은 muted 스타일. */
  metricType?: string;
  /** 등록 출처. `showAutoBadge=true` 일 때만 표시된다. */
  registration?: RegistrationSource;
  /** registration 배지 활성화 여부. 기본 false (TsdbDataViewerModal 행에서는 활성화). */
  showAutoBadge?: boolean;
  /** 추가 wrapper 클래스. */
  className?: string;
}

/** data_type 별 칩 배경 색상. 시각 구분 목적이며 텍스트 의미가 우선. */
const DATA_TYPE_COLOR: Record<DataType, string> = {
  int: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
  float: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
  string: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200',
  boolean:
    'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200',
  bytes: 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200',
  json: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
};

const chipBase = 'px-1.5 py-0.5 text-[10px] font-mono rounded whitespace-nowrap';

/**
 * 키 메타데이터(data_type/metric_type/registration) 를 칩 그룹으로 표시한다.
 *
 * 모든 prop 은 optional 이며, 값이 없으면 해당 칩만 누락된다.
 * showAutoBadge=false (기본) 인 경우 registration 칩은 어떤 값이든 표시되지 않는다.
 */
export function MetadataChips({
  dataType,
  metricType,
  registration,
  showAutoBadge = false,
  className,
}: MetadataChipsProps) {
  const wrapperClass = `inline-flex flex-wrap items-center gap-1 ${className ?? ''}`;
  return (
    <span className={wrapperClass} data-testid="metadata-chips">
      {dataType && (
        <span
          className={`${chipBase} ${DATA_TYPE_COLOR[dataType]}`}
          data-testid="metadata-data-type"
          title={`데이터 타입: ${dataType}`}
        >
          {dataType}
        </span>
      )}
      {metricType && metricType !== 'unknown' && (
        <span
          className={`${chipBase} bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200`}
          data-testid="metadata-metric-type"
          title={`메트릭 타입: ${metricType}`}
        >
          {metricType}
        </span>
      )}
      {metricType === 'unknown' && (
        <span
          className={`${chipBase} bg-(--color-bg-elevated) text-(--color-text-muted) opacity-70`}
          data-testid="metadata-metric-type-unknown"
          title="메트릭 타입 미지정 (런타임 자동 등록)"
        >
          unknown
        </span>
      )}
      {showAutoBadge && registration === 'auto' && (
        <span
          className={`${chipBase} border border-yellow-400 bg-yellow-50 text-yellow-800 dark:border-yellow-600 dark:bg-yellow-900 dark:text-yellow-200`}
          data-testid="metadata-registration-auto"
          title="런타임에 자동 등록된 키"
          aria-label="자동 등록"
        >
          auto
        </span>
      )}
      {showAutoBadge && registration === 'manual' && (
        <span
          className={`${chipBase} border border-slate-400 bg-slate-50 text-slate-700 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-300`}
          data-testid="metadata-registration-manual"
          title="설정 파일에 정의된 정적 키"
          aria-label="수동 등록"
        >
          manual
        </span>
      )}
    </span>
  );
}
