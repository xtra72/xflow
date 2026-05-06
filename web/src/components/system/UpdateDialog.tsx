// SPEC-WEB-006 v0.1.0 (M5, M6, M7, M8, M9) — Update Dialog 5-step UX.
//
// 단일 모달이지만 5개의 사용자 단계를 가진 stateful flow 를 제공한다.
//
//   1. info       — 현재 → 최신 버전 정보 + 채널 + Cancel/Next
//   2. confirm    — 경고 + 다운그레이드 force 옵션 + Back/Apply
//   3. apply      — useUpdateApply.mutate 트리거 (transient 로딩 화면)
//   4. progress   — useUpdateStatus 1초 폴링 + UpdateProgressStepper 시각화
//   5. result     — terminal status 별 3-variant (성공/재시작 필요/실패)
//
// 단계 전이 규칙:
//   - confirm 의 "업데이트 적용" → apply 단계 → mutation 결과 분기
//        success → progress (operation_id 보유)
//        error   → result (failure)
//   - progress 의 polling 결과 → result
//        completed         → result success
//        ready_to_restart  → result restart-required
//        failed            → result failure
//
// 다운그레이드 감지: latest_version 이 현재 version 보다 작거나 같으면 force
// 체크박스를 노출. version 비교는 단순 lexical 비교가 아니라 semver 우선순위
// (major.minor.patch + prerelease) 를 따라야 하지만 현재 fixture 한정 lexical
// 비교로 충분하다 (Phase E 에서 보강 가능).
//
// @spec SPEC-WEB-006 v0.1.0 (M5, M6, M7, M8, M9)

import { useCallback, useEffect, useMemo, useState } from 'react';
import { AlertTriangle, CheckCircle2, Loader2, X, XCircle } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { mapUpdateError } from '@/lib/errors/updaterErrorMapper';
import {
  useUpdateApply,
  useUpdateRollback,
  useUpdateStatus,
  type ApplyRequest,
  type ApplyResponse,
  type OperationStatus,
  type VersionInfo,
} from '@/services/api/systemUpdate';

import { RestartGuide } from './RestartGuide';
import { UpdateProgressStepper } from './UpdateProgressStepper';

// ─────────────────────────────────────────────────────────────────────
// Public types
// ─────────────────────────────────────────────────────────────────────

export interface UpdateDialogProps {
  /** 다이얼로그 표시 여부. */
  open: boolean;
  /** 닫기 콜백 (Cancel / Esc / 닫기 버튼 / 결과 닫기 모두 호출). */
  onClose: () => void;
  /** 현재 시스템 버전 정보 (백엔드 GET /system/version 결과). */
  version: VersionInfo;
}

/** 5단계 UX state machine. */
type DialogStep = 'info' | 'confirm' | 'apply' | 'progress' | 'result';

/** result 단계의 variant. */
type ResultVariant = 'success' | 'restart' | 'failure';

// ─────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────

/**
 * 다운그레이드 여부 판단.
 *
 * latest_version 이 null 이거나 current 와 동일하면 false.
 * 그 외에는 lexical 비교로 latest < current 인 경우 다운그레이드로 본다.
 *
 * 한계: '0.10.0' < '0.9.0' 같은 케이스는 lexical 비교에서 잘못 판정되지만,
 * Phase D 범위에서는 fixture 가 단순한 0.x 패치라 충분하다. 정밀 비교는 향후
 * semver 라이브러리 도입 시 보강.
 */
function isDowngrade(version: VersionInfo): boolean {
  if (!version.latest_version) return false;
  if (version.latest_version === version.version) return false;
  return version.latest_version < version.version;
}

/** terminal status → result variant 매핑. */
function statusToVariant(status: OperationStatus): ResultVariant | null {
  if (status === 'completed') return 'success';
  if (status === 'ready_to_restart') return 'restart';
  if (status === 'failed') return 'failure';
  return null;
}

// ─────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────

export function UpdateDialog({ open, onClose, version }: UpdateDialogProps) {
  // 5-step UI state.
  const [step, setStep] = useState<DialogStep>('info');
  // 다운그레이드 force 토글 (confirm 단계에서만 의미 있음).
  const [forceDowngrade, setForceDowngrade] = useState(false);
  // 작업 진입 후 보유한 operation_id — useUpdateStatus 활성화 트리거.
  const [operationId, setOperationId] = useState<string | null>(null);
  // mutate 실패 시의 에러 (apply step → result step 전환에 사용).
  const [applyError, setApplyError] = useState<unknown>(null);

  // Phase A 훅들. mutate 호출자는 단순한 콜백이며 React Query 가 isPending 등을 관리.
  const applyMutation = useUpdateApply();
  const rollbackMutation = useUpdateRollback();
  // status 쿼리는 operationId 가 있을 때만 폴링 활성.
  const statusQuery = useUpdateStatus({ enabled: operationId !== null });

  const downgrade = useMemo(() => isDowngrade(version), [version]);

  // 다이얼로그가 열릴 때마다 초기화 — 닫혔다 다시 열어도 깨끗한 상태로 시작.
  useEffect(() => {
    if (open) {
      setStep('info');
      setForceDowngrade(false);
      setOperationId(null);
      setApplyError(null);
    }
  }, [open]);

  // progress 단계에서 status terminal → result 단계로 자동 전환.
  useEffect(() => {
    if (step !== 'progress') return;
    const status = statusQuery.data?.status;
    if (!status) return;
    const variant = statusToVariant(status);
    if (variant) {
      setStep('result');
    }
  }, [step, statusQuery.data?.status]);

  // 결정된 result variant — apply 에러 우선, 다음 폴링 status.
  const resultVariant: ResultVariant | null = useMemo(() => {
    if (applyError) return 'failure';
    const status = statusQuery.data?.status;
    if (!status) return null;
    return statusToVariant(status);
  }, [applyError, statusQuery.data?.status]);

  // Esc 키 — info/confirm/result 단계에서만 닫는다. progress 는 명시적 닫기 버튼만 허용.
  useEffect(() => {
    if (!open) return;
    const handler = (e: globalThis.KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      if (step === 'progress' || step === 'apply') return;
      onClose();
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [open, onClose, step]);

  // 적용 트리거 핸들러.
  const handleApply = useCallback(() => {
    setApplyError(null);
    setStep('apply');
    const req: ApplyRequest = {};
    if (version.latest_version) {
      req.version = version.latest_version;
    }
    if (downgrade && forceDowngrade) {
      req.force = true;
    }
    applyMutation.mutate(req, {
      onSuccess: (data: ApplyResponse) => {
        setOperationId(data.operation_id);
        setStep('progress');
      },
      onError: (err: Error) => {
        setApplyError(err);
        setStep('result');
      },
    });
  }, [applyMutation, downgrade, forceDowngrade, version.latest_version]);

  // 다시 시도 — info 단계로 리셋 (mutation reset 호출 후 깨끗한 상태 보장).
  const handleRetry = useCallback(() => {
    applyMutation.reset();
    setApplyError(null);
    setOperationId(null);
    setStep('info');
    setForceDowngrade(false);
  }, [applyMutation]);

  // Rollback — useUpdateRollback mutation 트리거. 결과는 toast 등으로 부모가 처리할 수도 있으나
  // Phase D 에서는 단순 발사 후 dialog 는 그대로 둔다 (사용자가 close 결정).
  const handleRollback = useCallback(() => {
    rollbackMutation.mutate();
  }, [rollbackMutation]);

  // backdrop 클릭 닫기 — info/confirm/result 만 허용.
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target !== e.currentTarget) return;
      if (step === 'progress' || step === 'apply') return;
      onClose();
    },
    [onClose, step],
  );

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="update-dialog-title"
      data-testid="update-dialog"
      onClick={handleBackdropClick}
    >
      <div
        className={cn(
          'flex w-full max-w-[640px] flex-col rounded-lg shadow-xl',
          'bg-(--color-bg-surface) text-(--color-text-primary)',
        )}
        onClick={(e) => e.stopPropagation()}
      >
        {/* 헤더 */}
        <header className="flex items-center justify-between border-b border-(--color-border) px-5 py-3">
          <h2 id="update-dialog-title" className="text-base font-semibold">
            xflowd 업데이트
          </h2>
          {step !== 'progress' && step !== 'apply' ? (
            <button
              type="button"
              onClick={onClose}
              className="rounded-md p-1 text-(--color-text-muted) hover:bg-(--color-bg-elevated) hover:text-(--color-text-primary)"
              aria-label="닫기"
            >
              <X className="h-4 w-4" />
            </button>
          ) : null}
        </header>

        {/* 본문 — 단계별 분기 */}
        <div className="min-h-[180px] space-y-4 px-5 py-4">
          {step === 'info' ? (
            <InfoStep version={version} />
          ) : null}

          {step === 'confirm' ? (
            <ConfirmStep
              version={version}
              downgrade={downgrade}
              forceDowngrade={forceDowngrade}
              onForceToggle={() => setForceDowngrade((v) => !v)}
            />
          ) : null}

          {step === 'apply' ? <ApplyStep /> : null}

          {step === 'progress' && operationId ? (
            <ProgressStep
              status={statusQuery.data?.status ?? 'starting'}
              operationId={operationId}
            />
          ) : null}

          {step === 'result' && resultVariant ? (
            <div data-testid="update-dialog-step-result">
              <ResultStep
                variant={resultVariant}
                error={applyError ?? statusQuery.data?.error}
                operationId={operationId ?? undefined}
              />
            </div>
          ) : null}
        </div>

        {/* 푸터 — 단계별 액션 버튼 */}
        <footer className="flex items-center justify-end gap-2 border-t border-(--color-border) px-5 py-3">
          {step === 'info' ? (
            <>
              <FooterButton
                testId="update-dialog-cancel"
                variant="secondary"
                onClick={onClose}
              >
                취소
              </FooterButton>
              <FooterButton
                testId="update-dialog-next"
                variant="primary"
                onClick={() => setStep('confirm')}
              >
                다음
              </FooterButton>
            </>
          ) : null}

          {step === 'confirm' ? (
            <>
              <FooterButton
                testId="update-dialog-back"
                variant="secondary"
                onClick={() => setStep('info')}
              >
                이전
              </FooterButton>
              <FooterButton
                testId="update-dialog-apply"
                variant="primary"
                onClick={handleApply}
                disabled={downgrade && !forceDowngrade}
              >
                업데이트 적용
              </FooterButton>
            </>
          ) : null}

          {step === 'progress' ? (
            <FooterButton
              testId="update-dialog-progress-close"
              variant="secondary"
              onClick={onClose}
            >
              닫기 (백그라운드 계속)
            </FooterButton>
          ) : null}

          {step === 'result' && resultVariant ? (
            <ResultFooter
              variant={resultVariant}
              onClose={onClose}
              onRetry={handleRetry}
              onRollback={handleRollback}
              rollbackPending={rollbackMutation.isPending}
            />
          ) : null}
        </footer>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Step 1: info
// ─────────────────────────────────────────────────────────────────────

function InfoStep({ version }: { version: VersionInfo }) {
  return (
    <div data-testid="update-dialog-step-info" className="space-y-3 text-sm">
      <p className="text-(--color-text-muted)">
        새로운 xflowd 버전이 사용 가능합니다. 자세한 내용을 확인하고 적용 여부를
        결정하세요.
      </p>

      <dl className="grid grid-cols-2 gap-3 rounded border border-(--color-border) bg-(--color-bg-base) p-3 text-sm">
        <div>
          <dt className="text-xs uppercase tracking-wide text-(--color-text-muted)">
            현재 버전
          </dt>
          <dd className="mt-0.5 font-mono text-base font-semibold">
            {version.version}
          </dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wide text-(--color-text-muted)">
            대상 버전
          </dt>
          <dd className="mt-0.5 font-mono text-base font-semibold text-(--color-text-primary)">
            {version.latest_version ?? '(채널 최신)'}
          </dd>
        </div>
        <div className="col-span-2">
          <dt className="text-xs uppercase tracking-wide text-(--color-text-muted)">
            업데이트 채널
          </dt>
          <dd className="mt-0.5 font-mono">{version.channel}</dd>
        </div>
      </dl>

      <p className="text-xs text-(--color-text-muted)">
        다음 단계에서 다운그레이드 여부, 재시작 안내 등을 확인합니다.
      </p>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Step 2: confirm
// ─────────────────────────────────────────────────────────────────────

function ConfirmStep({
  version,
  downgrade,
  forceDowngrade,
  onForceToggle,
}: {
  version: VersionInfo;
  downgrade: boolean;
  forceDowngrade: boolean;
  onForceToggle: () => void;
}) {
  return (
    <div data-testid="update-dialog-step-confirm" className="space-y-3 text-sm">
      <div
        className={cn(
          'rounded-md border p-3',
          'border-yellow-300 bg-yellow-50 text-yellow-900',
          'dark:border-yellow-700 dark:bg-yellow-950 dark:text-yellow-200',
        )}
      >
        <div className="flex items-start gap-2">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <div className="space-y-1 text-xs leading-relaxed">
            <p className="font-medium">업데이트 적용 시 재시작이 필요합니다.</p>
            <p>
              백엔드 v0.1.0 한계로 in-process 재시작이 지원되지 않으며 운영자가
              직접 xflowd 를 재시작해야 합니다 (다음 단계에서 가이드 제공).
            </p>
            <p>
              실패 시 <span className="font-mono">Rollback</span> 으로 이전
              버전으로 즉시 되돌릴 수 있습니다.
            </p>
          </div>
        </div>
      </div>

      {downgrade ? (
        <label
          className={cn(
            'flex items-start gap-2 rounded-md border p-3 text-xs',
            'border-red-300 bg-red-50 text-red-900',
            'dark:border-red-700 dark:bg-red-950 dark:text-red-200',
          )}
        >
          <input
            type="checkbox"
            data-testid="update-dialog-force-checkbox"
            checked={forceDowngrade}
            onChange={onForceToggle}
            className="mt-0.5 h-3.5 w-3.5 shrink-0"
          />
          <span className="space-y-1">
            <span className="block font-medium">
              다운그레이드 강제 적용 (--force)
            </span>
            <span className="block">
              {version.latest_version} 은(는) 현재 {version.version} 보다 낮은
              버전입니다. 이 옵션을 활성화하지 않으면 백엔드가 거부합니다.
            </span>
          </span>
        </label>
      ) : null}

      <p className="text-xs text-(--color-text-muted)">
        대상 버전:{' '}
        <span className="font-mono">{version.latest_version ?? '(채널 최신)'}</span>
      </p>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Step 3: apply (transient)
// ─────────────────────────────────────────────────────────────────────

function ApplyStep() {
  return (
    <div
      data-testid="update-dialog-step-apply"
      className="flex flex-col items-center gap-3 py-6 text-sm text-(--color-text-muted)"
      aria-busy="true"
    >
      <Loader2 className="h-6 w-6 animate-spin" aria-hidden="true" />
      <p>업데이트 작업을 시작하는 중...</p>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Step 4: progress
// ─────────────────────────────────────────────────────────────────────

function ProgressStep({
  status,
  operationId,
}: {
  status: OperationStatus;
  operationId: string;
}) {
  return (
    <div
      data-testid="update-dialog-step-progress"
      className="space-y-3 text-sm"
    >
      <p className="text-(--color-text-muted)">
        업데이트가 진행 중입니다. 진행 상황은 1초마다 자동 갱신됩니다.
      </p>
      <UpdateProgressStepper
        currentStatus={status}
        operationId={operationId}
      />
      <StatusLabel status={status} />
    </div>
  );
}

function StatusLabel({ status }: { status: OperationStatus }) {
  const text: Record<OperationStatus, string> = {
    idle: '대기',
    starting: '작업 시작 중...',
    checking: '버전 확인 중...',
    downloading: '다운로드 중...',
    verifying: '검증 중...',
    applying: '적용 중...',
    ready_to_restart: '재시작 대기',
    completed: '완료',
    failed: '실패',
  };
  return (
    <p
      className="rounded border border-(--color-border) bg-(--color-bg-base) px-3 py-2 text-xs font-medium"
      aria-live="polite"
    >
      {text[status]}
    </p>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Step 5: result
// ─────────────────────────────────────────────────────────────────────

function ResultStep({
  variant,
  error,
  operationId,
}: {
  variant: ResultVariant;
  error?: unknown;
  operationId?: string;
}) {
  if (variant === 'success') {
    return (
      <div
        data-testid="update-dialog-result-success"
        className={cn(
          'space-y-2 rounded-md border p-4 text-sm',
          'border-emerald-300 bg-emerald-50 text-emerald-900',
          'dark:border-emerald-700 dark:bg-emerald-950 dark:text-emerald-200',
        )}
      >
        <div className="flex items-center gap-2">
          <CheckCircle2 className="h-5 w-5" aria-hidden="true" />
          <h3 className="text-sm font-semibold">업데이트 완료</h3>
        </div>
        <p className="text-xs leading-relaxed">
          xflowd 가 새 버전으로 업데이트되었습니다.
        </p>
        {operationId ? (
          <p className="font-mono text-[11px] opacity-70">
            operation_id: {operationId}
          </p>
        ) : null}
      </div>
    );
  }

  if (variant === 'restart') {
    return (
      <div
        data-testid="update-dialog-result-restart"
        className="space-y-2 text-sm"
      >
        <RestartGuide operationId={operationId} />
      </div>
    );
  }

  // failure
  const mapped = mapUpdateError(error);
  return (
    <div
      data-testid="update-dialog-result-failure"
      className={cn(
        'space-y-2 rounded-md border p-4 text-sm',
        'border-red-300 bg-red-50 text-red-900',
        'dark:border-red-700 dark:bg-red-950 dark:text-red-200',
      )}
    >
      <div className="flex items-center gap-2">
        <XCircle className="h-5 w-5" aria-hidden="true" />
        <h3 className="text-sm font-semibold">업데이트 실패</h3>
      </div>
      <p className="text-xs leading-relaxed">{mapped.userMessage}</p>
      {operationId ? (
        <p className="font-mono text-[11px] opacity-70">
          operation_id: {operationId}
        </p>
      ) : null}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Result footer (variant 별 버튼 구성)
// ─────────────────────────────────────────────────────────────────────

function ResultFooter({
  variant,
  onClose,
  onRetry,
  onRollback,
  rollbackPending,
}: {
  variant: ResultVariant;
  onClose: () => void;
  onRetry: () => void;
  onRollback: () => void;
  rollbackPending: boolean;
}) {
  if (variant === 'success' || variant === 'restart') {
    return (
      <FooterButton
        testId="update-dialog-result-close"
        variant="primary"
        onClick={onClose}
      >
        닫기
      </FooterButton>
    );
  }
  // failure
  return (
    <>
      <FooterButton
        testId="update-dialog-result-rollback"
        variant="secondary"
        onClick={onRollback}
        disabled={rollbackPending}
      >
        Rollback 시도
      </FooterButton>
      <FooterButton
        testId="update-dialog-result-retry"
        variant="secondary"
        onClick={onRetry}
      >
        다시 시도
      </FooterButton>
      <FooterButton
        testId="update-dialog-result-close"
        variant="primary"
        onClick={onClose}
      >
        닫기
      </FooterButton>
    </>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Footer button
// ─────────────────────────────────────────────────────────────────────

function FooterButton({
  testId,
  variant,
  onClick,
  disabled,
  children,
}: {
  testId: string;
  variant: 'primary' | 'secondary';
  onClick: () => void;
  disabled?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      data-testid={testId}
      onClick={onClick}
      disabled={disabled}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
        'disabled:cursor-not-allowed disabled:opacity-50',
        variant === 'primary'
          ? 'bg-yellow-500 text-white hover:bg-yellow-600 dark:bg-yellow-600 dark:hover:bg-yellow-700'
          : 'border border-(--color-border) bg-(--color-bg-base) text-(--color-text-primary) hover:bg-(--color-bg-hover)',
      )}
    >
      {children}
    </button>
  );
}

export default UpdateDialog;
