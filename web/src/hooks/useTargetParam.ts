// useTargetParam — URL `?target=` 쿼리에서 현재 자원 타깃을 읽는다
// (SPEC-REMOTE-001 M8, 그룹 J, REQ-J13).
//
// 목록/제어 페이지(FlowListPage 등)는 본 훅으로 타깃을 결정한다. 미지정/`local`
// 이면 로컬, `remote:{instanceId}` 면 원격이다. 로컬일 때 기존 동작은 불변이다.

import { useMemo } from 'react';
import { useSearchParams } from 'react-router';

import { parseTargetParam, type ResourceTarget } from '@/lib/remote/target';

/** URL `?target=` 에서 현재 자원 타깃을 파싱한다. */
export function useTargetParam(): ResourceTarget {
  const [searchParams] = useSearchParams();
  const raw = searchParams.get('target');
  return useMemo<ResourceTarget>(() => parseTargetParam(raw), [raw]);
}
