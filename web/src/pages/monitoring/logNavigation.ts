// 로그 항목 → 대상 페이지 링크.
//
// 로그에는 ID 가 없고 이름(componentName)만 있으므로, 목록 페이지로 이동해
// 그 이름으로 찾아 주는 방식이다(이름 기반 딥링크). 이름이 중복되면 목록이
// 검색어로 좁혀진 채 열리므로 사용자가 직접 고를 수 있다.

/** 이동할 수 있는 로그 소스 → 목록 페이지 경로 */
const LINKABLE_ROUTES: Record<string, string> = {
  agent: '/agents',
  flow: '/flows',
  node: '/nodes',
};

/**
 * 로그 항목이 가리키는 페이지 경로를 만든다. 이동할 수 없으면 null.
 *
 * `api` / `engine` / `system` 소스는 대응하는 목록 페이지가 없어 링크하지 않는다.
 * 이름이 비어 있어도 찾아갈 대상이 없으므로 링크하지 않는다.
 */
export function logComponentTarget(
  source: string | undefined,
  name: string | undefined,
): string | null {
  if (!source || !name) return null;
  const base = LINKABLE_ROUTES[source];
  if (!base) return null;
  return `${base}?name=${encodeURIComponent(name)}`;
}

/** 로그 항목을 클릭해 이동할 수 있는지 */
export function isLinkableLogComponent(
  source: string | undefined,
  name: string | undefined,
): boolean {
  return logComponentTarget(source, name) !== null;
}
