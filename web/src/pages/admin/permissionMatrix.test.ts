// 권한 행렬 변환 단위 테스트 (SPEC-AUTH-006 M2.3 — AC-04).
//
// 핵심 검증: 축을 카탈로그에서 파생하는가, 그리고 해당 리소스에 없는 액션의
// 셀을 비우는가. 두 성질이 깨지면 서버 카탈로그 변경이 화면에 반영되지 않거나
// 존재하지 않는 권한이 체크 가능한 것처럼 보인다.
//
// @spec SPEC-AUTH-006 v0.1.0 (M2.3)

import { describe, expect, it } from 'vitest';

import { ACTION_ORDER, buildPermissionMatrix } from './permissionMatrix';

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
