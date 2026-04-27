// 차트 채널 API 클라이언트.
// SPEC-CHART-001 REQ-M5-02: 활성 chart-emitter 채널 목록 조회.
// 백엔드 엔드포인트: GET /api/v1/charts/channels
// 응답 envelope: { success, data: { channels: ChartChannelSummary[] } }
// (client.ts 의 인터셉터가 envelope 를 벗겨 data 만 전달한다.)

import { get } from './client';

/**
 * 활성 chart-emitter 채널 요약 정보.
 * 백엔드 `system.ChartChannelInfo` 와 JSON 필드가 일치한다.
 */
export interface ChartChannelSummary {
  name: string;
  flow_id: string;
  node_id: string;
  buffer_size: number;
  retention_sec: number;
  subscriber_count: number;
  last_message_ms: number;
}

/**
 * 백엔드 응답의 내부 형상 (envelope 제거 후).
 * `{ channels: [...] }` 구조로 감싸져 있어 한 번 더 언랩한다.
 */
interface ChartChannelsEnvelope {
  channels: ChartChannelSummary[];
}

/**
 * 현재 실행 중인 chart-emitter 채널 목록을 조회한다.
 *
 * 실패 시 APIError 를 throw 한다. 호출자는 try/catch 또는
 * React Query 의 onError 로 처리한다.
 */
export async function listChartChannels(): Promise<ChartChannelSummary[]> {
  const body = await get<ChartChannelsEnvelope>('/charts/channels');
  return body?.channels ?? [];
}
