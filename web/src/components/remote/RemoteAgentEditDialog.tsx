// 원격 에이전트 편집/설정 다이얼로그 (SPEC-REMOTE-001 M7, 그룹 I, REQ-I09).
//
// 관리자가 원격(미러링된) 에이전트를 생성/수정하는 설정 surface 이다. 에이전트
// 종류(type)별 폼 대신, 안정적이고 종류 무관한 "정의 편집"(name + type + config
// JSON)을 제공한다(spec REQ-I09 — "에이전트 종류별 설정 폼 또는 정의 편집" 허용).
//
// 시크릿(REQ-I07): 수정 시 config 의 마스킹/미변경 시크릿 필드는 호출 측 훅에서
// omitMaskedSecrets 로 생략한다. 미러 config 는 이미 redaction 되어 시크릿 키가
// 부재하므로, 관리자가 JSON 에 새 시크릿 값을 명시하지 않는 한 노드가 기존값을
// backfill 한다(마스킹 자리표시자 영속 방지).
//
// 접근성: ConfirmDialog/PreRegisterNodeDialog 와 동일 패턴(role=dialog,
// aria-modal, 포커스/Escape/Tab 트랩).

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Loader2 } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

/** 편집 모드 — 생성 또는 수정. */
export type RemoteAgentEditMode = 'create' | 'update';

/** 다이얼로그가 제출 시 부모에 전달하는 값. */
export interface RemoteAgentEditValue {
  /** 에이전트 이름. */
  name: string;
  /** 에이전트 종류(create 에서만 의미 — update 는 변경하지 않음). */
  type: string;
  /** config 객체(JSON 파싱 결과). */
  config: Record<string, unknown>;
}

interface RemoteAgentEditDialogProps {
  /** 열림 여부. */
  open: boolean;
  /** 편집 모드. */
  mode: RemoteAgentEditMode;
  /** 진행 중 여부(저장 버튼 비활성 + 스피너). */
  pending: boolean;
  /** 초기 이름(update). */
  initialName?: string;
  /** 초기 종류(update — 읽기 전용 표시). */
  initialType?: string;
  /** 초기 config(update — redaction 된 정의). */
  initialConfig?: Record<string, unknown>;
  /**
   * 제출 콜백. 부모가 mutation 을 호출하고 성공/실패를 처리한다.
   * 실패 시 본 컴포넌트가 에러를 표시할 수 있도록 reject 를 전파한다.
   */
  onSubmit: (value: RemoteAgentEditValue) => Promise<void>;
  /** 취소/닫기 콜백. */
  onCancel: () => void;
}

/** 포커스 가능한 요소 셀렉터(포커스 트랩용). */
const FOCUSABLE =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * 원격 에이전트 생성/수정 다이얼로그.
 */
export function RemoteAgentEditDialog({
  open,
  mode,
  pending,
  initialName,
  initialType,
  initialConfig,
  onSubmit,
  onCancel,
}: RemoteAgentEditDialogProps): React.JSX.Element | null {
  const { t } = useTranslation();
  const dialogRef = useRef<HTMLDivElement>(null);
  const firstInputRef = useRef<HTMLInputElement>(null);

  const [name, setName] = useState('');
  const [type, setType] = useState('');
  const [configText, setConfigText] = useState('{}');
  const [error, setError] = useState<string | null>(null);

  // 초기 config 를 보기 좋은 JSON 문자열로 직렬화한다.
  const initialConfigText = useMemo(
    () => JSON.stringify(initialConfig ?? {}, null, 2),
    [initialConfig],
  );

  // 열릴 때마다 폼을 초기화하고 첫 입력에 포커스한다.
  useEffect(() => {
    if (open) {
      setName(initialName ?? '');
      setType(initialType ?? '');
      setConfigText(initialConfigText);
      setError(null);
      const id = window.setTimeout(() => firstInputRef.current?.focus(), 0);
      return () => window.clearTimeout(id);
    }
    return undefined;
  }, [open, initialName, initialType, initialConfigText]);

  // Escape 취소 + Tab 포커스 트랩.
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        if (!pending) onCancel();
        return;
      }
      if (e.key === 'Tab') {
        const root = dialogRef.current;
        if (!root) return;
        const focusable = Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE));
        if (focusable.length === 0) return;
        const first = focusable[0]!;
        const last = focusable[focusable.length - 1]!;
        const active = document.activeElement as HTMLElement | null;
        if (e.shiftKey && active === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && active === last) {
          e.preventDefault();
          first.focus();
        }
      }
    },
    [onCancel, pending],
  );

  const handleSubmit = (e: React.FormEvent): void => {
    e.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName) {
      setError(t('remote.agentEdit.nameRequired'));
      return;
    }
    if (mode === 'create' && !type.trim()) {
      setError(t('remote.agentEdit.typeRequired'));
      return;
    }
    let config: Record<string, unknown>;
    try {
      const parsed: unknown = configText.trim() === '' ? {} : JSON.parse(configText);
      if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
        setError(t('remote.agentEdit.configInvalid'));
        return;
      }
      config = parsed as Record<string, unknown>;
    } catch {
      setError(t('remote.agentEdit.configInvalid'));
      return;
    }
    setError(null);
    void onSubmit({ name: trimmedName, type: type.trim(), config }).catch(
      (err: unknown) => {
        setError(submitErrorMessage(err, t));
      },
    );
  };

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      data-testid="remote-agent-edit-dialog"
    >
      <div
        className="absolute inset-0 bg-black/40"
        aria-hidden="true"
        onClick={() => {
          if (!pending) onCancel();
        }}
      />

      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="remote-agent-edit-title"
        onKeyDown={handleKeyDown}
        className="relative z-10 w-full max-w-lg rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-6 shadow-xl"
      >
        <h2
          id="remote-agent-edit-title"
          className="text-lg font-semibold text-(--color-text-primary)"
        >
          {mode === 'create'
            ? t('remote.agentEdit.createTitle')
            : t('remote.agentEdit.updateTitle')}
        </h2>
        <p className="mt-2 text-sm text-(--color-text-muted)">
          {t('remote.agentEdit.desc')}
        </p>

        <form className="mt-4 space-y-4" onSubmit={handleSubmit}>
          <div>
            <label
              htmlFor="remote-agent-name"
              className="block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('remote.agentEdit.nameLabel')}
            </label>
            <input
              ref={firstInputRef}
              id="remote-agent-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={pending}
              data-testid="remote-agent-name"
              className="mt-1 w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) px-3 py-2 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
            />
          </div>

          <div>
            <label
              htmlFor="remote-agent-type"
              className="block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('remote.agentEdit.typeLabel')}
            </label>
            <input
              id="remote-agent-type"
              type="text"
              value={type}
              onChange={(e) => setType(e.target.value)}
              // 수정 모드에서는 종류 변경을 허용하지 않는다(읽기 전용).
              disabled={pending || mode === 'update'}
              placeholder={t('remote.agentEdit.typePlaceholder')}
              data-testid="remote-agent-type"
              className="mt-1 w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) px-3 py-2 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
            />
          </div>

          <div>
            <label
              htmlFor="remote-agent-config"
              className="block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('remote.agentEdit.configLabel')}
            </label>
            <p className="mt-0.5 text-xs text-(--color-text-muted)">
              {t('remote.agentEdit.configHint')}
            </p>
            <textarea
              id="remote-agent-config"
              value={configText}
              onChange={(e) => setConfigText(e.target.value)}
              disabled={pending}
              rows={10}
              spellCheck={false}
              data-testid="remote-agent-config"
              className="mt-1 w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) px-3 py-2 font-mono text-xs text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
            />
          </div>

          {error && (
            <p
              className="text-sm text-red-600 dark:text-red-400"
              data-testid="remote-agent-edit-error"
              role="alert"
            >
              {error}
            </p>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onCancel}
              disabled={pending}
              className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-50"
            >
              {t('common.cancel')}
            </button>
            <button
              type="submit"
              disabled={pending}
              data-testid="remote-agent-edit-submit"
              className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              {pending && <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />}
              {t('remote.agentEdit.save')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

/**
 * 제출 에러를 사용자 친화 메시지로 변환한다(REQ-I11 의미별 매핑).
 */
function submitErrorMessage(err: unknown, t: (k: string) => string): string {
  const status =
    err && typeof err === 'object' && 'status' in err
      ? (err as { status: unknown }).status
      : undefined;
  if (typeof status === 'number') {
    switch (status) {
      case 503:
        return t('remote.edit.errorUnavailable');
      case 504:
        return t('remote.edit.errorTimeout');
      case 502:
        return t('remote.edit.errorFailed');
      case 404:
        return t('remote.edit.errorNotExposed');
      default:
        break;
    }
  }
  return t('remote.edit.errorGeneric');
}
