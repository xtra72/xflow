// 디바이스 목록 컬럼 구성(서버 영속) 훅.
//
// 키 `device-list-columns` 의 전역 설정(앱 전체 공유, 사용자 구분 없음)으로
// 표시 컬럼 집합을 저장/조회한다. value 형태는 { columns: string[] }.
//
// 로드 실패/404(미저장) → 기본 컬럼 세트로 폴백한다. 저장은 토글 변경 시 PUT.

import { useCallback, useMemo } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import * as settingsService from '@/services/api/settingsService';
import { APIError } from '@/types/api';
import type { DeviceColumnsSetting } from '@/types/settings';

/** 디바이스 목록 컬럼 키. 테이블 헤더/셀 렌더와 1:1 매핑된다. */
export type DeviceListColumnKey =
  | 'name'
  | 'id'
  | 'type'
  | 'protocol'
  | 'status'
  | 'agent'
  | 'source'
  | 'last_seen';

/** 전체 컬럼 키 (정의 순서 = 테이블 렌더 순서). */
export const ALL_DEVICE_COLUMNS: DeviceListColumnKey[] = [
  'name',
  'id',
  'type',
  'protocol',
  'status',
  'agent',
  'source',
  'last_seen',
];

/**
 * 컬럼 키 → i18n 번역 키.
 *
 * 순수 모듈 스코프 상수라 t() 를 직접 호출할 수 없으므로, 번역 키만 저장하고
 * 소비 컴포넌트(DeviceListPage)에서 t(DEVICE_COLUMN_LABELS[col]) 로 변환한다.
 */
export const DEVICE_COLUMN_LABELS: Record<DeviceListColumnKey, string> = {
  name: 'devices.column.name',
  id: 'devices.column.id',
  type: 'devices.column.type',
  protocol: 'devices.column.protocol',
  status: 'devices.column.status',
  agent: 'devices.column.agent',
  source: 'devices.column.source',
  last_seen: 'devices.column.lastSeen',
};

/** 기본 표시 컬럼 (설정 미저장/로드 실패 시 폴백). 기존 하드코딩 컬럼 + id. */
export const DEFAULT_DEVICE_COLUMNS: DeviceListColumnKey[] = [
  'name',
  'id',
  'type',
  'protocol',
  'status',
  'agent',
  'source',
  'last_seen',
];

/** 전역 설정 키. */
export const DEVICE_COLUMNS_SETTING_KEY = 'device-list-columns';

/**
 * 저장된 value 를 유효한 컬럼 키 배열로 정규화한다.
 * - 알 수 없는 키는 제거(스키마 진화 호환).
 * - 정의 순서로 정렬(렌더 일관성).
 * - 비면 기본값으로 폴백(최소 1개 보장).
 */
function normalizeColumns(value: DeviceColumnsSetting | null | undefined): DeviceListColumnKey[] {
  const raw = value?.columns;
  if (!Array.isArray(raw)) return DEFAULT_DEVICE_COLUMNS;
  const filtered = ALL_DEVICE_COLUMNS.filter((k) => raw.includes(k));
  return filtered.length > 0 ? filtered : DEFAULT_DEVICE_COLUMNS;
}

/**
 * 디바이스 목록 컬럼 구성 로드/저장 훅.
 *
 * @returns columns(현재 표시 컬럼), setColumns(저장), isLoading.
 */
export function useDeviceColumns() {
  const queryClient = useQueryClient();

  const query = useQuery({
    queryKey: ['settings', DEVICE_COLUMNS_SETTING_KEY],
    queryFn: async () => {
      try {
        return await settingsService.getSetting<DeviceColumnsSetting>(
          DEVICE_COLUMNS_SETTING_KEY,
        );
      } catch (err) {
        // 미저장(404) → null(기본값 폴백). React Query 는 undefined 반환을 금지하므로
        // null 을 사용한다. 그 외 오류는 전파(아래 retry:false 로 1회만 시도).
        if (err instanceof APIError && err.status === 404) {
          return null;
        }
        throw err;
      }
    },
    // 로드 실패 시에도 기본 컬럼으로 동작하도록 재시도는 하지 않는다.
    retry: false,
    staleTime: 60_000,
  });

  const columns = useMemo(
    () => normalizeColumns(query.data),
    [query.data],
  );

  const mutation = useMutation({
    mutationFn: (cols: DeviceListColumnKey[]) =>
      settingsService.putSetting<DeviceColumnsSetting>(DEVICE_COLUMNS_SETTING_KEY, {
        columns: cols,
      }),
    onSuccess: (saved) => {
      // 서버 응답 value 로 캐시 갱신(낙관적 폴백 없이 권위 응답 사용).
      queryClient.setQueryData(['settings', DEVICE_COLUMNS_SETTING_KEY], saved);
    },
  });

  const setColumns = useCallback(
    (cols: DeviceListColumnKey[]) => {
      // 정의 순서로 정규화 + 최소 1개 보장 후 저장.
      const normalized = ALL_DEVICE_COLUMNS.filter((k) => cols.includes(k));
      const next = normalized.length > 0 ? normalized : DEFAULT_DEVICE_COLUMNS;
      mutation.mutate(next);
    },
    [mutation],
  );

  return {
    columns,
    setColumns,
    isLoading: query.isLoading,
    isSaving: mutation.isPending,
  };
}
