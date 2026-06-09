// SPEC-SUBFLOW-001 v1.3 (REQ-RU06): 원격 브릿지 flow-node 의 라이브 상태 매핑.
//
// flow_id 가 `remote://{instance_id}/{flow_id}` 인 flow-node 는 배포 시 LIVE BRIDGE 다:
// 대상 원격 노드가 참조 플로우를 실행하고 메시지를 WS 로 브릿지한다(백엔드 P1–P3).
// 캔버스는 이미 노드별 런타임 상태(NodeRuntimeStats.state, getFlowNodes 의 노드 state)를
// 받으므로, 그 generic state 문자열을 브릿지 상태(connected/running/offline/error)로
// 매핑해 노드 카드에 색상 점으로 반영한다. 별도 백엔드 status 엔드포인트는 추가하지 않는다.
//
// state 문자열의 정확한 어휘는 백엔드 소관이므로, 알려진 값을 분류하고 그 외 비어있지
// 않은 값은 'connected'(활성·연결됨, 중립 표시)로 폴백한다. state 가 없으면(플로우 미실행)
// 'unknown' — 라이브 상태 미표시, 정적 브릿지 인디케이터만 노출한다.

/** 원격 브릿지의 표시용 상태. */
export type RemoteBridgeStatus =
  | 'running'
  | 'connected'
  | 'offline'
  | 'error'
  | 'unknown';

/**
 * 노드 런타임 state 문자열을 원격 브릿지 표시 상태로 매핑한다(순수 함수).
 *
 * 대소문자/공백을 정규화한 뒤 알려진 어휘로 분류한다:
 * - running/active        → 'running' (초록, 실행 중)
 * - connected/online/ready→ 'connected' (파랑, 연결됨)
 * - offline/disconnected/stopped/idle → 'offline' (회색, 오프라인)
 * - error/failed/fault    → 'error' (빨강, 오류)
 * - 그 외 비어있지 않은 값 → 'connected' (중립 활성 표시)
 * - 비어있음/undefined     → 'unknown' (라이브 상태 없음)
 *
 * @param state - NodeRuntimeStats.state(플로우 미실행 시 undefined).
 * @returns 표시용 브릿지 상태.
 */
export function mapRuntimeStateToBridgeStatus(
  state: string | undefined,
): RemoteBridgeStatus {
  const normalized = (state ?? '').trim().toLowerCase();
  if (normalized === '') return 'unknown';
  switch (normalized) {
    case 'running':
    case 'active':
      return 'running';
    case 'connected':
    case 'online':
    case 'ready':
      return 'connected';
    case 'offline':
    case 'disconnected':
    case 'stopped':
    case 'idle':
      return 'offline';
    case 'error':
    case 'failed':
    case 'fault':
      return 'error';
    default:
      // 알 수 없는 비어있지 않은 state 는 중립적으로 "연결됨"으로 본다.
      return 'connected';
  }
}

/** 브릿지 상태 → Tailwind 색상 점 클래스(노드 카드 상태 점 표시용). */
export const BRIDGE_STATUS_DOT_CLASS: Record<RemoteBridgeStatus, string> = {
  running: 'bg-emerald-500',
  connected: 'bg-sky-500',
  offline: 'bg-zinc-400 dark:bg-zinc-500',
  error: 'bg-red-500',
  unknown: 'bg-zinc-300 dark:bg-zinc-600',
};

/** 브릿지 상태 → i18n 상태 용어 키(노드 카드 툴팁/접근성 라벨용). */
export const BRIDGE_STATUS_I18N_KEY: Record<RemoteBridgeStatus, string> = {
  running: 'remote.bridge.status.running',
  connected: 'remote.bridge.status.connected',
  offline: 'remote.bridge.status.offline',
  error: 'remote.bridge.status.error',
  unknown: 'remote.bridge.status.unknown',
};
