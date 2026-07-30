// SPEC-TRIGGER-PANEL-001 M4/M5: 트리거 설정 패널 순수 유틸리티.
//
// 카탈로그 스냅샷 주입(RD-11), FULL config 조립(RD-2), patch-then-PUT 정의
// 패치(RD-3/REQ-05-02), 동시 편집 감지(RD-8) 를 담당하는 부수효과 없는 함수 모음.
// 컴포넌트에서 분리하여 단위 테스트(§D/§E)가 UI 없이 검증하도록 한다.

/** 직렬화된 스케줄 한 건 (TriggerScheduleEditor 의 value/onChange 형태). */
export type SerializedSchedule = Record<string, unknown>;

/** 대시보드-로컬 명명 페이로드 카탈로그: name → payload 오브젝트. */
export type PayloadCatalog = Record<string, Record<string, unknown>>;

/** JSON 오브젝트(카탈로그 payload)의 깊은 복사. 스냅샷 격리에 사용한다. */
function deepClone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

/**
 * 카탈로그 페이로드를 특정 스케줄에 **스냅샷**으로 주입한다 (RD-11, REQ-04-02/04).
 *
 * 배정 시점에 `catalog[name]` 을 **깊은 복사**하여 `schedules[index].payload` 로
 * 인라인 복사한다. 깊은 복사이므로 이후 카탈로그를 편집해도 이미 주입된 스케줄의
 * payload 는 소급 변경되지 않는다(비소급). 스케줄은 사용자가 **재선택**할 때에만
 * 갱신된 값을 다시 복사한다. 노드는 이름/카탈로그를 절대 보지 않으며 resolved
 * inline payload 만 관측한다(REQ-04-03).
 *
 * @param schedules 현재 스케줄 배열
 * @param index 주입 대상 스케줄 인덱스
 * @param catalog 페이로드 카탈로그
 * @param name 배정할 카탈로그 이름
 * @returns 주입이 반영된 새 스케줄 배열 (원본 불변). 이름이 없으면 원본 그대로.
 */
export function injectCatalogSnapshot(
  schedules: SerializedSchedule[],
  index: number,
  catalog: PayloadCatalog,
  name: string,
): SerializedSchedule[] {
  const payload = catalog[name];
  if (payload === undefined) return schedules;
  const snapshot = deepClone(payload);
  return schedules.map((s, i) => (i === index ? { ...s, payload: snapshot } : s));
}

/**
 * FULL 트리거 config 를 조립한다 (RD-2, REQ-05-04).
 *
 * `Configure` 가 스케줄/페이로드를 리셋하므로 부분 델타는 설정 유실을 유발한다.
 * 현재 노드 config(baseConfig) 를 기반으로 편집된 schedules 를 교체하여 전체를
 * 전송한다. 노드 레벨 payload / source_ch_size 등 다른 config 키는 보존한다.
 */
export function buildFullTriggerConfig(
  baseConfig: Record<string, unknown>,
  schedules: SerializedSchedule[],
): Record<string, unknown> {
  return { ...baseConfig, schedules };
}

/** 정의(nodes/edges/inputs/outputs) 에서 노드 배열을 안전하게 추출한다. */
function definitionNodes(
  definition: Record<string, unknown>,
): Record<string, unknown>[] {
  return Array.isArray(definition.nodes)
    ? (definition.nodes as Record<string, unknown>[])
    : [];
}

/** 노드 오브젝트에서 config(reactflow data) 를 안전하게 추출한다. */
function nodeData(node: Record<string, unknown>): Record<string, unknown> {
  return node.data && typeof node.data === 'object'
    ? (node.data as Record<string, unknown>)
    : {};
}

/**
 * flow 정의에서 대상 nodeId 노드의 config(reactflow data) 를 fullConfig 로 패치한다
 * (RD-3, REQ-05-02: patch-then-PUT).
 *
 * 다른 노드/와이어/포트는 그대로 보존한다. 대상 노드는 `data` 의 nodeType/label 등
 * 표시 필드를 유지한 채 트리거 config 키(schedules/payload/...)만 덮어쓴다.
 * 플로우 상세(GET /flows/:id)는 정의를 `config` 에 담아 반환하며 노드 config 는
 * reactflow `data` 에 flat 하게 위치한다(EditorPage 하이드레이션과 동형).
 */
export function patchNodeConfigInDefinition(
  definition: Record<string, unknown>,
  nodeId: string,
  fullConfig: Record<string, unknown>,
): Record<string, unknown> {
  const patchedNodes = definitionNodes(definition).map((n) => {
    if (n.id !== nodeId) return n;
    return { ...n, data: { ...nodeData(n), ...fullConfig } };
  });
  return { ...definition, nodes: patchedNodes };
}

/** flow 정의에서 대상 nodeId 노드의 config(reactflow data) 를 찾는다(없으면 undefined). */
export function findNodeConfigInDefinition(
  definition: Record<string, unknown>,
  nodeId: string,
): Record<string, unknown> | undefined {
  const node = definitionNodes(definition).find((n) => n.id === nodeId);
  if (!node) return undefined;
  return nodeData(node);
}

/**
 * 동시 편집(덮어쓰기 가능성)을 감지한다 (RD-8, REQ-05-05).
 *
 * 지속화 직전 읽은 정의의 노드 config(schedules/payload) 가 패널 하이드레이션 시점
 * baseline 과 다르면 외부 편집이 있었던 것으로 보고 true 를 반환한다. version/etag
 * 낙관적 잠금은 도입하지 않으며(last-write-wins), 결과는 사용자 통지에만 쓰인다.
 */
export function detectConflict(
  freshNodeConfig: Record<string, unknown> | undefined,
  baselineConfig: Record<string, unknown>,
): boolean {
  if (!freshNodeConfig) return false;
  const pick = (c: Record<string, unknown>) =>
    JSON.stringify({ schedules: c.schedules ?? null, payload: c.payload ?? null });
  return pick(freshNodeConfig) !== pick(baselineConfig);
}
