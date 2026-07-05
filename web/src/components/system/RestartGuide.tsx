// SPEC-WEB-006 v0.1.0 (M8) — 운영자 재시작 안내 컴포넌트.
//
// 백엔드의 9-state machine 이 `ready_to_restart` 상태에 도달하면 in-process
// 재시작이 불가하므로 운영자가 직접 xflowd 를 재시작해야 한다. 본 컴포넌트는
// systemd / manual 두 가지 CLI 명령어를 카드 형태로 제시하고, 클립보드 복사
// 버튼으로 운영자 액션을 단축한다.
//
// 외부 의존성 없이 navigator.clipboard 만 사용하며, 복사 후 3초 동안 inline
// 체크 표시("복사됨") 를 노출한다.
//
// @spec SPEC-WEB-006 v0.1.0 (M8)

import { useCallback, useEffect, useRef, useState } from 'react';
import { Check, Copy, RotateCw } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

// ─────────────────────────────────────────────────────────────────────
// Props
// ─────────────────────────────────────────────────────────────────────

export interface RestartGuideProps {
  /** 표시할 operation_id (재시작 후 디버깅 추적용). */
  operationId?: string;
}

// ─────────────────────────────────────────────────────────────────────
// CLI 명령어 정의 (사전 정의 순서가 화면 노출 순서)
// ─────────────────────────────────────────────────────────────────────

interface CommandEntry {
  /** 내부 식별자. data-testid 와 상태 키로 사용. */
  id: 'systemd' | 'manual';
  /** 사용자 노출 라벨 i18n 키. 렌더 시 t(labelKey). */
  labelKey: string;
  /** 실제 복사될 명령어 문자열. */
  command: string;
  /** 부가 설명 i18n 키 (선택). */
  hintKey?: string;
}

const COMMANDS: ReadonlyArray<CommandEntry> = [
  {
    id: 'systemd',
    labelKey: 'system.restart.systemdLabel',
    command: 'sudo systemctl restart xflowd',
    hintKey: 'system.restart.systemdHint',
  },
  {
    id: 'manual',
    labelKey: 'system.restart.manualLabel',
    command: 'sudo pkill xflowd && sudo xflowd start',
    hintKey: 'system.restart.manualHint',
  },
];

// 복사 표시가 사라질 때까지의 시간 (ms).
const COPIED_BADGE_TTL_MS = 3000;

// ─────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────

export function RestartGuide({ operationId }: RestartGuideProps) {
  const { t } = useTranslation();
  // id → boolean 형태로 각 버튼의 "방금 복사됨" 상태를 독립적으로 추적한다.
  const [copiedMap, setCopiedMap] = useState<Record<string, boolean>>({});
  // 각 버튼의 reset timer (재클릭 시 이전 timer 취소).
  const timersRef = useRef<Record<string, ReturnType<typeof setTimeout>>>({});

  // unmount 시 모든 타이머 정리 — 리액트 unmount race 방지.
  useEffect(() => {
    const timers = timersRef.current;
    return () => {
      for (const t of Object.values(timers)) {
        clearTimeout(t);
      }
    };
  }, []);

  const handleCopy = useCallback(async (entry: CommandEntry) => {
    try {
      await navigator.clipboard.writeText(entry.command);
    } catch {
      // 복사 실패는 silent — 사용자가 수동 선택할 수 있도록 명령어는 화면에 그대로 노출됨.
      return;
    }
    // 이전 타이머 취소 후 새 타이머 등록.
    const prev = timersRef.current[entry.id];
    if (prev) clearTimeout(prev);
    setCopiedMap((m) => ({ ...m, [entry.id]: true }));
    timersRef.current[entry.id] = setTimeout(() => {
      setCopiedMap((m) => {
        const next = { ...m };
        delete next[entry.id];
        return next;
      });
      delete timersRef.current[entry.id];
    }, COPIED_BADGE_TTL_MS);
  }, []);

  return (
    <div
      data-testid="restart-guide"
      className={cn(
        'space-y-3 rounded-md border p-4',
        'border-yellow-300 bg-yellow-50 text-yellow-900',
        'dark:border-yellow-700 dark:bg-yellow-950 dark:text-yellow-200',
      )}
    >
      <header className="flex items-center gap-2">
        <RotateCw className="h-4 w-4 shrink-0" aria-hidden="true" />
        <h3
          data-testid="restart-guide-title"
          className="text-sm font-semibold"
        >
          {t('system.restart.title')}
        </h3>
      </header>

      <p className="text-xs leading-relaxed">{t('system.restart.desc')}</p>

      <ul className="space-y-2">
        {COMMANDS.map((cmd) => {
          const copied = copiedMap[cmd.id] === true;
          return (
            <li
              key={cmd.id}
              className={cn(
                'rounded border bg-white/60 p-2 text-xs',
                'border-yellow-200 dark:border-yellow-800 dark:bg-yellow-900/40',
              )}
            >
              <div className="mb-1 font-medium">{t(cmd.labelKey)}</div>
              <div className="flex items-center gap-1">
                <code
                  data-testid={`restart-guide-cmd-${cmd.id}`}
                  className={cn(
                    'flex-1 overflow-x-auto rounded px-2 py-1 font-mono text-[11px]',
                    'bg-yellow-100 text-yellow-900 dark:bg-yellow-800/40 dark:text-yellow-100',
                  )}
                >
                  {cmd.command}
                </code>
                <button
                  type="button"
                  data-testid={`restart-guide-copy-${cmd.id}`}
                  onClick={() => void handleCopy(cmd)}
                  className={cn(
                    'inline-flex h-7 w-7 shrink-0 items-center justify-center rounded',
                    'border border-yellow-300 bg-white text-yellow-700',
                    'hover:bg-yellow-50',
                    'dark:border-yellow-700 dark:bg-yellow-900 dark:text-yellow-200',
                    'dark:hover:bg-yellow-800',
                  )}
                  aria-label={t('system.restart.copyAria').replace(
                    '{label}',
                    t(cmd.labelKey),
                  )}
                >
                  <Copy className="h-3.5 w-3.5" aria-hidden="true" />
                </button>
                {copied ? (
                  <span
                    data-testid={`restart-guide-copied-${cmd.id}`}
                    className={cn(
                      'inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium',
                      'bg-emerald-100 text-emerald-700',
                      'dark:bg-emerald-900 dark:text-emerald-200',
                    )}
                  >
                    <Check className="h-3 w-3" aria-hidden="true" />
                    {t('system.restart.copied')}
                  </span>
                ) : null}
              </div>
              {cmd.hintKey ? (
                <p className="mt-1 text-[11px] opacity-80">{t(cmd.hintKey)}</p>
              ) : null}
            </li>
          );
        })}
      </ul>

      {operationId ? (
        <p
          data-testid="restart-guide-operation-id"
          className="text-[11px] font-mono opacity-70"
        >
          operation_id: {operationId}
        </p>
      ) : null}
    </div>
  );
}

export default RestartGuide;
