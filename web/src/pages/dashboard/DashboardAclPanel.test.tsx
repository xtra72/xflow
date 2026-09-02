// 대시보드 권한 부여 패널 + 검증기 테스트 (SPEC-DASHBOARD-004 M7 7.2).
//
// 파일 이름은 원래 `DashboardAdminPage.acl.test.tsx` 였다. 별도 관리 화면이
// 사라지고(M7 재작업) 이 패널이 편집 모드 셀렉터의 다이얼로그로 옮겨가면서,
// 실제 검증 대상인 `DashboardAclPanel` 이름으로 되돌렸다. 검증 내용은 그대로다 —
// 패널의 계약은 호출부가 바뀌어도 변하지 않는다.
//
// 두 축을 검증한다.
//   1) 검증기(dashboardAclValidation) — acceptance.md AC-17 의 9행이 각각 **서로
//      다른** 사유 키로 거부되는가. 서버 거부 사유와 1:1 이어야 사용자가 다음
//      행동을 정할 수 있다.
//   2) 패널 — 전량 치환(§4.5)으로 저장하는가, grant 없는 대시보드에서 열리지
//      않는가, 서버 400 을 일반 실패로 삼키지 않는가.
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.9 E3, §2.12 O1, §4.5, AC-17)

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import ko from '@/lib/i18n/ko.json';
import type { Dashboard, DashboardAclEntry } from '@/types/dashboard';
import { validateAclEntries } from './dashboardAclValidation';

// ---- 서비스 스텁 ----

const getAcl = vi.hoisted(() => vi.fn());
const putAcl = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/dashboardService', async () => {
  const actual = await vi.importActual<typeof import('@/services/api/dashboardService')>(
    '@/services/api/dashboardService',
  );
  return { ...actual, getAcl, putAcl };
});

const getUsers = vi.hoisted(() => vi.fn());
const getRoles = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/userService', async () => {
  const actual = await vi.importActual<typeof import('@/services/api/userService')>(
    '@/services/api/userService',
  );
  return { ...actual, getUsers, getRoles };
});

// ---- 권한 스텁 ----

const granted = vi.hoisted(() => ({ keys: new Set<string>() }));

vi.mock('@/hooks/usePermission', () => ({
  usePermission: () => ({
    hasPermission: (key: string) => granted.keys.has(key),
    hasAnyPermission: (keys: readonly string[]) => keys.some((k) => granted.keys.has(k)),
    canSeeMenu: (key: string) => granted.keys.has(key),
    isPermissionUnavailable: false,
  }),
}));

import {
  DashboardBadRequestError,
  DashboardForbiddenError,
} from '@/services/api/dashboardService';
import DashboardAclPanel from './DashboardAclPanel';

// ---------------------------------------------------------------------------
// (1) 검증기 — AC-17 9행 전수
// ---------------------------------------------------------------------------

const CTX = {
  owner: 'root',
  knownUsers: ['root', 'edi', 'vie'],
  knownRoles: ['admin', 'editor', 'viewer', 'operator'],
};

function entry(subject: string, level: string): DashboardAclEntry {
  return { subject, level: level as DashboardAclEntry['level'] };
}

describe('validateAclEntries — AC-17 거부 사유 (사유별로 서로 다른 키)', () => {
  it('유효한 user: / role: 항목은 통과한다', () => {
    expect(validateAclEntries([entry('user:edi', 'view')], CTX)).toBeNull();
    expect(validateAclEntries([entry('role:operator', 'edit')], CTX)).toBeNull();
  });

  const CASES: ReadonlyArray<[string, DashboardAclEntry[], string]> = [
    ['접두사 없음', [entry('edi', 'view')], 'dashboard.acl.error.prefixMissing'],
    [
      '미지원 접두사',
      [entry('group:dev', 'view')],
      'dashboard.acl.error.prefixUnsupported',
    ],
    [
      '존재하지 않는 사용자',
      [entry('user:nobody', 'view')],
      'dashboard.acl.error.userNotFound',
    ],
    [
      '존재하지 않는 역할',
      [entry('role:nosuchrole', 'view')],
      'dashboard.acl.error.roleNotFound',
    ],
    ['미지원 레벨', [entry('user:edi', 'manage')], 'dashboard.acl.error.levelInvalid'],
    ['소유자 자신', [entry('user:root', 'view')], 'dashboard.acl.error.owner'],
    [
      '중복 subject',
      [entry('user:edi', 'view'), entry('user:edi', 'edit')],
      'dashboard.acl.error.duplicate',
    ],
    ['빈 이름', [entry('user:', 'view')], 'dashboard.acl.error.nameEmpty'],
  ];

  it.each(CASES)('%s → %s', (_label, entries, expectedKey) => {
    expect(validateAclEntries(entries, CTX)?.messageKey).toBe(expectedKey);
  });

  it('9행의 사유 키가 서로 중복되지 않는다', () => {
    const keys = CASES.map(([, entries]) => validateAclEntries(entries, CTX)?.messageKey);
    expect(new Set(keys).size).toBe(CASES.length);
  });

  it('사유 키마다 ko 로케일 문구가 존재하고 서로 다르다', () => {
    const messages = CASES.map(([, , key]) => {
      const value = key
        .split('.')
        .reduce<unknown>(
          (o, p) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined),
          ko as unknown,
        );
      expect(typeof value).toBe('string');
      return value as string;
    });
    expect(new Set(messages).size).toBe(CASES.length);
  });

  it('사용자·역할 목록을 읽지 못하면 실재 검증을 건너뛴다 (서버가 판정한다)', () => {
    expect(
      validateAclEntries([entry('user:nobody', 'view')], { owner: 'root' }),
    ).toBeNull();
    expect(
      validateAclEntries([entry('role:nosuchrole', 'view')], { owner: 'root' }),
    ).toBeNull();
  });

  it('인증 비활성(owner 빈 문자열)이면 소유자 검사를 건너뛴다', () => {
    expect(
      validateAclEntries([entry('user:root', 'view')], { ...CTX, owner: '' }),
    ).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// (2) 패널
// ---------------------------------------------------------------------------

function makeDashboard(overrides: Partial<Dashboard> = {}): Dashboard {
  return {
    uid: 'd1',
    name: '운영 대시보드',
    owner: 'root',
    visibility: 'acl',
    is_default: false,
    sort_order: 0,
    version: 3,
    created_at: 0,
    updated_at: 0,
    can_edit: true,
    can_delete: true,
    can_grant: true,
    ...overrides,
  };
}

function renderPanel(dashboard = makeDashboard(), onClose = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const result = render(
    <I18nProvider>
      <QueryClientProvider client={client}>
        <DashboardAclPanel dashboard={dashboard} onClose={onClose} />
      </QueryClientProvider>
    </I18nProvider>,
  );
  return { ...result, onClose };
}

const alertText = () => screen.getByTestId('dashboard-acl-error').textContent ?? '';

beforeEach(() => {
  vi.clearAllMocks();
  granted.keys = new Set(['dashboard.read', 'dashboard.update', 'user.read', 'role.read']);
  getAcl.mockResolvedValue([]);
  putAcl.mockImplementation(async (_uid: string, entries: DashboardAclEntry[]) => entries);
  getUsers.mockResolvedValue([
    { username: 'root', role: 'admin', created_at: 0, updated_at: 0 },
    { username: 'edi', role: 'editor', created_at: 0, updated_at: 0 },
    { username: 'vie', role: 'viewer', created_at: 0, updated_at: 0 },
    { username: 'vie2', role: 'viewer', created_at: 0, updated_at: 0 },
  ]);
  getRoles.mockResolvedValue([
    { name: 'admin', description: '', builtin: true, permissions: [], created_at: 0, updated_at: 0 },
    { name: 'editor', description: '', builtin: true, permissions: [], created_at: 0, updated_at: 0 },
    { name: 'viewer', description: '', builtin: true, permissions: [], created_at: 0, updated_at: 0 },
  ]);
});

describe('DashboardAclPanel — 로드', () => {
  it('서버 ACL 을 편집 행으로 펼친다 (user:/role: 접두사 분리)', async () => {
    getAcl.mockResolvedValue([
      { subject: 'user:edi', level: 'edit', granted_by: 'root', granted_at: 1 },
      { subject: 'role:viewer', level: 'view', granted_by: 'root', granted_at: 1 },
    ]);
    renderPanel();

    await waitFor(() => expect(screen.getByTestId('dashboard-acl-row-0')).toBeInTheDocument());
    expect(screen.getByLabelText('종류 1')).toHaveValue('user');
    expect(screen.getByLabelText('대상 1')).toHaveValue('edi');
    expect(screen.getByLabelText('권한 1')).toHaveValue('edit');
    expect(screen.getByLabelText('종류 2')).toHaveValue('role');
    expect(screen.getByLabelText('대상 2')).toHaveValue('viewer');
  });

  it('403 이면 권한 부족 사유를 표시한다', async () => {
    getAcl.mockRejectedValue(new DashboardForbiddenError());
    renderPanel();
    await waitFor(() =>
      expect(alertText()).toContain('이 대시보드의 권한을 변경할 권한이 없습니다'),
    );
  });

  it('공개범위가 acl 이 아니면 저장이 지금은 무효라고 안내한다 (spec.md §2.9)', async () => {
    renderPanel(makeDashboard({ visibility: 'private' }));
    await waitFor(() =>
      expect(screen.getByTestId('dashboard-acl-inactive')).toBeInTheDocument(),
    );
  });

  it('공개범위가 acl 이면 그 안내를 표시하지 않는다', async () => {
    renderPanel(makeDashboard({ visibility: 'acl' }));
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '대상 추가' })).toBeEnabled(),
    );
    expect(screen.queryByTestId('dashboard-acl-inactive')).not.toBeInTheDocument();
  });
});

describe('DashboardAclPanel — 전량 치환 저장 (spec.md §4.5)', () => {
  it('편집 중인 목록 전체를 한 번의 PUT 으로 보낸다', async () => {
    getAcl.mockResolvedValue([{ subject: 'user:edi', level: 'view', granted_by: 'root' }]);
    const { onClose } = renderPanel();
    await waitFor(() => expect(screen.getByLabelText('대상 1')).toHaveValue('edi'));

    fireEvent.click(screen.getByRole('button', { name: '대상 추가' }));
    fireEvent.change(screen.getByLabelText('종류 2'), { target: { value: 'role' } });
    fireEvent.change(screen.getByLabelText('대상 2'), { target: { value: 'viewer' } });
    fireEvent.change(screen.getByLabelText('권한 2'), { target: { value: 'edit' } });
    fireEvent.click(screen.getByRole('button', { name: '저장' }));

    await waitFor(() =>
      expect(putAcl).toHaveBeenCalledWith('d1', [
        { subject: 'user:edi', level: 'view' },
        { subject: 'role:viewer', level: 'edit' },
      ]),
    );
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it('행을 지우고 저장하면 그 행이 빠진 **전체 목록**이 전송된다', async () => {
    getAcl.mockResolvedValue([
      { subject: 'user:edi', level: 'view', granted_by: 'root' },
      { subject: 'user:vie', level: 'edit', granted_by: 'root' },
    ]);
    renderPanel();
    await waitFor(() => expect(screen.getByLabelText('대상 2')).toHaveValue('vie'));

    fireEvent.click(screen.getByRole('button', { name: '대상 제거 1' }));
    fireEvent.click(screen.getByRole('button', { name: '저장' }));

    // 부분 갱신(DELETE 1건)이 아니라 남은 전체가 치환된다.
    await waitFor(() =>
      expect(putAcl).toHaveBeenCalledWith('d1', [{ subject: 'user:vie', level: 'edit' }]),
    );
  });

  it('빈 목록도 그대로 치환한다 (권한 전부 회수)', async () => {
    getAcl.mockResolvedValue([{ subject: 'user:edi', level: 'view', granted_by: 'root' }]);
    renderPanel();
    await waitFor(() => expect(screen.getByLabelText('대상 1')).toHaveValue('edi'));

    fireEvent.click(screen.getByRole('button', { name: '대상 제거 1' }));
    fireEvent.click(screen.getByRole('button', { name: '저장' }));

    await waitFor(() => expect(putAcl).toHaveBeenCalledWith('d1', []));
  });
});

describe('DashboardAclPanel — 거부 사유 구분 (AC-17)', () => {
  /** 행 1개를 채운 뒤 저장을 눌러 안내 문구를 얻는다. */
  async function saveWith(rows: Array<{ kind: string; name: string; level?: string }>) {
    renderPanel();
    await waitFor(() => expect(screen.getByRole('button', { name: '대상 추가' })).toBeEnabled());
    for (const [i, row] of rows.entries()) {
      fireEvent.click(screen.getByRole('button', { name: '대상 추가' }));
      fireEvent.change(screen.getByLabelText(`종류 ${i + 1}`), {
        target: { value: row.kind },
      });
      fireEvent.change(screen.getByLabelText(`대상 ${i + 1}`), {
        target: { value: row.name },
      });
      if (row.level) {
        fireEvent.change(screen.getByLabelText(`권한 ${i + 1}`), {
          target: { value: row.level },
        });
      }
    }
    fireEvent.click(screen.getByRole('button', { name: '저장' }));
    await waitFor(() => expect(screen.getByTestId('dashboard-acl-error')).toBeInTheDocument());
    return alertText();
  }

  it('소유자 등재는 소유권 우선 사유로 거부한다', async () => {
    const text = await saveWith([{ kind: 'user', name: 'root' }]);
    expect(text).toContain('소유자는 권한 목록에 넣을 수 없습니다');
    expect(putAcl).not.toHaveBeenCalled();
  });

  it('중복 대상은 중복 사유로 거부한다', async () => {
    const text = await saveWith([
      { kind: 'user', name: 'edi' },
      { kind: 'user', name: 'edi', level: 'edit' },
    ]);
    expect(text).toContain('중복된 대상입니다');
    expect(putAcl).not.toHaveBeenCalled();
  });

  it('존재하지 않는 사용자는 사용자 사유로 거부한다', async () => {
    const text = await saveWith([{ kind: 'user', name: 'nobody' }]);
    expect(text).toContain('존재하지 않는 사용자입니다');
    expect(putAcl).not.toHaveBeenCalled();
  });

  it('존재하지 않는 역할은 역할 사유로 거부한다', async () => {
    const text = await saveWith([{ kind: 'role', name: 'nosuchrole' }]);
    expect(text).toContain('존재하지 않는 역할입니다');
    expect(putAcl).not.toHaveBeenCalled();
  });

  it('네 사유의 문구가 서로 다르다', async () => {
    const texts: string[] = [];
    for (const rows of [
      [{ kind: 'user', name: 'root' }],
      [
        { kind: 'user', name: 'edi' },
        { kind: 'user', name: 'edi', level: 'edit' },
      ],
      [{ kind: 'user', name: 'nobody' }],
      [{ kind: 'role', name: 'nosuchrole' }],
    ]) {
      texts.push(await saveWith(rows));
      // 반복마다 완전히 언마운트한다 — 남은 패널이 있으면 다음 회차의 라벨 조회가
      // 중복으로 걸려 조회가 끝나지 않는다.
      cleanup();
    }
    expect(new Set(texts).size).toBe(4);
  });

  it('서버 400 은 일반 실패로 삼키지 않고 서버 사유를 그대로 노출한다', async () => {
    // 목록을 못 읽어 클라이언트가 실재를 확인할 수 없는 경우 — 서버가 최종 판정한다.
    granted.keys = new Set(['dashboard.read', 'dashboard.update']);
    putAcl.mockRejectedValue(
      new DashboardBadRequestError('존재하지 않는 사용자입니다: ghost'),
    );
    renderPanel();
    await waitFor(() => expect(screen.getByRole('button', { name: '대상 추가' })).toBeEnabled());

    fireEvent.click(screen.getByRole('button', { name: '대상 추가' }));
    fireEvent.change(screen.getByLabelText('대상 1'), { target: { value: 'ghost' } });
    fireEvent.click(screen.getByRole('button', { name: '저장' }));

    await waitFor(() => expect(putAcl).toHaveBeenCalled());
    await waitFor(() => {
      expect(alertText()).toContain('서버가 권한 저장을 거부했습니다');
      expect(alertText()).toContain('존재하지 않는 사용자입니다: ghost');
    });
  });

  it('저장 403 은 권한 사유로 안내한다', async () => {
    putAcl.mockRejectedValue(new DashboardForbiddenError());
    renderPanel();
    await waitFor(() => expect(screen.getByRole('button', { name: '대상 추가' })).toBeEnabled());

    fireEvent.click(screen.getByRole('button', { name: '대상 추가' }));
    fireEvent.change(screen.getByLabelText('대상 1'), { target: { value: 'edi' } });
    fireEvent.click(screen.getByRole('button', { name: '저장' }));

    await waitFor(() =>
      expect(alertText()).toContain('이 대시보드의 권한을 변경할 권한이 없습니다'),
    );
  });
});

describe('DashboardAclPanel — 선택 기능 O1 (역할별 사용자 수)', () => {
  it('role 대상에 그 역할의 사용자 수를 함께 표시한다', async () => {
    getAcl.mockResolvedValue([
      { subject: 'role:viewer', level: 'view', granted_by: 'root' },
    ]);
    renderPanel();
    await waitFor(() => expect(screen.getByLabelText('대상 1')).toHaveValue('viewer'));
    expect(screen.getByText('사용자 2명')).toBeInTheDocument();
  });

  it('사용자 목록을 읽을 수 없으면 수를 표시하지 않고 직접 입력으로 낮춘다', async () => {
    granted.keys = new Set(['dashboard.read', 'dashboard.update']);
    getAcl.mockResolvedValue([
      { subject: 'role:viewer', level: 'view', granted_by: 'root' },
    ]);
    renderPanel();
    await waitFor(() => expect(screen.getByLabelText('대상 1')).toHaveValue('viewer'));
    expect(screen.queryByText(/사용자 \d+명/)).not.toBeInTheDocument();
    expect(
      screen.getByText(/사용자·역할 목록을 읽을 수 없어 이름을 직접 입력합니다/),
    ).toBeInTheDocument();
    expect(getUsers).not.toHaveBeenCalled();
  });
});
