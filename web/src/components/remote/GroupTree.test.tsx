// GroupTree 테스트 (그룹 관리 좌측 트리).
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { ManagedNode, NodeGroup } from '@/types/remote';

import { GroupTree } from './GroupTree';

function node(id: string, group: string): ManagedNode {
  return {
    instance_id: id,
    hostname: `host-${id}`,
    version: 'v1',
    status: 'approved',
    online: true,
    last_seen: 0,
    group_name: group,
  };
}

const namedGroups: NodeGroup[] = [
  { group_name: 'dev', node_count: 1 },
  { group_name: 'prod', node_count: 2 },
];
const nodesByGroup = new Map<string, ManagedNode[]>([
  ['prod', [node('p1', 'prod'), node('p2', 'prod')]],
  ['dev', [node('d1', 'dev')]],
  ['', [node('u1', '')]],
]);

function renderTree(over?: {
  selectedGroup?: string | null;
  selectedNodeId?: string | null;
  createSelected?: boolean;
  onSelectGroup?: (n: string) => void;
  onSelectNode?: (id: string) => void;
  onSelectCreate?: () => void;
}): void {
  render(
    <I18nProvider>
      <GroupTree
        namedGroups={namedGroups}
        nodesByGroup={nodesByGroup}
        selectedGroup={over?.selectedGroup ?? null}
        selectedNodeId={over?.selectedNodeId ?? null}
        createSelected={over?.createSelected ?? false}
        onSelectGroup={over?.onSelectGroup ?? vi.fn()}
        onSelectNode={over?.onSelectNode ?? vi.fn()}
        onSelectCreate={over?.onSelectCreate ?? vi.fn()}
      />
    </I18nProvider>,
  );
}

describe('GroupTree', () => {
  it('명명 그룹 + 미분류 + 신규추가를 순서대로 렌더한다', () => {
    renderTree();
    expect(screen.getByTestId('tree-group-dev')).toBeInTheDocument();
    expect(screen.getByTestId('tree-group-prod')).toBeInTheDocument();
    // 미분류(group_name="") + 신규추가.
    expect(screen.getByTestId('tree-group-ungrouped')).toBeInTheDocument();
    expect(screen.getByTestId('tree-add-group')).toBeInTheDocument();
  });

  it('선택된 그룹은 기본 펼침되어 멤버 노드가 보인다', () => {
    renderTree({ selectedGroup: 'prod' });
    expect(screen.getByTestId('tree-node-p1')).toBeInTheDocument();
    expect(screen.getByTestId('tree-node-p2')).toBeInTheDocument();
  });

  it('그룹/노드/신규추가 클릭이 각 콜백을 호출한다', () => {
    const onSelectGroup = vi.fn();
    const onSelectNode = vi.fn();
    const onSelectCreate = vi.fn();
    renderTree({ selectedGroup: 'prod', onSelectGroup, onSelectNode, onSelectCreate });

    fireEvent.click(screen.getByTestId('tree-group-dev'));
    expect(onSelectGroup).toHaveBeenCalledWith('dev');

    fireEvent.click(screen.getByTestId('tree-node-p1'));
    expect(onSelectNode).toHaveBeenCalledWith('p1');

    fireEvent.click(screen.getByTestId('tree-add-group'));
    expect(onSelectCreate).toHaveBeenCalled();
  });

  it('미분류 그룹 선택 콜백은 빈 문자열을 넘긴다', () => {
    const onSelectGroup = vi.fn();
    renderTree({ onSelectGroup });
    fireEvent.click(screen.getByTestId('tree-group-ungrouped'));
    expect(onSelectGroup).toHaveBeenCalledWith('');
  });
});
