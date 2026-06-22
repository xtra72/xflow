// SPEC-WEB-007 — SettingsPage 시스템 탭 테스트.
//
// 설정 페이지의 "시스템" 탭을 활성화했을 때 로컬(self) 인스턴스의
// 시스템 정보 카드(SystemInfoCard, SystemRuntimeCard)가 렌더링되는지 검증한다.
//
// 두 카드는 내부적으로 react-query 훅(useSystemVersion / useSystemMetrics)을
// 소비하므로, SystemStatusPage.test.tsx 의 mocking/wrapper 패턴을 동일하게 따른다.
//
// @spec SPEC-WEB-007

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import type { PropsWithChildren } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// ─────────────────────────────────────────────────────────────────────
// 카드가 의존하는 데이터 훅 mock — 반환값을 결정적으로 제어한다.
// ─────────────────────────────────────────────────────────────────────

const useSystemVersionMock = vi.hoisted(() => vi.fn());
const useSystemMetricsMock = vi.hoisted(() => vi.fn());

// 로그 레벨 API mock — 컴포넌트별 오버라이드 테이블을 결정적으로 제어한다.
const getLogLevelsMock = vi.hoisted(() => vi.fn());
const setComponentLogLevelMock = vi.hoisted(() => vi.fn());
const resetComponentLogLevelMock = vi.hoisted(() => vi.fn());
const setLogLevelMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/systemUpdate', async () => {
  const actual = await vi.importActual<
    typeof import('@/services/api/systemUpdate')
  >('@/services/api/systemUpdate');
  return {
    ...actual,
    useSystemVersion: useSystemVersionMock,
  };
});

vi.mock('@/services/api/monitorService', async () => {
  const actual = await vi.importActual<
    typeof import('@/services/api/monitorService')
  >('@/services/api/monitorService');
  return {
    ...actual,
    useSystemMetrics: useSystemMetricsMock,
    getLogLevels: getLogLevelsMock,
    setComponentLogLevel: setComponentLogLevelMock,
    resetComponentLogLevel: resetComponentLogLevelMock,
    setLogLevel: setLogLevelMock,
  };
});

// ─────────────────────────────────────────────────────────────────────
// authStore mock — 시스템 탭 RBAC 분기를 위해 admin 사용자 주입.
// selector 호출과 직접 호출을 모두 지원한다.
// ─────────────────────────────────────────────────────────────────────

const authState = {
  user: { name: 'admin', role: 'admin' as const },
};

vi.mock('@/stores/authStore', async () => {
  const actual = await vi.importActual<typeof import('@/stores/authStore')>(
    '@/stores/authStore',
  );
  return {
    ...actual,
    useAuthStore: Object.assign(
      (selector?: (state: typeof authState) => unknown) =>
        selector ? selector(authState) : authState,
      { getState: () => authState },
    ),
  };
});

// uiStore mock — addNotification 호출만 캡처한다.
const addNotificationMock = vi.hoisted(() => vi.fn());

vi.mock('@/stores/uiStore', async () => {
  const actual = await vi.importActual<typeof import('@/stores/uiStore')>(
    '@/stores/uiStore',
  );
  return {
    ...actual,
    useUIStore: Object.assign(
      (selector?: (state: { addNotification: typeof addNotificationMock }) => unknown) => {
        const state = { addNotification: addNotificationMock };
        return selector ? selector(state) : state;
      },
      { getState: () => ({ addNotification: addNotificationMock }) },
    ),
  };
});

import type { VersionInfo } from '@/services/api/systemUpdate';
import type { SystemMetrics } from '@/services/api/monitorService';
import { I18nProvider, useTranslation } from '@/lib/i18n';

import SettingsPage from './SettingsPage';

// ─────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────

function makeVersion(overrides: Partial<VersionInfo> = {}): VersionInfo {
  return {
    version: 'v0.3.0',
    commit: 'abc1234',
    build_date: '2026-06-22T00:00:00Z',
    go_version: 'go1.23',
    channel: 'stable',
    update_available: false,
    latest_version: null,
    os: 'linux',
    arch: 'amd64',
    hostname: 'xflow-node-01',
    mode: 'server',
    uptime_seconds: 3600,
    ...overrides,
  };
}

function makeMetrics(overrides: Partial<SystemMetrics> = {}): SystemMetrics {
  return {
    cpu_usage_percent: 0,
    memory_usage_percent: 42.5,
    go_routines: 12,
    go_mem_alloc_mb: 24,
    go_mem_sys_mb: 64,
    uptime_seconds: 3600,
    ...overrides,
  };
}

function buildWrapper() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  }
  return { Wrapper };
}

beforeEach(() => {
  // 로그 레벨 API 기본 동작 — 각 테스트에서 필요 시 재정의한다.
  getLogLevelsMock.mockResolvedValue({ default_level: 'info', components: {} });
  setComponentLogLevelMock.mockResolvedValue(undefined);
  resetComponentLogLevelMock.mockResolvedValue(undefined);
  setLogLevelMock.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.clearAllMocks();
});

/** 시스템 정보 카드 훅의 기본 mock 반환값을 설정한다. */
function primeSystemCards() {
  useSystemVersionMock.mockReturnValue({
    data: makeVersion(),
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  });
  useSystemMetricsMock.mockReturnValue({
    data: makeMetrics(),
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  });
}

/** 컴포넌트별 로그 레벨 오버라이드 샘플(다양한 종류 포함). */
const SAMPLE_OVERRIDES: Record<string, string> = {
  'agent.mqtt-client': 'debug',
  'flow.autostart': 'warn',
  'db.postgres': 'error',
};

// ─────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────

describe('SettingsPage 시스템 탭 — 시스템 정보 카드', () => {
  it('시스템 탭 활성화 시 인스턴스 정보 / 런타임 메트릭 카드를 렌더한다', () => {
    useSystemVersionMock.mockReturnValue({
      data: makeVersion(),
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    useSystemMetricsMock.mockReturnValue({
      data: makeMetrics(),
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    const { Wrapper } = buildWrapper();
    render(<SettingsPage />, { wrapper: Wrapper });

    // 시스템 탭으로 전환한다.
    fireEvent.click(screen.getByRole('button', { name: '시스템' }));

    // 두 카드가 렌더링되었는지 testid + heading 으로 확인한다.
    const infoCard = screen.getByTestId('sysinfo-card');
    expect(within(infoCard).getByText('인스턴스 정보')).toBeInTheDocument();

    const runtimeCard = screen.getByTestId('runtime-card');
    expect(within(runtimeCard).getByText('런타임 메트릭')).toBeInTheDocument();
  });

  it('두 카드는 데이터 훅(useSystemVersion / useSystemMetrics)을 소비한다', () => {
    useSystemVersionMock.mockReturnValue({
      data: makeVersion(),
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    useSystemMetricsMock.mockReturnValue({
      data: makeMetrics(),
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    const { Wrapper } = buildWrapper();
    render(<SettingsPage />, { wrapper: Wrapper });
    fireEvent.click(screen.getByRole('button', { name: '시스템' }));

    expect(useSystemVersionMock).toHaveBeenCalled();
    expect(useSystemMetricsMock).toHaveBeenCalled();
  });
});

// ─────────────────────────────────────────────────────────────────────
// 컴포넌트별 로그 레벨 오버라이드 테이블
// ─────────────────────────────────────────────────────────────────────

describe('SettingsPage 시스템 탭 — 컴포넌트별 로그 레벨', () => {
  /** 시스템 탭을 활성화하고 오버라이드가 로드될 때까지 기다린다. */
  async function renderSystemTab() {
    primeSystemCards();
    const { Wrapper } = buildWrapper();
    render(<SettingsPage />, { wrapper: Wrapper });
    fireEvent.click(screen.getByRole('button', { name: '시스템' }));
    // 비동기 getLogLevels 로드 완료 대기.
    await screen.findByText('agent.mqtt-client');
  }

  it('종류(category)를 한글 라벨 배지로 표시한다', async () => {
    getLogLevelsMock.mockResolvedValue({
      default_level: 'info',
      components: SAMPLE_OVERRIDES,
    });

    await renderSystemTab();

    // 첫 세그먼트 → 한글 종류 라벨 매핑이 각 행 배지에 적용되었는지 확인.
    // 종류 필터 select에도 동일 라벨이 존재하므로 행(<tr>) 범위로 한정한다.
    const agentRow = screen.getByText('agent.mqtt-client').closest('tr');
    const flowRow = screen.getByText('flow.autostart').closest('tr');
    const dbRow = screen.getByText('db.postgres').closest('tr');
    expect(agentRow).not.toBeNull();
    expect(flowRow).not.toBeNull();
    expect(dbRow).not.toBeNull();
    expect(within(agentRow as HTMLElement).getByText('에이전트')).toBeInTheDocument();
    expect(within(flowRow as HTMLElement).getByText('플로우')).toBeInTheDocument();
    expect(within(dbRow as HTMLElement).getByText('데이터베이스')).toBeInTheDocument();
  });

  it('이름 검색 필터로 표시 행을 좁히고, 결과 0건 빈 상태를 표출한다', async () => {
    getLogLevelsMock.mockResolvedValue({
      default_level: 'info',
      components: SAMPLE_OVERRIDES,
    });

    await renderSystemTab();

    const search = screen.getByLabelText('컴포넌트 검색');
    fireEvent.change(search, { target: { value: 'postgres' } });

    expect(screen.getByText('db.postgres')).toBeInTheDocument();
    expect(screen.queryByText('agent.mqtt-client')).not.toBeInTheDocument();

    // 매칭 0건 → 전용 빈 상태 메시지.
    fireEvent.change(search, { target: { value: '존재하지않는컴포넌트' } });
    expect(screen.getByText('조건에 맞는 컴포넌트가 없습니다')).toBeInTheDocument();
  });

  it('행 레벨 select 변경 시 setComponentLogLevel을 호출한다', async () => {
    getLogLevelsMock.mockResolvedValue({
      default_level: 'info',
      components: SAMPLE_OVERRIDES,
    });

    await renderSystemTab();

    const levelSelect = screen.getByLabelText('agent.mqtt-client 로그 레벨');
    fireEvent.change(levelSelect, { target: { value: 'error' } });

    await waitFor(() => {
      expect(setComponentLogLevelMock).toHaveBeenCalledWith('agent.mqtt-client', 'error');
    });
  });

  it('전체 선택 후 일괄 리셋 시 모든 컴포넌트에 resetComponentLogLevel을 호출한다', async () => {
    getLogLevelsMock.mockResolvedValue({
      default_level: 'info',
      components: SAMPLE_OVERRIDES,
    });

    await renderSystemTab();

    // 표시된 컴포넌트 전체 선택.
    fireEvent.click(screen.getByLabelText('표시된 컴포넌트 전체 선택'));
    expect(screen.getByText('3개 선택됨')).toBeInTheDocument();

    // 일괄 리셋 실행.
    fireEvent.click(screen.getByRole('button', { name: '선택 리셋' }));

    await waitFor(() => {
      expect(resetComponentLogLevelMock).toHaveBeenCalledTimes(3);
    });
    expect(resetComponentLogLevelMock).toHaveBeenCalledWith('agent.mqtt-client');
    expect(resetComponentLogLevelMock).toHaveBeenCalledWith('flow.autostart');
    expect(resetComponentLogLevelMock).toHaveBeenCalledWith('db.postgres');
  });
});

// ─────────────────────────────────────────────────────────────────────
// 언어 탭 — i18n 연동 (재현 테스트)
//
// 언어 탭에서 언어를 바꾸면 i18n 시스템(I18nProvider/useTranslation)에
// 실제로 반영되어야 한다. 수정 전에는 LanguageTab 이 별도 localStorage 키
// ('xflow-language')와 로컬 state 만 사용하여 i18n('xflow-locale')에 전혀
// 반영되지 않으므로 이 테스트가 실패한다.
// ─────────────────────────────────────────────────────────────────────

describe('SettingsPage 언어 탭 — i18n 연동', () => {
  /** i18n 의 현재 locale 을 화면에 노출하는 프로브 컴포넌트. */
  function LocaleProbe() {
    const { locale } = useTranslation();
    return <span data-testid="locale-probe">{locale}</span>;
  }

  /** SettingsPage 와 LocaleProbe 를 실제 I18nProvider + QueryClient 로 감싼다. */
  function buildI18nWrapper() {
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    function Wrapper({ children }: PropsWithChildren) {
      return (
        <QueryClientProvider client={client}>
          <I18nProvider>
            {children}
            <LocaleProbe />
          </I18nProvider>
        </QueryClientProvider>
      );
    }
    return { Wrapper };
  }

  beforeEach(() => {
    // i18n 은 localStorage('xflow-locale')에서 초기 locale 을 읽으므로
    // 각 테스트 전에 깨끗한 상태(기본 'ko')로 초기화한다.
    localStorage.clear();
  });

  it('English 라디오 클릭 시 i18n locale 이 en 으로 전환된다', () => {
    primeSystemCards();
    const { Wrapper } = buildI18nWrapper();
    render(<SettingsPage />, { wrapper: Wrapper });

    // 초기 locale 은 'ko'.
    expect(screen.getByTestId('locale-probe')).toHaveTextContent('ko');

    // 언어 탭으로 전환.
    fireEvent.click(screen.getByRole('button', { name: '언어' }));

    // "English" 라디오 선택.
    fireEvent.click(screen.getByRole('radio', { name: 'English' }));

    // (c) 프로브: i18n locale 이 en 으로 반영되었는가.
    expect(screen.getByTestId('locale-probe')).toHaveTextContent('en');

    // (b) localStorage: i18n 이 읽는 키('xflow-locale')에 저장되었는가.
    expect(localStorage.getItem('xflow-locale')).toBe('en');
  });
});
