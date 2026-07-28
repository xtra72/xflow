// 원격 관리 클라이언트 설정 react-query 훅 (SPEC-REMOTE-001 원격 관리 클라이언트 설정 UI).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  getRemoteClientConfig,
  updateRemoteClientConfig,
  type RemoteClientConfigUpdate,
} from '@/services/api/remoteConfigService';

/** 원격 클라이언트 설정 쿼리 키. */
const REMOTE_CLIENT_CONFIG_KEY = ['system', 'remote-config'] as const;

/** 현재 원격 클라이언트 설정을 조회한다(admin 전용). */
export function useRemoteClientConfig() {
  return useQuery({
    queryKey: REMOTE_CLIENT_CONFIG_KEY,
    queryFn: () => getRemoteClientConfig(),
    staleTime: 30_000,
    refetchOnWindowFocus: false,
    retry: false,
  });
}

/** 원격 클라이언트 설정을 부분 업데이트한다. 성공 시 조회 쿼리를 무효화한다. */
export function useUpdateRemoteClientConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: RemoteClientConfigUpdate) => updateRemoteClientConfig(req),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: REMOTE_CLIENT_CONFIG_KEY });
    },
  });
}
