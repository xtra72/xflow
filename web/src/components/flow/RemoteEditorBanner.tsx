// RemoteEditorBanner — 원격 노드의 플로우를 시각 편집기에서 편집 중임을 명확히
// 표시하는 상단 배너 (SPEC-REMOTE-001, 원격 편집기 시각 구분).
//
// 로컬 편집기(EditorPage 로컬 모드)에는 이런 배너가 전혀 없으므로, 본 배너의
// 존재만으로 "원격 노드 편집"임을 한눈에 구분할 수 있다. 시각 언어는
// RemoteTargetBanner(목록/대시보드용)와 일관되게 violet 액센트 + Server 아이콘을
// 사용하되, 편집 맥락(노드 호스트명 + 플로우명 + 노드로 돌아가기)에 맞춘다.
//
// 호스트명은 instanceId 로부터 해석되며(실패 시 단축 instanceId 로 폴백),
// 원본 instanceId 는 title(툴팁)로만 노출한다(원시 UUID 를 본문에 노출하지 않음).

import { ArrowLeft, Server } from 'lucide-react';
import { Link } from 'react-router';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

interface RemoteEditorBannerProps {
  /** 원격 편집 대상 노드 표시명(호스트명 또는 단축 instanceId 폴백). */
  hostname: string;
  /** 원본 instanceId(툴팁/접근성용 — 본문에는 노출하지 않음). */
  instanceId: string;
  /** 편집 중인 플로우 이름(신규/미정 시 빈 문자열). */
  flowName: string;
  /** 신규 생성 모드 여부. */
  isNew: boolean;
  /** 노드 관리 화면으로 돌아가는 경로. */
  backHref: string;
}

/**
 * 원격 노드 플로우 편집 전용 상단 배너.
 *
 * 로컬 편집기에는 렌더되지 않는다(호출 측에서 isRemote 로 게이팅).
 */
export function RemoteEditorBanner({
  hostname,
  instanceId,
  flowName,
  isNew,
  backHref,
}: RemoteEditorBannerProps): React.JSX.Element {
  const { t } = useTranslation();

  const displayFlowName =
    flowName || (isNew ? t('remote.editor.newFlow') : t('remote.editor.untitled'));

  return (
    <div
      data-testid="remote-editor-banner"
      className={cn(
        'flex items-center justify-between gap-3 px-4 py-2 text-sm',
        'border-b-2 border-violet-400 bg-violet-50 text-violet-900',
        'dark:border-violet-500 dark:bg-violet-950 dark:text-violet-100',
      )}
      role="status"
    >
      <div className="flex min-w-0 items-center gap-2">
        <Server className="h-4 w-4 shrink-0" aria-hidden="true" />
        <span
          className="shrink-0 font-semibold"
          title={instanceId}
          data-testid="remote-editor-banner-target"
        >
          {t('remote.editor.bannerTitle')}
          <span className="ml-1 font-bold">{hostname}</span>
        </span>
        <span aria-hidden="true" className="shrink-0 text-violet-400 dark:text-violet-500">
          ·
        </span>
        <span
          className="min-w-0 truncate text-violet-700 dark:text-violet-300"
          title={displayFlowName}
        >
          {displayFlowName}
        </span>
      </div>

      <Link
        to={backHref}
        data-testid="remote-editor-banner-back"
        className={cn(
          'inline-flex shrink-0 items-center gap-1 rounded-md px-2 py-1 text-xs font-medium',
          'border border-violet-300 transition-colors hover:bg-violet-100',
          'dark:border-violet-700 dark:hover:bg-violet-900',
        )}
      >
        <ArrowLeft className="h-3.5 w-3.5" aria-hidden="true" />
        {t('remote.editor.backToNode')}
      </Link>
    </div>
  );
}
