// AgentDetailPanel.StoreTab — 특성화(characterization) 테스트.
//
// @spec SPEC-PANEL-SETTINGS-001 (T10 / PRESERVE)
//
// 목적: 공용 `StoreEntryTable` 추출(T5) 전에 에이전트 상세 Store 리스트의 현재
// 관측 가능한 동작을 캡처하는 회귀 안전망(R1 완화). 추출 이후에도 이 테스트가
// 그대로 통과해야 하며(AC-16 회귀 0), 여기서 검증하는 동작은 다음과 같다:
//   - 헤더/본문이 동일한 컬럼 목록을 공유하여 `actions` 를 포함한 컬럼이 렌더된다.
//   - `actions` 컬럼의 액션 버튼 세트가 정적/동적 키에 따라 달라진다.
//   - `storeEntrySort` 기반 헤더 정렬(asc/desc 토글)이 동작한다.
//   - 컬럼 표시/숨김 토글이 헤더에 반영되고 per-agent localStorage 로 영속된다.
//   - 저장된 컬럼 가시성이 재마운트 시 복원된다.
//
// 부수 의존(데이터/뮤테이션/시리즈)은 스텁으로 격리한다(storeTab.test.tsx 와 동일 패턴).

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { TargetProvider } from '@/lib/remote/TargetProvider';
import type { ResourceTarget } from '@/lib/remote/target';
import type { AgentInfo } from '@/types/agent';

const LOCAL_TARGET: ResourceTarget = { type: 'local' };

// config.keys 에 indoor:temp 만 등록 → indoor:temp = 정적, 나머지 2개 = 동적.
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
        field: 'temperature',
        tags: { room: '1' },
        updated_at: new Date().toISOString(),
      },
      {
        key: 'outdoor:humidity',
        value: 55,
        namespace: 'default',
        field: 'humidity',
        tags: {},
        updated_at: new Date().toISOString(),
      },
      {
        key: 'dynamic:count',
        value: 7,
        namespace: 'default',
        field: 'unknown',
        tags: {},
        updated_at: new Date().toISOString(),
      },
    ],
  },
} as unknown as AgentInfo;

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

vi.mock('@/services/api/store', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/api/store')>();
  return {
    ...actual,
    useStoreTagPairs: () => ({ data: [], isLoading: false, isError: false }),
    useStoreKeysWithTags: () => ({ data: { keys: [], tags: {}, keyObjects: [] } }),
    useSetStoreKeyMeta: () => ({ isPending: false, mutateAsync: vi.fn() }),
  };
});

const addNotification = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: { addNotification: () => void }) => unknown) =>
    selector({ addNotification }),
}));

import AgentDetailPanel from './AgentDetailPanel';

const COLUMNS_STORAGE_KEY = 'xflow.store.columns.s-1';

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <TargetProvider target={LOCAL_TARGET}>
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
  window.localStorage.clear();
});

// 테이블 본문 행의 key(첫 셀 title) 순서를 추출한다(축약/확장 무관하게 안정적).
function rowKeys(): string[] {
  const table = screen.getByRole('table');
  const bodyRows = within(table)
    .getAllByRole('row')
    .filter((r) => within(r).queryAllByRole('cell').length > 0);
  return bodyRows.map((r) => {
    const firstCell = within(r).getAllByRole('cell')[0];
    const keyEl = firstCell?.querySelector('[title]');
    return keyEl?.getAttribute('title') ?? firstCell?.textContent?.trim() ?? '';
  });
}

describe('StoreTab 특성화 — 헤더/본문 단일 목록 + actions 컬럼', () => {
  it('기본 컬럼 헤더가 actions 를 포함해 렌더된다', () => {
    renderPanel();
    const table = screen.getByRole('table');
    // 헤더는 STORE_COLUMNS 단일 출처를 매핑한다.
    expect(within(table).getByText('agents.detail.store.colKey')).toBeInTheDocument();
    expect(within(table).getByText('agents.detail.store.colField')).toBeInTheDocument();
    expect(within(table).getByText('agents.detail.store.colValue')).toBeInTheDocument();
    expect(within(table).getByText('agents.detail.store.colNamespace')).toBeInTheDocument();
    expect(within(table).getByText('agents.detail.store.colBinding')).toBeInTheDocument();
    expect(within(table).getByText('agents.detail.store.colUpdated')).toBeInTheDocument();
    // actions 컬럼(숨김 불가)이 헤더에 항상 존재한다.
    expect(within(table).getByText('agents.detail.store.colActions')).toBeInTheDocument();
  });

  it('본문 3개 행이 렌더된다(정적 1 + 동적 2)', () => {
    renderPanel();
    expect(rowKeys().sort()).toEqual([
      'dynamic:count',
      'indoor:temp',
      'outdoor:humidity',
    ]);
  });

  it('actions 액션 버튼 세트가 정적/동적에 따라 달라진다', () => {
    renderPanel();
    // 편집(메타)/초기화는 모든 non-readOnly 행에 노출 → 3개.
    expect(
      screen.getAllByLabelText('agents.detail.store.editMetaAriaLabel'),
    ).toHaveLength(3);
    expect(
      screen.getAllByLabelText('agents.detail.store.resetAriaLabel'),
    ).toHaveLength(3);
    // 정적으로 변환/이름변경은 동적 키(2개)에만 노출 → 2개.
    expect(
      screen.getAllByLabelText('agents.detail.store.promoteAriaLabel'),
    ).toHaveLength(2);
    expect(
      screen.getAllByLabelText('agents.detail.store.renameAriaLabel'),
    ).toHaveLength(2);
  });

  it('binding 컬럼이 정적/동적 배지를 렌더한다', () => {
    renderPanel();
    const table = screen.getByRole('table');
    // indoor:temp = static 배지 1개, 나머지 2개 = dynamic 배지.
    expect(within(table).getAllByText('agents.detail.store.static')).toHaveLength(1);
    expect(within(table).getAllByText('agents.detail.store.dynamic')).toHaveLength(2);
  });
});

describe('StoreTab 특성화 — storeEntrySort 헤더 정렬', () => {
  it('키 헤더 클릭 시 asc, 재클릭 시 desc 로 토글된다', () => {
    renderPanel();
    const header = screen.getAllByLabelText('agents.detail.store.sortAriaLabel')[0]!;
    fireEvent.click(header); // asc
    expect(rowKeys()).toEqual(['dynamic:count', 'indoor:temp', 'outdoor:humidity']);
    fireEvent.click(header); // desc
    expect(rowKeys()).toEqual(['outdoor:humidity', 'indoor:temp', 'dynamic:count']);
  });
});

describe('StoreTab 특성화 — 컬럼 표시/숨김 + localStorage 영속', () => {
  it('namespace 컬럼을 숨기면 헤더에서 사라지고 localStorage 에 영속된다', () => {
    renderPanel();
    const table = screen.getByRole('table');
    expect(within(table).getByText('agents.detail.store.colNamespace')).toBeInTheDocument();

    // 컬럼 설정 메뉴 열기 → namespace 체크박스 토글.
    fireEvent.click(screen.getByTestId('store-columns-settings'));
    const menu = screen.getByRole('menu');
    const nsLabel = within(menu)
      .getByText('agents.detail.store.colNamespace')
      .closest('label')!;
    const nsCheckbox = nsLabel.querySelector('input[type="checkbox"]')!;
    fireEvent.click(nsCheckbox);

    // 헤더에서 namespace 컬럼이 사라진다(헤더/본문 단일 출처).
    expect(
      within(screen.getByRole('table')).queryByText('agents.detail.store.colNamespace'),
    ).not.toBeInTheDocument();

    // per-agent localStorage 에 가시 컬럼 배열이 저장되고 namespace 를 제외한다.
    const raw = window.localStorage.getItem(COLUMNS_STORAGE_KEY);
    expect(raw).not.toBeNull();
    const stored = JSON.parse(raw!) as string[];
    expect(stored).not.toContain('namespace');
    expect(stored).toContain('key');
  });

  it('저장된 컬럼 가시성이 재마운트 시 복원된다(value 숨김)', () => {
    // value 를 제외한 가시 컬럼을 사전 저장.
    window.localStorage.setItem(
      COLUMNS_STORAGE_KEY,
      JSON.stringify(['key', 'field', 'namespace', 'binding', 'ttl', 'updated']),
    );
    renderPanel();
    const table = screen.getByRole('table');
    expect(within(table).getByText('agents.detail.store.colKey')).toBeInTheDocument();
    // value 컬럼은 저장값에 없으므로 복원 시 숨겨진다.
    expect(
      within(table).queryByText('agents.detail.store.colValue'),
    ).not.toBeInTheDocument();
  });
});
