// 권한 카탈로그 → 리소스×액션 행렬 + 메뉴 노출 목록 변환 (SPEC-AUTH-006 M2.3 — AC-04).
//
// 역할 관리 화면과 분리해 둔다. 순수 함수라 화면 렌더 없이 직접 검증할 수 있고,
// 페이지 파일이 컴포넌트 외 심볼을 내보내지 않아 fast-refresh 경계도 유지된다.
//
// 카탈로그에는 성격이 다른 두 축이 섞여 있다.
//   - 데이터 권한: `<리소스>.<액션>` (agent.read, flow.execute ...)
//   - 메뉴 노출:   `nav.<메뉴>`      (nav.flow, nav.dashboard ...)
// 액션 자리에 메뉴 식별자가 오는 것은 의도된 설계다 — internal/rbac/catalog.go 의
// ResourceNav 주석대로, 데이터를 읽을 수 있어도 메뉴는 감출 수 있어야 하므로 두 축을
// 독립적으로 부여한다. 그래서 화면에서도 축을 섞지 않는다: nav 를 행렬 행으로 두면
// 메뉴 이름 11개가 액션 열로 승격되어 행렬의 76% 가 빈 칸이 된다.
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

/**
 * 메뉴 노출 축의 리소스 이름.
 *
 * 이 리소스는 행렬에서 제외하고 별도 체크리스트로 그린다 (buildNavMenuItems).
 */
export const NAV_RESOURCE = 'nav';

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

/** 메뉴 노출 체크리스트 한 항목. */
export interface NavMenuItem {
  /** 권한 키 전체(`nav.flow`). 체크박스 값이자 제출 키다. */
  key: string;
  /** 메뉴 식별자(`flow`). 화면 라벨로 쓴다. */
  menu: string;
}

/** `<리소스>.<액션>` 을 분해한다. 형식이 아니면 null. */
function splitKey(key: string): { resource: string; action: string } | null {
  const dot = key.indexOf('.');
  if (dot <= 0 || dot === key.length - 1) return null;
  return { resource: key.slice(0, dot), action: key.slice(dot + 1) };
}

/**
 * 평면 권한 키 배열에서 데이터 리소스×액션 행렬을 만든다.
 *
 * - 행(리소스)은 카탈로그 등장 순서를 따른다(서버가 사전순으로 내려준다).
 * - 열(액션)은 ACTION_ORDER 순서이되, 카탈로그에 없는 액션은 열 자체를 만들지 않는다.
 * - `nav.*` 는 행렬에서 제외한다 — 메뉴 노출 축이라 액션 자리에 메뉴 이름이 온다.
 *   행렬에 두면 메뉴 이름이 액션 열이 되어 데이터 리소스와 열 범위가 어긋난다.
 * - `<리소스>.<액션>` 형식이 아닌 키는 조용히 무시한다(행렬이 깨지지 않는다).
 */
export function buildPermissionMatrix(catalog: readonly string[]): PermissionMatrix {
  const known = new Set<string>();
  const resources: string[] = [];
  const seenResources = new Set<string>();
  const actionsSeen: string[] = [];
  const seenActions = new Set<string>();

  for (const key of catalog) {
    const parsed = splitKey(key);
    if (parsed === null) continue;

    const { resource, action } = parsed;
    if (resource === NAV_RESOURCE) continue;
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

/**
 * 평면 권한 키 배열에서 메뉴 노출 목록(`nav.*`)을 만든다.
 *
 * 카탈로그에서 파생하므로 서버에 `nav.<새메뉴>` 가 추가되면 화면 수정 없이 항목이
 * 늘어난다 — 행렬의 액션 축이 ACTION_ORDER 뒤에 새 액션을 덧붙이는 것과 같은 성질이다.
 * 순서는 카탈로그 등장 순서를 따르고, 중복 키는 한 번만 담는다.
 */
export function buildNavMenuItems(catalog: readonly string[]): NavMenuItem[] {
  const items: NavMenuItem[] = [];
  const seen = new Set<string>();

  for (const key of catalog) {
    const parsed = splitKey(key);
    if (parsed === null || parsed.resource !== NAV_RESOURCE) continue;
    if (seen.has(key)) continue;

    seen.add(key);
    items.push({ key, menu: parsed.action });
  }

  return items;
}
