// NodePalette deprecated 별칭 필터링 회귀 테스트.
//
// 노드 타입 레지스트리 API(/nodes)는 옛 `_` HVAC 별칭을 deprecated 로 함께
// 반환한다(source === 'builtin-deprecated'). 팔레트에는 canonical `-` 타입만
// 노출되어 중복이 없어야 한다.

import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { NodeTypeInfo } from '@/types/node';

const useNodeTypesMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useNodeTypes', () => ({
  useNodeTypes: () => useNodeTypesMock(),
}));

import { NodePalette } from './NodePalette';

const CANONICAL: NodeTypeInfo = {
  type: 'samsung-hvacr01-status',
  category: 'io',
  description: 'Samsung 상태 수신',
  source: 'builtin',
};
const DEPRECATED: NodeTypeInfo = {
  type: 'samsung_hvacr01_status',
  category: 'io',
  description: 'Samsung 상태 수신 (deprecated)',
  source: 'builtin-deprecated',
};
// 같은 io 카테고리(첫 그룹, 기본 펼침)에 둬 렌더를 검증한다.
const CANONICAL_2: NodeTypeInfo = {
  type: 'lg-hvacr02',
  category: 'io',
  description: 'LG 통합',
  source: 'builtin',
};

describe('NodePalette — deprecated 별칭 필터링', () => {
  it('source === "builtin-deprecated" 인 노드 타입을 팔레트에서 제외한다', () => {
    useNodeTypesMock.mockReturnValue({
      data: [CANONICAL, DEPRECATED, CANONICAL_2],
      isLoading: false,
    });
    render(<NodePalette />);

    // canonical 타입들은 노출된다 (첫 io 카테고리는 기본 펼침).
    expect(screen.getByText('samsung-hvacr01-status')).toBeInTheDocument();
    expect(screen.getByText('lg-hvacr02')).toBeInTheDocument();
    // deprecated 별칭(옛 `_` 표기)은 노출되지 않는다.
    expect(screen.queryByText('samsung_hvacr01_status')).not.toBeInTheDocument();
  });

  it('데이터가 없으면 빈 목록으로 안전하게 렌더한다', () => {
    useNodeTypesMock.mockReturnValue({ data: undefined, isLoading: false });
    expect(() => render(<NodePalette />)).not.toThrow();
  });
});
