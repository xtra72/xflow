// 원격 관리 — 미러 자원 통합 개요 페이지 (SPEC-REMOTE-001 M5/M8, G03 + REQ-J14).
//
// 전 노드의 자원을 출처 노드 태그와 함께 보여주는 "교차 노드 개요(read-only
// overview)" 전용 화면이다. 노드별 깊은 제어(생성/수정/삭제)는 본 페이지에서
// 제거하고(REQ-J14), 단일 노드 깊은 제어는 "원격 노드 제어"(RemoteControlPage)
// 와 통합 로컬 페이지(`?target=remote:{id}`)로 일원화한다.
//
// 제공 기능:
//   - 종류 탭(flows/agents/devices)으로 자원 종류 전환.
//   - 통합(aggregated) 미러 테이블 — 출처 노드 태그 + 온라인 상태.
//   - 각 행의 빠른 수명주기 명령(deploy/start/stop — RemoteCommandButtons).
//   - 각 행의 "이 노드에서 제어" 링크 → 통합 로컬 페이지로 진입(deep control).
//
// 제거된 기능(→ 원격 노드 제어/통합 로컬 페이지로 이전):
//   - 통합/노드별 뷰 토글, 노드 선택 드롭다운.
//   - 자원 생성·수정(시각 편집기)·삭제, 에이전트 편집 다이얼로그.
//
// 권한: admin 전용 (라우트 가드 + 백엔드 검증). server 모드에서만 쿼리 발행.

import { useCallback, useState } from 'react';
import { useNavigate } from 'react-router';
import { Boxes } from 'lucide-react';

import { MirrorResourceTable } from '@/components/remote/MirrorResourceTable';
import { RemoteNotServerNotice } from '@/components/remote/RemoteNotServerNotice';
import {
  useAllMirror,
  useManagedNodes,
  useRemoteMode,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { serializeTargetParam } from '@/lib/remote/target';
import { cn } from '@/lib/utils/cn';
import type { MirroredResource, MirroredResourceKind } from '@/types/remote';

/** 종류 탭 정의. */
const KIND_TABS: { kind: MirroredResourceKind; labelKey: string }[] = [
  { kind: 'flow', labelKey: 'remote.kind.flows' },
  { kind: 'agent', labelKey: 'remote.kind.agents' },
  { kind: 'device', labelKey: 'remote.kind.devices' },
];

/** 자원 종류 → 통합 로컬 페이지 베이스 경로(REQ-J14). */
const KIND_BASE_PATH: Record<MirroredResourceKind, string> = {
  flow: '/flows',
  agent: '/agents',
  device: '/devices',
};

/** res.kind(string)가 알려진 미러 종류인지 좁히는 타입 가드. */
function isMirroredKind(kind: string): kind is MirroredResourceKind {
  return kind === 'flow' || kind === 'agent' || kind === 'device';
}

export default function RemoteResourcesPage(): React.JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [kind, setKind] = useState<MirroredResourceKind>('flow');

  // server 모드가 아니면 모든 미러/노드 쿼리를 막아 404 노이즈를 방지한다.
  const { data: remoteMode } = useRemoteMode();
  const isServer = remoteMode?.mode === 'server';

  const { data: nodes } = useManagedNodes(undefined, isServer);

  // 통합(aggregated) 미러 쿼리만 사용한다(노드별 뷰 제거 — REQ-J14).
  const query = useAllMirror(kind, isServer);
  const resources = query.data ?? [];
  const isLoading = query.isLoading;
  const error = query.error;

  // "이 노드에서 제어" — 출처 노드의 통합 로컬 페이지로 이동한다(REQ-J14).
  // target 직렬화를 RemoteControlPage 와 동일하게 사용해 일관성을 유지한다.
  const handleControl = useCallback(
    (res: MirroredResource): void => {
      // res.kind 는 와이어 타입상 string 이므로 알려진 종류만 매핑한다(폴백 없음).
      const base = isMirroredKind(res.kind) ? KIND_BASE_PATH[res.kind] : undefined;
      if (!base) return;
      const target = serializeTargetParam({
        type: 'remote',
        instanceId: res.source_instance_id,
      });
      navigate(`${base}?target=${target}`);
    },
    [navigate],
  );

  // 비-server 모드: 안내만 표시하고 미러 쿼리는 발행하지 않는다.
  if (remoteMode && !isServer) {
    return (
      <div className="space-y-6">
        <ResourcesHeader />
        <RemoteNotServerNotice />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <ResourcesHeader />

      {/* 종류 탭 */}
      <div
        className="flex gap-1 border-b border-(--color-border-default)"
        role="tablist"
        aria-label={t('remote.kindTabsLabel')}
      >
        {KIND_TABS.map(({ kind: k, labelKey }) => (
          <button
            key={k}
            type="button"
            role="tab"
            aria-selected={kind === k}
            data-testid={`remote-kind-${k}`}
            onClick={() => setKind(k)}
            className={cn(
              '-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors',
              kind === k
                ? 'border-blue-600 text-blue-700 dark:border-blue-400 dark:text-blue-400'
                : 'border-transparent text-(--color-text-muted) hover:text-(--color-text-secondary)',
            )}
          >
            {t(labelKey)}
          </button>
        ))}
      </div>

      {/* 본문 */}
      <ResourceBody
        isLoading={isLoading}
        hasError={!!error}
        onRetry={() => query.refetch()}
        isEmpty={resources.length === 0}
      >
        <MirrorResourceTable
          resources={resources}
          showSource
          nodes={nodes}
          onControl={handleControl}
        />
      </ResourceBody>
    </div>
  );
}

// ---- 페이지 헤더 ----

function ResourcesHeader(): React.JSX.Element {
  const { t } = useTranslation();
  return (
    <header data-testid="remote-resources-header">
      <h1 className="text-2xl font-semibold text-(--color-text-primary)">
        {t('remote.resourcesTitle')}
      </h1>
      <p className="mt-1 text-sm text-(--color-text-muted)">
        {t('remote.resourcesSubtitle')}
      </p>
    </header>
  );
}

// ---- 본문 상태 래퍼 ----

interface ResourceBodyProps {
  isLoading: boolean;
  hasError: boolean;
  onRetry: () => void;
  isEmpty: boolean;
  children: React.ReactNode;
}

/** 로딩/에러/빈 상태/정상을 분기 렌더한다. */
function ResourceBody({
  isLoading,
  hasError,
  onRetry,
  isEmpty,
  children,
}: ResourceBodyProps): React.JSX.Element {
  const { t } = useTranslation();

  if (isLoading) {
    return (
      <div className="space-y-2" data-testid="remote-resources-loading">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="h-12 animate-pulse rounded bg-(--color-bg-elevated)" />
        ))}
      </div>
    );
  }

  if (hasError) {
    return (
      <div
        className="rounded-md border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20"
        data-testid="remote-resources-error"
      >
        <p className="text-sm text-red-700 dark:text-red-400">{t('remote.loadError')}</p>
        <button
          type="button"
          onClick={onRetry}
          className="mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600"
        >
          {t('common.retry')}
        </button>
      </div>
    );
  }

  if (isEmpty) {
    return (
      <div
        className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-16 text-center"
        data-testid="remote-resources-empty"
      >
        <Boxes className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" aria-hidden="true" />
        <p className="mt-4 text-sm text-(--color-text-muted)">{t('remote.noResources')}</p>
      </div>
    );
  }

  return <>{children}</>;
}
