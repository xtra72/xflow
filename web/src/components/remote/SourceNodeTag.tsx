// 통합 미러 뷰에서 자원의 출처 노드를 표시하는 태그 (SPEC-REMOTE-001 M5, G03).
//
// 출처 노드 식별자(source_instance_id)를 칩 형태로 표시하고, 그 노드의 라이브
// 상태(online)를 점으로 함께 표시한다. online=false 는 last-known(오프라인)
// 표식이다 (REQ-E06).

import { Server } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

interface SourceNodeTagProps {
  /** 출처 노드 식별자. */
  sourceInstanceId: string;
  /** 출처 노드 라이브 상태 (false = last-known/offline). */
  online: boolean;
  /** 사람이 읽기 좋은 노드 표시명 (hostname 등). 미지정 시 식별자 노출. */
  displayName?: string;
  /** 추가 className. */
  className?: string;
}

/**
 * 자원의 출처 노드를 칩으로 표시한다. 노드가 오프라인이면 "마지막 정보"
 * 보조 라벨과 회색 점으로 last-known 임을 알린다.
 */
export function SourceNodeTag({
  sourceInstanceId,
  online,
  displayName,
  className,
}: SourceNodeTagProps): React.JSX.Element {
  const { t } = useTranslation();
  const label = displayName?.trim() ? displayName : sourceInstanceId;

  return (
    <span
      data-testid="source-node-tag"
      data-source-instance-id={sourceInstanceId}
      data-online={online}
      title={sourceInstanceId}
      className={cn(
        'inline-flex max-w-full items-center gap-1.5 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-0.5 text-xs',
        className,
      )}
    >
      <Server className="h-3 w-3 shrink-0 text-(--color-text-muted)" aria-hidden="true" />
      <span className="truncate font-medium text-(--color-text-secondary)">{label}</span>
      <span
        aria-hidden="true"
        className={cn(
          'h-1.5 w-1.5 shrink-0 rounded-full',
          online ? 'bg-green-500 dark:bg-green-400' : 'bg-gray-400 dark:bg-gray-500',
        )}
      />
      {!online && (
        <span className="shrink-0 text-(--color-text-muted)">{t('remote.lastKnown')}</span>
      )}
    </span>
  );
}
