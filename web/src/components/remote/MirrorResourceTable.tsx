// 미러 자원 목록 테이블 (SPEC-REMOTE-001 M5, G03 + G04).
//
// flow/agent/device 미러 행을 표시한다. 통합(aggregated) 뷰에서는 출처 노드
// 태그(SourceNodeTag)를 함께 노출하며, 오프라인 출처 노드는 last-known 표식을
// 보인다 (REQ-E06). 각 행에는 종류별 원격 명령 버튼(RemoteCommandButtons)을 둔다.

import { useTranslation } from '@/lib/i18n';
import { formatDate } from '@/lib/utils/format';
import type { ManagedNode, MirroredResource } from '@/types/remote';

import { NodeOnlineIndicator } from './NodeOnlineIndicator';
import { RemoteCommandButtons } from './RemoteCommandButtons';
import { SourceNodeTag } from './SourceNodeTag';

interface MirrorResourceTableProps {
  /** 표시할 미러 자원 목록. */
  resources: MirroredResource[];
  /** 출처 노드 태그 열 표시 여부 (통합 뷰=true, 노드별 뷰=false). */
  showSource: boolean;
  /** 출처 노드 표시명 조회용 노드 목록 (hostname fallback). */
  nodes?: ManagedNode[];
}

/**
 * 미러 자원 테이블. showSource=true 이면 출처 노드 열을 추가한다.
 */
export function MirrorResourceTable({
  resources,
  showSource,
  nodes,
}: MirrorResourceTableProps): React.JSX.Element {
  const { t } = useTranslation();

  // instance_id → hostname 매핑 (출처 태그 표시명 fallback).
  const hostnameById = new Map<string, string>(
    (nodes ?? []).map((n) => [n.instance_id, n.hostname]),
  );

  return (
    <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
      <table
        className="min-w-full divide-y divide-(--color-border-default)"
        aria-label={t('remote.resourcesTableLabel')}
      >
        <thead className="bg-(--color-bg-primary)">
          <tr>
            <th scope="col" className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
              {t('remote.col.name')}
            </th>
            <th scope="col" className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
              {t('remote.col.resourceStatus')}
            </th>
            {showSource && (
              <th scope="col" className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
                {t('remote.col.sourceNode')}
              </th>
            )}
            <th scope="col" className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
              {t('remote.col.updatedAt')}
            </th>
            <th scope="col" className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
              {t('remote.col.actions')}
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-(--color-border-default) bg-(--color-bg-surface)">
          {resources.map((res) => (
            <tr
              key={`${res.source_instance_id}:${res.kind}:${res.id}`}
              data-testid="mirror-resource-row"
              data-resource-id={res.id}
              data-source-instance-id={res.source_instance_id}
            >
              <td className="whitespace-nowrap px-4 py-3 text-sm font-medium text-(--color-text-primary)">
                {res.name || res.id}
              </td>
              <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
                {res.status ? res.status : '-'}
              </td>
              {showSource && (
                <td className="whitespace-nowrap px-4 py-3">
                  <SourceNodeTag
                    sourceInstanceId={res.source_instance_id}
                    online={res.online}
                    displayName={hostnameById.get(res.source_instance_id)}
                  />
                </td>
              )}
              <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
                {res.updated_at > 0 ? formatDate(new Date(res.updated_at), 'long') : '-'}
              </td>
              <td className="whitespace-nowrap px-4 py-3 text-right">
                <div className="inline-flex items-center justify-end gap-2">
                  {/* 노드별 뷰에서는 출처 태그 대신 online 점만 간단히 표시 */}
                  {!showSource && (
                    <NodeOnlineIndicator online={res.online} showLabel={false} />
                  )}
                  <RemoteCommandButtons resource={res} />
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
