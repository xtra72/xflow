// 권한 행렬/메뉴 노출 변환 단위 테스트 (SPEC-AUTH-006 M2.3 — AC-04).
//
// 핵심 검증: 축을 카탈로그에서 파생하는가, 해당 리소스에 없는 액션의 셀을 비우는가,
// 그리고 데이터 축과 메뉴 노출 축(nav.*)을 분리하는가. 세 성질이 깨지면 서버 카탈로그
// 변경이 화면에 반영되지 않거나, 존재하지 않는 권한이 체크 가능한 것처럼 보이거나,
// 메뉴 이름이 액션 열로 승격되어 행렬이 대부분 빈 칸이 된다.
//
// @spec SPEC-AUTH-006 v0.1.0 (M2.3)

import { describe, expect, it } from 'vitest';

import {
  ACTION_ORDER,
  NAV_RESOURCE,
  buildNavMenuItems,
  buildPermissionMatrix,
} from './permissionMatrix';

/** 서버 카탈로그(internal/rbac/catalog.go)의 사전순 부분집합. */
const CATALOG = [
  'agent.create',
  'agent.delete',
  'agent.execute',
  'agent.read',
  'agent.update',
  'dashboard.read',
  'dashboard.update',
  'node.read',
];

/** 메뉴 노출 축이 섞인 카탈로그. 서버는 두 축을 한 배열로 내려준다. */
const CATALOG_WITH_NAV = [
  ...CATALOG,
  'nav.agent',
  'nav.dashboard',
  'nav.flow',
];

describe('buildPermissionMatrix', () => {
  it('액션 축을 표준 순서(read→create→update→delete→execute)로 정렬한다', () => {
    const matrix = buildPermissionMatrix(CATALOG);
    expect(matrix.actions).toEqual(['read', 'create', 'update', 'delete', 'execute']);
  });

  it('리소스 행을 카탈로그 등장 순서로 만든다', () => {
    const matrix = buildPermissionMatrix(CATALOG);
    expect(matrix.rows.map((r) => r.resource)).toEqual(['agent', 'dashboard', 'node']);
  });

  it('해당 리소스에 없는 액션 셀을 null 로 비운다 (node 는 read 만)', () => {
    const matrix = buildPermissionMatrix(CATALOG);
    const node = matrix.rows.find((r) => r.resource === 'node');

    expect(node?.cells).toEqual(['node.read', null, null, null, null]);
  });

  it('dashboard 는 read/update 만 채우고 create/delete/execute 를 비운다', () => {
    const matrix = buildPermissionMatrix(CATALOG);
    const dashboard = matrix.rows.find((r) => r.resource === 'dashboard');

    expect(dashboard?.cells).toEqual([
      'dashboard.read',
      null,
      'dashboard.update',
      null,
      null,
    ]);
  });

  it('모든 액션을 가진 리소스는 셀이 비지 않는다', () => {
    const matrix = buildPermissionMatrix(CATALOG);
    const agent = matrix.rows.find((r) => r.resource === 'agent');

    expect(agent?.cells).toEqual([
      'agent.read',
      'agent.create',
      'agent.update',
      'agent.delete',
      'agent.execute',
    ]);
  });

  it('카탈로그에 없는 액션은 열 자체를 만들지 않는다', () => {
    const matrix = buildPermissionMatrix(['node.read']);

    expect(matrix.actions).toEqual(['read']);
    expect(matrix.rows).toEqual([{ resource: 'node', cells: ['node.read'] }]);
  });

  it('표준 순서 밖의 새 액션은 뒤에 등장 순서로 덧붙인다 (서버 카탈로그 확장 대응)', () => {
    const matrix = buildPermissionMatrix(['flow.read', 'flow.approve', 'flow.archive']);

    expect(matrix.actions).toEqual(['read', 'approve', 'archive']);
    expect(matrix.rows[0]!.cells).toEqual(['flow.read', 'flow.approve', 'flow.archive']);
  });

  it('`<resource>.<action>` 형식이 아닌 키는 무시하고 행렬을 유지한다', () => {
    const matrix = buildPermissionMatrix(['broken', '.leading', 'trailing.', 'node.read']);

    expect(matrix.rows.map((r) => r.resource)).toEqual(['node']);
    expect(matrix.actions).toEqual(['read']);
  });

  it('빈 카탈로그는 빈 행렬을 만든다 (조회 실패 시 크래시 방지)', () => {
    expect(buildPermissionMatrix([])).toEqual({ actions: [], rows: [] });
  });

  it('cells 길이는 항상 actions 축과 같다', () => {
    const matrix = buildPermissionMatrix(CATALOG);
    for (const row of matrix.rows) {
      expect(row.cells).toHaveLength(matrix.actions.length);
    }
  });

  it('ACTION_ORDER 는 스펙의 5개 액션이다', () => {
    expect(ACTION_ORDER).toEqual(['read', 'create', 'update', 'delete', 'execute']);
  });
});

// 메뉴 노출 축은 액션 자리에 메뉴 식별자가 온다(nav.flow ...). 행렬에 그대로 두면
// 메뉴 이름이 액션 열로 승격되어 데이터 리소스와 열 범위가 겹치지 않고, 표의 대부분이
// 빈 칸이 된다. 두 축을 분리해 두는 것이 여기서 검증하는 성질이다.
describe('buildPermissionMatrix — 메뉴 노출 축(nav) 분리', () => {
  it('nav 를 행렬 행으로 만들지 않는다', () => {
    const matrix = buildPermissionMatrix(CATALOG_WITH_NAV);

    expect(matrix.rows.map((r) => r.resource)).toEqual(['agent', 'dashboard', 'node']);
    expect(matrix.rows.find((r) => r.resource === NAV_RESOURCE)).toBeUndefined();
  });

  it('메뉴 이름을 액션 열로 승격하지 않는다 — 축은 5개 CRUD 액션뿐이다', () => {
    const matrix = buildPermissionMatrix(CATALOG_WITH_NAV);

    expect(matrix.actions).toEqual(['read', 'create', 'update', 'delete', 'execute']);
  });

  it('nav 가 섞여도 데이터 행렬의 셀 구성은 변하지 않는다', () => {
    const withNav = buildPermissionMatrix(CATALOG_WITH_NAV);
    const withoutNav = buildPermissionMatrix(CATALOG);

    expect(withNav).toEqual(withoutNav);
  });

  it('nav 만 있는 카탈로그는 빈 행렬이 된다 (열이 만들어지지 않는다)', () => {
    expect(buildPermissionMatrix(['nav.flow', 'nav.agent'])).toEqual({
      actions: [],
      rows: [],
    });
  });

  it('표준 순서 밖의 새 데이터 액션은 nav 와 무관하게 축 뒤에 덧붙는다', () => {
    const matrix = buildPermissionMatrix(['flow.read', 'nav.flow', 'flow.approve']);

    expect(matrix.actions).toEqual(['read', 'approve']);
    expect(matrix.rows).toEqual([
      { resource: 'flow', cells: ['flow.read', 'flow.approve'] },
    ]);
  });
});

describe('buildNavMenuItems', () => {
  it('카탈로그의 nav.* 키만 정확히 나열한다', () => {
    expect(buildNavMenuItems(CATALOG_WITH_NAV)).toEqual([
      { key: 'nav.agent', menu: 'agent' },
      { key: 'nav.dashboard', menu: 'dashboard' },
      { key: 'nav.flow', menu: 'flow' },
    ]);
  });

  it('카탈로그 등장 순서를 보존한다 (서버가 정한 순서를 화면이 뒤집지 않는다)', () => {
    const items = buildNavMenuItems(['nav.system', 'nav.agent', 'nav.role']);

    expect(items.map((i) => i.menu)).toEqual(['system', 'agent', 'role']);
  });

  it('서버가 추가한 새 메뉴 키는 화면 수정 없이 목록에 나타난다', () => {
    const items = buildNavMenuItems([...CATALOG_WITH_NAV, 'nav.foo']);

    expect(items.map((i) => i.key)).toContain('nav.foo');
    expect(items.at(-1)).toEqual({ key: 'nav.foo', menu: 'foo' });
  });

  it('데이터 권한 키는 담지 않는다', () => {
    expect(buildNavMenuItems(CATALOG)).toEqual([]);
  });

  it('접두어만 비슷한 키(navigation.read)를 메뉴로 오인하지 않는다', () => {
    expect(buildNavMenuItems(['navigation.read', 'nav.flow'])).toEqual([
      { key: 'nav.flow', menu: 'flow' },
    ]);
  });

  it('중복 키는 한 번만 담는다 (체크박스 중복 렌더 방지)', () => {
    expect(buildNavMenuItems(['nav.flow', 'nav.flow'])).toEqual([
      { key: 'nav.flow', menu: 'flow' },
    ]);
  });

  it('`<리소스>.<액션>` 형식이 아닌 키는 무시한다', () => {
    expect(buildNavMenuItems(['nav', 'nav.', '.nav', 'nav.flow'])).toEqual([
      { key: 'nav.flow', menu: 'flow' },
    ]);
  });

  it('빈 카탈로그는 빈 목록을 만든다 (조회 실패 시 크래시 방지)', () => {
    expect(buildNavMenuItems([])).toEqual([]);
  });

  it('행렬과 메뉴 목록은 카탈로그를 빠짐없이 나눠 갖는다', () => {
    const matrix = buildPermissionMatrix(CATALOG_WITH_NAV);
    const nav = buildNavMenuItems(CATALOG_WITH_NAV);
    const covered = [
      ...matrix.rows.flatMap((r) => r.cells.filter((c): c is string => c !== null)),
      ...nav.map((i) => i.key),
    ];

    expect(covered.sort()).toEqual([...CATALOG_WITH_NAV].sort());
  });
});
