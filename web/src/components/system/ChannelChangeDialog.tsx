// SPEC-UPDATE-002 v0.1.0 (M8) — Channel Change Dialog.
//
// 관리자가 업데이트 채널 (stable | beta | nightly) 을 변경하는 다이얼로그.
//
// 동작 시나리오:
//   1. SystemVersionCard 의 admin 채널 배지 클릭 → 본 dialog open
//   2. 사용자가 dropdown 에서 새 채널 선택 + "변경" 클릭
//   3. PUT /system/update/channel 호출 (useChangeChannel mutation)
//   4. 성공:
//        - useSystemVersion 과 useChannelInfo 쿼리가 무효화됨 (훅 내부에서 처리)
//        - "channel: stable → beta" 형태의 success 토스트
//        - 응답 `message` 가 있으면 추가 info 토스트로 yaml 영구 저장 안내 노출
//        - onClose 호출 → 다이얼로그 닫힘
//   5. 실패:
//        - mapUpdateError 로 401/400 등을 한글 메시지로 변환 → error 토스트
//        - 다이얼로그는 닫지 않음 (사용자가 다시 시도하거나 취소 가능)
//
// 사용자 가이드:
//   - 본 변경은 in-memory 만 적용된다는 점을 본문 경고로 명시.
//   - 영구 저장은 `xflowd update channel <name>` CLI 또는 yaml 직접 수정.
//
// @spec SPEC-UPDATE-002 v0.1.0 (M8)

import { useEffect, useState } from 'react';
import { AlertTriangle, X } from 'lucide-react';

import { mapUpdateError } from '@/lib/errors/updaterErrorMapper';
import {
  useChangeChannel,
  useChannelInfo,
  type Channel,
} from '@/services/api/systemUpdate';
import { useUIStore } from '@/stores/uiStore';
import { cn } from '@/lib/utils/cn';

// ─────────────────────────────────────────────────────────────────────
// Public types
// ─────────────────────────────────────────────────────────────────────

export interface ChannelChangeDialogProps {
  /** 다이얼로그 open 여부. */
  open: boolean;
  /** 닫기 콜백 (취소 / 닫기 / 성공 후 자동 호출). */
  onClose: () => void;
  /**
   * 현재 시스템에 설정된 채널 (SystemVersionCard 의 versionInfo.channel).
   * 다이얼로그 내부 dropdown 의 default selection 으로 사용된다.
   */
  currentChannel: Channel;
}

// ─────────────────────────────────────────────────────────────────────
// 채널 enum fallback (useChannelInfo 가 아직 로딩 중일 때 사용).
// ─────────────────────────────────────────────────────────────────────

const FALLBACK_CHANNELS: readonly Channel[] = ['stable', 'beta', 'nightly'];

// ─────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────

export function ChannelChangeDialog({
  open,
  onClose,
  currentChannel,
}: ChannelChangeDialogProps) {
  const channelInfo = useChannelInfo();
  const changeMutation = useChangeChannel();
  const addNotification = useUIStore((s) => s.addNotification);

  // 다이얼로그가 열릴 때마다 currentChannel 로 selection 을 리셋한다.
  // 닫혔다 다시 열어도 이전 선택이 잔존하지 않도록 보장.
  const [selectedChannel, setSelectedChannel] = useState<Channel>(currentChannel);

  useEffect(() => {
    if (open) {
      setSelectedChannel(currentChannel);
    }
  }, [open, currentChannel]);

  if (!open) return null;

  const isPending = changeMutation.isPending;
  const isSameChannel = selectedChannel === currentChannel;
  const availableChannels: readonly Channel[] =
    channelInfo.data?.available ?? FALLBACK_CHANNELS;

  // 확인 핸들러: 동일 채널이면 mutation 없이 닫고, 다르면 PUT 호출.
  // 성공/실패 분기는 try/catch + addNotification 으로 처리한다.
  const handleConfirm = async () => {
    if (isSameChannel) {
      onClose();
      return;
    }

    try {
      const result = await changeMutation.mutateAsync({
        channel: selectedChannel,
      });
      addNotification({
        type: 'success',
        message: `채널 변경 완료: ${result.previous} → ${result.current}`,
      });
      // v0.2.0 백엔드는 yaml 영구 저장 안내 등을 message 로 내려준다.
      if (result.message) {
        addNotification({
          type: 'info',
          message: result.message,
        });
      }
      onClose();
    } catch (err) {
      const mapped = mapUpdateError(err);
      addNotification({
        type: 'error',
        message: mapped.userMessage,
      });
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="channel-dialog-title"
      data-testid="channel-change-dialog"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
    >
      <div className="w-full max-w-md rounded-lg bg-(--color-bg-elevated) p-6 shadow-xl">
        <header className="mb-4 flex items-center justify-between gap-2">
          <h2
            id="channel-dialog-title"
            className="text-lg font-semibold text-(--color-text-primary)"
          >
            업데이트 채널 변경
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={isPending}
            aria-label="닫기"
            data-testid="channel-dialog-close"
            className="rounded p-1 text-(--color-text-muted) hover:bg-(--color-bg-hover) disabled:cursor-not-allowed disabled:opacity-50"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </button>
        </header>

        {/* 현재 채널 + 새 채널 dropdown */}
        <div className="mb-4">
          <p className="mb-2 text-sm text-(--color-text-muted)">
            현재 채널:{' '}
            <strong
              className="font-mono text-(--color-text-primary)"
              data-testid="channel-dialog-current"
            >
              {currentChannel}
            </strong>
          </p>
          <label
            htmlFor="channel-select"
            className="mb-1 block text-sm font-medium text-(--color-text-primary)"
          >
            새 채널
          </label>
          <select
            id="channel-select"
            data-testid="channel-select"
            value={selectedChannel}
            onChange={(e) => setSelectedChannel(e.target.value as Channel)}
            disabled={isPending}
            className={cn(
              'w-full rounded-md border border-(--color-border) bg-(--color-bg-base)',
              'px-3 py-2 text-sm text-(--color-text-primary)',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            {availableChannels.map((ch) => (
              <option key={ch} value={ch}>
                {ch}
              </option>
            ))}
          </select>
        </div>

        {/* 경고 박스 — 다음 check 부터 적용 + in-memory 안내. */}
        <div
          data-testid="channel-dialog-warning"
          className={cn(
            'mb-4 flex items-start gap-2 rounded-md border px-3 py-2 text-sm',
            'border-yellow-300 bg-yellow-50 text-yellow-900',
            'dark:border-yellow-700 dark:bg-yellow-950 dark:text-yellow-200',
          )}
        >
          <AlertTriangle
            className="mt-0.5 h-4 w-4 flex-shrink-0"
            aria-hidden="true"
          />
          <div className="space-y-1">
            <p>
              채널을 변경하면 다음 업데이트 확인부터 새 채널의 버전이 노출됩니다.
            </p>
            <p className="text-xs opacity-90">
              본 변경은 in-memory 만 적용됩니다. 영구 저장은{' '}
              <code className="font-mono">xflowd update channel &lt;name&gt;</code>{' '}
              CLI 를 사용하세요.
            </p>
          </div>
        </div>

        {/* 액션 버튼 */}
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            disabled={isPending}
            data-testid="channel-dialog-cancel"
            className={cn(
              'rounded-md border border-(--color-border) bg-(--color-bg-base)',
              'px-4 py-2 text-sm text-(--color-text-primary)',
              'hover:bg-(--color-bg-hover)',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            취소
          </button>
          <button
            type="button"
            onClick={handleConfirm}
            disabled={isPending || isSameChannel}
            data-testid="channel-dialog-confirm"
            className={cn(
              'rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white',
              'hover:bg-blue-700',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            {isPending ? '변경 중...' : '변경'}
          </button>
        </div>
      </div>
    </div>
  );
}

export default ChannelChangeDialog;
