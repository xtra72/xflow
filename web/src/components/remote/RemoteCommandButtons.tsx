// 미러 자원에 대한 원격 명령 액션 버튼 (SPEC-REMOTE-001 M5, G04).
//
// 자원 종류(flow/agent/device)에 따라 적절한 명령(start/stop/deploy 등)을
// 대상 노드에 발행한다. 발행 중에는 진행 표시(스피너), 성공/실패는 토스트로
// 피드백한다. 출처 노드가 오프라인이면 버튼을 비활성화하고 안내 title 을 둔다
// (오프라인 노드에는 명령을 적용할 수 없음 — REQ-D08/B07).

import { Loader2, Play, RotateCw, Square } from 'lucide-react';

import { useSendCommand } from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import type { MirroredResource } from '@/types/remote';

interface RemoteCommandButtonsProps {
  /** 명령 대상 미러 자원. */
  resource: MirroredResource;
}

/** 단일 명령 액션 정의. */
interface CommandAction {
  /** 액션 식별자 (start/stop/deploy). */
  action: string;
  /** 버튼 라벨 i18n 키. */
  labelKey: string;
  /** 아이콘. */
  icon: React.ComponentType<{ className?: string }>;
  /** 위험(빨강) 스타일 여부. */
  destructive?: boolean;
}

/** 자원 종류별 가용 명령. domain 은 kind 와 동일하게 사용한다. */
const ACTIONS_BY_KIND: Record<string, CommandAction[]> = {
  flow: [
    { action: 'deploy', labelKey: 'remote.command.deploy', icon: RotateCw },
    { action: 'start', labelKey: 'remote.command.start', icon: Play },
    { action: 'stop', labelKey: 'remote.command.stop', icon: Square, destructive: true },
  ],
  agent: [
    { action: 'start', labelKey: 'remote.command.start', icon: Play },
    { action: 'stop', labelKey: 'remote.command.stop', icon: Square, destructive: true },
  ],
  device: [],
};

/**
 * 자원 종류에 맞는 명령 버튼 묶음을 렌더한다. 발행/피드백을 자체 처리한다.
 */
export function RemoteCommandButtons({
  resource,
}: RemoteCommandButtonsProps): React.JSX.Element | null {
  const { t } = useTranslation();
  const sendCommand = useSendCommand();
  const addNotification = useUIStore((s) => s.addNotification);

  const actions = ACTIONS_BY_KIND[resource.kind] ?? [];
  if (actions.length === 0) return null;

  // 현재 발행 중인 액션(있다면)을 추적하여 해당 버튼만 스피너를 표시한다.
  const pendingAction =
    sendCommand.isPending &&
    sendCommand.variables?.instanceID === resource.source_instance_id
      ? sendCommand.variables?.req.action
      : undefined;

  const handleCommand = (action: string): void => {
    sendCommand.mutate(
      {
        instanceID: resource.source_instance_id,
        req: { domain: resource.kind, action, args: { id: resource.id } },
      },
      {
        onSuccess: () => {
          addNotification({
            type: 'success',
            message: t('remote.command.success'),
          });
        },
        onError: (err: unknown) => {
          addNotification({
            type: 'error',
            message: commandErrorMessage(err, t),
          });
        },
      },
    );
  };

  return (
    <span
      className="inline-flex items-center gap-1"
      role="group"
      aria-label={t('remote.command.actionsLabel')}
    >
      {actions.map(({ action, labelKey, icon: Icon, destructive }) => {
        const isPending = pendingAction === action;
        const disabled = !resource.online || sendCommand.isPending;
        const label = t(labelKey);
        return (
          <button
            key={action}
            type="button"
            disabled={disabled}
            onClick={() => handleCommand(action)}
            title={resource.online ? label : t('remote.command.offlineHint')}
            aria-label={label}
            data-testid={`remote-command-${action}`}
            className={cn(
              'inline-flex items-center gap-1 rounded-md border px-2 py-1 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-40',
              destructive
                ? 'border-red-200 text-red-700 hover:bg-red-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950'
                : 'border-(--color-border-strong) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
            )}
          >
            {isPending ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
            ) : (
              <Icon className="h-3.5 w-3.5" aria-hidden="true" />
            )}
            {label}
          </button>
        );
      })}
    </span>
  );
}

/**
 * 명령 발행 에러를 사용자 친화 한글 메시지로 변환한다.
 * 백엔드 상태 코드 매핑:
 *   503 → 미승인/오프라인 (적용 불가)
 *   504 → 타임아웃 (미적용)
 *   502 → 노드 적용 실패
 */
function commandErrorMessage(err: unknown, t: (k: string) => string): string {
  const status = extractStatus(err);
  switch (status) {
    case 503:
      return t('remote.command.errorUnavailable');
    case 504:
      return t('remote.command.errorTimeout');
    case 502:
      return t('remote.command.errorFailed');
    default:
      return t('remote.command.errorGeneric');
  }
}

/** APIError 등에서 HTTP status 를 안전하게 추출한다. */
function extractStatus(err: unknown): number | undefined {
  if (err && typeof err === 'object' && 'status' in err) {
    const s = (err as { status: unknown }).status;
    if (typeof s === 'number') return s;
  }
  return undefined;
}
