// SPEC-DASHBOARD-002 (REQ-02/REQ-03): AgentStatusPanel 상태 매트릭스 + 렌더 검증.
//
// 데이터 훅(useAgentStatsTarget/useAgentDetailTarget)을 mock 하여 상태별(미설정/로딩/
// 에러/정상/원격 graceful) 렌더와 공통 통계 타일, EnhancedMessagesStats 요약을 검증한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, within } from '@testing-library/react';

import type { AgentInfo, AgentStatsInfo } from '@/types/agent';
import type { DetailQueryResult } from '@/hooks/useDetailTargets';

const hooks = vi.hoisted(() => ({
  stats: { data: undefined, isLoading: false, error: null } as DetailQueryResult<AgentStatsInfo>,
  detail: { data: undefined, isLoading: false, error: null } as DetailQueryResult<AgentInfo>,
}));

vi.mock('@/hooks/useDetailTargets', () => ({
  useAgentStatsTarget: () => hooks.stats,
  useAgentDetailTarget: () => hooks.detail,
}));

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import AgentStatusPanel from './AgentStatusPanel';

function stats(overrides: Partial<AgentStatsInfo> = {}): AgentStatsInfo {
  return {
    id: 'a-1',
    status: 'running',
    uptime: '1h 2m',
    messages_in: 1234,
    messages_out: 5678,
    error_count: 9,
    connected: true,
    dropped_messages: 3,
    ...overrides,
  };
}

function detail(overrides: Partial<AgentInfo> = {}): AgentInfo {
  return {
    id: 'a-1',
    name: '테스트 에이전트',
    type: 'mqtt-client',
    status: 'running',
    enabled: true,
    ...overrides,
  };
}

function renderPanel(config: Record<string, unknown>) {
  return render(<AgentStatusPanel panelId="p1" title="에이전트 상태" config={config} />);
}

describe('AgentStatusPanel (SPEC-DASHBOARD-002)', () => {
  beforeEach(() => {
    hooks.stats = { data: undefined, isLoading: false, error: null };
    hooks.detail = { data: undefined, isLoading: false, error: null };
  });

  it('AC-03-1: agentId 미설정이면 안내 문구(notConfigured)를 표시한다', () => {
    renderPanel({ agentId: '' });
    expect(screen.getByText('dashboard.agentStatus.notConfigured')).toBeInTheDocument();
  });

  it('AC-03-2: 로딩 중이면 타이틀 + 스피너를 표시한다(blank 아님)', () => {
    hooks.stats = { data: undefined, isLoading: true, error: null };
    const { container } = renderPanel({ agentId: 'a-1' });
    expect(screen.getByText('에이전트 상태')).toBeInTheDocument();
    expect(container.querySelector('.animate-spin')).toBeInTheDocument();
  });

  it('AC-03-3: 에러/무응답이면 cannotLoad 안내를 표시한다', () => {
    hooks.stats = { data: undefined, isLoading: false, error: new Error('boom') };
    renderPanel({ agentId: 'a-1' });
    expect(screen.getByText('dashboard.agentStatus.cannotLoad')).toBeInTheDocument();
  });

  it('AC-02-1/AC-03-4: 정상 시 헤더(name/type) + 공통 통계 타일을 표시한다', () => {
    hooks.stats = { data: stats(), isLoading: false, error: null };
    hooks.detail = { data: detail(), isLoading: false, error: null };
    renderPanel({ agentId: 'a-1' });

    // 헤더 name/type
    expect(screen.getByText('테스트 에이전트')).toBeInTheDocument();
    expect(screen.getByText('mqtt-client')).toBeInTheDocument();
    // 공통 통계(toLocaleString 포맷)
    expect(screen.getByText((1234).toLocaleString())).toBeInTheDocument();
    expect(screen.getByText((5678).toLocaleString())).toBeInTheDocument();
    expect(screen.getByText('1h 2m')).toBeInTheDocument();
    // enabled 배지
    expect(screen.getByText('dashboard.agentStatus.enabled')).toBeInTheDocument();
  });

  it('AC-02-2: messages(EnhancedMessagesStats) 존재 시 external/internal 요약을 표시한다', () => {
    hooks.stats = {
      data: stats({
        messages: {
          total: { received: 0, sent: 0, errored: 0 },
          external: { received: 11, sent: 22, errored: 0 },
          internal: { received: 33, sent: 44, errored: 0 },
        },
      }),
      isLoading: false,
      error: null,
    };
    hooks.detail = { data: detail(), isLoading: false, error: null };
    renderPanel({ agentId: 'a-1' });

    expect(screen.getByText('agents.detail.stats.messageDetail')).toBeInTheDocument();
    // 값 여섯이 각자 타일이다 — 묶음(외부/내부)은 타일 이름이 말한다.
    expect(screen.getByTestId('agent-message-externalSent-value').textContent).toBe(
      (22).toLocaleString(),
    );
    expect(screen.getByTestId('agent-message-internalReceived-value').textContent).toBe(
      (33).toLocaleString(),
    );
  });

  it('AC-02-2: messages 미존재 시 요약 그리드를 생략한다(오류 없음)', () => {
    hooks.stats = { data: stats({ messages: undefined }), isLoading: false, error: null };
    hooks.detail = { data: detail(), isLoading: false, error: null };
    renderPanel({ agentId: 'a-1' });
    expect(screen.queryByText('agents.detail.stats.messageDetail')).toBeNull();
  });

  it('AC-03-5: 원격 제약(detail 없음) 시 name→agentId, type→"-" 로 graceful 표기', () => {
    hooks.stats = { data: stats(), isLoading: false, error: null };
    hooks.detail = { data: undefined, isLoading: false, error: null };
    renderPanel({ agentId: 'a-1' });

    // 통계 타일은 유지된다.
    expect(screen.getByText((1234).toLocaleString())).toBeInTheDocument();
    // name 대체(agentId). 종류는 배지로 옮겼고, 값이 없으면 그 배지를 내지 않는다
    // — 빈 알약("-")은 무엇을 보는 자리인지 읽히지 않는다.
    expect(screen.getByText('a-1')).toBeInTheDocument();
    expect(screen.queryByTestId('agent-status-badge-type')).not.toBeInTheDocument();
  });

  it('AC-06-3: viewMode 미설정 시 기본 tile 뷰(기존 통계 타일)로 렌더한다(하위호환)', () => {
    hooks.stats = { data: stats(), isLoading: false, error: null };
    hooks.detail = { data: detail(), isLoading: false, error: null };
    renderPanel({ agentId: 'a-1' });

    expect(screen.getByText('agents.detail.stats.totalIn')).toBeInTheDocument();
    expect(screen.queryByTestId('agent-status-diagram')).toBeNull();
  });

  it('AC-05-1/AC-06-2: viewMode=diagram 이면 SVG 다이어그램을 렌더하고 통계 타일을 생략한다', () => {
    hooks.stats = { data: stats(), isLoading: false, error: null };
    hooks.detail = { data: detail(), isLoading: false, error: null };
    renderPanel({ agentId: 'a-1', viewMode: 'diagram' });

    expect(screen.getByTestId('agent-status-diagram')).toBeInTheDocument();
    // 타일 뷰 전용 라벨은 diagram 뷰에서 생략된다.
    expect(screen.queryByText('agents.detail.stats.totalIn')).toBeNull();
  });

  it('AC-07-1: summary_stats 존재 시 tile·diagram 두 뷰 모두에서 요약 카운트를 표시한다', () => {
    const withSummary = stats({
      summary_stats: [
        { key: 'devicesTotal', value: 12 },
        { key: 'devicesOnline', value: 8 },
      ],
    });

    // tile 뷰
    hooks.stats = { data: withSummary, isLoading: false, error: null };
    hooks.detail = { data: detail(), isLoading: false, error: null };
    const { unmount } = renderPanel({ agentId: 'a-1', viewMode: 'tile' });
    const tileSummary = screen.getByTestId('agent-status-summary');
    expect(within(tileSummary).getByText('devicesTotal')).toBeInTheDocument();
    expect(within(tileSummary).getByText((12).toLocaleString())).toBeInTheDocument();
    unmount();

    // diagram 뷰
    renderPanel({ agentId: 'a-1', viewMode: 'diagram' });
    const diagSummary = screen.getByTestId('agent-status-summary');
    expect(within(diagSummary).getByText('devicesOnline')).toBeInTheDocument();
    expect(within(diagSummary).getByText((8).toLocaleString())).toBeInTheDocument();
  });

  it('AC-07-2: summary_stats 부재 시 요약 영역을 생략한다(graceful)', () => {
    hooks.stats = { data: stats(), isLoading: false, error: null };
    hooks.detail = { data: detail(), isLoading: false, error: null };
    renderPanel({ agentId: 'a-1' });
    expect(screen.queryByTestId('agent-status-summary')).toBeNull();
  });
});
