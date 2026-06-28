// SPEC-WEB-006 v0.1.0 (M6) — 11-state machine 진행 시각화 컴포넌트.
// SPEC-UPDATE-002 v0.1.0 (M-1) — restarting + health_checking 단계 추가.
//
// 백엔드의 OperationStatus 11종 (idle | starting | checking | downloading |
// verifying | applying | ready_to_restart | restarting | health_checking |
// completed | failed) 을 6개의 사용자 가시 단계 (checking → downloading →
// verifying → applying → restarting → health_checking) 로 매핑하여 horizontal
// stepper 형태로 시각화한다.
//
// 매핑 규칙:
//   - idle                  → 모든 단계 pending
//   - starting              → checking 단계 active (작업 시작 직후)
//   - checking              → checking 단계 active
//   - downloading           → checking complete + downloading active
//   - verifying             → 1-2 complete + verifying active
//   - applying              → 1-3 complete + applying active
//   - ready_to_restart      → 1-4 complete + 5-6 pending (v0.1.0 호환 흐름 종료)
//   - restarting            → 1-4 complete + restarting active (auto_restart=true)
//   - health_checking       → 1-5 complete + health_checking active (auto_restart=true)
//   - completed             → 모든 단계 complete (auto_restart 흐름 종결)
//   - failed                → 마지막 active 단계가 실패 표시 (정확한 위치 미상이면 최후)
//
// v0.1.0 backward compat: auto_restart 미지정 흐름은 ready_to_restart 에서
// 운영자 수동 재시작을 기다리며 5-6 단계는 pending 으로 남는다.
//
// 본 컴포넌트는 순수 표시(presentational) 컴포넌트이며 hook 의존성이 없다.
//
// @spec SPEC-WEB-006 v0.1.0 (M6)
// @spec SPEC-UPDATE-002 v0.1.0 (M-1)

import { Check, Loader2, X } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import type { OperationStatus } from '@/services/api/systemUpdate';

// ─────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────

export interface UpdateProgressStepperProps {
  /** 현재 백엔드 작업 상태. */
  currentStatus: OperationStatus;
  /** 디버깅용 operation_id (선택). */
  operationId?: string;
}

/**
 * 사용자 가시 단계 — 6개로 압축.
 *
 * v0.1.0: checking/downloading/verifying/applying.
 * v0.2.0 (SPEC-UPDATE-002 M-1): restarting/health_checking 추가.
 */
type StepKey =
  | 'checking'
  | 'downloading'
  | 'verifying'
  | 'applying'
  | 'restarting'
  | 'health_checking';

/** 단계의 시각 상태. */
type StepState = 'pending' | 'active' | 'complete' | 'failed';

interface StepDefinition {
  key: StepKey;
  /** 라벨 i18n 키. 렌더 시 t(labelKey) 로 변환한다. */
  labelKey: string;
}

const STEPS: ReadonlyArray<StepDefinition> = [
  { key: 'checking', labelKey: 'system.update.stepper.checking' },
  { key: 'downloading', labelKey: 'system.update.stepper.downloading' },
  { key: 'verifying', labelKey: 'system.update.stepper.verifying' },
  { key: 'applying', labelKey: 'system.update.stepper.applying' },
  { key: 'restarting', labelKey: 'system.update.stepper.restarting' },
  { key: 'health_checking', labelKey: 'system.update.stepper.health_checking' },
];

// applying 단계의 인덱스 — ready_to_restart 매핑 시 사용 (1-4 complete 후 정지).
const APPLYING_INDEX = 3;

// ─────────────────────────────────────────────────────────────────────
// 매핑 로직
// ─────────────────────────────────────────────────────────────────────

/**
 * OperationStatus 의 단계 인덱스를 반환한다.
 *
 * v0.1.0 매핑:
 *   - idle               → -1 (모든 단계 pending)
 *   - starting/checking  → 0
 *   - downloading        → 1
 *   - verifying          → 2
 *   - applying           → 3
 *   - ready_to_restart   → APPLYING_INDEX + 1 (1-4 complete, 5-6 pending; v0.1.0 흐름 종결)
 *   - completed          → STEPS.length (모든 단계 complete; auto_restart 흐름 종결)
 *   - failed             → 마지막 알려진 위치 미상이므로 -2 sentinel.
 *
 * v0.2.0 신규 매핑 (SPEC-UPDATE-002 M-1):
 *   - restarting        → 4 (atomic replace + drain + syscall.Exec)
 *   - health_checking   → 5 (자가 health probe)
 *
 * 핵심 구분: `ready_to_restart` 는 v0.1.0 backward compat 종결점으로 5-6
 * 단계는 미진입(pending) 으로 남기지만, `completed` 는 auto_restart 흐름이
 * 정상 종결되어 모든 단계가 complete 다.
 *
 * @spec SPEC-UPDATE-002 v0.1.0 (M-1, M14)
 */
function statusToStepIndex(status: OperationStatus): number {
  switch (status) {
    case 'idle':
      return -1;
    case 'starting':
    case 'checking':
      return 0;
    case 'downloading':
      return 1;
    case 'verifying':
      return 2;
    case 'applying':
      return 3;
    case 'ready_to_restart':
      // v0.1.0 backward compat: 1-4 complete, 5-6 pending.
      return APPLYING_INDEX + 1;
    case 'restarting':
      return 4;
    case 'health_checking':
      return 5;
    case 'completed':
      return STEPS.length;
    case 'failed':
      return -2;
  }
}

/**
 * 단계의 시각 상태를 결정한다.
 *
 * 특수 케이스:
 *   - `ready_to_restart` 는 v0.1.0 흐름 종료점이므로 applying(=stepIdx 3) 까지
 *     complete + 5-6 단계는 pending (active 없음). 이 케이스는 호출자가
 *     `currentIdx = APPLYING_INDEX + 1` 를 전달하지만 자체적으로는 active 가
 *     없는 종료 상태이므로 아래 isReadyToRestart 플래그로 처리한다.
 *
 * @spec SPEC-UPDATE-002 v0.1.0 (M-1)
 */
function deriveStepState(
  stepIdx: number,
  currentIdx: number,
  isFailed: boolean,
  isReadyToRestart: boolean,
): StepState {
  if (isFailed) {
    // 정확한 실패 단계가 알려지지 않았으므로 마지막 단계만 failed 처리.
    if (stepIdx === STEPS.length - 1) return 'failed';
    return 'pending';
  }
  if (isReadyToRestart) {
    // 1-4 complete (applying 까지 끝남), 5-6 미진입.
    if (stepIdx <= APPLYING_INDEX) return 'complete';
    return 'pending';
  }
  if (currentIdx === -1) return 'pending';
  if (stepIdx < currentIdx) return 'complete';
  if (stepIdx === currentIdx) return 'active';
  return 'pending';
}

// ─────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────

export function UpdateProgressStepper({
  currentStatus,
  operationId,
}: UpdateProgressStepperProps) {
  const { t } = useTranslation();
  const isFailed = currentStatus === 'failed';
  const isReadyToRestart = currentStatus === 'ready_to_restart';
  const currentIdx = statusToStepIndex(currentStatus);

  return (
    <div
      data-testid="update-progress-stepper"
      data-status={currentStatus}
      className="space-y-2"
    >
      <ol
        className="flex items-center gap-1"
        aria-label={t('system.update.progressAria')}
      >
        {STEPS.map((step, idx) => {
          const state = deriveStepState(
            idx,
            currentIdx,
            isFailed,
            isReadyToRestart,
          );
          const isLast = idx === STEPS.length - 1;
          return (
            <li
              key={step.key}
              data-testid={`stepper-step-${step.key}`}
              data-state={state}
              className="flex flex-1 items-center gap-1"
            >
              <div className="flex flex-col items-center gap-1">
                <StepBadge state={state} index={idx} />
                <span
                  className={cn(
                    'text-[10px] font-medium',
                    state === 'pending' &&
                      'text-(--color-text-muted)',
                    state === 'active' && 'text-blue-600 dark:text-blue-300',
                    state === 'complete' &&
                      'text-emerald-600 dark:text-emerald-300',
                    state === 'failed' && 'text-red-600 dark:text-red-300',
                  )}
                >
                  {t(step.labelKey)}
                </span>
              </div>
              {!isLast ? (
                <div
                  className={cn(
                    'h-0.5 flex-1 rounded',
                    state === 'complete'
                      ? 'bg-emerald-300 dark:bg-emerald-600'
                      : 'bg-(--color-border)',
                  )}
                  aria-hidden="true"
                />
              ) : null}
            </li>
          );
        })}
      </ol>

      {operationId ? (
        <p
          data-testid="stepper-operation-id"
          className="text-[11px] font-mono text-(--color-text-muted)"
        >
          operation_id: {operationId}
        </p>
      ) : null}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Sub-component: 단계 배지 (원형 + 아이콘 또는 인덱스 숫자)
// ─────────────────────────────────────────────────────────────────────

function StepBadge({ state, index }: { state: StepState; index: number }) {
  const { t } = useTranslation();
  const base =
    'inline-flex h-6 w-6 items-center justify-center rounded-full border text-[11px] font-mono';

  if (state === 'complete') {
    return (
      <span
        className={cn(
          base,
          'border-emerald-400 bg-emerald-100 text-emerald-700',
          'dark:border-emerald-600 dark:bg-emerald-900 dark:text-emerald-200',
        )}
        aria-label={t('system.update.stepper.complete')}
      >
        <Check className="h-3 w-3" aria-hidden="true" />
      </span>
    );
  }
  if (state === 'active') {
    return (
      <span
        className={cn(
          base,
          'border-blue-400 bg-blue-100 text-blue-700',
          'dark:border-blue-600 dark:bg-blue-900 dark:text-blue-200',
        )}
        aria-label={t('system.update.stepper.active')}
      >
        <Loader2 className="h-3 w-3 animate-spin" aria-hidden="true" />
      </span>
    );
  }
  if (state === 'failed') {
    return (
      <span
        className={cn(
          base,
          'border-red-400 bg-red-100 text-red-700',
          'dark:border-red-600 dark:bg-red-900 dark:text-red-200',
        )}
        aria-label={t('system.update.stepper.failed')}
      >
        <X className="h-3 w-3" aria-hidden="true" />
      </span>
    );
  }
  // pending
  return (
    <span
      className={cn(
        base,
        'border-(--color-border) bg-(--color-bg-base) text-(--color-text-muted)',
      )}
      aria-label={t('system.update.stepper.pending')}
    >
      {index + 1}
    </span>
  );
}

export default UpdateProgressStepper;
