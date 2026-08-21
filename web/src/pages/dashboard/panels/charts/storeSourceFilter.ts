// 차트 Store 소스 키 선택기의 순수 필터 로직.
//
// 설정 UI(StoreSourceSection)에서 StoreKeyObject 목록을 4가지 기준으로 필터링한다:
//   - key 이름 검색(부분 일치, 대소문자 무시)
//   - field 정확 일치
//   - data_type 정확 일치
//   - tag 필터("tagKey=tagValue" 형태, 선택된 모든 페어를 AND 로 적용)
//
// 렌더 컴포넌트에서 분리해 단위 테스트가 가능하도록 순수 함수로 유지한다.
//
// @spec SPEC-WEB-005

import type { DataType, StoreKeyObject } from '@/services/api/store';

/** 필터 조건. 빈/미지정 필드는 해당 기준을 적용하지 않음을 의미한다. */
export interface StoreKeyFilter {
  /** key 이름 부분 검색(대소문자 무시). 빈 문자열이면 미적용. */
  search?: string;
  /** field 정확 일치. 빈/undefined 면 미적용. */
  fieldName?: string;
  /** data_type 정확 일치. undefined 면 미적용. */
  dataType?: DataType;
  /** 선택된 "tagKey=tagValue" 집합. 모든 항목을 AND 로 적용. 비어있으면 미적용. */
  tagFilters?: Set<string>;
}

/** "tagKey=tagValue" 문자열을 만든다(필터 식별자). */
export function makeTagFilterId(tagKey: string, tagValue: string): string {
  return `${tagKey}=${tagValue}`;
}

/**
 * 단일 StoreKeyObject 가 선택된 모든 태그 필터를 만족하는지 검사한다.
 *
 * 같은 tagKey 의 여러 값이 선택된 경우(OR), 서로 다른 tagKey 끼리(AND) 결합한다.
 * 예) 선택 {room=1, room=2, type=temp} → (room∈{1,2}) AND (type=temp).
 */
export function matchesTagFilters(
  obj: StoreKeyObject,
  tagFilters: Set<string>,
): boolean {
  if (tagFilters.size === 0) return true;
  // tagKey 별로 허용 값 집합을 모은다.
  const byKey = new Map<string, Set<string>>();
  for (const id of tagFilters) {
    const eq = id.indexOf('=');
    if (eq < 0) continue;
    const k = id.slice(0, eq);
    const v = id.slice(eq + 1);
    let set = byKey.get(k);
    if (!set) {
      set = new Set();
      byKey.set(k, set);
    }
    set.add(v);
  }
  const tags = obj.tags ?? {};
  for (const [k, allowed] of byKey) {
    const actual = tags[k];
    if (actual === undefined || !allowed.has(actual)) return false;
  }
  return true;
}

/**
 * StoreKeyObject 배열을 필터 조건으로 좁힌다(원본 순서 보존).
 *
 * 모든 기준은 AND 로 결합되며, 비어있는 기준은 무시된다.
 */
export function filterStoreKeyObjects(
  objects: StoreKeyObject[],
  filter: StoreKeyFilter,
): StoreKeyObject[] {
  const search = (filter.search ?? '').trim().toLowerCase();
  const fieldName = filter.fieldName ?? '';
  const dataType = filter.dataType;
  const tagFilters = filter.tagFilters ?? new Set<string>();

  return objects.filter((obj) => {
    if (search && !obj.key.toLowerCase().includes(search)) return false;
    if (fieldName && obj.field !== fieldName) return false;
    if (dataType && obj.data_type !== dataType) return false;
    if (!matchesTagFilters(obj, tagFilters)) return false;
    return true;
  });
}

/** 키 객체 배열에서 등장하는 distinct field 목록(정렬). */
export function distinctMetricTypes(objects: StoreKeyObject[]): string[] {
  const set = new Set<string>();
  for (const o of objects) {
    if (o.field) set.add(o.field);
  }
  return [...set].sort();
}

/** 키 객체 배열에서 등장하는 distinct data_type 목록(정렬). */
export function distinctDataTypes(objects: StoreKeyObject[]): DataType[] {
  const set = new Set<DataType>();
  for (const o of objects) {
    if (o.data_type) set.add(o.data_type);
  }
  return [...set].sort();
}

// ---- 정렬 (Store 키 테이블 컬럼) ----

/** 스토어 키 테이블의 정렬 대상 필드. */
export type StoreSortField = 'key' | 'field' | 'data_type' | 'tags';

/** 정렬 상태(필드 + 방향). null 이면 정렬 없음. */
export type StoreSortState =
  | { field: StoreSortField; order: 'asc' | 'desc' }
  | null;

/** StoreKeyObject 에서 정렬 비교용 문자열 값을 추출한다. */
function sortValue(obj: StoreKeyObject, field: StoreSortField): string {
  switch (field) {
    case 'key':
      return obj.key;
    case 'field':
      return obj.field ?? '';
    case 'data_type':
      return obj.data_type ?? '';
    case 'tags':
      // 태그는 정렬 안정성을 위해 키 사전순으로 "k=v" 직렬화한다.
      return Object.keys(obj.tags ?? {})
        .sort()
        .map((k) => `${k}=${obj.tags![k]}`)
        .join(',');
    default: {
      const _exhaustive: never = field;
      return _exhaustive;
    }
  }
}

/**
 * StoreKeyObject 배열을 정렬 상태에 따라 정렬한다(불변). sort 가 null 이면 입력을
 * 그대로 반환하여 원본 순서를 보존한다. 차트 Store 키 테이블의 컬럼 정렬에 쓰인다.
 *
 * @spec SPEC-WEB-005
 */
export function sortStoreKeyObjects(
  objects: StoreKeyObject[],
  sort: StoreSortState,
): StoreKeyObject[] {
  if (!sort) return objects;
  const copy = objects.slice();
  copy.sort((a, b) => {
    const cmp = sortValue(a, sort.field).localeCompare(sortValue(b, sort.field));
    return sort.order === 'asc' ? cmp : -cmp;
  });
  return copy;
}
