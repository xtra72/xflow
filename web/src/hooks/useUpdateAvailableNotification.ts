// SPEC-WEB-006 v0.1.0 (M4) — 업데이트 가용 토스트 트리거 훅.
//
// useSystemVersion 의 update_available 이 false → true 로 전이되는 순간에만
// uiStore.addNotification(type='info') 을 호출한다. prev=undefined (초기 로드)
// 에서는 발화하지 않아 새로고침 시마다 토스트가 뜨는 스팸을 방지한다.
//
// 호출 위치: AppLayout 에서 한 번만 호출하면 60초 폴링 결과가 도착할 때마다
// 자동으로 전이가 감지된다. 폴링은 useSystemVersion 측 React Query cache 가
// 1회만 발생하므로 (queryKey 동일) 성능 부담은 무시 가능하다.
//
// @spec SPEC-WEB-006 v0.1.0 (M4)

import { useEffect, useRef } from 'react';

import { useSystemVersion } from '@/services/api/systemUpdate';
import { useUIStore } from '@/stores/uiStore';

/**
 * 시스템 버전 폴링 결과의 false → true 전이를 감지해 단발성 info 토스트를
 * 발생시키는 사이드 이펙트 훅. 반환값 없음.
 */
export function useUpdateAvailableNotification(): void {
  const { data } = useSystemVersion();
  // prev=undefined 는 "아직 한 번도 관측되지 않음" 을 의미한다.
  // 첫 polling 결과가 어떤 값이든 (true/false) 트리거가 되어선 안 된다.
  const prevAvailableRef = useRef<boolean | undefined>(undefined);
  const addNotification = useUIStore((s) => s.addNotification);

  useEffect(() => {
    const current = data?.update_available;
    const prev = prevAvailableRef.current;

    // 이전 관측치가 명시적으로 false 였고 현재 true 인 순간에만 발화.
    if (prev === false && current === true) {
      const versionLabel = data?.latest_version ?? '확인 필요';
      addNotification({
        type: 'info',
        message: `새 버전 사용 가능: ${versionLabel}`,
      });
    }

    // current 가 정의되어 있을 때만 prev 를 갱신해 undefined→true 직접 전이를 방지한다.
    if (current !== undefined) {
      prevAvailableRef.current = current;
    }
  }, [data?.update_available, data?.latest_version, addNotification]);
}
