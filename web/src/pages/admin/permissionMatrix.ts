// 권한 카탈로그 → 리소스×액션 행렬 변환 (SPEC-AUTH-006 M2.3 — AC-04).
//
// 역할 관리 화면과 분리해 둔다. 순수 함수라 화면 렌더 없이 직접 검증할 수 있고,
// 페이지 파일이 컴포넌트 외 심볼을 내보내지 않아 fast-refresh 경계도 유지된다.
//
// @spec SPEC-AUTH-006 v0.1.0 (M2.3)

/**
 * 액션 축의 표준 순서. 카탈로그에 실제로 등장하는 액션만 축에 남는다.
 *
 * 카탈로그에 이 목록 밖의 액션이 새로 생기면 뒤에 등장 순서대로 덧붙여, 화면
 * 수정 없이도 새 액션이 행렬에 나타난다.
 */
export const ACTION_ORDER: readonly string[] = [
  'read',
  'create',
  'update',
  'delete',
  'execute',
];

/** 행렬 한 행. `cells` 는 액션 축과 길이가 같고, 없는 조합은 null 이다. */
export interface PermissionMatrixRow {
  resource: string;
  cells: (string | null)[];
}

/** 리소스×액션 행렬. */
export interface PermissionMatrix {
  actions: string[];
  rows: PermissionMatrixRow[];
}

/**
 * 평면 권한 키 배열에서 리소스×액션 행렬을 만든다.
 *
 * - 행(리소스)은 카탈로그 등장 순서를 따른다(서버가 사전순으로 내려준다).
 * - 열(액션)은 ACTION_ORDER 순서이되, 카탈로그에 없는 액션은 열 자체를 만들지 않는다.
 * - `<resource>.<action>` 형식이 아닌 키는 조용히 무시한다(행렬이 깨지지 않는다).
 */
export function buildPermissionMatrix(catalog: readonly string[]): PermissionMatrix {
  const known = new Set<string>();
  const resources: string[] = [];
  const seenResources = new Set<string>();
  const actionsSeen: string[] = [];
  const seenActions = new Set<string>();

  for (const key of catalog) {
    const dot = key.indexOf('.');
    if (dot <= 0 || dot === key.length - 1) continue;

    const resource = key.slice(0, dot);
    const action = key.slice(dot + 1);
    known.add(`${resource}.${action}`);

    if (!seenResources.has(resource)) {
      seenResources.add(resource);
      resources.push(resource);
    }
    if (!seenActions.has(action)) {
      seenActions.add(action);
      actionsSeen.push(action);
    }
  }

  const ordered = ACTION_ORDER.filter((action) => seenActions.has(action));
  const extras = actionsSeen.filter((action) => !ACTION_ORDER.includes(action));
  const actions = [...ordered, ...extras];

  return {
    actions,
    rows: resources.map((resource) => ({
      resource,
      cells: actions.map((action) =>
        known.has(`${resource}.${action}`) ? `${resource}.${action}` : null,
      ),
    })),
  };
}
