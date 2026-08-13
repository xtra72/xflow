// React Query hook for the ChirpStack gateway roster (SPEC-CHIRPSTACK-003 M4).
//
// 백엔드 exec 계약: `POST /agents/{id}/exec` 에 `{command: "list_gateways"}` 를 보내면
//   { "gateways": [ { gateway_id, device_count, last_seen_ms, devices: [...] } ] }
// 를 반환한다. xsfm 로스터와 달리 `status` 엔벨로프가 없으므로 `res.gateways` 를
// 직접 읽는다. 빈 로스터는 `{"gateways":[]}` 이며 null 이 아니지만, 키 자체가 없는
// 응답에 대비해 `?? []` 로 방어한다(useStations 의 누락 배열 정규화 관용구와 동일).
//
// 로컬 에이전트 로스터는 SSE 푸시가 아닌 exec 폴링으로 갱신한다(SSE 는 원격 프록시 전용).

import { useQuery } from '@tanstack/react-query';

import * as agentService from '@/services/api/agentService';

// ---- 타입 ----

/**
 * 하나의 (디바이스, 게이트웨이) 링크. 하나의 업링크를 여러 게이트웨이가 동시에
 * 수신하므로 같은 `dev_eui` 가 여러 게이트웨이 아래에 서로 다른 rssi/snr/channel 로
 * 등장하는 것이 정상이다 — 게이트웨이 간 중복 제거를 하면 안 된다.
 */
export interface ChirpstackGatewayDevice {
  dev_eui: string;
  /** 통합 레지스트리 UUID v4. 저장소 미설정 시 빈 문자열일 수 있다. */
  device_id: string;
  device_name: string;
  device_profile_name: string;
  /** 이 게이트웨이가 수신한 신호 세기(dBm). 게이트웨이별 값. */
  rssi: number;
  /** 이 게이트웨이가 수신한 SNR(dB). 와이어 값은 고정 소수가 아니다(9.25 / 12 모두 등장). */
  snr: number;
  /**
   * 수신 게이트웨이의 concentrator IF 채널 인덱스 — **게이트웨이 로컬 하드웨어 값**이다.
   * 주파수가 아니며 게이트웨이 간 비교할 수 없다(두 게이트웨이의 channel 3 은 같은
   * 주파수를 뜻하지 않는다). 실제 RF 주파수는 `frequency_hz` 를 사용한다.
   */
  channel: number;
  /** 프레임 레벨 RF 주파수(Hz). 한 업링크에서 파생된 모든 링크에 공통이다. */
  frequency_hz: number;
  spreading_factor: number;
  bandwidth: number;
  /** epoch milliseconds. */
  last_seen_ms: number;
  /** 서버가 호출 시점에 파생한 값(저장 필드 아님). 클라이언트에서 재계산하지 않는다. */
  stale: boolean;
}

/** 게이트웨이 1건. 이름/위치는 업링크에서 얻을 수 없어 EUI64 hex ID 만 존재한다. */
export interface ChirpstackGateway {
  gateway_id: string;
  device_count: number;
  /** 이 게이트웨이가 보유한 링크 중 가장 최근 수신 시각(epoch ms). */
  last_seen_ms: number;
  devices: ChirpstackGatewayDevice[];
}

// ---- 쿼리 키 ----

const gatewaysKey = (agentId: string) => ['chirpstack-gateways', agentId] as const;

/** 게이트웨이 탭 기본 폴링 주기(ms). 조회 전용 캐시 읽기이므로 보수적으로 잡는다. */
export const GATEWAYS_POLL_INTERVAL_MS = 10_000;

// ---- 쿼리 ----

/**
 * 게이트웨이 로스터 조회 (list_gateways). 게이트웨이는 `gateway_id` 오름차순,
 * 각 게이트웨이의 `devices` 는 `dev_eui` 오름차순으로 백엔드가 결정적 정렬해 반환한다.
 *
 * @param refetchInterval - 지정 시 주기 폴링(ms). 미지정 시 폴링 없음.
 */
export function useGateways(agentId: string, refetchInterval?: number) {
  return useQuery({
    queryKey: gatewaysKey(agentId),
    queryFn: async () => {
      const res = await agentService.execAgent(agentId, { command: 'list_gateways' });
      const gateways = (res as unknown as { gateways?: ChirpstackGateway[] }).gateways ?? [];
      // devices 누락 방어(디바이스 0건 게이트웨이는 이론상 발생하지 않지만 렌더가 깨지지 않도록).
      return gateways.map((g) => ({ ...g, devices: g.devices ?? [] }));
    },
    enabled: !!agentId,
    refetchInterval,
  });
}
