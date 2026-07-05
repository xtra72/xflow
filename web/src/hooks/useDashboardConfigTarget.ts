// useDashboardConfigTarget — 대시보드 config 소스를 target 으로 전환한다
// (SPEC-REMOTE-001 M10, 그룹 L, REQ-L01/L02/L03/L10).
//
//   - 로컬: useUIStore 에 동기화된 config(useDashboardSync 가 서버와 PUT/GET
//     양방향 동기). 본 훅은 로컬 분기에서 config 를 읽지 않는다 — 로컬 경로는
//     기존 DashboardPage(useUIStore + useDashboardSync)를 그대로 사용하므로,
//     본 훅은 원격 분기에서만 의미를 가진다(원격 read-only config 취득).
//   - 원격: GET /remote/nodes/{id}/dashboards/{shared|mine} 로 노드의 대시보드
//     config 를 READ-ONLY 취득한다(REQ-L01). 편집/PUT/sync 없음(v1.5 비목표 —
//     REQ-L12). config 내부 deviceId 는 그 노드 기준 해석(노드-로컬 — REQ-L03).
//
// 원격 config 는 단기 변동이 적으므로 적당한 polling 주기를 둔다(서버 TTL 캐시가
// 노드 부하를 완화 — REQ-J16). 실패(503/504/502/404)는 error 로 노출되어 호출
// 측이 editError 매핑으로 표시한다(REQ-L11).

import { useQuery } from '@tanstack/react-query';

import { isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';
import * as remoteService from '@/services/api/remoteService';
import type { RemoteDashboardScope } from '@/services/api/remoteService';
import type { DashboardPayload } from '@/types/dashboard';

/** 원격 대시보드 config 갱신 주기(ms). config 는 완만 변동이므로 길게 둔다. */
const REMOTE_CONFIG_REFETCH_MS = 15000;

// ---- 응답 형태 정규화 (페이로드 robust 파싱) ----
//
// 노드 프록시(GET /remote/nodes/{id}/dashboards/{shared|mine})는 현재 두 가지
// 형태 중 하나를 반환할 수 있다. RemoteDashboardView 가 항상 패널을 렌더하도록
// 양쪽을 모두 수용해 `DashboardPayload` 로 정규화한다:
//
//   1) DTO 형태(정상/미래):  { scope, owner, version, updatedAt, payload }
//      - payload 가 이미 디코드된 DashboardPayload 객체. 그대로 사용.
//   2) RAW 노드 형태(현행):  { Scope, Owner, Version, UpdatedAt, Payload }
//      - 대문자 키 + Payload 는 대시보드 JSON 의 base64 문자열. 디코드 필요.
//
// 렌더에 실제로 필요한 것은 payload 뿐이므로 owner/scope 매핑은 생략한다.
// 디코드/파싱 실패는 throw 하지 않고 undefined 를 돌려준다(→ 빈 상태, 크래시 X).

/** 값이 비-null 객체인지 좁힌다. */
function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

/** DashboardPayload 후보 객체인지(최소 식별 필드) 판별한다. */
function looksLikeDashboardPayload(value: unknown): value is DashboardPayload {
  return isObjectRecord(value) && 'dashboardPages' in value;
}

/**
 * base64 문자열을 유니코드 안전하게 디코드해 JSON 파싱한다. 실패 시 undefined.
 *
 * 대시보드 JSON 은 ASCII-safe 이지만, 멀티바이트가 섞여도 깨지지 않도록
 * atob → 바이트 배열 → TextDecoder(UTF-8) 경로로 디코드한다.
 */
function decodeBase64Json(encoded: string): DashboardPayload | undefined {
  try {
    const binary = atob(encoded);
    const bytes = Uint8Array.from(binary, (ch) => ch.charCodeAt(0));
    const json = new TextDecoder('utf-8').decode(bytes);
    const parsed: unknown = JSON.parse(json);
    return looksLikeDashboardPayload(parsed) ? parsed : undefined;
  } catch {
    return undefined;
  }
}

/**
 * 노드 프록시 응답을 `DashboardPayload` 로 정규화한다(DTO·RAW 양쪽 수용).
 *
 * 해석 우선순위:
 *   1) `payload` 가 DashboardPayload 객체 → 그대로.
 *   2) `Payload` 가 DashboardPayload 객체(혹시 노드가 디코드한 경우) → 그대로.
 *   3) `payload`/`Payload` 가 문자열 → base64 디코드 + JSON 파싱.
 * 위 어느 것도 payload 를 resolve 하지 못하면 undefined(→ 빈 상태).
 *
 * @param raw - 서비스 레이어가 언래핑한 응답(형태 불명 — unknown).
 */
export function normalizeDashboardSnapshot(
  raw: unknown,
): DashboardPayload | undefined {
  if (!isObjectRecord(raw)) return undefined;

  // 소문자/대문자 양쪽 payload 키를 후보로 본다(DTO=payload, RAW=Payload).
  const candidates: unknown[] = [raw.payload, raw.Payload];

  for (const candidate of candidates) {
    // 이미 디코드된 객체.
    if (looksLikeDashboardPayload(candidate)) return candidate;
    // base64 문자열 → 디코드 시도.
    if (typeof candidate === 'string' && candidate.length > 0) {
      const decoded = decodeBase64Json(candidate);
      if (decoded) return decoded;
    }
  }

  return undefined;
}

/** useDashboardConfigTarget 반환 형태. */
export interface DashboardConfigTargetResult {
  /** 취득한 READ-ONLY config payload(원격, 미수신 시 undefined). */
  payload: DashboardPayload | undefined;
  isLoading: boolean;
  error: unknown;
  refetch: () => void;
}

/**
 * 원격 노드의 대시보드 config 를 스코프별로 READ-ONLY 취득한다(REQ-L01).
 *
 * @param target - 원격 노드 타깃(로컬이면 비활성 — payload undefined).
 * @param scope - 'shared' | 'mine'(로컬 탭과 동일 의미).
 * @param enabled - 쿼리 활성 여부(노드 ready 아닐 때 false 로 발행 차단).
 */
export function useDashboardConfigTarget(
  target: ResourceTarget,
  scope: RemoteDashboardScope,
  enabled = true,
): DashboardConfigTargetResult {
  const remote = isRemoteTarget(target);
  const instanceId = remote ? target.instanceId : '';

  // 응답 형태가 불확정(DTO 또는 RAW base64)이므로 unknown 으로 받아 normalize 한다.
  // getRemoteDashboard 는 DashboardSnapshot 을 선언하지만 현행 노드는 RAW 형태를
  // 반환할 수 있어, 타입 단언 대신 unknown 로 좁혀 robust 하게 처리한다.
  const query = useQuery<unknown>({
    queryKey: ['remote', 'dashboard', instanceId, scope],
    queryFn: () => remoteService.getRemoteDashboard(instanceId, scope),
    enabled: remote && enabled && !!instanceId,
    refetchInterval: REMOTE_CONFIG_REFETCH_MS,
  });

  return {
    payload: remote ? normalizeDashboardSnapshot(query.data) : undefined,
    isLoading: remote ? query.isLoading : false,
    error: remote ? query.error : null,
    refetch: () => void query.refetch(),
  };
}
