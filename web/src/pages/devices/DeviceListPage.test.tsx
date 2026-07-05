// DeviceListPage 컬럼 구성 테스트.
//
// 데이터/타깃 훅과 useDeviceColumns 를 mock 하여 다음을 검증한다:
//   - 선택된 컬럼만 헤더에 렌더 (예: id/source 미선택 시 헤더에서 제외)
//   - 컬럼 설정 팝오버 토글 → 체크박스 변경 시 setColumns 호출 (서버 저장 경로)
//   - 최소 1개 강제 (단일 컬럼일 때 해당 체크박스 비활성)

import { render, screen, fireEvent, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { DeviceInfo } from '@/types/device';

// ---- 데이터/타깃 훅 mock ----
const useDevicesTargetMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useResourceTargets', () => ({
  useDevicesTarget: useDevicesTargetMock,
}));

vi.mock('@/hooks/useTargetParam', () => ({
  useTargetParam: () => ({ type: 'local' }),
}));

vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: () => ({ nodeLabel: '', nodeReady: true }),
}));

// 상세 패널은 본 테스트 범위 밖 — 가벼운 스텁으로 대체.
vi.mock('./DeviceDetailPanel', () => ({
  default: () => <div data-testid="detail-panel" />,
}));

// ---- useDeviceColumns mock ----
const setColumnsMock = vi.hoisted(() => vi.fn());
const useDeviceColumnsMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useDeviceColumns', async () => {
  // 실제 상수(ALL_DEVICE_COLUMNS, 라벨)는 그대로 사용하고 훅만 교체한다.
  const actual = await vi.importActual<typeof import('@/hooks/useDeviceColumns')>(
    '@/hooks/useDeviceColumns',
  );
  return {
    ...actual,
    useDeviceColumns: useDeviceColumnsMock,
  };
});

import DeviceListPage from './DeviceListPage';

function renderPage() {
  return render(
    <I18nProvider>
      <DeviceListPage />
    </I18nProvider>,
  );
}

function makeDevice(overrides: Partial<DeviceInfo> = {}): DeviceInfo {
  return {
    id: 'uuid-1',
    uid: 'uuid-1',
    name: '거실 에어컨',
    type: 'indoor',
    protocol: 'lgap',
    agent_name: 'agent-a',
    source: 'config',
    online: true,
    last_seen: new Date().toISOString(),
    capabilities: [],
    ...overrides,
  };
}

beforeEach(() => {
  setColumnsMock.mockReset();
  useDeviceColumnsMock.mockReset();
  useDevicesTargetMock.mockReset();
  useDevicesTargetMock.mockReturnValue({
    data: { data: [makeDevice()] },
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  });
});

describe('DeviceListPage 컬럼 구성', () => {
  it('선택된 컬럼만 테이블 헤더에 렌더한다', () => {
    useDeviceColumnsMock.mockReturnValue({
      columns: ['name', 'status'],
      setColumns: setColumnsMock,
      isLoading: false,
      isSaving: false,
    });

    renderPage();

    const headers = screen.getAllByRole('columnheader');
    const headerTexts = headers.map((h) => h.textContent ?? '');
    // 선택 컬럼은 헤더에 존재
    expect(headerTexts.some((t) => t.includes('이름'))).toBe(true);
    expect(headerTexts.some((t) => t.includes('상태'))).toBe(true);
    // 미선택 컬럼(ID, 등록, 에이전트)은 헤더에서 제외
    expect(headerTexts.some((t) => t === 'ID')).toBe(false);
    expect(headerTexts.some((t) => t.includes('에이전트'))).toBe(false);
  });

  it('컬럼 설정 팝오버에서 체크박스 토글 시 setColumns 를 호출한다', () => {
    useDeviceColumnsMock.mockReturnValue({
      columns: ['name', 'id', 'status'],
      setColumns: setColumnsMock,
      isLoading: false,
      isSaving: false,
    });

    renderPage();

    // 팝오버 열기
    fireEvent.click(screen.getByRole('button', { name: '컬럼 설정' }));
    const menu = screen.getByRole('menu');

    // '에이전트'(미선택) 체크 → 추가
    const agentCheckbox = within(menu)
      .getByText('에이전트')
      .closest('label')!
      .querySelector('input')!;
    fireEvent.click(agentCheckbox);

    expect(setColumnsMock).toHaveBeenCalledWith(['name', 'id', 'status', 'agent']);
  });

  it('표시 컬럼이 1개뿐이면 해당 체크박스를 해제할 수 없다(최소 1개 강제)', () => {
    useDeviceColumnsMock.mockReturnValue({
      columns: ['name'],
      setColumns: setColumnsMock,
      isLoading: false,
      isSaving: false,
    });

    renderPage();
    fireEvent.click(screen.getByRole('button', { name: '컬럼 설정' }));
    const menu = screen.getByRole('menu');

    const nameCheckbox = within(menu)
      .getByText('이름')
      .closest('label')!
      .querySelector('input')!;
    expect(nameCheckbox).toBeDisabled();

    // 클릭해도 setColumns 가 호출되지 않는다
    fireEvent.click(nameCheckbox);
    expect(setColumnsMock).not.toHaveBeenCalled();
  });
});
