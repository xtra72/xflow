// 미러 자원 목록 테이블 (SPEC-REMOTE-001 M5, G03 + G04).
//
// flow/agent/device 미러 행을 표시한다. 통합(aggregated) 뷰에서는 출처 노드
// 태그(SourceNodeTag)를 함께 노출하며, 오프라인 출처 노드는 last-known 표식을
// 보인다 (REQ-E06). 각 행에는 종류별 원격 명령 버튼(RemoteCommandButtons)을 둔다.

import { Pencil, Trash2 } from 'lucide-react';

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
  /**
   * 자원 수정 콜백 (M7, REQ-I10). 지정 시 편집 액션을 노출한다. flow 는 시각
   * 편집기로, agent 는 설정 다이얼로그로 라우팅하는 책임은 부모가 갖는다.
   */
  onEdit?: (resource: MirroredResource) => void;
  /** 자원 삭제 콜백 (M7, REQ-I03/I04). 지정 시 삭제 액션을 노출한다. */
  onDelete?: (resource: MirroredResource) => void;
  /**
   * 편집/삭제 게이팅 (M7, REQ-I05/I10). 자원이 편집 가능한지(승인+온라인 노드의
   * 노출 자원) 반환한다. 미지정 시 res.online 만으로 판정한다.
   */
  canEdit?: (resource: MirroredResource) => boolean;
}

/**
 * 미러 자원 테이블. showSource=true 이면 출처 노드 열을 추가한다.
 */
export function MirrorResourceTable({
  resources,
  showSource,
  nodes,
  onEdit,
  onDelete,
  canEdit,
}: MirrorResourceTableProps): React.JSX.Element {
  const { t } = useTranslation();

  // instance_id → hostname 매핑 (출처 태그 표시명 fallback).
  const hostnameById = new Map<string, string>(
    (nodes ?? []).map((n) => [n.instance_id, n.hostname]),
  );

  // 편집 액션 노출 여부 — 콜백이 하나라도 있으면 편집 컬럼을 활성화한다.
  const showEditActions = !!onEdit || !!onDelete;
  // 자원별 편집 가능 여부 판정 (기본: online).
  const editable = (res: MirroredResource): boolean =>
    canEdit ? canEdit(res) : res.online;

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
                  {/* M7: 편집/삭제 액션 (REQ-I10). device 는 편집 대상 아님. */}
                  {showEditActions && res.kind !== 'device' && (
                    <>
                      {onEdit && (
                        <button
                          type="button"
                          onClick={() => onEdit(res)}
                          disabled={!editable(res)}
                          data-testid="mirror-resource-edit"
                          aria-label={t('remote.action.edit')}
                          title={
                            editable(res)
                              ? t('remote.action.edit')
                              : t('remote.edit.gateHint')
                          }
                          className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-2 py-1 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
                        >
                          <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
                          {t('remote.action.edit')}
                        </button>
                      )}
                      {onDelete && (
                        <button
                          type="button"
                          onClick={() => onDelete(res)}
                          disabled={!editable(res)}
                          data-testid="mirror-resource-delete"
                          aria-label={t('remote.action.delete')}
                          title={
                            editable(res)
                              ? t('remote.action.delete')
                              : t('remote.edit.gateHint')
                          }
                          className="inline-flex items-center gap-1 rounded-md border border-red-200 px-2 py-1 text-xs font-medium text-red-700 transition-colors hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950"
                        >
                          <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                        </button>
                      )}
                    </>
                  )}
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
