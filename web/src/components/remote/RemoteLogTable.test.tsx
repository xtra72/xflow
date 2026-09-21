// 원격 관리 로그 표의 계약 (@SPEC:SPEC-REMOTE-LOG-001).
//
// 로케일 둘을 모두 본다 — 기본 로케일이 ko 라 한쪽만 보면 en 쪽 누락이 초록으로
// 통과하는 함정이 이 저장소에 있다.

import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import RemoteLogTable, { nodeLabel } from './RemoteLogTable';
import { logActionKey } from './remoteLogAction';
import type { RemoteLogEntry } from '@/types/remote';

function entry(over: Partial<RemoteLogEntry> = {}): RemoteLogEntry {
  return {
    id: 1,
    instance_id: 'node-a',
    actor: 'admin',
    action: 'connect',
    result: 'ok',
    timestamp: Date.parse('2026-09-20T09:00:00Z'),
    ...over,
  };
}

function renderTable(
  entries: RemoteLogEntry[],
  locale: 'ko' | 'en' = 'ko',
  showNode = true,
  extra: Partial<React.ComponentProps<typeof RemoteLogTable>> = {},
) {
  localStorage.setItem('xflow-locale', locale);
  return render(
    <I18nProvider>
      <RemoteLogTable entries={entries} showNode={showNode} {...extra} />
    </I18nProvider>,
  );
}

describe('logActionKey — 사건을 읽는 말로 옮긴다', () => {
  it('이미지 업데이트는 command + system/update 조합에서 나온다', () => {
    expect(
      logActionKey(entry({ action: 'command', domain: 'system', command_action: 'update' })),
    ).toBe('remote.log.action.imageUpdate');
  });

  it('다른 명령은 일반 명령으로 읽는다', () => {
    expect(logActionKey(entry({ action: 'command', domain: 'flow', command_action: 'deploy' }))).toBe(
      'remote.log.action.command',
    );
  });

  it('생애 사건은 그 액션 이름을 그대로 쓴다', () => {
    expect(logActionKey(entry({ action: 'disconnect' }))).toBe('remote.log.action.disconnect');
    expect(logActionKey(entry({ action: 'access' }))).toBe('remote.log.action.access');
  });
});

describe('RemoteLogTable', () => {
  it('기록이 없으면 빈 안내를 보여준다', () => {
    renderTable([]);
    expect(screen.getByTestId('remote-log-empty')).toBeInTheDocument();
  });

  it('노드 칸은 접을 수 있다 — 한 노드만 보는 자리용', () => {
    const { unmount } = renderTable([entry()], 'ko', true);
    expect(screen.getByText('node-a')).toBeInTheDocument();
    unmount();

    renderTable([entry()], 'ko', false);
    expect(screen.queryByText('node-a')).toBeNull();
  });

  it('명령 줄은 도메인/액션을 상세에 적는다', () => {
    renderTable([entry({ action: 'command', domain: 'flow', command_action: 'deploy' })]);
    expect(screen.getByText('flow / deploy')).toBeInTheDocument();
  });

  it.each(['ko', 'en'] as const)('%s 로케일에서 원문 키가 남지 않는다', (locale) => {
    renderTable(
      [
        entry({ id: 1, action: 'connect' }),
        entry({ id: 2, action: 'disconnect' }),
        entry({ id: 3, action: 'access' }),
        entry({ id: 4, action: 'register' }),
        entry({ id: 5, action: 'delete' }),
        entry({ id: 6, action: 'command', domain: 'system', command_action: 'update' }),
      ],
      locale,
    );
    const text = screen.getByTestId('remote-log-table').textContent ?? '';
    expect(text).not.toContain('remote.log.');
  });
});

describe('nodeLabel — 이름 우선, 모르면 id', () => {
  const names = new Map([['node-a', 'xagent03']]);

  it('이름을 알면 이름으로 적는다', () => {
    expect(nodeLabel('node-a', names)).toBe('xagent03');
  });

  it('삭제된 노드의 과거 기록은 이름을 모르므로 id 로 남는다', () => {
    expect(nodeLabel('gone-1', names)).toBe('gone-1');
  });

  it('ID 보기로 전환하면 이름을 알아도 id 를 적는다', () => {
    expect(nodeLabel('node-a', names, true)).toBe('node-a');
  });
});

describe('RemoteLogTable — 정렬 헤더', () => {
  it('sort/onSort 를 주면 헤더가 눌린다', () => {
    const onSort = vi.fn();
    renderTable([entry()], 'ko', true, {
      sort: { field: 'ts', direction: 'desc' },
      onSort,
    });
    fireEvent.click(screen.getByText('수행'));
    expect(onSort).toHaveBeenCalledWith('actor');
  });

  it('sort 가 없으면 평범한 헤더다 — 노드 탭처럼 정렬이 필요 없는 자리', () => {
    const onSort = vi.fn();
    renderTable([entry()], 'ko', true, { onSort });
    expect(screen.getByText('수행').tagName).toBe('TH');
  });

  it('이름 표시일 때도 id 는 title 로 확인할 수 있다', () => {
    renderTable([entry()], 'ko', true, { nodeNames: new Map([['node-a', 'xagent03']]) });
    const cell = screen.getByText('xagent03');
    expect(cell).toHaveAttribute('title', 'node-a');
  });
});
