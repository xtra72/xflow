// RemoteTargetBanner — 원격 노드 타깃으로 페이지를 보고 있음을 명확히 표시한다
// (SPEC-REMOTE-001 M8, 그룹 J, REQ-J13).
//
// 목록/제어 페이지 상단에 렌더링되어, 현재 로컬이 아닌 특정 원격 노드의 자원을
// 보고/제어 중임을 알리고, 로컬로 돌아가는 링크를 제공한다. 노드가 오프라인/
// 미승인(nodeReady=false)이면 제어가 비활성됨을 경고한다.
//
// 로컬 타깃이면 아무것도 렌더링하지 않는다(기존 UI 불변).

import { Network, X } from 'lucide-react';
import { Link } from 'react-router';

import { useTranslation } from '@/lib/i18n';
import { isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';
import { cn } from '@/lib/utils/cn';

interface RemoteTargetBannerProps {
  target: ResourceTarget;
  /** 표시명(없으면 instanceId). */
  nodeLabel?: string;
  /** 노드 승인+온라인 여부(false 면 제어 비활성 경고). */
  nodeReady: boolean;
  /** 로컬로 돌아가는 경로(기본: 현재 경로의 target 제거). */
  localHref: string;
}

/** 현재 원격 타깃을 알리는 배너(로컬이면 null). */
export function RemoteTargetBanner({
  target,
  nodeLabel,
  nodeReady,
  localHref,
}: RemoteTargetBannerProps): React.JSX.Element | null {
  const { t } = useTranslation();
  if (!isRemoteTarget(target)) return null;

  const label = nodeLabel || target.instanceId;

  return (
    <div
      data-testid="remote-target-banner"
      className={cn(
        'flex items-center justify-between gap-3 rounded-md border px-4 py-2.5 text-sm',
        nodeReady
          ? 'border-blue-300 bg-blue-50 text-blue-800 dark:border-blue-700 dark:bg-blue-950 dark:text-blue-200'
          : 'border-amber-300 bg-amber-50 text-amber-800 dark:border-amber-700 dark:bg-amber-950 dark:text-amber-200',
      )}
      role="status"
    >
      <div className="flex min-w-0 items-center gap-2">
        <Network className="h-4 w-4 shrink-0" aria-hidden="true" />
        <span className="truncate">
          {t('remote.target.viewing')}
          <strong className="ml-1 font-semibold">{label}</strong>
          {!nodeReady && (
            <span className="ml-2 text-xs font-medium">
              ({t('remote.target.controlDisabled')})
            </span>
          )}
        </span>
      </div>
      <Link
        to={localHref}
        data-testid="remote-target-exit"
        className="inline-flex shrink-0 items-center gap-1 rounded-md border border-current/30 px-2 py-1 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
      >
        <X className="h-3.5 w-3.5" aria-hidden="true" />
        {t('remote.target.backToLocal')}
      </Link>
    </div>
  );
}
