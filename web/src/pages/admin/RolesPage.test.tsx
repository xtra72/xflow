// 역할 관리 화면 테스트 (SPEC-AUTH-006 M2.3 — AC-04).
//
// 서비스 계층만 스텁하고 훅·페이지·행렬 변환은 실제 코드를 돌린다. i18n 도 실제
// Provider 를 사용하므로 누락된 키는 렌더 문자열에서 즉시 드러난다.
//
// @spec SPEC-AUTH-006 v0.1.0 (M2.3)

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import { APIError } from '@/types/api';
import type { RoleResponse } from '@/services/api/userService';
import RolesPage from './RolesPage';

// ---- 서비스 스텁 ----

const getRoles = vi.hoisted(() => vi.fn());
const createRole = vi.hoisted(() => vi.fn());
const updateRole = vi.hoisted(() => vi.fn());
const deleteRole = vi.hoisted(() => vi.fn());
const getPermissionCatalog = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/userService', async () => {
  const actual = await vi.importActual<typeof import('@/services/api/userService')>(
    '@/services/api/userService',
  );
  return { ...actual, getRoles, createRole, updateRole, deleteRole, getPermissionCatalog };
});

// ---- 권한 스텁 ----

const granted = vi.hoisted(() => ({ keys: new Set<string>() }));

vi.mock('@/hooks/usePermission', () => ({
  usePermission: () => ({
    hasPermission: (key: string) => granted.keys.has(key),
    hasAnyPermission: (keys: readonly string[]) => keys.some((k) => granted.keys.has(k)),
    isPermissionUnavailable: false,
  }),
}));

function grant(...keys: string[]): void {
  granted.keys = new Set(keys);
}

// ---- 픽스처 ----

/**
 * 서버 카탈로그(internal/rbac/catalog.go)의 축소판.
 * agent 는 5개 액션 전부, dashboard 는 read/update, node 는 read 만 갖는다.
 */
const CATALOG = [
  'agent.create',
  'agent.delete',
  'agent.execute',
  'agent.read',
  'agent.update',
  'dashboard.read',
  'dashboard.update',
  'node.read',
];

const ROLES: RoleResponse[] = [
  {
    name: 'admin',
    description: '전체 권한',
    builtin: true,
    permissions: CATALOG,
    created_at: 1_700_000_000_000,
    updated_at: 1_700_000_000_000,
  },
  {
    name: 'editor',
    description: '조회 + 편집',
    builtin: true,
    permissions: ['agent.read', 'agent.update'],
    created_at: 1_700_000_000_000,
    updated_at: 1_700_000_000_000,
  },
  {
    name: 'viewer',
    description: '조회 전용',
    builtin: true,
    permissions: ['agent.read'],
    created_at: 1_700_000_000_000,
    updated_at: 1_700_000_000_000,
  },
  {
    name: 'operator',
    description: '커스텀 역할',
    builtin: false,
    permissions: ['agent.read', 'agent.execute'],
    created_at: 1_700_000_000_000,
    updated_at: 1_700_000_000_000,
  },
];

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <I18nProvider>
      <QueryClientProvider client={client}>
        <RolesPage />
      </QueryClientProvider>
    </I18nProvider>,
  );
}

async function waitForList(): Promise<void> {
  await screen.findByTestId('admin-role-row-operator');
}

/** 생성 폼을 연다. 행렬이 렌더될 때까지 기다린다. */
async function openCreateForm(): Promise<HTMLElement> {
  fireEvent.click(screen.getByRole('button', { name: /역할 생성/ }));
  await screen.findByTestId('permission-matrix');
  return screen.getByTestId('admin-roles-editor');
}

beforeEach(() => {
  vi.clearAllMocks();
  grant('role.read', 'role.create', 'role.update', 'role.delete');
  getRoles.mockResolvedValue(ROLES);
  getPermissionCatalog.mockResolvedValue(CATALOG);
});

describe('RolesPage 목록 (AC-04)', () => {
  it('이름·설명·빌트인 여부·권한 개수를 표시한다', async () => {
    renderPage();
    await waitForList();

    const row = screen.getByTestId('admin-role-row-operator');
    expect(within(row).getByText('operator')).toBeInTheDocument();
    expect(within(row).getByText('커스텀 역할')).toBeInTheDocument();
    // 커스텀 역할은 빌트인 배지가 없다.
    expect(within(row).queryByText('빌트인')).not.toBeInTheDocument();
    // 권한 개수 = permissions 배열 길이.
    expect(within(row).getByText('2')).toBeInTheDocument();

    const builtin = screen.getByTestId('admin-role-row-admin');
    expect(within(builtin).getByText('빌트인')).toBeInTheDocument();
    expect(within(builtin).getByText(String(CATALOG.length))).toBeInTheDocument();
  });

  it('역할이 없으면 빈 목록 안내를 표시한다', async () => {
    getRoles.mockResolvedValue([]);
    renderPage();

    expect(await screen.findByText('등록된 역할이 없습니다.')).toBeInTheDocument();
  });

  it('매우 긴 역할 이름도 말줄임 클래스로 레이아웃을 유지한다', async () => {
    getRoles.mockResolvedValue([{ ...ROLES[3], name: 'a'.repeat(32) }]);
    renderPage();

    const row = await screen.findByTestId(`admin-role-row-${'a'.repeat(32)}`);
    expect(within(row).getByText('a'.repeat(32)).className).toContain('truncate');
  });
});

// 빌트인 보호 범위는 서버 계약(internal/api/handler/role.go)을 그대로 비춘다.
// SPEC-AUTH-006 acceptance.md AC-04 의 문구는 "빌트인 역할 행은 수정·삭제 컨트롤이
// 비활성" 이지만, SPEC-AUTH-005 §2.4(UB1)가 실제로 금지하는 것은 (3) 빌트인 역할의
// 삭제와 (4) admin 역할의 권한 수정뿐이고 서버도 그렇게 구현되어 있다. AC-04 문구를
// 문자 그대로 따르면 신규 설치처럼 커스텀 역할이 없는 환경에서 편집 가능한 역할이
// 하나도 없어져 역할 관리 화면이 사실상 동작하지 않는다. 서버 계약을 따른다.
describe('RolesPage 빌트인 역할 보호 (AC-04, 서버 계약 기준)', () => {
  it('admin 행은 수정·삭제가 모두 비활성이고 사유가 표시된다', async () => {
    renderPage();
    await waitForList();

    const row = screen.getByTestId('admin-role-row-admin');
    const edit = within(row).getByRole('button', { name: '수정 admin' });
    const remove = within(row).getByRole('button', { name: '삭제 admin' });

    expect(edit).toBeDisabled();
    expect(edit).toHaveAttribute('aria-disabled', 'true');
    expect(edit).toHaveAttribute(
      'title',
      'admin 역할은 항상 전체 권한을 보유하므로 수정할 수 없습니다.',
    );
    expect(remove).toBeDisabled();
    expect(
      within(row).getByText(
        'admin 역할은 항상 전체 권한을 보유하므로 수정할 수 없습니다.',
      ),
    ).toBeInTheDocument();
  });

  it.each(['editor', 'viewer'])(
    '%s 행은 권한 수정이 가능하고 삭제만 비활성이다',
    async (name) => {
      renderPage();
      await waitForList();

      const row = screen.getByTestId(`admin-role-row-${name}`);
      // 서버는 editor/viewer 의 권한 수정을 허용한다.
      expect(within(row).getByRole('button', { name: `수정 ${name}` })).toBeEnabled();

      const remove = within(row).getByRole('button', { name: `삭제 ${name}` });
      expect(remove).toBeDisabled();
      expect(remove).toHaveAttribute('aria-disabled', 'true');
      expect(remove).toHaveAttribute('title', '빌트인 역할은 삭제할 수 없습니다.');
      expect(
        within(row).getByText('빌트인 역할은 삭제할 수 없습니다.'),
      ).toBeInTheDocument();
    },
  );

  it('빌트인 역할을 편집하면 이름 입력이 잠긴다 — 서버가 이름 변경을 거부한다', async () => {
    renderPage();
    await waitForList();

    const row = screen.getByTestId('admin-role-row-editor');
    fireEvent.click(within(row).getByRole('button', { name: '수정 editor' }));

    const nameInput = await screen.findByDisplayValue('editor');
    expect(nameInput).toHaveAttribute('readonly');
    expect(
      screen.getByText(
        '빌트인 역할의 이름은 변경할 수 없습니다. 권한만 수정할 수 있습니다.',
      ),
    ).toBeInTheDocument();
  });

  it('커스텀 역할은 수정·삭제가 활성이다', async () => {
    renderPage();
    await waitForList();

    const row = screen.getByTestId('admin-role-row-operator');
    expect(within(row).getByRole('button', { name: '수정 operator' })).toBeEnabled();
    expect(within(row).getByRole('button', { name: '삭제 operator' })).toBeEnabled();
  });
});

describe('RolesPage 권한 행렬 (AC-04)', () => {
  it('리소스×액션 축을 카탈로그에서 파생한다', async () => {
    renderPage();
    await waitForList();
    const form = await openCreateForm();

    const headers = within(form)
      .getAllByRole('columnheader')
      .map((h) => h.textContent);
    expect(headers).toEqual(['리소스', 'read', 'create', 'update', 'delete', 'execute']);

    expect(screen.getByTestId('matrix-row-agent')).toBeInTheDocument();
    expect(screen.getByTestId('matrix-row-dashboard')).toBeInTheDocument();
    expect(screen.getByTestId('matrix-row-node')).toBeInTheDocument();
  });

  it('해당 리소스에 없는 액션 셀은 비운다 — 비활성 체크박스를 그리지 않는다', async () => {
    renderPage();
    await waitForList();
    await openCreateForm();

    // node 는 read 만 있다.
    expect(
      within(screen.getByTestId('matrix-cell-node-read')).getByRole('checkbox'),
    ).toBeInTheDocument();
    for (const action of ['create', 'update', 'delete', 'execute']) {
      const cell = screen.getByTestId(`matrix-cell-node-${action}`);
      expect(cell).toBeEmptyDOMElement();
      expect(within(cell).queryByRole('checkbox')).not.toBeInTheDocument();
    }

    // dashboard 는 read/update 만 있다.
    expect(
      within(screen.getByTestId('matrix-cell-dashboard-update')).getByRole('checkbox'),
    ).toBeInTheDocument();
    expect(screen.getByTestId('matrix-cell-dashboard-delete')).toBeEmptyDOMElement();
  });

  it('카탈로그에 있는 조합만 체크박스가 되고 전체 개수가 카탈로그와 일치한다', async () => {
    renderPage();
    await waitForList();
    const form = await openCreateForm();

    expect(within(form).getAllByRole('checkbox')).toHaveLength(CATALOG.length);
  });

  it('카탈로그를 불러오는 동안 로딩 표시를 보여준다', async () => {
    // 끝나지 않는 프라미스로 로딩 상태를 고정한다.
    getPermissionCatalog.mockReturnValue(new Promise(() => {}));
    renderPage();
    await waitForList();

    fireEvent.click(screen.getByRole('button', { name: /역할 생성/ }));

    const form = screen.getByTestId('admin-roles-editor');
    expect(within(form).getByText('로딩 중...')).toBeInTheDocument();
    expect(screen.queryByTestId('permission-matrix')).not.toBeInTheDocument();
  });

  it('카탈로그 조회 실패 시 행렬 대신 안내를 표시한다 (크래시 없음)', async () => {
    getPermissionCatalog.mockRejectedValue(new APIError('INTERNAL', 'boom', 500));
    renderPage();
    await waitForList();

    fireEvent.click(screen.getByRole('button', { name: /역할 생성/ }));

    expect(
      await screen.findByText('권한 카탈로그를 불러오지 못했습니다.'),
    ).toBeInTheDocument();
    expect(screen.queryByTestId('permission-matrix')).not.toBeInTheDocument();
  });
});

describe('RolesPage 생성 (AC-04)', () => {
  it('체크한 권한만 생성 요청에 담는다', async () => {
    createRole.mockResolvedValue(ROLES[3]);
    renderPage();
    await waitForList();
    const form = await openCreateForm();

    fireEvent.change(within(form).getAllByRole('textbox')[0]!, {
      target: { value: 'auditor' },
    });
    fireEvent.change(within(form).getAllByRole('textbox')[1]!, {
      target: { value: '감사 전용' },
    });
    fireEvent.click(screen.getByLabelText('agent.read'));
    fireEvent.click(screen.getByLabelText('node.read'));
    fireEvent.click(within(form).getByRole('button', { name: '저장' }));

    await waitFor(() => expect(createRole).toHaveBeenCalledTimes(1));
    const req = createRole.mock.calls[0]![0];
    expect(req.name).toBe('auditor');
    expect(req.description).toBe('감사 전용');
    expect([...req.permissions].sort()).toEqual(['agent.read', 'node.read']);
  });

  it('체크를 해제한 권한은 요청에서 빠진다', async () => {
    createRole.mockResolvedValue(ROLES[3]);
    renderPage();
    await waitForList();
    const form = await openCreateForm();

    fireEvent.change(within(form).getAllByRole('textbox')[0]!, {
      target: { value: 'auditor' },
    });
    fireEvent.click(screen.getByLabelText('agent.read'));
    fireEvent.click(screen.getByLabelText('agent.update'));
    fireEvent.click(screen.getByLabelText('agent.read')); // 해제
    fireEvent.click(within(form).getByRole('button', { name: '저장' }));

    await waitFor(() => expect(createRole).toHaveBeenCalledTimes(1));
    expect(createRole.mock.calls[0]![0].permissions).toEqual(['agent.update']);
  });

  it('409 ROLE_EXISTS 거부 시 원인을 안내하고 목록은 변하지 않는다', async () => {
    createRole.mockRejectedValue(new APIError('ROLE_EXISTS', '중복', 409));
    renderPage();
    await waitForList();
    const form = await openCreateForm();

    const before = screen.getAllByTestId(/^admin-role-row-/).length;
    fireEvent.change(within(form).getAllByRole('textbox')[0]!, {
      target: { value: 'operator' },
    });
    fireEvent.click(within(form).getByRole('button', { name: '저장' }));

    expect(await screen.findByTestId('admin-action-error')).toHaveTextContent(
      '이미 존재하는 역할 이름입니다.',
    );
    expect(screen.getAllByTestId(/^admin-role-row-/)).toHaveLength(before);
    expect(getRoles).toHaveBeenCalledTimes(1);
  });
});

describe('RolesPage 수정 (AC-04)', () => {
  it('기존 권한을 체크 상태로 불러온다', async () => {
    renderPage();
    await waitForList();

    fireEvent.click(screen.getByRole('button', { name: '수정 operator' }));
    await screen.findByTestId('permission-matrix');

    expect(screen.getByLabelText('agent.read')).toBeChecked();
    expect(screen.getByLabelText('agent.execute')).toBeChecked();
    expect(screen.getByLabelText('agent.delete')).not.toBeChecked();
  });

  it('이름을 바꾸지 않으면 name 을 요청에서 생략한다 (서버가 값을 보존한다)', async () => {
    updateRole.mockResolvedValue(ROLES[3]);
    renderPage();
    await waitForList();

    fireEvent.click(screen.getByRole('button', { name: '수정 operator' }));
    const form = await screen.findByTestId('admin-roles-editor');
    fireEvent.click(screen.getByLabelText('agent.update'));
    fireEvent.click(within(form).getByRole('button', { name: '저장' }));

    await waitFor(() => expect(updateRole).toHaveBeenCalledTimes(1));
    const [name, req] = updateRole.mock.calls[0]!;
    expect(name).toBe('operator');
    expect(req.name).toBeUndefined();
    expect([...req.permissions].sort()).toEqual([
      'agent.execute',
      'agent.read',
      'agent.update',
    ]);
  });

  it('이름을 바꾸면 rename 을 요청에 담는다', async () => {
    updateRole.mockResolvedValue(ROLES[3]);
    renderPage();
    await waitForList();

    fireEvent.click(screen.getByRole('button', { name: '수정 operator' }));
    const form = await screen.findByTestId('admin-roles-editor');
    fireEvent.change(within(form).getAllByRole('textbox')[0]!, {
      target: { value: 'operator-2' },
    });
    fireEvent.click(within(form).getByRole('button', { name: '저장' }));

    await waitFor(() => expect(updateRole).toHaveBeenCalledTimes(1));
    expect(updateRole.mock.calls[0]![1].name).toBe('operator-2');
  });

  it('설명은 수정 모드에서 비활성이고 사유를 표시한다 (서버가 변경을 지원하지 않는다)', async () => {
    renderPage();
    await waitForList();

    fireEvent.click(screen.getByRole('button', { name: '수정 operator' }));
    const form = await screen.findByTestId('admin-roles-editor');
    const description = within(form).getAllByRole('textbox')[1]!;

    expect(description).toBeDisabled();
    expect(description).toHaveAttribute('title', '설명은 생성 후 변경할 수 없습니다.');
  });
});

describe('RolesPage 삭제 (AC-04)', () => {
  it('409 ROLE_IN_USE 거부 시 원인을 안내하고 목록은 변하지 않는다', async () => {
    deleteRole.mockRejectedValue(new APIError('ROLE_IN_USE', '사용 중', 409));
    renderPage();
    await waitForList();

    const before = screen.getAllByTestId(/^admin-role-row-/).length;
    fireEvent.click(screen.getByRole('button', { name: '삭제 operator' }));
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: '삭제' }),
    );

    expect(await screen.findByTestId('admin-action-error')).toHaveTextContent(
      '이 역할을 사용 중인 사용자가 있어 삭제할 수 없습니다.',
    );
    expect(screen.getAllByTestId(/^admin-role-row-/)).toHaveLength(before);
    expect(screen.getByTestId('admin-role-row-operator')).toBeInTheDocument();
  });

  it('삭제에 성공하면 목록을 다시 읽는다', async () => {
    deleteRole.mockResolvedValue(undefined);
    renderPage();
    await waitForList();

    fireEvent.click(screen.getByRole('button', { name: '삭제 operator' }));
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: '삭제' }),
    );

    await waitFor(() => expect(deleteRole).toHaveBeenCalledWith('operator'));
    await waitFor(() => expect(getRoles).toHaveBeenCalledTimes(2));
  });

  it('취소하면 삭제를 요청하지 않는다', async () => {
    renderPage();
    await waitForList();

    fireEvent.click(screen.getByRole('button', { name: '삭제 operator' }));
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: '취소' }),
    );

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(deleteRole).not.toHaveBeenCalled();
  });
});

describe('RolesPage 권한 게이팅', () => {
  it('role.create 가 없으면 생성 버튼을 비활성하고 사유를 노출한다', async () => {
    grant('role.read');
    renderPage();
    await waitForList();

    const button = screen.getByRole('button', { name: /역할 생성/ });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('aria-disabled', 'true');
    expect(button).toHaveAttribute('title', '권한이 필요합니다');
  });

  it('role.update / role.delete 가 없으면 커스텀 역할도 수정·삭제가 비활성이다', async () => {
    grant('role.read');
    renderPage();
    await waitForList();

    const row = screen.getByTestId('admin-role-row-operator');
    for (const name of ['수정 operator', '삭제 operator']) {
      const button = within(row).getByRole('button', { name });
      expect(button).toBeDisabled();
      expect(button).toHaveAttribute('title', '권한이 필요합니다');
    }
  });
});

describe('RolesPage 조회 실패', () => {
  it('403 이면 권한 부족을 사용자 언어로 안내하고 재시도하지 않는다 (AC-08)', async () => {
    getRoles.mockRejectedValue(new APIError('FORBIDDEN', 'forbidden', 403));
    renderPage();

    expect(
      await screen.findByText('이 작업을 수행할 권한이 없습니다.'),
    ).toBeInTheDocument();
    await waitFor(() => expect(getRoles).toHaveBeenCalledTimes(1));
  });
});
