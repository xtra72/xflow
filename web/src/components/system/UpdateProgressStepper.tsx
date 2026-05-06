// SPEC-WEB-006 v0.1.0 (M6) — 9-state machine 진행 시각화 컴포넌트.
//
// 백엔드의 OperationStatus 9종 (idle | starting | checking | downloading |
// verifying | applying | ready_to_restart | completed | failed) 을 4개의
// 사용자 가시 단계 (checking → downloading → verifying → applying) 로 매핑하여
// horizontal stepper 형태로 시각화한다.
//
// 매핑 규칙:
//   - idle                  → 모든 단계 pending
//   - starting              → checking 단계 active (작업 시작 직후)
//   - checking              → checking 단계 active
//   - downloading           → checking complete + downloading active
//   - verifying             → 1-2 complete + verifying active
//   - applying              → 1-3 complete + applying active
//   - ready_to_restart      → 모든 단계 complete (재시작 대기)
//   - completed             → 모든 단계 complete
//   - failed                → 마지막 active 단계가 실패 표시 (정확한 위치 미상이면 최후)
//
// 본 컴포넌트는 순수 표시(presentational) 컴포넌트이며 hook 의존성이 없다.
//
// @spec SPEC-WEB-006 v0.1.0 (M6)

import { Check, Loader2, X } from 'lucide-react';

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

/** 사용자 가시 단계 — 4개로 압축 (checking/downloading/verifying/applying). */
type StepKey = 'checking' | 'downloading' | 'verifying' | 'applying';

/** 단계의 시각 상태. */
type StepState = 'pending' | 'active' | 'complete' | 'failed';

interface StepDefinition {
  key: StepKey;
  label: string;
}

const STEPS: ReadonlyArray<StepDefinition> = [
  { key: 'checking', label: '확인' },
  { key: 'downloading', label: '다운로드' },
  { key: 'verifying', label: '검증' },
  { key: 'applying', label: '적용' },
];

// ─────────────────────────────────────────────────────────────────────
// 매핑 로직
// ─────────────────────────────────────────────────────────────────────

/**
 * OperationStatus 의 단계 인덱스를 반환한다.
 *   - idle               → -1 (모든 단계 pending)
 *   - starting/checking  → 0
 *   - downloading        → 1
 *   - verifying          → 2
 *   - applying           → 3
 *   - ready_to_restart   → 4 (4 이상은 모든 단계 complete)
 *   - completed          → 4
 *   - failed             → 마지막 알려진 위치 미상이므로 -2 sentinel.
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
    case 'completed':
      return STEPS.length;
    case 'failed':
      return -2;
  }
}

/** 단계의 시각 상태를 결정한다. */
function deriveStepState(
  stepIdx: number,
  currentIdx: number,
  isFailed: boolean,
): StepState {
  if (isFailed) {
    // 정확한 실패 단계가 알려지지 않았으므로 마지막 단계만 failed 처리.
    if (stepIdx === STEPS.length - 1) return 'failed';
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
  const isFailed = currentStatus === 'failed';
  const currentIdx = statusToStepIndex(currentStatus);

  return (
    <div
      data-testid="update-progress-stepper"
      data-status={currentStatus}
      className="space-y-2"
    >
      <ol
        className="flex items-center gap-1"
        aria-label="업데이트 진행 단계"
      >
        {STEPS.map((step, idx) => {
          const state = deriveStepState(idx, currentIdx, isFailed);
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
                  {step.label}
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
        aria-label="완료"
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
        aria-label="진행 중"
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
        aria-label="실패"
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
      aria-label="대기"
    >
      {index + 1}
    </span>
  );
}

export default UpdateProgressStepper;
