// 에이전트 상태 패널의 통계 타일 — 표시 선택과 항목별 디자인.

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { AgentInfo, AgentStatsInfo } from '@/types/agent';
import type { DetailQueryResult } from '@/hooks/useDetailTargets';

const mocks = vi.hoisted(() => ({
  detail: undefined as unknown,
  stats: undefined as unknown,
}));

vi.mock('@/hooks/useDetailTargets', () => ({
  useAgentDetailTarget: () => mocks.detail,
  useAgentStatsTarget: () => mocks.stats,
}));

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import AgentStatusPanel from './AgentStatusPanel';

const INFO = { id: 'a-1', name: '게이트웨이', type: 'modbus-gateway' } as AgentInfo;
const STATS = {
  messages_in: 100,
  messages_out: 50,
  error_count: 2,
  uptime: '1h',
  dropped_messages: 7,
  messages: {
    total: { received: 11, sent: 12, errored: 13 },
    external: { received: 21, sent: 22, errored: 23 },
    internal: { received: 31, sent: 32, errored: 33 },
  },
} as AgentStatsInfo;

function renderPanel(config: Record<string, unknown>) {
  mocks.detail = { data: INFO, isLoading: false } as DetailQueryResult<AgentInfo>;
  mocks.stats = { data: STATS, isLoading: false } as DetailQueryResult<AgentStatsInfo>;
  render(<AgentStatusPanel panelId="p1" title="상태" config={{ agentId: 'a-1', ...config }} />);
}

describe('통계 타일 표시 선택', () => {
  it('미설정이면 다섯 다 기본 순서로 낸다 — 드롭 메시지도 같은 무리다', () => {
    renderPanel({});
    for (const tile of ['messagesIn', 'messagesOut', 'errors', 'uptime', 'dropped']) {
      expect(screen.getByTestId(`agent-stat-${tile}`)).toBeInTheDocument();
    }
  });

  it('고른 것만 낸다', () => {
    renderPanel({ statTiles: ['errors'] });
    expect(screen.getByTestId('agent-stat-errors')).toBeInTheDocument();
    expect(screen.queryByTestId('agent-stat-uptime')).not.toBeInTheDocument();
  });

  it('고른 차례대로 그린다', () => {
    renderPanel({ statTiles: ['uptime', 'errors'] });
    const order = Array.from(
      screen.getByTestId('agent-stat-uptime').parentElement!.children,
    ).map((el) => el.getAttribute('data-testid'));
    expect(order).toEqual(['agent-stat-uptime', 'agent-stat-errors']);
  });

  it('전부 끄면 타일 줄이 없다', () => {
    renderPanel({ statTiles: [] });
    expect(screen.queryByTestId('agent-stat-errors')).not.toBeInTheDocument();
  });

  it('타일이 읽는 값이 통계와 이어져 있다', () => {
    renderPanel({});
    expect(screen.getByTestId('agent-stat-messagesIn').textContent).toContain('100');
    expect(screen.getByTestId('agent-stat-dropped').textContent).toContain('7');
  });

  it('다이어그램 뷰에는 타일이 없다', () => {
    renderPanel({ viewMode: 'diagram' });
    expect(screen.queryByTestId('agent-stat-errors')).not.toBeInTheDocument();
  });
});

describe('항목별 타일 디자인', () => {
  it('고른 타일에만 걸린다', () => {
    renderPanel({ statTileStyles: { errors: { value_font: { size: 28 } } } });
    expect(screen.getByTestId('agent-stat-errors-value')).toHaveStyle({ fontSize: '28px' });
    expect(screen.getByTestId('agent-stat-uptime-value')).not.toHaveStyle({ fontSize: '28px' });
  });

  it('배경색을 정하지 않으면 기본 배경 클래스가 그대로 산다', () => {
    renderPanel({});
    expect(screen.getByTestId('agent-stat-errors').className).toContain('bg-(--color-bg-primary)');
  });

  it('배경색을 정하면 기본 배경 클래스를 걷어낸다', () => {
    renderPanel({ statTileStyles: { errors: { bg: '#331111' } } });
    const tile = screen.getByTestId('agent-stat-errors');
    expect(tile.className).not.toContain('bg-(--color-bg-primary)');
    expect(tile).toHaveStyle({ backgroundColor: '#331111' });
  });
});

describe('메시지 상세 타일', () => {
  it('미설정이면 여섯 다 낸다 — 값마다 제 타일', () => {
    renderPanel({});
    for (const tile of [
      'externalReceived',
      'externalSent',
      'externalErrored',
      'internalReceived',
      'internalSent',
      'internalErrored',
    ]) {
      expect(screen.getByTestId(`agent-message-${tile}`)).toBeInTheDocument();
    }
  });

  it('카드 한 장이 값 셋을 담는다', () => {
    renderPanel({});
    const external = screen.getByTestId('agent-message-group-external');
    for (const tile of ['externalReceived', 'externalSent', 'externalErrored']) {
      expect(external).toContainElement(screen.getByTestId(`agent-message-${tile}`));
    }
  });

  it('한 묶음의 값을 모두 끄면 그 카드가 사라진다', () => {
    renderPanel({ messageTiles: ['externalReceived', 'externalSent'] });
    expect(screen.getByTestId('agent-message-group-external')).toBeInTheDocument();
    expect(screen.queryByTestId('agent-message-group-internal')).not.toBeInTheDocument();
  });

  it('낱개로 고를 수 있다', () => {
    renderPanel({ messageTiles: ['externalErrored', 'internalErrored'] });
    expect(screen.getByTestId('agent-message-externalErrored')).toBeInTheDocument();
    expect(screen.queryByTestId('agent-message-externalReceived')).not.toBeInTheDocument();
    expect(screen.getByTestId('agent-message-internalErrored')).toBeInTheDocument();
  });

  it('전부 끄면 메시지 상세 영역이 없다', () => {
    renderPanel({ messageTiles: [] });
    expect(screen.queryByTestId('agent-message-grid')).not.toBeInTheDocument();
  });

  it('타일이 읽는 값이 통계와 이어져 있다', () => {
    renderPanel({});
    expect(screen.getByTestId('agent-message-externalReceived').textContent).toContain('21');
    expect(screen.getByTestId('agent-message-internalErrored').textContent).toContain('33');
  });

  it('디자인은 카드 단위로 걸린다 — 타이틀은 묶음 이름, 값 글자는 그 카드 안 값들에', () => {
    renderPanel({
      messageTileStyles: { external: { label_font: { size: 9 }, value_font: { size: 22 }, bg: '#331111' } },
    });
    expect(screen.getByTestId('agent-message-group-external')).toHaveStyle({
      backgroundColor: '#331111',
    });
    expect(screen.getByTestId('agent-message-group-external-label')).toHaveStyle({ fontSize: '9px' });
    expect(screen.getByTestId('agent-message-externalReceived-value')).toHaveStyle({ fontSize: '22px' });
    expect(screen.getByTestId('agent-message-internalReceived-value')).not.toHaveStyle({
      fontSize: '22px',
    });
  });

  it('값 색 규칙은 값마다 따로 판정한다', () => {
    // 외부: 받음 21 · 보냄 22 · 오류 23. 22 이상만 빨갛게.
    renderPanel({
      messageTileStyles: { external: { valueColors: [{ op: 'gte', value: '22', color: '#ff0000' }] } },
    });
    expect(screen.getByTestId('agent-message-externalReceived-value')).not.toHaveStyle({
      color: '#ff0000',
    });
    expect(screen.getByTestId('agent-message-externalErrored-value')).toHaveStyle({ color: '#ff0000' });
  });

  it('메시지 통계가 없으면 영역 자체가 없다 — 없는 값을 지어내지 않는다', () => {
    mocks.detail = { data: INFO, isLoading: false };
    mocks.stats = { data: { ...STATS, messages: undefined }, isLoading: false };
    render(<AgentStatusPanel panelId="p1" title="상태" config={{ agentId: 'a-1' }} />);
    expect(screen.queryByTestId('agent-message-grid')).not.toBeInTheDocument();
  });
})

describe('타일 격자 배치', () => {
  it('통계는 8x4 격자에 2x2 타일이 기본 — 한 줄에 넷', () => {
    renderPanel({});
    const grid = screen.getByTestId('agent-stat-grid');
    expect(grid).toHaveStyle({ gridTemplateColumns: 'repeat(8, minmax(0, 1fr))' });
    expect(screen.getByTestId('agent-stat-messagesIn')).toHaveStyle({
      gridColumn: '1 / span 2',
      gridRow: '1 / span 2',
    });
    expect(screen.getByTestId('agent-stat-uptime')).toHaveStyle({ gridColumn: '7 / span 2' });
    // 다섯째는 다음 줄로.
    expect(screen.getByTestId('agent-stat-dropped')).toHaveStyle({ gridRow: '3 / span 2' });
  });

  it('메시지 상세는 8x2 격자에 4x2 카드가 기본 — 둘이 딱 맞는다', () => {
    renderPanel({});
    const grid = screen.getByTestId('agent-message-grid');
    expect(grid).toHaveStyle({ gridTemplateColumns: 'repeat(8, minmax(0, 1fr))' });
    expect(screen.getByTestId('agent-message-group-external')).toHaveStyle({
      gridColumn: '1 / span 4',
      gridRow: '1 / span 2',
    });
    expect(screen.getByTestId('agent-message-group-internal')).toHaveStyle({
      gridColumn: '5 / span 4',
    });
  });

  it('저장된 자리를 그대로 쓴다', () => {
    renderPanel({ statTileAreas: { errors: { x: 5, y: 3, w: 4, h: 2 } } });
    expect(screen.getByTestId('agent-stat-errors')).toHaveStyle({
      gridColumn: '5 / span 4',
      gridRow: '3 / span 2',
    });
  });

  it('격자 크기를 바꾸면 따라간다', () => {
    renderPanel({ statGrid: { rows: 2, cols: 4 } });
    expect(screen.getByTestId('agent-stat-grid')).toHaveStyle({
      gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
    });
  });

  it('격자를 줄여도 타일이 사라지지 않는다', () => {
    renderPanel({
      statGrid: { rows: 2, cols: 4 },
      statTileAreas: { errors: { x: 7, y: 3, w: 2, h: 2 } },
    });
    expect(screen.getByTestId('agent-stat-errors')).toBeInTheDocument();
  });
})

describe('타일 글자와 값 색', () => {
  it('타이틀과 값에 각자의 글자 설정이 걸린다', () => {
    renderPanel({ statTileStyles: { errors: { label_font: { size: 10 }, value_font: { size: 30 } } } });
    expect(screen.getByTestId('agent-stat-errors-label')).toHaveStyle({ fontSize: '10px' });
    expect(screen.getByTestId('agent-stat-errors-value')).toHaveStyle({ fontSize: '30px' });
  });

  it('값 색 규칙은 원값으로 판정한다 — 자릿점 찍은 문자열로는 숫자 비교가 안 된다', () => {
    // messages_in 은 100. 자릿점을 찍으면 "100" 그대로지만 1000 이면 "1,000" 이 된다.
    renderPanel({
      statTileStyles: {
        messagesIn: { valueColors: [{ op: 'gte', value: '50', color: '#ff0000' }] },
      },
    });
    expect(screen.getByTestId('agent-stat-messagesIn-value')).toHaveStyle({ color: '#ff0000' });
  });

  it('규칙에 맞지 않으면 색을 바꾸지 않는다', () => {
    renderPanel({
      statTileStyles: {
        errors: { valueColors: [{ op: 'gte', value: '100', color: '#ff0000' }] },
      },
    });
    expect(screen.getByTestId('agent-stat-errors-value')).not.toHaveStyle({ color: '#ff0000' });
  });

  it('옛 형태로 저장된 글꼴은 값에 걸린다 — 설정이 사라지면 안 된다', () => {
    renderPanel({ statTileStyles: { errors: { size: 28 } } });
    expect(screen.getByTestId('agent-stat-errors-value')).toHaveStyle({ fontSize: '28px' });
  });
})

describe('공통 속성과 배지', () => {
  it('공통 글자 설정이 모든 타일에 걸린다', () => {
    renderPanel({ tileValueFont: { size: 26 } });
    expect(screen.getByTestId('agent-stat-errors-value')).toHaveStyle({ fontSize: '26px' });
    expect(screen.getByTestId('agent-stat-uptime-value')).toHaveStyle({ fontSize: '26px' });
  });

  it('타일별 설정이 공통 설정을 덮되, 덮지 않은 항목은 남는다', () => {
    renderPanel({
      tileValueFont: { size: 26, color: '#00ff00' },
      statTileStyles: { errors: { value_font: { size: 40 } } },
    });
    // 크기는 타일별이 이기고, 색은 공통이 그대로 산다.
    expect(screen.getByTestId('agent-stat-errors-value')).toHaveStyle({
      fontSize: '40px',
      color: '#00ff00',
    });
  });

  it('공통 배경색이 모든 타일에 걸린다', () => {
    renderPanel({ tileBg: '#112233' });
    expect(screen.getByTestId('agent-stat-errors')).toHaveStyle({ backgroundColor: '#112233' });
  });

  it('배지를 끄면 상태 배지 줄이 없다', () => {
    renderPanel({ showBadges: false });
    expect(screen.queryByTestId('agent-status-badges')).not.toBeInTheDocument();
  });

  it('기본은 배지 표시', () => {
    renderPanel({});
    expect(screen.getByTestId('agent-status-badges')).toBeInTheDocument();
  });
})

describe('배지 항목 고르기', () => {
  it('기본은 종류 · 상태 · 사용 셋', () => {
    mocks.detail = { data: { ...INFO, status: 'running', enabled: true }, isLoading: false };
    mocks.stats = { data: STATS, isLoading: false };
    render(<AgentStatusPanel panelId="p1" title="상태" config={{ agentId: 'a-1' }} />);
    expect(screen.getByTestId('agent-status-badge-type')).toBeInTheDocument();
  });

  it('고른 것만 낸다', () => {
    renderPanel({ badgeItems: ['type'] });
    expect(screen.getByTestId('agent-status-badge-type')).toBeInTheDocument();
    expect(screen.queryByTestId('agent-status-badge-enabled')).not.toBeInTheDocument();
  });

  it('전부 끄면 배지 줄이 없다', () => {
    renderPanel({ badgeItems: [] });
    expect(screen.queryByTestId('agent-status-badges')).not.toBeInTheDocument();
  });

  it('항목마다 모양을 따로 정한다', () => {
    renderPanel({ badgeStyles: { type: { value_font: { size: 16 }, bg: '#112233' } } });
    expect(screen.getByTestId('agent-status-badge-type')).toHaveStyle({
      fontSize: '16px',
      backgroundColor: '#112233',
    });
  });

  it('타이틀 옆 종류 글자는 없앴다 — 배지가 대신한다', () => {
    renderPanel({ badgeItems: [] });
    expect(screen.queryByText('modbus-gateway')).not.toBeInTheDocument();
  });
})
