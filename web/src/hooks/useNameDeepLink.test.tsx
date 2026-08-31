// 이름 기반 딥링크 수신 훅 테스트.

import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

import { useNameDeepLink } from './useNameDeepLink';

interface Item {
  id: string;
  name: string;
}

const getName = (i: Item) => i.name;
const getId = (i: Item) => i.id;

/** 훅만 호출하는 최소 컴포넌트 */
function Probe({ items, apply }: { items: Item[]; apply: (s: string, id: string | null) => void }) {
  useNameDeepLink(items, getName, getId, apply);
  return null;
}

function renderAt(url: string, items: Item[], apply: (s: string, id: string | null) => void) {
  return render(
    <MemoryRouter initialEntries={[url]}>
      <Probe items={items} apply={apply} />
    </MemoryRouter>,
  );
}

describe('useNameDeepLink', () => {
  const items: Item[] = [
    { id: 'a1', name: 'modbus-1' },
    { id: 'a2', name: 'modbus-2' },
  ];

  it('이름이 일치하면 검색어와 항목 id 를 넘긴다', () => {
    const apply = vi.fn();
    renderAt('/agents?name=modbus-2', items, apply);

    expect(apply).toHaveBeenCalledWith('modbus-2', 'a2');
  });

  it('일치하는 항목이 없으면 검색어만 적용한다', () => {
    const apply = vi.fn();
    renderAt('/agents?name=없는이름', items, apply);

    expect(apply).toHaveBeenCalledWith('없는이름', null);
  });

  it('이름이 중복되면 첫 항목을 고른다', () => {
    const apply = vi.fn();
    const dup: Item[] = [
      { id: 'x1', name: 'dup' },
      { id: 'x2', name: 'dup' },
    ];
    renderAt('/agents?name=dup', dup, apply);

    expect(apply).toHaveBeenCalledWith('dup', 'x1');
  });

  it('쿼리가 없으면 아무것도 하지 않는다', () => {
    const apply = vi.fn();
    renderAt('/agents', items, apply);

    expect(apply).not.toHaveBeenCalled();
  });

  it('목록이 아직 비어 있으면 기다렸다가 도착 후 한 번 적용한다', () => {
    const apply = vi.fn();
    const { rerender } = render(
      <MemoryRouter initialEntries={['/agents?name=modbus-1']}>
        <Probe items={[]} apply={apply} />
      </MemoryRouter>,
    );
    expect(apply).not.toHaveBeenCalled();

    rerender(
      <MemoryRouter initialEntries={['/agents?name=modbus-1']}>
        <Probe items={items} apply={apply} />
      </MemoryRouter>,
    );
    expect(apply).toHaveBeenCalledTimes(1);
    expect(apply).toHaveBeenCalledWith('modbus-1', 'a1');
  });

  it('같은 이름에 대해 여러 번 적용하지 않는다', () => {
    // 매 렌더마다 적용하면 사용자가 검색어를 지워도 곧바로 되돌려진다.
    const apply = vi.fn();
    const { rerender } = renderAt('/agents?name=modbus-1', items, apply);
    rerender(
      <MemoryRouter initialEntries={['/agents?name=modbus-1']}>
        <Probe items={items} apply={apply} />
      </MemoryRouter>,
    );

    expect(apply).toHaveBeenCalledTimes(1);
  });
});
