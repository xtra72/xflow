// SPEC-WEB-007 v0.1.0 (M2, M7, M8, M9) — SystemInfoCard 컴포넌트 테스트.
//
// "이 인스턴스 (self)" Identity 카드. useSystemVersion 훅을 mock 으로 주입하여
// 로딩/에러/성공 상태와 필드 렌더링·모드 라벨 매핑·monospaced 칩을 검증한다.
//
// 커버 AC:
//   - AC-4  Identity 필드 렌더 (hostname/OS·Arch/version) + monospaced 칩
//   - AC-5  mode 3종 한글 라벨 매핑
//   - AC-11 영역별 독립 로딩/에러 ("다시 시도" refetch)
//   - AC-12 "이 인스턴스 (self)" 라벨 존재
//
// @spec SPEC-WEB-007 v0.1.0 (M2, M7, M8, M9)

import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { VersionInfo } from '@/services/api/systemUpdate';

// useSystemVersion 훅 mock — 상태를 테스트별로 제어한다.
const useSystemVersionMock = vi.hoisted(() => vi.fn());
vi.mock('@/services/api/systemUpdate', async () => {
  const actual = await vi.importActual<
    typeof import('@/services/api/systemUpdate')
  >('@/services/api/systemUpdate');
  return {
    ...actual,
    useSystemVersion: useSystemVersionMock,
  };
});

import { SystemInfoCard } from './SystemInfoCard';

// ─────────────────────────────────────────────────────────────────────
// Test fixtures
// ─────────────────────────────────────────────────────────────────────

function makeVersion(overrides: Partial<VersionInfo> = {}): VersionInfo {
  return {
    version: 'v0.18.6',
    commit: 'abc1234',
    build_date: '2026-04-30T12:00:00Z',
    go_version: 'go1.25.0',
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

interface QueryState {
  data?: VersionInfo;
  isLoading?: boolean;
  isError?: boolean;
  error?: Error;
  refetch?: () => void;
}

function setQueryState(state: QueryState) {
  useSystemVersionMock.mockReturnValue({
    data: state.data,
    isLoading: state.isLoading ?? false,
    isError: state.isError ?? false,
    error: state.error,
    refetch: state.refetch ?? vi.fn(),
  });
}

beforeEach(() => {
  useSystemVersionMock.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

// ─────────────────────────────────────────────────────────────────────
// AC-4 — Identity 전 필드 렌더 + monospaced 칩
// ─────────────────────────────────────────────────────────────────────

describe('SystemInfoCard — Identity 필드 (AC-4)', () => {
  it('hostname/OS·Arch/version 을 모두 표시한다', () => {
    setQueryState({ data: makeVersion() });
    render(<SystemInfoCard />);

    expect(screen.getByTestId('sysinfo-hostname')).toHaveTextContent(
      'xflow-node-01',
    );
    expect(screen.getByTestId('sysinfo-os-arch')).toHaveTextContent(
      'linux/amd64',
    );
    expect(screen.getByTestId('sysinfo-version')).toHaveTextContent('v0.18.6');
  });

  it('version/hostname/OS·Arch 는 monospaced 칩(font-mono)으로 표시된다', () => {
    setQueryState({ data: makeVersion() });
    render(<SystemInfoCard />);

    for (const id of ['sysinfo-version', 'sysinfo-hostname', 'sysinfo-os-arch']) {
      expect(screen.getByTestId(id).className).toMatch(/font-mono/);
    }
  });
});

// ─────────────────────────────────────────────────────────────────────
// AC-5 — 모드 한글 라벨 매핑
// ─────────────────────────────────────────────────────────────────────

describe('SystemInfoCard — 모드 라벨 매핑 (AC-5)', () => {
  it.each([
    ['server', '관리 서버'],
    ['client', '클라이언트 노드'],
    ['disabled', '독립 실행 (standalone)'],
  ] as const)('mode=%s → "%s" 라벨을 표시한다', (mode, label) => {
    setQueryState({ data: makeVersion({ mode }) });
    render(<SystemInfoCard />);
    expect(screen.getByTestId('sysinfo-mode')).toHaveTextContent(label);
  });
});

// ─────────────────────────────────────────────────────────────────────
// AC-12 — "이 인스턴스 (self)" 라벨
// ─────────────────────────────────────────────────────────────────────

describe('SystemInfoCard — self 라벨 (AC-12)', () => {
  it('"이 인스턴스 (self)" 라벨이 표시된다', () => {
    setQueryState({ data: makeVersion() });
    render(<SystemInfoCard />);
    expect(screen.getByTestId('sysinfo-card')).toHaveTextContent(
      /이 인스턴스 \(self\)/,
    );
  });

  it('mode=server 일 때 원격 노드 보조 안내 링크를 표시한다', () => {
    setQueryState({ data: makeVersion({ mode: 'server' }) });
    render(<SystemInfoCard />);
    expect(screen.getByTestId('sysinfo-remote-hint')).toHaveTextContent(
      /원격 노드 목록/,
    );
  });

  it('mode=disabled 일 때 원격 노드 보조 안내 링크를 표시하지 않는다', () => {
    setQueryState({ data: makeVersion({ mode: 'disabled' }) });
    render(<SystemInfoCard />);
    expect(screen.queryByTestId('sysinfo-remote-hint')).not.toBeInTheDocument();
  });
});

// ─────────────────────────────────────────────────────────────────────
// AC-11 — 영역별 독립 로딩/에러
// ─────────────────────────────────────────────────────────────────────

describe('SystemInfoCard — 로딩/에러 독립 (AC-11)', () => {
  it('isLoading=true → 스켈레톤을 표시한다', () => {
    setQueryState({ isLoading: true });
    render(<SystemInfoCard />);
    expect(screen.getByTestId('sysinfo-loading')).toBeInTheDocument();
  });

  it('isError=true → 에러 + "다시 시도" 버튼을 표시한다', () => {
    setQueryState({ isError: true, error: new Error('boom') });
    render(<SystemInfoCard />);
    expect(screen.getByTestId('sysinfo-error')).toBeInTheDocument();
    expect(screen.getByTestId('sysinfo-retry')).toHaveTextContent(/다시 시도/);
  });

  it('"다시 시도" 클릭 시 refetch 가 호출된다', () => {
    const refetch = vi.fn();
    setQueryState({ isError: true, error: new Error('boom'), refetch });
    render(<SystemInfoCard />);

    fireEvent.click(screen.getByTestId('sysinfo-retry'));
    expect(refetch).toHaveBeenCalledTimes(1);
  });
});
