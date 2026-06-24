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

import { useTranslation } from '@/lib/i18n';
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
  /**
   * v0.2.0 (M9): admin 사용자 여부.
   * true 일 때 confirm step 에 target dropdown + auto_restart checkbox 노출.
   * default false → v0.1.0 호환 (xflowd + auto_restart=false 만 사용).
   *
   * @spec SPEC-UPDATE-002 v0.1.0 (M9, M14)
   */
  isAdmin?: boolean;
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

export function UpdateDialog({ open, onClose, version, isAdmin = false }: UpdateDialogProps) {
  const { t } = useTranslation();
  // v0.2.0 (M9, M14): isAdmin 미지정 시 false (v0.1.0 호환).
  // 향후 confirm step 의 target dropdown + auto_restart checkbox 활성화에 사용.
  // 현재 phase E 에서는 prop 만 정의 + 통합 테스트 검증 가능 상태로 유지.
  // 5-step UI state.
  const [step, setStep] = useState<DialogStep>('info');
  // 다운그레이드 force 토글 (confirm 단계에서만 의미 있음).
  const [forceDowngrade, setForceDowngrade] = useState(false);
  // v0.2.0 (M-1): auto_restart 옵션 (default false, v0.1.0 호환).
  const [autoRestart, setAutoRestart] = useState(false);
  // v0.2.0 (M9): target 바이너리 (default xflowd, v0.1.0 호환).
  const [target, setTarget] = useState<'xflowd' | 'xflow-agent' | 'xflow'>('xflowd');
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
      setAutoRestart(false);
      setTarget('xflowd');
      setOperationId(null);
      setApplyError(null);
    }
  }, [open]);

  // v0.2.0 (Scenario 11): target=xflow 로 변경되면 auto_restart 자동 해제.
  // xflow CLI 는 one-shot 도구이므로 재시작 의미 없음.
  useEffect(() => {
    if (target === 'xflow' && autoRestart) {
      setAutoRestart(false);
    }
  }, [target, autoRestart]);

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
    // v0.2.0 (M9): target 이 default(xflowd)가 아닐 때만 명시 — 백엔드 default 활용.
    if (target !== 'xflowd') {
      req.target = target;
    }
    // v0.2.0 (M-1, Scenario 11): auto_restart 는 체크된 경우에만 포함.
    // target=xflow 인 경우 useEffect 가 autoRestart 를 false 로 강제하므로 이 분기로
    // 자연스럽게 auto_restart 가 인자에서 빠진다.
    if (autoRestart && target !== 'xflow') {
      req.auto_restart = true;
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
  }, [applyMutation, autoRestart, downgrade, forceDowngrade, target, version.latest_version]);

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
            {t('system.update.title')}
          </h2>
          {step !== 'progress' && step !== 'apply' ? (
            <button
              type="button"
              onClick={onClose}
              className="rounded-md p-1 text-(--color-text-muted) hover:bg-(--color-bg-elevated) hover:text-(--color-text-primary)"
              aria-label={t('common.close')}
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
              isAdmin={isAdmin}
              autoRestart={autoRestart}
              onAutoRestartToggle={() => setAutoRestart((v) => !v)}
              target={target}
              onTargetChange={setTarget}
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
                {t('common.cancel')}
              </FooterButton>
              <FooterButton
                testId="update-dialog-next"
                variant="primary"
                onClick={() => setStep('confirm')}
              >
                {t('system.update.next')}
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
                {t('system.update.back')}
              </FooterButton>
              <FooterButton
                testId="update-dialog-apply"
                variant="primary"
                onClick={handleApply}
                disabled={downgrade && !forceDowngrade}
              >
                {t('system.update.apply')}
              </FooterButton>
            </>
          ) : null}

          {step === 'progress' ? (
            <FooterButton
              testId="update-dialog-progress-close"
              variant="secondary"
              onClick={onClose}
            >
              {t('system.update.progressCloseBackground')}
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
  const { t } = useTranslation();
  return (
    <div data-testid="update-dialog-step-info" className="space-y-3 text-sm">
      <p className="text-(--color-text-muted)">{t('system.update.info.desc')}</p>

      <dl className="grid grid-cols-2 gap-3 rounded border border-(--color-border) bg-(--color-bg-base) p-3 text-sm">
        <div>
          <dt className="text-xs uppercase tracking-wide text-(--color-text-muted)">
            {t('system.update.info.currentVersion')}
          </dt>
          <dd className="mt-0.5 font-mono text-base font-semibold">
            {version.version}
          </dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wide text-(--color-text-muted)">
            {t('system.update.info.targetVersion')}
          </dt>
          <dd className="mt-0.5 font-mono text-base font-semibold text-(--color-text-primary)">
            {version.latest_version ?? t('system.update.info.channelLatest')}
          </dd>
        </div>
        <div className="col-span-2">
          <dt className="text-xs uppercase tracking-wide text-(--color-text-muted)">
            {t('system.update.info.channel')}
          </dt>
          <dd className="mt-0.5 font-mono">{version.channel}</dd>
        </div>
      </dl>

      <p className="text-xs text-(--color-text-muted)">
        {t('system.update.info.nextStepHint')}
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
  isAdmin,
  autoRestart,
  onAutoRestartToggle,
  target,
  onTargetChange,
}: {
  version: VersionInfo;
  downgrade: boolean;
  forceDowngrade: boolean;
  onForceToggle: () => void;
  isAdmin: boolean;
  autoRestart: boolean;
  onAutoRestartToggle: () => void;
  target: 'xflowd' | 'xflow-agent' | 'xflow';
  onTargetChange: (value: 'xflowd' | 'xflow-agent' | 'xflow') => void;
}) {
  const { t } = useTranslation();
  // v0.2.0 (Scenario 11): xflow CLI 는 one-shot 도구라 auto_restart 의미 없음.
  const autoRestartDisabled = target === 'xflow';

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
            <p className="font-medium">{t('system.update.confirm.restartWarning')}</p>
            <p>
              {t('system.update.confirm.autoRestartLinePre')}{' '}
              <span className="font-mono">
                {t('system.update.confirm.autoRestartTerm')}
              </span>{' '}
              {t('system.update.confirm.autoRestartLinePost')}
            </p>
            <p>
              {t('system.update.confirm.rollbackLinePre')}{' '}
              <span className="font-mono">
                {t('system.update.confirm.rollbackTerm')}
              </span>{' '}
              {t('system.update.confirm.rollbackLinePost')}
            </p>
          </div>
        </div>
      </div>

      {/* v0.2.0 (M9): admin 전용 target dropdown — xflowd / xflow-agent / xflow 선택. */}
      {isAdmin ? (
        <label className="flex items-center gap-2 rounded-md border border-(--color-border) bg-(--color-bg-base) p-3 text-xs">
          <span className="font-medium text-(--color-text-primary)">
            {t('system.update.confirm.targetLabel')}
          </span>
          <select
            data-testid="update-dialog-target-select"
            value={target}
            onChange={(e) =>
              onTargetChange(
                e.target.value as 'xflowd' | 'xflow-agent' | 'xflow',
              )
            }
            className={cn(
              'rounded-md border border-(--color-border) bg-(--color-bg-surface) px-2 py-1 text-xs',
              'text-(--color-text-primary)',
            )}
          >
            <option value="xflowd">
              {t('system.update.confirm.targetOptionXflowd')}
            </option>
            <option value="xflow-agent">
              {t('system.update.confirm.targetOptionAgent')}
            </option>
            <option value="xflow">
              {t('system.update.confirm.targetOptionXflow')}
            </option>
          </select>
        </label>
      ) : null}

      {/* v0.2.0 (M-1): auto_restart 체크박스 — 모든 사용자에게 노출. */}
      <label
        className={cn(
          'flex items-start gap-2 rounded-md border p-3 text-xs',
          'border-(--color-border) bg-(--color-bg-base) text-(--color-text-primary)',
          autoRestartDisabled && 'opacity-60',
        )}
      >
        <input
          type="checkbox"
          data-testid="update-dialog-auto-restart-checkbox"
          checked={autoRestart}
          onChange={onAutoRestartToggle}
          disabled={autoRestartDisabled}
          className="mt-0.5 h-3.5 w-3.5 shrink-0"
        />
        <span className="space-y-1">
          <span className="block font-medium">
            {t('system.update.confirm.autoRestartTitle')}
          </span>
          <span className="block text-(--color-text-muted)">
            {autoRestartDisabled
              ? t('system.update.confirm.autoRestartDisabledDesc')
              : t('system.update.confirm.autoRestartEnabledDesc')}
          </span>
        </span>
      </label>

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
              {t('system.update.confirm.forceTitle')}
            </span>
            <span className="block">
              {t('system.update.confirm.forceDesc')
                .replace('{latest}', version.latest_version ?? '')
                .replace('{current}', version.version)}
            </span>
          </span>
        </label>
      ) : null}

      <p className="text-xs text-(--color-text-muted)">
        {t('system.update.confirm.targetVersionLabel')}{' '}
        <span className="font-mono">
          {version.latest_version ?? t('system.update.info.channelLatest')}
        </span>
      </p>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Step 3: apply (transient)
// ─────────────────────────────────────────────────────────────────────

function ApplyStep() {
  const { t } = useTranslation();
  return (
    <div
      data-testid="update-dialog-step-apply"
      className="flex flex-col items-center gap-3 py-6 text-sm text-(--color-text-muted)"
      aria-busy="true"
    >
      <Loader2 className="h-6 w-6 animate-spin" aria-hidden="true" />
      <p>{t('system.update.apply_.starting')}</p>
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
  const { t } = useTranslation();
  return (
    <div
      data-testid="update-dialog-step-progress"
      className="space-y-3 text-sm"
    >
      <p className="text-(--color-text-muted)">
        {t('system.update.progress.running')}
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
  const { t } = useTranslation();
  // @spec SPEC-UPDATE-002 v0.1.0 (M1): 11-state machine — restarting / health_checking 신규.
  // 상태별 라벨은 system.update.statusLabel.<status> 키로 해석한다.
  return (
    <p
      className="rounded border border-(--color-border) bg-(--color-bg-base) px-3 py-2 text-xs font-medium"
      aria-live="polite"
    >
      {t(`system.update.statusLabel.${status}`)}
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
  const { t } = useTranslation();
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
          <h3 className="text-sm font-semibold">
            {t('system.update.result.successTitle')}
          </h3>
        </div>
        <p className="text-xs leading-relaxed">
          {t('system.update.result.successDesc')}
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
        <h3 className="text-sm font-semibold">
          {t('system.update.result.failureTitle')}
        </h3>
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
  const { t } = useTranslation();
  if (variant === 'success' || variant === 'restart') {
    return (
      <FooterButton
        testId="update-dialog-result-close"
        variant="primary"
        onClick={onClose}
      >
        {t('common.close')}
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
        {t('system.update.rollback')}
      </FooterButton>
      <FooterButton
        testId="update-dialog-result-retry"
        variant="secondary"
        onClick={onRetry}
      >
        {t('system.update.retry')}
      </FooterButton>
      <FooterButton
        testId="update-dialog-result-close"
        variant="primary"
        onClick={onClose}
      >
        {t('common.close')}
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
