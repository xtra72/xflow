// ReleaseStorePanel 은 관리 서버 호스팅 릴리스 저장소(프로그램 이미지) 관리 패널이다.
//
// 목적: 아키텍처별 `xflowd` 바이너리 + Ed25519 서명을 이 관리 서버에 저장한다.
// 업데이트 소스를 이 서버로 지정한 노드는 자기 런타임 아키텍처(GOOS/GOARCH)에 맞는
// 이미지를 자동 다운로드한다(RPi armv6/armv7 은 모두 arm 으로 보고).
//
// 구성:
//   - 헤더 + 설명.
//   - "새 버전" 폼: 버전(semver) + 채널 + 노트 → createRelease.
//   - 버전 목록: 각 버전마다 5슬롯 아키텍처 매트릭스(업로드됨/누락) + 슬롯별 업로드
//     컨트롤(바이너리 + 서명 파일) + 자산/버전 삭제(ConfirmDialog).
//
// 보안: 공개키(서명 검증용)는 노드 로컬 신뢰 앵커이므로 서버가 저장/전달하지 않는다.
// 무결성은 각 노드가 자기 로컬 공개키로 서명을 검증해 보장한다.

import { useState } from 'react';
import {
  AlertCircle,
  CheckCircle2,
  Loader2,
  Package,
  Plus,
  Trash2,
  Upload,
} from 'lucide-react';

import { ConfirmDialog } from '@/components/remote/ConfirmDialog';
import {
  useCreateRelease,
  useDeleteRelease,
  useDeleteReleaseAsset,
  useReleases,
  useUploadReleaseAsset,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { formatBytes, formatDate } from '@/lib/utils/format';
import { useUIStore } from '@/stores/uiStore';
import type { ReleaseAsset, ReleaseRecord } from '@/types/remote';

/** 매트릭스로 표시할 표준 아키텍처 슬롯(노드 런타임 GOOS/GOARCH). */
const ARCH_SLOTS: readonly { os: string; arch: string }[] = [
  { os: 'linux', arch: 'amd64' },
  { os: 'linux', arch: 'arm64' },
  { os: 'linux', arch: 'arm' },
  { os: 'darwin', arch: 'amd64' },
  { os: 'darwin', arch: 'arm64' },
] as const;

/** semver(vMAJOR.MINOR.PATCH) 검증 정규식. */
const SEMVER_RE = /^v\d+\.\d+\.\d+$/;

/** 릴리스 채널 옵션. */
const CHANNELS = ['stable', 'beta', 'nightly'] as const;

/** "os/arch" 슬롯 라벨. */
function slotLabel(os: string, arch: string): string {
  return `${os}/${arch}`;
}

/** sha256 짧은 표시(앞 12자). */
function shortSha(sha: string): string {
  return sha.length > 12 ? sha.slice(0, 12) : sha;
}

/** 삭제 확인 대상 상태. */
type DeleteTarget =
  | { kind: 'version'; version: string }
  | { kind: 'asset'; version: string; os: string; arch: string };

export function ReleaseStorePanel(): React.JSX.Element {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const releasesQuery = useReleases();
  const createRelease = useCreateRelease();
  const deleteRelease = useDeleteRelease();
  const deleteAsset = useDeleteReleaseAsset();

  const [version, setVersion] = useState('');
  const [channel, setChannel] = useState<string>('stable');
  const [notes, setNotes] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null);

  const releases = releasesQuery.data ?? [];

  const handleCreate = (): void => {
    const v = version.trim();
    if (!SEMVER_RE.test(v)) {
      addNotification({ type: 'error', message: t('remote.releaseStore.invalidVersion') });
      return;
    }
    createRelease.mutate(
      { version: v, channel, notes: notes.trim() || undefined },
      {
        onSuccess: () => {
          addNotification({ type: 'success', message: t('remote.releaseStore.created') });
          setVersion('');
          setNotes('');
        },
        onError: () =>
          addNotification({ type: 'error', message: t('remote.releaseStore.createFailed') }),
      },
    );
  };

  const confirmDelete = (): void => {
    if (!deleteTarget) return;
    const onSuccess = (message: string): void => {
      addNotification({ type: 'success', message });
      setDeleteTarget(null);
    };
    const onError = (): void => {
      addNotification({ type: 'error', message: t('remote.releaseStore.deleteFailed') });
      setDeleteTarget(null);
    };

    if (deleteTarget.kind === 'version') {
      deleteRelease.mutate(deleteTarget.version, {
        onSuccess: () => onSuccess(t('remote.releaseStore.versionDeleted')),
        onError,
      });
    } else {
      const { version: v, os, arch } = deleteTarget;
      deleteAsset.mutate(
        { version: v, os, arch },
        {
          onSuccess: () => onSuccess(t('remote.releaseStore.assetDeleted')),
          onError,
        },
      );
    }
  };

  const deletePending = deleteRelease.isPending || deleteAsset.isPending;

  return (
    <div className="space-y-6" data-testid="release-store-panel">
      {/* 헤더 */}
      <header>
        <div className="flex items-center gap-2">
          <Package className="h-5 w-5 text-(--color-text-muted)" aria-hidden="true" />
          <h1 className="text-2xl font-semibold text-(--color-text-primary)">
            {t('remote.releaseStore.title')}
          </h1>
        </div>
        <p className="mt-1 text-sm text-(--color-text-muted)">
          {t('remote.releaseStore.subtitle')}
        </p>
      </header>

      {/* 새 버전 폼 */}
      <section
        aria-labelledby="release-new-heading"
        className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
      >
        <h2
          id="release-new-heading"
          className="mb-3 text-sm font-semibold text-(--color-text-primary)"
        >
          {t('remote.releaseStore.newVersion')}
        </h2>
        <div className="flex flex-wrap items-end gap-3">
          <div>
            <label
              htmlFor="release-version"
              className="mb-1 block text-xs text-(--color-text-muted)"
            >
              {t('remote.releaseStore.versionLabel')}
            </label>
            <input
              id="release-version"
              type="text"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              placeholder={t('remote.releaseStore.versionPlaceholder')}
              data-testid="release-version-input"
              className="w-36 rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
            />
          </div>
          <div>
            <label
              htmlFor="release-channel"
              className="mb-1 block text-xs text-(--color-text-muted)"
            >
              {t('remote.releaseStore.channelLabel')}
            </label>
            <select
              id="release-channel"
              value={channel}
              onChange={(e) => setChannel(e.target.value)}
              data-testid="release-channel-select"
              className="rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1.5 text-sm text-(--color-text-primary)"
            >
              {CHANNELS.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </div>
          <div className="min-w-48 flex-1">
            <label
              htmlFor="release-notes"
              className="mb-1 block text-xs text-(--color-text-muted)"
            >
              {t('remote.releaseStore.notesLabel')}
            </label>
            <input
              id="release-notes"
              type="text"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder={t('remote.releaseStore.notesPlaceholder')}
              data-testid="release-notes-input"
              className="w-full rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
            />
          </div>
          <button
            type="button"
            onClick={handleCreate}
            disabled={createRelease.isPending}
            data-testid="release-create-button"
            className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            {createRelease.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
            ) : (
              <Plus className="h-4 w-4" aria-hidden="true" />
            )}
            {t('remote.releaseStore.create')}
          </button>
        </div>
      </section>

      {/* 버전 목록 */}
      {releasesQuery.isError ? (
        <div
          data-testid="release-store-error"
          className="rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400"
        >
          {t('remote.releaseStore.loadError')}
        </div>
      ) : releases.length === 0 ? (
        <div
          data-testid="release-store-empty"
          className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-12 text-center"
        >
          <Package className="mx-auto h-10 w-10 text-gray-300 dark:text-gray-600" aria-hidden="true" />
          <p className="mt-3 text-sm text-(--color-text-muted)">
            {t('remote.releaseStore.empty')}
          </p>
        </div>
      ) : (
        <div className="space-y-4" data-testid="release-list">
          {releases.map((release) => (
            <ReleaseCard
              key={release.version}
              release={release}
              onDeleteVersion={() =>
                setDeleteTarget({ kind: 'version', version: release.version })
              }
              onDeleteAsset={(os, arch) =>
                setDeleteTarget({ kind: 'asset', version: release.version, os, arch })
              }
            />
          ))}
        </div>
      )}

      {/* 삭제 확인 다이얼로그 */}
      <ConfirmDialog
        open={deleteTarget !== null}
        title={
          deleteTarget?.kind === 'version'
            ? t('remote.releaseStore.deleteVersionTitle')
            : t('remote.releaseStore.deleteAssetTitle')
        }
        description={
          deleteTarget === null
            ? ''
            : deleteTarget.kind === 'version'
              ? t('remote.releaseStore.deleteVersionDesc').replace(
                  '{version}',
                  deleteTarget.version,
                )
              : t('remote.releaseStore.deleteAssetDesc')
                  .replace('{version}', deleteTarget.version)
                  .replace('{slot}', slotLabel(deleteTarget.os, deleteTarget.arch))
        }
        confirmLabel={
          deleteTarget?.kind === 'version'
            ? t('remote.releaseStore.deleteVersion')
            : t('remote.releaseStore.deleteAsset')
        }
        pending={deletePending}
        onConfirm={confirmDelete}
        onCancel={() => {
          if (!deletePending) setDeleteTarget(null);
        }}
      />
    </div>
  );
}

// ---- 버전 카드 ----

interface ReleaseCardProps {
  release: ReleaseRecord;
  onDeleteVersion: () => void;
  onDeleteAsset: (os: string, arch: string) => void;
}

function ReleaseCard({
  release,
  onDeleteVersion,
  onDeleteAsset,
}: ReleaseCardProps): React.JSX.Element {
  const { t } = useTranslation();

  // os/arch → 자산 매핑(빠른 조회).
  const assetByKey = new Map<string, ReleaseAsset>();
  for (const a of release.assets) {
    assetByKey.set(slotLabel(a.os, a.arch), a);
  }
  // 표준 슬롯에 포함되지 않은 추가 업로드 자산.
  const standardKeys = new Set(ARCH_SLOTS.map((s) => slotLabel(s.os, s.arch)));
  const extraAssets = release.assets.filter(
    (a) => !standardKeys.has(slotLabel(a.os, a.arch)),
  );

  return (
    <section
      data-testid="release-card"
      data-version={release.version}
      className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
    >
      {/* 버전 헤더 */}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="font-mono text-base font-semibold text-(--color-text-primary)">
              {release.version}
            </h3>
            <span className="rounded bg-(--color-bg-elevated) px-2 py-0.5 text-xs font-medium text-(--color-text-secondary)">
              {release.channel}
            </span>
          </div>
          {release.notes && (
            <p className="mt-1 text-sm text-(--color-text-secondary)">{release.notes}</p>
          )}
          {release.published_at > 0 && (
            <p className="mt-1 text-xs text-(--color-text-muted)">
              {t('remote.releaseStore.publishedAt')}{' '}
              {formatDate(new Date(release.published_at), 'long')}
            </p>
          )}
        </div>
        <button
          type="button"
          onClick={onDeleteVersion}
          data-testid="release-delete-version"
          aria-label={t('remote.releaseStore.deleteVersion')}
          title={t('remote.releaseStore.deleteVersion')}
          className="inline-flex shrink-0 items-center gap-1 rounded-md border border-red-200 px-2.5 py-1.5 text-xs font-medium text-red-700 transition-colors hover:bg-red-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950"
        >
          <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
        </button>
      </div>

      {/* 아키텍처 매트릭스 */}
      <div className="mt-4">
        <h4 className="mb-2 text-xs font-semibold uppercase tracking-wider text-(--color-text-muted)">
          {t('remote.releaseStore.matrixHeading')}
        </h4>
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {ARCH_SLOTS.map((slot) => {
            const key = slotLabel(slot.os, slot.arch);
            const asset = assetByKey.get(key);
            return (
              <ArchSlot
                key={key}
                version={release.version}
                os={slot.os}
                arch={slot.arch}
                asset={asset}
                onDeleteAsset={() => onDeleteAsset(slot.os, slot.arch)}
              />
            );
          })}
        </div>

        {/* 표준 외 추가 자산 */}
        {extraAssets.length > 0 && (
          <div className="mt-3">
            <h4 className="mb-2 text-xs font-semibold uppercase tracking-wider text-(--color-text-muted)">
              {t('remote.releaseStore.extraAssets')}
            </h4>
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              {extraAssets.map((asset) => (
                <ArchSlot
                  key={slotLabel(asset.os, asset.arch)}
                  version={release.version}
                  os={asset.os}
                  arch={asset.arch}
                  asset={asset}
                  onDeleteAsset={() => onDeleteAsset(asset.os, asset.arch)}
                />
              ))}
            </div>
          </div>
        )}
      </div>
    </section>
  );
}

// ---- 아키텍처 슬롯 (업로드됨/누락 + 업로드 컨트롤) ----

interface ArchSlotProps {
  version: string;
  os: string;
  arch: string;
  /** 업로드된 자산(없으면 누락 슬롯). */
  asset: ReleaseAsset | undefined;
  onDeleteAsset: () => void;
}

function ArchSlot({
  version,
  os,
  arch,
  asset,
  onDeleteAsset,
}: ArchSlotProps): React.JSX.Element {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);
  const upload = useUploadReleaseAsset();

  const [binary, setBinary] = useState<File | null>(null);
  const [signature, setSignature] = useState<File | null>(null);

  const slot = slotLabel(os, arch);

  const handleUpload = (): void => {
    if (!binary || !signature) {
      addNotification({
        type: 'error',
        message: t('remote.releaseStore.uploadIncomplete'),
      });
      return;
    }
    upload.mutate(
      { version, os, arch, binary, signature },
      {
        onSuccess: () => {
          addNotification({
            type: 'success',
            message: t('remote.releaseStore.uploaded.success').replace('{slot}', slot),
          });
          setBinary(null);
          setSignature(null);
        },
        onError: () =>
          addNotification({ type: 'error', message: t('remote.releaseStore.uploadFailed') }),
      },
    );
  };

  return (
    <div
      data-testid="release-arch-slot"
      data-slot={slot}
      data-uploaded={asset ? 'true' : 'false'}
      className="rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-3"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="font-mono text-sm font-medium text-(--color-text-primary)">
          {slot}
        </span>
        {asset ? (
          <span className="inline-flex items-center gap-1 text-xs font-medium text-emerald-600 dark:text-emerald-400">
            <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />
            {t('remote.releaseStore.uploaded')}
          </span>
        ) : (
          <span className="inline-flex items-center gap-1 text-xs font-medium text-(--color-text-muted)">
            <AlertCircle className="h-3.5 w-3.5" aria-hidden="true" />
            {t('remote.releaseStore.missing')}
          </span>
        )}
      </div>

      {asset ? (
        <div className="mt-2 space-y-1">
          <p className="text-xs text-(--color-text-muted)">{formatBytes(asset.size)}</p>
          <p className="truncate font-mono text-xs text-(--color-text-muted)" title={asset.sha256}>
            {shortSha(asset.sha256)}
          </p>
          {!asset.has_sig && (
            <p className="inline-flex items-center gap-1 text-xs font-medium text-amber-600 dark:text-amber-400">
              <AlertCircle className="h-3 w-3" aria-hidden="true" />
              {t('remote.releaseStore.noSignature')}
            </p>
          )}
          <button
            type="button"
            onClick={onDeleteAsset}
            data-testid="release-delete-asset"
            aria-label={t('remote.releaseStore.deleteAsset')}
            title={t('remote.releaseStore.deleteAsset')}
            className="mt-1 inline-flex items-center gap-1 rounded border border-(--color-border-strong) px-2 py-1 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
          >
            <Trash2 className="h-3 w-3" aria-hidden="true" />
            {t('remote.releaseStore.deleteAsset')}
          </button>
        </div>
      ) : (
        <div className="mt-2 space-y-2">
          <div>
            <label
              htmlFor={`binary-${version}-${slot}`}
              className="mb-0.5 block text-xs text-(--color-text-muted)"
            >
              {t('remote.releaseStore.binaryLabel')}
            </label>
            <input
              id={`binary-${version}-${slot}`}
              type="file"
              data-testid="release-binary-input"
              onChange={(e) => setBinary(e.target.files?.[0] ?? null)}
              className="block w-full text-xs text-(--color-text-secondary) file:mr-2 file:rounded file:border file:border-(--color-border-strong) file:bg-(--color-bg-surface) file:px-2 file:py-1 file:text-xs file:text-(--color-text-secondary)"
            />
          </div>
          <div>
            <label
              htmlFor={`signature-${version}-${slot}`}
              className="mb-0.5 block text-xs text-(--color-text-muted)"
            >
              {t('remote.releaseStore.signatureLabel')}
            </label>
            <input
              id={`signature-${version}-${slot}`}
              type="file"
              data-testid="release-signature-input"
              onChange={(e) => setSignature(e.target.files?.[0] ?? null)}
              className="block w-full text-xs text-(--color-text-secondary) file:mr-2 file:rounded file:border file:border-(--color-border-strong) file:bg-(--color-bg-surface) file:px-2 file:py-1 file:text-xs file:text-(--color-text-secondary)"
            />
          </div>
          <button
            type="button"
            onClick={handleUpload}
            disabled={upload.isPending}
            data-testid="release-upload-button"
            className="inline-flex w-full items-center justify-center gap-1.5 rounded bg-blue-600 px-2.5 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            {upload.isPending ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
            ) : (
              <Upload className="h-3.5 w-3.5" aria-hidden="true" />
            )}
            {t('remote.releaseStore.upload')}
          </button>
        </div>
      )}
    </div>
  );
}
