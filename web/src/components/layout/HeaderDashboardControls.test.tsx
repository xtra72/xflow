// HeaderDashboardControls 테스트 (SPEC-DASHBOARD-004 M6 6.2 / 6.5).
//
// 검증:
//   - 셀렉터가 **목록 API 축**(`dashboards`)에서 렌더된다 — 본문 축
//     (`dashboardPages`)이 아니라. 두 축이 갈리는 상황을 만들어 구분한다.
//   - 이름 변경 / 기본 지정 / 삭제가 각각 can_edit / can_grant / can_delete 를
//     따르며 서로 독립이다(서버의 인가 분기와 1:1).
//   - 컨트롤은 숨기지 않고 비활성 + 사유 툴팁이다(SPEC-AUTH-006 §4.2).

import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Dashboard } from '@/types/dashboard';

vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const resolve = (key: string): string => {
    const v = key.split('.').reduce<unknown>(
      (o, p) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined),
      ko,
    );
    return typeof v === 'string' ? v : key;
  };
  return { useTranslation: () => ({ t: resolve, locale: 'ko' as const, setLocale: () => {} }) };
});

vi.mock('@/pages/dashboard/CreateDashboardDialog', () => ({ default: () => null }));

const authState = {
  authEnabled: true,
  permissions: new Set<string>(),
  permissionStatus: 'loaded' as const,
};
vi.mock('@/stores/authStore', () => ({
  useAuthStore: Object.assign(
    (selector?: (state: typeof authState) => unknown) =>
      selector ? selector(authState) : authState,
    { getState: () => authState },
  ),
}));

import { useUIStore } from '@/stores/uiStore';

import HeaderDashboardControls from './HeaderDashboardControls';

function makeDashboard(overrides: Partial<Dashboard> = {}): Dashboard {
  return {
    uid: 'd1',
    name: '운영 대시보드',
    owner: 'edi',
    visibility: 'shared',
    is_default: true,
    sort_order: 0,
    version: 1,
    created_at: 0,
    updated_at: 0,
    can_edit: true,
    can_delete: true,
    can_grant: true,
    ...overrides,
  };
}

function seed(list: Dashboard[], activeUid = list[0]?.uid ?? '') {
  useUIStore.getState().setDashboards(list);
  useUIStore.getState().setActiveDashboard(activeUid);
}

const renameButton = () => screen.getByRole('button', { name: '대시보드 이름 편집' });
const deleteButton = () => screen.getByRole('button', { name: '대시보드 삭제' });
const defaultButton = () => screen.getByRole('button', { name: '기본 대시보드로 설정' });
const addButton = () => screen.getByRole('button', { name: '대시보드 추가' });

beforeEach(() => {
  authState.permissions = new Set(['dashboard.read', 'dashboard.create']);
  seed([makeDashboard()]);
});

describe('HeaderDashboardControls — 목록 API 축 렌더 (6.5)', () => {
  it('셀렉터 항목이 목록 API 축에서 나온다', () => {
    seed([
      makeDashboard({ uid: 'srv-1', name: '운영', is_default: true }),
      makeDashboard({ uid: 'srv-2', name: '분석', is_default: false }),
    ]);
    render(<HeaderDashboardControls />);

    const select = screen.getByRole('combobox', { name: '대시보드 선택' });
    const options = Array.from(select.querySelectorAll('option'));
    expect(options.map((o) => o.value)).toEqual(['srv-1', 'srv-2']);
    // 기본 대시보드에는 별표가 붙는다(is_default 는 목록 API 필드).
    expect(options[0]).toHaveTextContent('★ 운영');
    expect(options[1]).toHaveTextContent('분석');
  });

  it('목록 축이 비면 셀렉터도 비어 있다(본문 축의 자리표시자를 쓰지 않는다)', () => {
    // setDashboards([]) 는 목록 축을 비우지만, 본문 축에는 부팅 전 자리표시자가
    // 남을 수 있다. 셀렉터는 목록 축만 본다.
    useUIStore.getState().setDashboards([]);
    render(<HeaderDashboardControls />);

    const select = screen.getByRole('combobox', { name: '대시보드 선택' });
    expect(select.querySelectorAll('option')).toHaveLength(0);
  });

  it('목록 API 가 준 이름을 표시한다', () => {
    seed([makeDashboard({ uid: 'u9', name: '서버가 준 이름' })]);
    render(<HeaderDashboardControls />);

    expect(screen.getByRole('combobox', { name: '대시보드 선택' })).toHaveTextContent(
      '서버가 준 이름',
    );
  });
});

describe('HeaderDashboardControls — 컨트롤별 플래그 독립 검증 (6.2)', () => {
  it('이름 변경은 can_edit 만 따른다', () => {
    // is_default=false 로 둔다 — 이미 기본인 대시보드는 "기본 지정" 이 판정과
    // 무관하게 비활성이라 플래그 독립성을 구분할 수 없다.
    seed([
      makeDashboard({ uid: 'd1', is_default: false, can_edit: false, can_grant: true, can_delete: true }),
      makeDashboard({ uid: 'd2', name: '두번째', is_default: false }),
    ]);
    render(<HeaderDashboardControls />);

    expect(renameButton()).toBeInTheDocument();
    expect(renameButton()).toBeDisabled();
    expect(renameButton()).toHaveAttribute('title', '이 대시보드를 편집할 권한이 없습니다');
    // 나머지 두 컨트롤은 영향받지 않는다.
    expect(defaultButton()).toBeEnabled();
    expect(deleteButton()).toBeEnabled();
  });

  it('기본 지정은 can_grant 만 따른다', () => {
    seed([
      makeDashboard({ uid: 'd1', is_default: false, can_edit: true, can_grant: false, can_delete: true }),
      makeDashboard({ uid: 'd2', name: '두번째', is_default: false }),
    ]);
    render(<HeaderDashboardControls />);

    expect(defaultButton()).toBeInTheDocument();
    expect(defaultButton()).toBeDisabled();
    expect(defaultButton()).toHaveAttribute(
      'title',
      '이 대시보드의 공개 설정을 변경할 권한이 없습니다',
    );
    expect(renameButton()).toBeEnabled();
    expect(deleteButton()).toBeEnabled();
  });

  it('삭제는 can_delete 만 따른다', () => {
    seed([
      makeDashboard({ uid: 'd1', is_default: false, can_edit: true, can_grant: true, can_delete: false }),
      makeDashboard({ uid: 'd2', name: '두번째', is_default: false }),
    ]);
    render(<HeaderDashboardControls />);

    expect(deleteButton()).toBeInTheDocument();
    expect(deleteButton()).toBeDisabled();
    expect(deleteButton()).toHaveAttribute('title', '이 대시보드를 삭제할 권한이 없습니다');
    expect(renameButton()).toBeEnabled();
    expect(defaultButton()).toBeEnabled();
  });

  it('세 플래그가 모두 true 면 세 컨트롤이 활성이다', () => {
    seed([
      makeDashboard({ uid: 'd1', is_default: false }),
      makeDashboard({ uid: 'd2', name: '두번째', is_default: false }),
    ]);
    render(<HeaderDashboardControls />);

    expect(renameButton()).toBeEnabled();
    expect(defaultButton()).toBeEnabled();
    expect(deleteButton()).toBeEnabled();
  });

  it('생성은 전역 dashboard.create 판정이다(대시보드 플래그와 무관)', () => {
    authState.permissions = new Set(['dashboard.read']);
    seed([makeDashboard({ can_edit: true, can_grant: true, can_delete: true })]);
    render(<HeaderDashboardControls />);

    expect(addButton()).toBeInTheDocument();
    expect(addButton()).toBeDisabled();
    expect(addButton()).toHaveAttribute('title', '대시보드를 생성할 권한이 없습니다');
  });
});
