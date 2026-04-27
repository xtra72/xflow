// TagFilterChips 컴포넌트 및 matchesTagFilter 유틸 테스트.
//
// - 칩 클릭으로 선택 토글 (재클릭 시 해제)
// - 전체 해제 버튼으로 모든 선택 제거
// - AND 로직: 여러 필터 선택 시 모두 만족해야 true
// - 필터 없음(빈 셋) 이면 항상 true
// - 태그가 없는 엔트리는 필터 활성 시 false
//
// @spec SPEC-STORE-003

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { TagFilterChips, matchesTagFilter, makeFilterId } from './TagFilterChips';
import type { StoreTagPair } from '@/services/api/store';

const SAMPLE_PAIRS: StoreTagPair[] = [
  { key: 'room', values: ['1', '2', '3'] },
  { key: 'type', values: ['temperature', 'humidity'] },
];

describe('TagFilterChips', () => {
  it('제공된 모든 태그 키와 값을 칩으로 렌더링한다', () => {
    render(
      <TagFilterChips
        pairs={SAMPLE_PAIRS}
        selected={new Set()}
        onToggle={vi.fn()}
        onClearAll={vi.fn()}
      />,
    );
    // 태그 키 레이블 확인
    expect(screen.getByText('room:')).toBeInTheDocument();
    expect(screen.getByText('type:')).toBeInTheDocument();
    // 각 태그 값이 칩 버튼으로 렌더링됐는지 확인
    expect(screen.getByTestId('tag-filter-room-1')).toBeInTheDocument();
    expect(screen.getByTestId('tag-filter-room-2')).toBeInTheDocument();
    expect(screen.getByTestId('tag-filter-room-3')).toBeInTheDocument();
    expect(screen.getByTestId('tag-filter-type-temperature')).toBeInTheDocument();
    expect(screen.getByTestId('tag-filter-type-humidity')).toBeInTheDocument();
  });

  it('칩 클릭 시 makeFilterId 를 사용한 onToggle 호출', () => {
    const onToggle = vi.fn();
    render(
      <TagFilterChips
        pairs={SAMPLE_PAIRS}
        selected={new Set()}
        onToggle={onToggle}
        onClearAll={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByTestId('tag-filter-room-1'));
    expect(onToggle).toHaveBeenCalledWith('room=1');
  });

  it('선택된 칩은 aria-pressed=true 를 갖는다', () => {
    render(
      <TagFilterChips
        pairs={SAMPLE_PAIRS}
        selected={new Set(['room=1', 'type=temperature'])}
        onToggle={vi.fn()}
        onClearAll={vi.fn()}
      />,
    );
    expect(screen.getByTestId('tag-filter-room-1')).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByTestId('tag-filter-room-2')).toHaveAttribute('aria-pressed', 'false');
    expect(screen.getByTestId('tag-filter-type-temperature')).toHaveAttribute(
      'aria-pressed',
      'true',
    );
  });

  it('선택이 없을 때는 "전체 해제" 버튼을 표시하지 않는다', () => {
    render(
      <TagFilterChips
        pairs={SAMPLE_PAIRS}
        selected={new Set()}
        onToggle={vi.fn()}
        onClearAll={vi.fn()}
      />,
    );
    expect(screen.queryByRole('button', { name: /전체 해제/ })).not.toBeInTheDocument();
  });

  it('선택이 있을 때 "전체 해제" 클릭 시 onClearAll 호출', () => {
    const onClearAll = vi.fn();
    render(
      <TagFilterChips
        pairs={SAMPLE_PAIRS}
        selected={new Set(['room=1'])}
        onToggle={vi.fn()}
        onClearAll={onClearAll}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /전체 해제/ }));
    expect(onClearAll).toHaveBeenCalled();
  });

  it('선택된 필터를 헤더에 "key=value" 형식으로 요약 표시', () => {
    render(
      <TagFilterChips
        pairs={SAMPLE_PAIRS}
        selected={new Set(['room=1', 'type=temperature'])}
        onToggle={vi.fn()}
        onClearAll={vi.fn()}
      />,
    );
    // 요약 텍스트 포함 여부
    expect(screen.getByText(/필터:/)).toBeInTheDocument();
    // "room=1, type=temperature" 형식의 요약이 존재해야 함
    const summary = screen.getByText(/room=1/);
    expect(summary.textContent).toContain('room=1');
    expect(summary.textContent).toContain('type=temperature');
  });
});

describe('makeFilterId', () => {
  it('key=value 형식 문자열을 만든다', () => {
    expect(makeFilterId('room', '1')).toBe('room=1');
    expect(makeFilterId('type', 'temperature')).toBe('type=temperature');
  });

  it('값에 = 가 포함되어도 첫 번째 = 만 구분자로 사용된다', () => {
    // 현재 구현: 단순 concat. 값에 = 가 포함되면 파싱 측에서 split 경계가 모호.
    // 이 테스트는 makeFilterId 자체의 동작을 고정(document the behavior)한다.
    expect(makeFilterId('a', 'b=c')).toBe('a=b=c');
  });
});

describe('matchesTagFilter', () => {
  it('선택이 비어있으면 항상 true', () => {
    expect(matchesTagFilter({ room: '1' }, new Set())).toBe(true);
    expect(matchesTagFilter(null, new Set())).toBe(true);
    expect(matchesTagFilter(undefined, new Set())).toBe(true);
  });

  it('단일 필터가 일치하면 true', () => {
    expect(matchesTagFilter({ room: '1' }, new Set(['room=1']))).toBe(true);
  });

  it('단일 필터가 불일치하면 false', () => {
    expect(matchesTagFilter({ room: '2' }, new Set(['room=1']))).toBe(false);
  });

  it('AND 로직: 모든 필터가 일치해야 true', () => {
    const tags = { room: '1', type: 'temperature' };
    expect(matchesTagFilter(tags, new Set(['room=1', 'type=temperature']))).toBe(true);
    // 하나라도 불일치하면 false
    expect(matchesTagFilter(tags, new Set(['room=1', 'type=humidity']))).toBe(false);
    expect(matchesTagFilter(tags, new Set(['room=2', 'type=temperature']))).toBe(false);
  });

  it('태그가 null/undefined 인데 필터 활성이면 false', () => {
    expect(matchesTagFilter(null, new Set(['room=1']))).toBe(false);
    expect(matchesTagFilter(undefined, new Set(['room=1']))).toBe(false);
  });

  it('빈 태그 객체는 필터 활성 시 false', () => {
    expect(matchesTagFilter({}, new Set(['room=1']))).toBe(false);
  });

  it('같은 키의 서로 다른 값 동시 선택 시 (AND) 항상 false', () => {
    // room=1 이면서 room=2 인 엔트리는 존재할 수 없으므로 AND 논리 상 false.
    const tags = { room: '1' };
    expect(matchesTagFilter(tags, new Set(['room=1', 'room=2']))).toBe(false);
  });

  it('태그 값에 = 가 포함된 경우 첫 번째 = 만 구분자로 사용', () => {
    // makeFilterId('tag', 'a=b') → "tag=a=b" → split 결과 key="tag", value="a=b"
    expect(matchesTagFilter({ tag: 'a=b' }, new Set(['tag=a=b']))).toBe(true);
    expect(matchesTagFilter({ tag: 'x' }, new Set(['tag=a=b']))).toBe(false);
  });

  it('잘못된 형식의 필터 ID (= 없음) 는 무시되어 항상 일치로 간주', () => {
    // 현재 구현: "= 없음" 이면 continue → 다른 필터가 없으면 true.
    // 실제로는 makeFilterId 를 통해서만 생성되므로 이 경로는 극히 드물다.
    expect(matchesTagFilter({ room: '1' }, new Set(['malformed']))).toBe(true);
  });
});
