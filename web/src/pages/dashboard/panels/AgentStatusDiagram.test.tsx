// SPEC-DASHBOARD-003 (REQ-05, AC-05-1~3): 메시지 흐름 다이어그램 SVG 렌더/폴백 검증.
//
// i18n 은 key 를 그대로 반환하도록 mock 한다(라벨 문자열이 아닌 카운트/구조를 검증).

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { AgentStatsInfo } from '@/types/agent';

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import AgentStatusDiagram from './AgentStatusDiagram';

function base(overrides: Partial<AgentStatsInfo> = {}): AgentStatsInfo {
  return {
    id: 'a-1',
    status: 'running',
    messages_in: 100,
    messages_out: 50,
    error_count: 2,
    connected: true,
    dropped_messages: 7,
    uptime: '3h 4m',
    buffer: { pending: 5, capacity: 20 },
    ...overrides,
  };
}

describe('AgentStatusDiagram (SPEC-DASHBOARD-003)', () => {
  it('AC-05-1: 중첩 messages 존재 시 SVG 다이어그램 + External/Internal 화살표 카운트를 렌더한다', () => {
    render(
      <AgentStatusDiagram
        data={base({
          messages: {
            total: { received: 0, sent: 0, errored: 0 },
            external: { received: 111, sent: 222, errored: 3 },
            internal: { received: 44, sent: 55, errored: 1 },
          },
        })}
      />,
    );

    expect(screen.getByTestId('agent-status-diagram')).toBeInTheDocument();
    // External 화살표: 중첩 external 값 사용
    expect(screen.getByTestId('agent-status-arrow-external-in')).toHaveTextContent('111');
    expect(screen.getByTestId('agent-status-arrow-external-out')).toHaveTextContent('222');
    expect(screen.getByTestId('agent-status-arrow-external-err')).toHaveTextContent('3');
    // Internal 화살표: 중첩 존재 시에만 표기
    expect(screen.getByTestId('agent-status-arrow-internal-in')).toHaveTextContent('44');
    expect(screen.getByTestId('agent-status-arrow-internal-out')).toHaveTextContent('55');
    expect(screen.getByTestId('agent-status-arrow-internal-err')).toHaveTextContent('1');
  });

  it('AC-05-2: 보조 지표(dropped/buffer/uptime)를 함께 표기한다', () => {
    render(
      <AgentStatusDiagram
        data={base({
          messages: {
            total: { received: 0, sent: 0, errored: 0 },
            external: { received: 1, sent: 1, errored: 0 },
            internal: { received: 0, sent: 0, errored: 0 },
          },
        })}
      />,
    );
    expect(screen.getByTestId('agent-status-diagram-dropped')).toHaveTextContent('7');
    expect(screen.getByTestId('agent-status-diagram-buffer')).toHaveTextContent('5');
    expect(screen.getByTestId('agent-status-diagram-buffer')).toHaveTextContent('20');
    expect(screen.getByTestId('agent-status-diagram-uptime')).toHaveTextContent('3h 4m');
  });

  it('AC-05-3: messages 부재 시 flat 필드로 폴백하고 Internal 화살표를 생략한다', () => {
    render(<AgentStatusDiagram data={base({ messages: undefined })} />);

    // flat 폴백 값(messages_in/out, error_count)
    expect(screen.getByTestId('agent-status-arrow-external-in')).toHaveTextContent('100');
    expect(screen.getByTestId('agent-status-arrow-external-out')).toHaveTextContent('50');
    expect(screen.getByTestId('agent-status-arrow-external-err')).toHaveTextContent('2');
    // Internal 화살표 부재(단일 External↔Agent 흐름만)
    expect(screen.queryByTestId('agent-status-arrow-internal-in')).toBeNull();
    expect(screen.queryByTestId('agent-status-arrow-internal-out')).toBeNull();
  });
});
