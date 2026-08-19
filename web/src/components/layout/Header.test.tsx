// Header 페이지 제목 해석 테스트 (SPEC-REMOTE-001 M8 UX).
//
// 범위:
//   - 원격 노드 컨텍스트 경로(/admin/remote/nodes/:instanceId/...)에서는 사이드바
//     브랜드("XFlow")와 중복되는 폴백 대신 대상 노드 이름("원격 · {호스트명}")을
//     제목으로 표시한다. 노드 상세 미조회 시 단축 instanceId 로 폴백한다.
//   - 로컬/기타 경로의 제목은 변경되지 않는다(회귀 방지).

import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

// ---- 데이터 훅 모킹 (제목 해석과 무관한 우측 액션/상태는 안정 stub) ----
vi.mock('@/hooks/useAuth', () => ({
  useAuth: () => ({ user: null, authEnabled: false, logout: vi.fn() }),
}));
vi.mock('@/hooks/useWebSocket', () => ({
  useWebSocket: () => ({ state: 'connected' }),
}));
vi.mock('@/services/api/systemUpdate', () => ({
  useSystemVersion: () => ({ data: undefined }),
}));

// useRemoteNodeDetail — 호스트명 해석 소스. 테스트별로 반환을 바꾼다.
const useRemoteNodeDetailMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useRemote', () => ({
  useRemoteNodeDetail: useRemoteNodeDetailMock,
}));

// 제목과 무관한 무거운 자식들은 가벼운 stub 으로 대체한다.
vi.mock('@/components/theme/ThemeSelector', () => ({
  ThemeSelector: () => null,
}));
vi.mock('@/components/theme/ThemeEditorModal', () => ({
  ThemeEditorModal: () => null,
}));
vi.mock('@/components/system/UpdateAvailableBadge', () => ({
  UpdateAvailableBadge: () => null,
}));
vi.mock('@/pages/dashboard/CreateDashboardDialog', () => ({
  default: () => null,
}));
vi.mock('@/pages/auth/ChangePasswordDialog', () => ({
  default: () => null,
}));

import Header from './Header';

function renderHeader(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <I18nProvider>
        <Header />
      </I18nProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  useRemoteNodeDetailMock.mockReset();
  useRemoteNodeDetailMock.mockReturnValue({ data: undefined });
});

describe('Header — 원격 노드 컨텍스트 제목', () => {
  it('원격 플로우 편집기 경로에서 호스트명을 제목으로 표시한다(XFlow 아님)', () => {
    useRemoteNodeDetailMock.mockReturnValue({ data: { hostname: 'gw-1' } });
    renderHeader(
      '/admin/remote/nodes/inst-uuid-1234/flows/flow-1/edit',
    );

    const heading = screen.getByRole('heading', { level: 1 });
    expect(heading).toHaveTextContent('원격 · gw-1');
    expect(heading).not.toHaveTextContent('XFlow');
  });

  it('호스트명 미조회 시 단축 instanceId 로 폴백한다(원시 UUID 전체 미노출)', () => {
    useRemoteNodeDetailMock.mockReturnValue({ data: undefined });
    renderHeader(
      '/admin/remote/nodes/abcdef0123456789/flows/new',
    );

    const heading = screen.getByRole('heading', { level: 1 });
    // 앞 8자 + 생략부호.
    expect(heading).toHaveTextContent('원격 · abcdef01…');
    // 전체 UUID 는 제목 본문에 노출하지 않는다.
    expect(heading).not.toHaveTextContent('abcdef0123456789');
  });

  it('원격 노드 대시보드 경로(편집기 외)에서도 노드 이름을 제목으로 쓴다', () => {
    useRemoteNodeDetailMock.mockReturnValue({ data: { hostname: 'edge-7' } });
    renderHeader('/admin/remote/nodes/inst-9/system');

    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(
      '원격 · edge-7',
    );
  });
});

describe('Header — 로컬/기타 경로 제목(회귀 방지)', () => {
  it('플로우 목록 경로의 제목은 변경되지 않는다', () => {
    renderHeader('/flows');
    // nav.flows 번역값(한국어 "플로우").
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('플로우');
    // 원격 노드 상세는 조회하지 않는다.
    expect(useRemoteNodeDetailMock).toHaveBeenCalledWith('', false);
  });

  it('미인식 경로는 기존 폴백("XFlow")을 유지한다', () => {
    renderHeader('/some/unknown/route');
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('XFlow');
  });

  it('로컬 에디터 경로는 "플로우"(nav.editor) 제목을 유지한다', () => {
    renderHeader('/editor/flow-1');
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('플로우');
  });
});

// 사용자 관리 화면은 본문에서 제목을 걷어냈다. 제목이 앱 헤더에서 실제로
// 그려지는지 여기서 확인하지 않으면 "어디에도 제목이 없는" 회귀가 통과한다.
describe('Header — 사용자 관리 제목', () => {
  it('/admin/users 에서 "사용자 관리"를 제목으로 표시한다', () => {
    renderHeader('/admin/users');

    const heading = screen.getByRole('heading', { level: 1 });
    expect(heading).toHaveTextContent('사용자 관리');
    // 폴백으로 떨어지지 않는다.
    expect(heading).not.toHaveTextContent('XFlow');
  });

  it('역할 탭(`?tab=roles`)에서도 같은 제목을 유지한다 — 제목은 경로로 정해진다', () => {
    renderHeader('/admin/users?tab=roles');

    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('사용자 관리');
  });

  it('Header 는 이 경로에서 null 을 반환하지 않는다 (대시보드 경로만 숨긴다)', () => {
    const { container } = renderHeader('/admin/users');
    expect(container.querySelector('header')).not.toBeNull();
  });
});

// 원격 관리 하위 4개 화면도 본문에서 제목을 걷어냈다. 사용자 관리와 같은 이유로
// 앱 헤더가 실제로 제목을 그리는지 여기서 못 박는다.
describe('Header — 원격 관리 하위 화면 제목', () => {
  const CASES: ReadonlyArray<readonly [path: string, title: string]> = [
    ['/admin/remote', '노드 관리'],
    ['/admin/remote/groups', '그룹 관리'],
    ['/admin/remote/enrollment', '등록 관리'],
    ['/admin/remote/releases', '릴리스 저장소'],
  ];

  it.each(CASES)('%s 에서 "%s"를 제목으로 표시한다', (path, title) => {
    renderHeader(path);

    const heading = screen.getByRole('heading', { level: 1 });
    expect(heading).toHaveTextContent(title);
    // 폴백으로 떨어지지 않는다.
    expect(heading).not.toHaveTextContent('XFlow');
  });

  it('원격 노드 동적 경로는 노드 이름 제목을 유지한다(정확 일치이므로 "노드 관리"로 새지 않는다)', () => {
    useRemoteNodeDetailMock.mockReturnValue({ data: { hostname: 'gw-9' } });
    renderHeader('/admin/remote/nodes/inst-9/flows/flow-1/edit');

    const heading = screen.getByRole('heading', { level: 1 });
    expect(heading).toHaveTextContent('원격 · gw-9');
    expect(heading).not.toHaveTextContent('노드 관리');
  });

  it('원격 노드 상세 경로도 마찬가지다 — 정적 매핑을 타지 않는다', () => {
    useRemoteNodeDetailMock.mockReturnValue({ data: { hostname: 'edge-3' } });
    renderHeader('/admin/remote/nodes/inst-3/system');

    const heading = screen.getByRole('heading', { level: 1 });
    expect(heading).toHaveTextContent('원격 · edge-3');
    expect(heading).not.toHaveTextContent('노드 관리');
  });
});
