// ReleaseStorePanel 테스트 (관리 서버 호스팅 릴리스 저장소).
import { fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { ReleaseRecord } from '@/types/remote';

// ---- 훅 mock (hoisted vi.fn 패턴) ----
const releasesData = vi.hoisted(() => ({ value: [] as ReleaseRecord[], isError: false }));
const createMutate = vi.hoisted(() => vi.fn());
const uploadMutate = vi.hoisted(() => vi.fn());
const deleteReleaseMutate = vi.hoisted(() => vi.fn());
const deleteAssetMutate = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useRemote', () => ({
  useReleases: () => ({ data: releasesData.value, isError: releasesData.isError }),
  useCreateRelease: () => ({ mutate: createMutate, isPending: false }),
  useUploadReleaseAsset: () => ({ mutate: uploadMutate, isPending: false }),
  useDeleteRelease: () => ({ mutate: deleteReleaseMutate, isPending: false }),
  useDeleteReleaseAsset: () => ({ mutate: deleteAssetMutate, isPending: false }),
}));

const addNotificationMock = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector?: (s: { addNotification: typeof addNotificationMock }) => unknown) => {
    const state = { addNotification: addNotificationMock };
    return selector ? selector(state) : state;
  },
}));

import { ReleaseStorePanel } from './ReleaseStorePanel';

function renderPanel(): void {
  render(
    <I18nProvider>
      <ReleaseStorePanel />
    </I18nProvider>,
  );
}

const RELEASE: ReleaseRecord = {
  version: 'v1.3.0',
  channel: 'stable',
  notes: '첫 릴리스',
  published_at: 1700000000000,
  assets: [
    {
      os: 'linux',
      arch: 'amd64',
      filename: 'xflowd-linux-amd64',
      size: 12_345_678,
      sha256: 'abcdef0123456789aaaa',
      has_sig: true,
      uploaded_at: 1700000000000,
    },
  ],
};

beforeEach(() => {
  releasesData.value = [];
  releasesData.isError = false;
  createMutate.mockReset();
  uploadMutate.mockReset();
  deleteReleaseMutate.mockReset();
  deleteAssetMutate.mockReset();
  addNotificationMock.mockReset();
});

describe('ReleaseStorePanel', () => {
  it('릴리스가 없으면 빈 안내를 표시한다', () => {
    renderPanel();
    expect(screen.getByTestId('release-store-empty')).toBeInTheDocument();
  });

  it('로드 에러 시 에러 안내를 표시한다', () => {
    releasesData.isError = true;
    renderPanel();
    expect(screen.getByTestId('release-store-error')).toBeInTheDocument();
  });

  it('버전 목록을 렌더한다(버전 + 채널)', () => {
    releasesData.value = [RELEASE];
    renderPanel();
    const card = screen.getByTestId('release-card');
    expect(card.getAttribute('data-version')).toBe('v1.3.0');
    expect(card).toHaveTextContent('v1.3.0');
    expect(card).toHaveTextContent('stable');
  });

  it('5개 표준 아키텍처 슬롯을 표시한다', () => {
    releasesData.value = [RELEASE];
    renderPanel();
    const slots = screen.getAllByTestId('release-arch-slot');
    expect(slots).toHaveLength(5);
    // linux/amd64 는 업로드됨, 나머지는 누락.
    const uploaded = slots.filter((s) => s.getAttribute('data-uploaded') === 'true');
    expect(uploaded).toHaveLength(1);
    expect(uploaded[0]!.getAttribute('data-slot')).toBe('linux/amd64');
  });

  it('유효한 semver 로 버전 생성 시 hook 을 호출한다', () => {
    renderPanel();
    fireEvent.change(screen.getByTestId('release-version-input'), {
      target: { value: 'v2.0.0' },
    });
    fireEvent.change(screen.getByTestId('release-channel-select'), {
      target: { value: 'beta' },
    });
    fireEvent.click(screen.getByTestId('release-create-button'));
    expect(createMutate).toHaveBeenCalledTimes(1);
    const [arg] = createMutate.mock.calls[0] as [{ version: string; channel?: string }];
    expect(arg.version).toBe('v2.0.0');
    expect(arg.channel).toBe('beta');
  });

  it('잘못된 버전은 생성하지 않고 오류를 알린다', () => {
    renderPanel();
    fireEvent.change(screen.getByTestId('release-version-input'), {
      target: { value: 'not-semver' },
    });
    fireEvent.click(screen.getByTestId('release-create-button'));
    expect(createMutate).not.toHaveBeenCalled();
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'error' }),
    );
  });

  it('누락 슬롯에 바이너리+서명 업로드 시 (version, os, arch, files) 로 hook 을 호출한다', () => {
    releasesData.value = [RELEASE];
    renderPanel();
    // linux/arm64 슬롯(누락)을 찾는다.
    const slot = screen
      .getAllByTestId('release-arch-slot')
      .find((s) => s.getAttribute('data-slot') === 'linux/arm64')!;
    const scoped = within(slot);
    const binary = new File(['bin'], 'xflowd');
    const signature = new File(['sig'], 'xflowd.sig');
    fireEvent.change(scoped.getByTestId('release-binary-input'), {
      target: { files: [binary] },
    });
    fireEvent.change(scoped.getByTestId('release-signature-input'), {
      target: { files: [signature] },
    });
    fireEvent.click(scoped.getByTestId('release-upload-button'));
    expect(uploadMutate).toHaveBeenCalledTimes(1);
    const [arg] = uploadMutate.mock.calls[0] as [
      { version: string; os: string; arch: string; binary: File; signature: File },
    ];
    expect(arg.version).toBe('v1.3.0');
    expect(arg.os).toBe('linux');
    expect(arg.arch).toBe('arm64');
    expect(arg.binary).toBe(binary);
    expect(arg.signature).toBe(signature);
  });

  it('파일이 불완전하면 업로드하지 않고 오류를 알린다', () => {
    releasesData.value = [RELEASE];
    renderPanel();
    const slot = screen
      .getAllByTestId('release-arch-slot')
      .find((s) => s.getAttribute('data-slot') === 'linux/arm')!;
    const scoped = within(slot);
    const binary = new File(['bin'], 'xflowd');
    fireEvent.change(scoped.getByTestId('release-binary-input'), {
      target: { files: [binary] },
    });
    // 서명 미선택 상태에서 업로드.
    fireEvent.click(scoped.getByTestId('release-upload-button'));
    expect(uploadMutate).not.toHaveBeenCalled();
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'error' }),
    );
  });

  it('버전 삭제는 ConfirmDialog 를 거쳐 deleteRelease 를 호출한다', () => {
    releasesData.value = [RELEASE];
    renderPanel();
    fireEvent.click(screen.getByTestId('release-delete-version'));
    // ConfirmDialog 가 열린다.
    expect(screen.getByTestId('confirm-dialog')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('confirm-dialog-confirm'));
    expect(deleteReleaseMutate).toHaveBeenCalledWith('v1.3.0', expect.anything());
  });

  it('자산 삭제는 ConfirmDialog 를 거쳐 deleteReleaseAsset 를 호출한다', () => {
    releasesData.value = [RELEASE];
    renderPanel();
    // 업로드된 슬롯(linux/amd64)에 자산 삭제 버튼이 있다.
    const slot = screen
      .getAllByTestId('release-arch-slot')
      .find((s) => s.getAttribute('data-slot') === 'linux/amd64')!;
    fireEvent.click(within(slot).getByTestId('release-delete-asset'));
    expect(screen.getByTestId('confirm-dialog')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('confirm-dialog-confirm'));
    expect(deleteAssetMutate).toHaveBeenCalledWith(
      { version: 'v1.3.0', os: 'linux', arch: 'amd64' },
      expect.anything(),
    );
  });

  it('표준 외 추가 자산을 별도로 표시한다', () => {
    releasesData.value = [
      {
        ...RELEASE,
        assets: [
          ...RELEASE.assets,
          {
            os: 'windows',
            arch: 'amd64',
            filename: 'xflowd.exe',
            size: 100,
            sha256: 'deadbeef',
            has_sig: false,
            uploaded_at: 1700000000000,
          },
        ],
      },
    ];
    renderPanel();
    const extra = screen
      .getAllByTestId('release-arch-slot')
      .find((s) => s.getAttribute('data-slot') === 'windows/amd64');
    expect(extra).toBeDefined();
  });
});
