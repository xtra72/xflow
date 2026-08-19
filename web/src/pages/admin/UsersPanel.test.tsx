// 사용자 목록 패널 테스트 (SPEC-AUTH-006 M2.2 — AC-03).
//
// 사용자 관리 화면이 사용자/역할 두 탭 컨테이너가 되면서 본문이 패널
// (UsersPanel)로 분리되었다. 기존 검증은 전부 옮겨 왔고, 레이아웃 변경
// (제목 이관·전체 폭·PermissionButton·자기 자신 행 삭제 제거) 케이스를 더했다.
//
// 서비스 계층(userService)만 스텁하고 훅·페이지는 실제 코드를 그대로 돌린다.
// i18n 도 실제 Provider 를 쓰므로, 키가 로케일에 없으면 t() 가 키 문자열을
// 그대로 반환해 테스트에서 드러난다(미해결 키 회귀 방지).
//
// @spec SPEC-AUTH-006 v0.1.0 (M2.2)

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import { APIError } from '@/types/api';
import type { RoleResponse, UserResponse } from '@/services/api/userService';
import { useAuthStore } from '@/stores/authStore';
import UsersPanel from './UsersPanel';

// ---- 서비스 스텁 ----

const getUsers = vi.hoisted(() => vi.fn());
const createUser = vi.hoisted(() => vi.fn());
const updateUserRole = vi.hoisted(() => vi.fn());
const resetUserPassword = vi.hoisted(() => vi.fn());
const deleteUser = vi.hoisted(() => vi.fn());
const getRoles = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/userService', async () => {
  const actual = await vi.importActual<typeof import('@/services/api/userService')>(
    '@/services/api/userService',
  );
  return {
    ...actual,
    getUsers,
    createUser,
    updateUserRole,
    resetUserPassword,
    deleteUser,
    getRoles,
  };
});

// ---- 권한 스텁 ----
// 폴백 규칙은 usePermission 자체 테스트(M1)가 담당한다. 여기서는 보유 권한
// 집합만 결정적으로 바꿔 게이팅 분기를 검증한다.

const granted = vi.hoisted(() => ({ keys: new Set<string>() }));

vi.mock('@/hooks/usePermission', () => ({
  usePermission: () => ({
    hasPermission: (key: string) => granted.keys.has(key),
    hasAnyPermission: (keys: readonly string[]) =>
      keys.some((k) => granted.keys.has(k)),
    canSeeMenu: (key: string) => granted.keys.has(key),
    isPermissionUnavailable: false,
  }),
}));

function grant(...keys: string[]): void {
  granted.keys = new Set(keys);
}

// ---- 픽스처 ----

const USERS: UserResponse[] = [
  { username: 'admin', role: 'admin', created_at: 1_700_000_000_000, updated_at: 1_700_000_100_000 },
  { username: 'alice', role: 'editor', created_at: 1_700_000_200_000, updated_at: 1_700_000_300_000 },
];

const ROLES: RoleResponse[] = [
  { name: 'admin', description: '전체 권한', builtin: true, permissions: ['user.read'], created_at: 1, updated_at: 1 },
  { name: 'editor', description: '편집', builtin: true, permissions: [], created_at: 1, updated_at: 1 },
  { name: 'viewer', description: '조회', builtin: true, permissions: [], created_at: 1, updated_at: 1 },
  { name: 'operator', description: '커스텀', builtin: false, permissions: [], created_at: 1, updated_at: 1 },
];

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <I18nProvider>
      <QueryClientProvider client={client}>
        <UsersPanel />
      </QueryClientProvider>
    </I18nProvider>,
  );
}

/** 목록이 그려질 때까지 기다린다. */
async function waitForList(): Promise<void> {
  await screen.findByTestId('admin-user-row-alice');
}

beforeEach(() => {
  vi.clearAllMocks();
  grant('user.read', 'user.create', 'user.update', 'user.delete', 'role.read');
  getUsers.mockResolvedValue(USERS);
  getRoles.mockResolvedValue(ROLES);
  // 기본은 '로그인 계정 정보 없음'. 자기 자신 행 판정을 보는 테스트만 지정한다.
  useAuthStore.setState({ user: null });
});

/** 로그인 계정을 지정한다. authStore 의 User.name 이 서버의 username 이다. */
function signInAs(username: string): void {
  useAuthStore.setState({ user: { name: username, role: 'admin' } });
}

describe('UsersPanel 목록 (AC-03)', () => {
  it('username·역할·생성일·수정일을 표시한다', async () => {
    renderPage();
    await waitForList();

    const row = screen.getByTestId('admin-user-row-alice');
    expect(within(row).getByText('alice')).toBeInTheDocument();
    // 역할은 인라인 변경 가능한 select 로 현재 값을 보여준다.
    expect(within(row).getByLabelText('역할 변경')).toHaveValue('editor');
    // 생성일/수정일 두 셀이 epoch ms 를 사람이 읽는 문자열로 변환한다.
    expect(within(row).getAllByText(/2023/)).toHaveLength(2);
  });

  it('비밀번호 해시를 어디에도 표시하지 않는다', async () => {
    renderPage();
    await waitForList();

    const body = document.body.textContent ?? '';
    expect(body).not.toMatch(/password_hash/i);
    expect(body).not.toMatch(/\$2[aby]\$/); // bcrypt 해시 접두사
    // 컬럼은 username/역할/생성일/수정일/액션 5개뿐이다.
    expect(screen.getAllByRole('columnheader')).toHaveLength(5);
  });

  it('사용자가 없으면 빈 목록 안내를 표시한다', async () => {
    getUsers.mockResolvedValue([]);
    renderPage();

    expect(await screen.findByText('등록된 사용자가 없습니다.')).toBeInTheDocument();
  });

  it('삭제된 역할을 가진 사용자도 크래시 없이 현재 역할을 유지해 보여준다', async () => {
    getUsers.mockResolvedValue([
      { username: 'ghost', role: 'deleted-role', created_at: 1, updated_at: 1 },
    ]);
    renderPage();

    const row = await screen.findByTestId('admin-user-row-ghost');
    expect(within(row).getByLabelText('역할 변경')).toHaveValue('deleted-role');
  });

  it('매우 긴 역할 이름도 말줄임 클래스로 레이아웃을 유지한다', async () => {
    getUsers.mockResolvedValue([
      { username: 'bob', role: 'a'.repeat(32), created_at: 1, updated_at: 1 },
    ]);
    renderPage();

    const row = await screen.findByTestId('admin-user-row-bob');
    expect(within(row).getByLabelText('역할 변경').className).toContain('truncate');
  });
});

describe('UsersPanel 등록 폼 (AC-03)', () => {
  it('역할 선택지를 GET /roles 결과(빌트인 3종 + 커스텀)로 채운다', async () => {
    renderPage();
    await waitForList();

    fireEvent.click(screen.getByRole('button', { name: /사용자 등록/ }));

    const form = screen.getByTestId('admin-users-create-form');
    const options = within(form)
      .getAllByRole('option')
      .map((o) => (o as HTMLOptionElement).value);

    expect(options).toEqual(['', 'admin', 'editor', 'viewer', 'operator']);
  });

  it('username·비밀번호·역할을 담아 등록을 요청한다', async () => {
    createUser.mockResolvedValue(USERS[1]);
    renderPage();
    await waitForList();

    fireEvent.click(screen.getByRole('button', { name: /사용자 등록/ }));
    const form = screen.getByTestId('admin-users-create-form');

    fireEvent.change(within(form).getByRole('textbox'), { target: { value: 'carol' } });
    fireEvent.change(
      within(form).getByPlaceholderText('8자 이상'),
      { target: { value: 'secret-password' } },
    );
    fireEvent.change(within(form).getByRole('combobox'), { target: { value: 'operator' } });
    fireEvent.click(within(form).getByRole('button', { name: '등록' }));

    await waitFor(() =>
      expect(createUser).toHaveBeenCalledWith({
        username: 'carol',
        password: 'secret-password',
        role: 'operator',
      }),
    );
  });

  it('409 USER_EXISTS 거부 시 원인을 안내하고 폼을 닫지 않는다', async () => {
    createUser.mockRejectedValue(new APIError('USER_EXISTS', '이미 존재', 409));
    renderPage();
    await waitForList();

    fireEvent.click(screen.getByRole('button', { name: /사용자 등록/ }));
    const form = screen.getByTestId('admin-users-create-form');
    fireEvent.change(within(form).getByRole('textbox'), { target: { value: 'admin' } });
    fireEvent.change(within(form).getByPlaceholderText('8자 이상'), {
      target: { value: 'secret-password' },
    });
    fireEvent.change(within(form).getByRole('combobox'), { target: { value: 'admin' } });
    fireEvent.click(within(form).getByRole('button', { name: '등록' }));

    const alert = await screen.findByTestId('admin-action-error');
    expect(alert).toHaveTextContent('이미 존재하는 사용자명입니다.');
    expect(screen.getByTestId('admin-users-create-form')).toBeInTheDocument();
  });

  it('role.read 가 없으면 역할 목록을 조회하지 않고 자유 입력으로 낮춘다', async () => {
    grant('user.read', 'user.create');
    renderPage();
    await waitForList();

    fireEvent.click(screen.getByRole('button', { name: /사용자 등록/ }));
    const form = screen.getByTestId('admin-users-create-form');

    expect(getRoles).not.toHaveBeenCalled();
    expect(within(form).queryByRole('combobox')).not.toBeInTheDocument();
    expect(
      screen.getByText('역할 목록을 불러올 수 없어 직접 입력합니다.'),
    ).toBeInTheDocument();
  });
});

describe('UsersPanel 역할 변경·비밀번호 재설정 (AC-03)', () => {
  it('인라인 역할 변경을 서버에 반영한다', async () => {
    updateUserRole.mockResolvedValue({ ...USERS[1], role: 'viewer' });
    renderPage();
    await waitForList();

    const row = screen.getByTestId('admin-user-row-alice');
    fireEvent.change(within(row).getByLabelText('역할 변경'), {
      target: { value: 'viewer' },
    });

    await waitFor(() =>
      expect(updateUserRole).toHaveBeenCalledWith('alice', { role: 'viewer' }),
    );
  });

  it('자기 자신 강등이 409 로 거부되면 사유를 안내하고 목록은 변하지 않는다', async () => {
    updateUserRole.mockRejectedValue(
      new APIError('LAST_ADMIN_USER', '마지막 관리자', 409),
    );
    renderPage();
    await waitForList();

    const row = screen.getByTestId('admin-user-row-admin');
    fireEvent.change(within(row).getByLabelText('역할 변경'), {
      target: { value: 'viewer' },
    });

    expect(await screen.findByTestId('admin-action-error')).toHaveTextContent(
      '관리 권한을 가진 마지막 사용자입니다.',
    );
    // 목록은 서버 값 그대로다 — 낙관적 갱신을 하지 않으므로 재조회도 없다.
    expect(getUsers).toHaveBeenCalledTimes(1);
  });

  it('비밀번호 재설정이 400 이면 검증 규칙을 안내한다 (8자 미만)', async () => {
    resetUserPassword.mockRejectedValue(
      new APIError('BAD_REQUEST', 'password 는 최소 8자', 400),
    );
    renderPage();
    await waitForList();

    fireEvent.click(
      within(screen.getByTestId('admin-user-row-alice')).getByRole('button', {
        name: /비밀번호 재설정/,
      }),
    );
    const dialog = screen.getByRole('dialog');
    fireEvent.change(dialog.querySelectorAll('input')[0]!, { target: { value: 'short' } });
    fireEvent.click(within(dialog).getByRole('button', { name: '확인' }));

    expect(await screen.findByTestId('admin-action-error')).toHaveTextContent(
      '입력값이 올바르지 않습니다.',
    );
  });

  it('관리자 비밀번호 재설정은 현재 비밀번호를 요구하지 않는다', async () => {
    resetUserPassword.mockResolvedValue(undefined);
    renderPage();
    await waitForList();

    const row = screen.getByTestId('admin-user-row-alice');
    fireEvent.click(within(row).getByRole('button', { name: /비밀번호 재설정/ }));

    const dialog = screen.getByRole('dialog');
    // 입력란은 새 비밀번호 하나뿐이다.
    const inputs = dialog.querySelectorAll('input');
    expect(inputs).toHaveLength(1);

    fireEvent.change(inputs[0]!, { target: { value: 'new-password-1' } });
    fireEvent.click(within(dialog).getByRole('button', { name: '확인' }));

    await waitFor(() =>
      expect(resetUserPassword).toHaveBeenCalledWith('alice', {
        password: 'new-password-1',
      }),
    );
  });
});

describe('UsersPanel 삭제와 409 잠금 방지 (AC-03)', () => {
  it('마지막 관리자 삭제가 409 로 거부되면 원인을 안내하고 목록은 변하지 않는다', async () => {
    deleteUser.mockRejectedValue(
      new APIError('LAST_ADMIN_USER', '관리 권한을 보유한 마지막 사용자', 409),
    );
    renderPage();
    await waitForList();

    const before = screen.getAllByRole('row').length;
    const row = screen.getByTestId('admin-user-row-admin');
    fireEvent.click(within(row).getByRole('button', { name: /삭제 admin/ }));
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: '삭제' }),
    );

    const alert = await screen.findByTestId('admin-action-error');
    expect(alert).toHaveTextContent('관리 권한을 가진 마지막 사용자입니다.');

    // 목록 상태 불변: 행 수도 대상 행도 그대로다. 목록 재조회도 없다.
    expect(screen.getAllByRole('row')).toHaveLength(before);
    expect(screen.getByTestId('admin-user-row-admin')).toBeInTheDocument();
    expect(getUsers).toHaveBeenCalledTimes(1);
  });

  // 자기 자신 행에는 삭제 컨트롤이 없지만(아래 레이아웃 describe 참조), 서버는
  // 여전히 409 SELF_DELETION 을 돌려줄 수 있다(다른 세션에서의 계정 전환 등).
  // 오류 매핑은 그대로 유지되어야 한다 — 컨트롤 제거는 UI 어포던스일 뿐이고
  // 실제 강제는 계속 서버가 한다.
  it('409 SELF_DELETION 응답도 원인을 구분해 안내한다', async () => {
    deleteUser.mockRejectedValue(new APIError('SELF_DELETION', '자기 자신', 409));
    renderPage();
    await waitForList();

    fireEvent.click(
      within(screen.getByTestId('admin-user-row-admin')).getByRole('button', {
        name: /삭제 admin/,
      }),
    );
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: '삭제' }),
    );

    expect(await screen.findByTestId('admin-action-error')).toHaveTextContent(
      '현재 로그인한 계정은 삭제할 수 없습니다.',
    );
  });

  it('삭제에 성공하면 목록을 다시 읽는다', async () => {
    deleteUser.mockResolvedValue(undefined);
    renderPage();
    await waitForList();

    fireEvent.click(
      within(screen.getByTestId('admin-user-row-alice')).getByRole('button', {
        name: /삭제 alice/,
      }),
    );
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: '삭제' }),
    );

    await waitFor(() => expect(deleteUser).toHaveBeenCalledWith('alice'));
    await waitFor(() => expect(getUsers).toHaveBeenCalledTimes(2));
  });

  it('취소하면 삭제를 요청하지 않는다', async () => {
    renderPage();
    await waitForList();

    fireEvent.click(
      within(screen.getByTestId('admin-user-row-alice')).getByRole('button', {
        name: /삭제 alice/,
      }),
    );
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: '취소' }),
    );

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(deleteUser).not.toHaveBeenCalled();
  });
});

describe('UsersPanel 권한 게이팅', () => {
  it('user.create 가 없으면 등록 버튼을 비활성하고 사유를 노출한다', async () => {
    grant('user.read', 'role.read');
    renderPage();
    await waitForList();

    const button = screen.getByRole('button', { name: /사용자 등록/ });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('aria-disabled', 'true');
    expect(button).toHaveAttribute('title', '권한이 필요합니다');
  });

  it('등록 버튼은 PermissionButton 계약을 따른다 — 권한 없으면 클릭도 먹히지 않는다', async () => {
    grant('user.read', 'role.read');
    renderPage();
    await waitForList();

    // PermissionButton 은 권한이 없으면 onClick 자체를 붙이지 않는다.
    // disabled 를 우회한 합성 클릭에도 폼이 열리지 않아야 한다.
    fireEvent.click(screen.getByRole('button', { name: /사용자 등록/ }));
    expect(screen.queryByTestId('admin-users-create-form')).not.toBeInTheDocument();
  });

  it('user.update / user.delete 가 없으면 역할 변경·재설정·삭제가 비활성이다', async () => {
    grant('user.read', 'role.read');
    renderPage();
    await waitForList();

    const row = screen.getByTestId('admin-user-row-alice');
    expect(within(row).getByLabelText('역할 변경')).toBeDisabled();
    expect(within(row).getByRole('button', { name: /비밀번호 재설정/ })).toBeDisabled();
    expect(within(row).getByRole('button', { name: /삭제 alice/ })).toBeDisabled();
  });
});

describe('UsersPanel 조회 실패', () => {
  it('403 이면 권한 부족을 사용자 언어로 안내한다 (AC-08)', async () => {
    getUsers.mockRejectedValue(new APIError('FORBIDDEN', 'forbidden', 403));
    renderPage();

    expect(
      await screen.findByText('이 작업을 수행할 권한이 없습니다.'),
    ).toBeInTheDocument();
    // 자동 재시도 없음 — 한 번만 호출된다.
    await waitFor(() => expect(getUsers).toHaveBeenCalledTimes(1));
  });
});

describe('UsersPanel 레이아웃', () => {
  it('본문에는 화면 제목을 그리지 않는다 — 앱 헤더가 표시한다', async () => {
    renderPage();
    await waitForList();

    // 제목이 본문에도 있으면 헤더와 중복된다.
    expect(screen.queryByRole('heading', { name: '사용자 관리' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { level: 1 })).not.toBeInTheDocument();
  });

  it('목록 영역에 최대 폭 제약이 없다 — 페이지 전체 폭을 쓴다', async () => {
    const { container } = renderPage();
    await waitForList();

    const root = screen.getByTestId('admin-users-panel');
    expect(root.className).not.toMatch(/max-w-/);
    expect(root.className).not.toMatch(/mx-auto/);
    // 목록까지 이르는 어떤 조상에도 폭 제약이 없어야 한다.
    for (const el of container.querySelectorAll('div')) {
      expect(el.className).not.toMatch(/max-w-(?:sm|md|lg|xl|\d)/);
    }
  });

  it('등록 버튼은 플로우 등록 버튼과 같은 레이아웃 클래스를 쓴다', async () => {
    renderPage();
    await waitForList();

    const button = screen.getByRole('button', { name: /사용자 등록/ });
    // FlowListPage 의 생성 버튼과 동일한 형상(px-4 py-2 + 다크 변형).
    for (const cls of ['px-4', 'py-2', 'bg-blue-600', 'dark:bg-blue-500', 'transition-colors']) {
      expect(button.className).toContain(cls);
    }
  });
});

describe('UsersPanel 자기 자신 행의 삭제 컨트롤', () => {
  it('로그인한 계정 행에는 삭제 컨트롤이 없고, 다른 계정 행에는 있다', async () => {
    signInAs('admin');
    renderPage();
    await waitForList();

    const own = screen.getByTestId('admin-user-row-admin');
    const other = screen.getByTestId('admin-user-row-alice');

    // 비활성이 아니라 아예 존재하지 않는다 — 권한을 더 받아도 성공할 수 없는
    // 동작이라 요청할 여지를 남기지 않는다(서버도 409 로 항상 거부한다).
    expect(within(own).queryByRole('button', { name: /삭제 admin/ })).not.toBeInTheDocument();
    expect(within(other).getByRole('button', { name: /삭제 alice/ })).toBeInTheDocument();

    // 비밀번호 재설정은 자기 자신 행에도 남는다 — 성공 가능한 동작이다.
    expect(
      within(own).getByRole('button', { name: /비밀번호 재설정/ }),
    ).toBeInTheDocument();
  });

  it('로그인 계정이 바뀌면 삭제 컨트롤이 사라지는 행도 따라 바뀐다', async () => {
    signInAs('alice');
    renderPage();
    await waitForList();

    expect(
      within(screen.getByTestId('admin-user-row-alice')).queryByRole('button', {
        name: /삭제 alice/,
      }),
    ).not.toBeInTheDocument();
    expect(
      within(screen.getByTestId('admin-user-row-admin')).getByRole('button', {
        name: /삭제 admin/,
      }),
    ).toBeInTheDocument();
  });

  it('로그인 계정 정보가 없으면 어떤 행도 삭제 컨트롤을 잃지 않는다', async () => {
    renderPage();
    await waitForList();

    expect(
      within(screen.getByTestId('admin-user-row-admin')).getByRole('button', {
        name: /삭제 admin/,
      }),
    ).toBeInTheDocument();
  });
});
