// sysmetrics 패널 공통 config 헬퍼.
//
// 표시 옵션(열 수·갱신 주기·표시 구간)의 어휘와 규약은 모니터링 패널과 같다. 두
// 계열이 나란히 놓이는데 설정 이름이 다르면 사용자가 매번 다시 배워야 하므로,
// 공용으로 쓸 수 있는 것은 재작성하지 않고 그대로 가져다 쓴다.
//
// 여기서 새로 정의하는 것은 sysmetrics 에만 있는 두 축이다:
//   - 어느 에이전트를 볼지 (agent_id 정본 + agent_name 표시용)
//   - 어느 대상을 볼지 (인터페이스 / 마운트 / 장치)

import {
  readMaxCols,
  readRefreshMs,
  readWindowSec,
  readAccent,
} from '@/pages/dashboard/panels/monitor/monitorPanelConfig';

// 모니터링 패널과 같은 규약을 쓰는 헬퍼는 그대로 다시 내보낸다. 패널 코드가
// 두 경로에서 import 하지 않도록 창구를 하나로 둔다.
export { readMaxCols, readRefreshMs, readWindowSec, readAccent };

// --- 에이전트 바인딩 ---

/** 패널이 바인딩된 에이전트 */
export interface SysMetricsAgentRef {
  /** 안정적 ID (정본). 이름이 바뀌어도 연결이 끊기지 않는다. */
  agentId: string;
  /** 표시용 이름 스냅샷. 조회는 항상 agentId 로 한다. */
  agentName: string;
}

/**
 * 패널 config 에서 에이전트 참조를 읽는다.
 *
 * `agent_id` 가 정본이고 `agent_name` 은 표시용이다. 이름만 저장하면 에이전트를
 * 리네임했을 때 패널이 조용히 끊긴다 (SPEC-WEB-006 규약).
 */
export function readAgentRef(config: Record<string, unknown> | undefined): SysMetricsAgentRef {
  const agentId = typeof config?.agent_id === 'string' ? config.agent_id : '';
  const agentName = typeof config?.agent_name === 'string' ? config.agent_name : '';
  return { agentId, agentName };
}

// --- 대상 선택 ---

/** 대상 축 — 백엔드 targets 의 키와 같다. */
export type SysMetricsTargetKind = 'mountpoints' | 'devices' | 'interfaces';

/**
 * 패널이 그릴 대상 이름 목록을 읽는다.
 *
 * **빈 배열은 "종합"이며 기본값이다.** 개별 대상을 고르지 않은 패널은 전체 합산을
 * 그린다 — 이 규약 덕분에 "종합"과 "개별"을 별도 패널 유형으로 나누지 않아도 된다.
 */
export function readTargets(
  config: Record<string, unknown> | undefined,
  kind: SysMetricsTargetKind,
): string[] {
  const raw = config?.[kind];
  if (!Array.isArray(raw)) return [];
  return raw.filter((v): v is string => typeof v === 'string' && v !== '');
}

// --- 표시 항목 ---

// 시스템 패널의 표시 항목은 값 단위다 — `sysMetricsFields.ts` 의 SYSTEM_FIELDS 를 쓴다.
// 옛 그룹 키(cpu / memory / diskIo / network)는 그 파일의 normalizeSystemItems 가 옮긴다.

/** 스토리지 패널의 표시 항목 */
export type StorageItemKey = 'usage' | 'used' | 'free' | 'total';

/** 스토리지 패널 기본 항목 */
export const STORAGE_ITEMS: StorageItemKey[] = ['usage', 'used', 'free', 'total'];

/**
 * 표시 항목 목록을 읽는다.
 *
 * 키가 아예 없으면(신규 패널·구버전 config) 기본 항목을 쓴다. **빈 배열은 기본값으로
 * 되돌리지 않는다** — "사용자가 모두 껐다"는 정상 상태이며, 되돌리면 끈 항목이
 * 새로고침마다 살아난다. 모니터링 패널과 같은 규약이다.
 */
export function readItems<K extends string>(
  config: Record<string, unknown> | undefined,
  known: readonly K[],
  fallback: readonly K[],
): K[] {
  const raw = config?.items;
  if (!Array.isArray(raw)) return [...fallback];
  // 모르는 키는 버린다 — 패널 유형을 바꾸며 남은 다른 어휘의 항목이 섞일 수 있다.
  return raw.filter((v): v is K => typeof v === 'string' && (known as readonly string[]).includes(v));
}
