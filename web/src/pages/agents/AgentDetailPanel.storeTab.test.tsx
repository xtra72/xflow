// AgentDetailPanel.StoreTab — 타입(metric_type)/태그 표시·필터·편집 테스트
// (SPEC-STORE-003 v0.4.0).
//
// 범위:
//   - 모든 엔트리(정적/동적)에 metric_type 배지가 노출된다 (동적/누락 → "unknown").
//   - 태그가 key=value 칩으로 표시된다.
//   - 메트릭 타입 필터(select)로 클라이언트 측 필터링이 동작한다.
//   - 행의 편집 버튼 → EditKeyMetaDialog 표시 → 저장 시 useSetStoreKeyMeta 호출 +
//     metric_type/tags 전체 교체 payload 전송.
//   - 원격(READ-ONLY) 타깃에서는 편집 버튼이 노출되지 않는다.
//
// 데이터/뮤테이션/시리즈 등 부수 의존은 스텁으로 격리한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { TargetProvider } from '@/lib/remote/TargetContext';
import type { ResourceTarget } from '@/lib/remote/target';
import type { AgentInfo } from '@/types/agent';

const LOCAL_TARGET: ResourceTarget = { type: 'local' };
const REMOTE_TARGET: ResourceTarget = { type: 'remote', instanceId: 'node-a' };

// State 엔트리는 모든 엔트리에 metric_type/tags 를 포함한다 (백엔드 v0.4.0).
const STORE_AGENT: AgentInfo = {
  id: 's-1',
  name: 'store-a',
  type: 'store',
  status: 'running',
  connected: true,
  uptime: '5m',
  config: { keys: [{ key: 'indoor:temp' }] },
  stats: undefined,
  state: {
    total_keys: 3,
    max_history_size: 0,
    entries: [
      {
        key: 'indoor:temp',
        value: 21.5,
        namespace: 'default',
        metric_type: 'temperature',
        tags: { room: '1' },
        updated_at: new Date().toISOString(),
      },
      {
        key: 'outdoor:humidity',
        value: 55,
        namespace: 'default',
        metric_type: 'humidity',
        tags: {},
        updated_at: new Date().toISOString(),
      },
      {
        key: 'dynamic:count',
        value: 7,
        namespace: 'default',
        metric_type: 'unknown',
        tags: {},
        updated_at: new Date().toISOString(),
      },
    ],
  },
} as unknown as AgentInfo;

// --- mutation 스파이 ---
const setKeyMetaMutateAsync = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useDetailTargets', () => ({
  useAgentDetailTarget: () => ({ data: STORE_AGENT, isLoading: false, error: null }),
  useAgentStatsTarget: () => ({ data: undefined, isLoading: false, error: null }),
}));

vi.mock('@/hooks/useAgent', () => ({
  useAgent: () => ({ data: STORE_AGENT, isLoading: false }),
  useConfigureAgent: () => ({ isPending: false, isError: false, mutateAsync: vi.fn() }),
  useExecAgent: () => ({ isPending: false, mutate: vi.fn() }),
}));

vi.mock('@/hooks/useRemote', () => ({
  useUpdateRemoteAgent: () => ({ isPending: false, mutateAsync: vi.fn() }),
}));

vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: () => ({
    isRemote: false,
    nodeReady: true,
    nodeLabel: undefined,
    canControl: () => true,
  }),
}));

vi.mock('@/hooks/useDevice', () => ({
  useDevicesRealtime: () => ({ data: { data: [] }, isLoading: false }),
}));

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock('@/services/api/monitorService', () => ({
  getLogLevels: () => Promise.resolve({ components: {} }),
  setComponentLogLevel: () => Promise.resolve(),
  resetComponentLogLevel: () => Promise.resolve(),
}));

vi.mock('@/services/api/seriesDataSource', () => ({
  useSeriesDataSource: () => ({ useKeys: () => ({ data: { keys: [] } }) }),
}));

// store 서비스: hook 들은 스텁, 순수 함수는 실제 구현 유지.
vi.mock('@/services/api/store', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/api/store')>();
  return {
    ...actual,
    useStoreTagPairs: () => ({ data: [], isLoading: false, isError: false }),
    useStoreKeysWithTags: () => ({ data: { keys: [], tags: {}, keyObjects: [] } }),
    useSetStoreKeyMeta: () => ({ isPending: false, mutateAsync: setKeyMetaMutateAsync }),
  };
});

const addNotification = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: { addNotification: () => void }) => unknown) =>
    selector({ addNotification }),
}));

import AgentDetailPanel from './AgentDetailPanel';

function renderPanel(target: ResourceTarget) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <TargetProvider target={target}>
        <AgentDetailPanel
          agentId={STORE_AGENT.id}
          agentType={STORE_AGENT.type}
          agentName={STORE_AGENT.name}
        />
      </TargetProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('StoreTab — 타입/태그 표시', () => {
  it('모든 엔트리에 metric_type 배지가 노출된다 (동적은 unknown)', () => {
    renderPanel(LOCAL_TARGET);
    // 배지는 테이블 행에 렌더된다. 필터 select 의 option 과 구분하기 위해
    // 테이블(table) 스코프로 한정해 조회한다.
    const table = screen.getByRole('table');
    expect(within(table).getByText('temperature')).toBeInTheDocument();
    expect(within(table).getByText('humidity')).toBeInTheDocument();
    // dynamic:count 는 metric_type=unknown.
    expect(within(table).getAllByText('unknown').length).toBeGreaterThanOrEqual(1);
  });

  it('태그가 key=value 칩으로 표시된다', () => {
    renderPanel(LOCAL_TARGET);
    expect(screen.getByText('room=1')).toBeInTheDocument();
  });
});

describe('StoreTab — 타입/태그 편집', () => {
  it('편집 버튼 → 다이얼로그 → 저장 시 useSetStoreKeyMeta 가 전체 교체 payload 로 호출된다', async () => {
    setKeyMetaMutateAsync.mockResolvedValueOnce({
      key: 'indoor:temp',
      metric_type: 'temperature',
      tags: { room: '1' },
    });
    renderPanel(LOCAL_TARGET);

    // indoor:temp 행의 편집 버튼 클릭.
    // i18n 모킹(t: k => k)에서는 {key} 치환이 일어나지 않아 모든 행의 편집 버튼이
    // 동일한 키 문자열 aria-label 을 가지므로, 첫 번째(indoor:temp) 버튼을 선택한다.
    const editBtn = screen.getAllByLabelText('agents.detail.store.editMetaAriaLabel')[0]!;
    fireEvent.click(editBtn);

    // 다이얼로그가 사전 채움된다.
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText('indoor:temp')).toBeInTheDocument();
    expect(within(dialog).getByLabelText(/property\.meta\.metricTypeOptional/)).toHaveValue(
      'temperature',
    );

    await act(async () => {
      fireEvent.click(screen.getByTestId('edit-key-meta-confirm'));
    });

    expect(setKeyMetaMutateAsync).toHaveBeenCalledTimes(1);
    expect(setKeyMetaMutateAsync).toHaveBeenCalledWith({
      agentName: 'store-a',
      key: 'indoor:temp',
      meta: { metric_type: 'temperature', tags: { room: '1' } },
    });
  });
});

describe('StoreTab — 원격 READ-ONLY', () => {
  it('원격 타깃에서는 편집 버튼이 노출되지 않는다', () => {
    renderPanel(REMOTE_TARGET);
    expect(
      screen.queryByLabelText('agents.detail.store.editMetaAriaLabel'),
    ).not.toBeInTheDocument();
  });
});

describe('StoreTab — 검색 필터', () => {
  it('key 부분일치로 행을 필터한다', () => {
    renderPanel(LOCAL_TARGET);
    fireEvent.change(screen.getByTestId('store-search-input'), {
      target: { value: 'indoor' },
    });
    expect(screen.getByTitle('indoor:temp')).toBeInTheDocument();
    expect(screen.queryByTitle('outdoor:humidity')).not.toBeInTheDocument();
    expect(screen.queryByTitle('dynamic:count')).not.toBeInTheDocument();
  });

  it('metric_type 으로도 검색된다 (key 에 없는 텍스트)', () => {
    renderPanel(LOCAL_TARGET);
    fireEvent.change(screen.getByTestId('store-search-input'), {
      target: { value: 'humidity' },
    });
    expect(screen.getByTitle('outdoor:humidity')).toBeInTheDocument();
    expect(screen.queryByTitle('indoor:temp')).not.toBeInTheDocument();
  });

  it('태그(room=1)로도 검색된다', () => {
    renderPanel(LOCAL_TARGET);
    fireEvent.change(screen.getByTestId('store-search-input'), {
      target: { value: 'room=1' },
    });
    expect(screen.getByTitle('indoor:temp')).toBeInTheDocument();
    expect(screen.queryByTitle('outdoor:humidity')).not.toBeInTheDocument();
  });

  it('매칭이 없으면 필터 안내 문구를 표시한다', () => {
    renderPanel(LOCAL_TARGET);
    fireEvent.change(screen.getByTestId('store-search-input'), {
      target: { value: 'zzz-nomatch' },
    });
    expect(
      screen.getByText('agents.detail.store.emptyNoFilterMatch'),
    ).toBeInTheDocument();
  });
});

describe('StoreTab — 컬럼 정렬', () => {
  // 테이블 본문 행의 key 컬럼(첫 번째 셀) 순서를 추출한다.
  function rowKeys(): string[] {
    const table = screen.getByRole('table');
    const bodyRows = within(table)
      .getAllByRole('row')
      // 헤더 행(th 포함) 제외.
      .filter((r) => within(r).queryAllByRole('cell').length > 0);
    return bodyRows.map((r) => {
      // 키 컬럼은 8자 초과 시 표시 텍스트를 축약하므로, 전체 키가 담긴
      // title 속성으로 행 키를 읽는다(축약/확장 무관하게 안정적).
      const firstCell = within(r).getAllByRole('cell')[0];
      const keyEl = firstCell?.querySelector('[title]');
      return keyEl?.getAttribute('title') ?? firstCell?.textContent?.trim() ?? '';
    });
  }

  it('키 헤더 클릭 시 오름차순으로 정렬된다', () => {
    renderPanel(LOCAL_TARGET);
    // i18n 모킹(t: k => k)에서는 {label} 치환이 일어나지 않아 모든 정렬 헤더가
    // 동일한 키 문자열 aria-label 을 가지므로, 첫 번째(키 컬럼) 헤더를 선택한다.
    fireEvent.click(screen.getAllByLabelText('agents.detail.store.sortAriaLabel')[0]!);
    expect(rowKeys()).toEqual([
      'dynamic:count',
      'indoor:temp',
      'outdoor:humidity',
    ]);
  });

  it('키 헤더 재클릭 시 내림차순으로 토글된다', () => {
    renderPanel(LOCAL_TARGET);
    const header = screen.getAllByLabelText('agents.detail.store.sortAriaLabel')[0]!;
    fireEvent.click(header); // asc
    fireEvent.click(header); // desc
    expect(rowKeys()).toEqual([
      'outdoor:humidity',
      'indoor:temp',
      'dynamic:count',
    ]);
  });
});
