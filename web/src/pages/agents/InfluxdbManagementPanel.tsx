// InfluxDB 에이전트의 bucket / measurement 관리 패널.
//
// AgentDetailPanel 의 '관리' 탭에서 렌더된다 (influxdb 타입 에이전트 한정).
//
// 기능:
//   - 버킷 목록 표 (name / retention / id)
//   - 버킷 생성 폼 (name + retention 선택)
//   - 버킷 삭제 (확인 다이얼로그)
//   - 버킷 초기화(truncate) (강한 확인 다이얼로그 — 데이터 전체 삭제 경고)
//   - 버킷 선택 시 measurement 목록 표시 + measurement 삭제 (확인)
//   - 로딩 / 에러 / 빈 상태 처리
//
// v3 에이전트는 관리 미지원 → 백엔드 501/400 → APIError.
// 이 경우 버킷 목록 조회가 실패하며, 미지원 안내 배너를 표시한다.

import { useCallback, useMemo, useState } from 'react';
import { AlertTriangle, Database, Info, Plus, RefreshCw, Table2, Trash2 } from 'lucide-react';

import { ConfirmDialog } from '@/components/property/ConfirmDialog';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { APIError } from '@/types/api';
import { useUIStore } from '@/stores/uiStore';
import type { InfluxBucket } from '@/services/api/influxdbManagement';
import {
  useBuckets,
  useCreateBucket,
  useDeleteBucket,
  useDeleteMeasurement,
  useMeasurements,
  useTruncateBucket,
} from '@/hooks/useInfluxdbManagement';

/** retention 프리셋 옵션 (초 단위). 0 = 무제한. */
const RETENTION_OPTIONS = [
  { seconds: 0, labelKey: 'agents.detail.influxdb.retentionInfinite' },
  { seconds: 3600, labelKey: 'agents.detail.influxdb.retention1h' },
  { seconds: 86400, labelKey: 'agents.detail.influxdb.retention1d' },
  { seconds: 604800, labelKey: 'agents.detail.influxdb.retention1w' },
  { seconds: 2592000, labelKey: 'agents.detail.influxdb.retention30d' },
] as const;

/** retention 초를 사람이 읽기 쉬운 문자열로 변환한다(표 셀용). */
function formatRetention(seconds: number, infiniteLabel: string): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return infiniteLabel;
  if (seconds % 2592000 === 0) return `${seconds / 2592000}d (30d unit)`;
  if (seconds % 604800 === 0) return `${seconds / 604800}w`;
  if (seconds % 86400 === 0) return `${seconds / 86400}d`;
  if (seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds % 60 === 0) return `${seconds / 60}m`;
  return `${seconds}s`;
}

/**
 * v3 미지원 등으로 관리 기능이 제공되지 않는 서버인지 판별한다.
 *
 * 백엔드는 v3 에이전트에 대해 501(Not Implemented) 또는 400 을 반환한다.
 * 이 경우 재시도해도 의미가 없으므로 안내 배너를 노출한다.
 */
function isUnsupportedError(err: unknown): boolean {
  if (err instanceof APIError) {
    return err.status === 501 || err.status === 400;
  }
  return false;
}

/** 에러를 사용자용 메시지로 변환한다. APIError 는 백엔드 메시지를 그대로 노출한다. */
function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof APIError && err.message) return err.message;
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}

interface InfluxdbManagementPanelProps {
  /** 관리 대상 에이전트 이름. REST 라우트의 {agent_name} 세그먼트에 사용된다. */
  agentName?: string;
}

export default function InfluxdbManagementPanel({
  agentName,
}: InfluxdbManagementPanelProps) {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const bucketsQuery = useBuckets(agentName);
  const createBucket = useCreateBucket(agentName);
  const deleteBucket = useDeleteBucket(agentName);
  const truncateBucket = useTruncateBucket(agentName);

  // --- 선택된 버킷 (measurement 목록 표시) ---
  const [selectedBucket, setSelectedBucket] = useState<string | null>(null);

  // --- 생성 폼 상태 ---
  const [newBucketName, setNewBucketName] = useState('');
  const [newRetention, setNewRetention] = useState<number>(0);

  // --- 확인 다이얼로그 상태 ---
  const [deletingBucket, setDeletingBucket] = useState<string | null>(null);
  const [truncatingBucket, setTruncatingBucket] = useState<string | null>(null);
  // truncate 는 강한 확인 — 사용자가 버킷 이름을 정확히 입력해야 활성화된다.
  const [truncateConfirmInput, setTruncateConfirmInput] = useState('');

  const buckets = bucketsQuery.data ?? [];
  const unsupported = isUnsupportedError(bucketsQuery.error);

  // --- 핸들러: 버킷 생성 ---
  const handleCreateBucket = useCallback(() => {
    const name = newBucketName.trim();
    if (!name) return;
    createBucket.mutate(
      { name, retentionSeconds: newRetention },
      {
        onSuccess: () => {
          addNotification({
            type: 'success',
            message: t('agents.detail.influxdb.createBucketSuccess').replace(
              '{name}',
              name,
            ),
          });
          setNewBucketName('');
          setNewRetention(0);
        },
        onError: (err) => {
          addNotification({
            type: 'error',
            message: errorMessage(
              err,
              t('agents.detail.influxdb.createBucketError'),
            ),
          });
        },
      },
    );
  }, [newBucketName, newRetention, createBucket, addNotification, t]);

  // --- 핸들러: 버킷 삭제 ---
  const handleConfirmDeleteBucket = useCallback(async () => {
    if (!deletingBucket) return;
    const bucket = deletingBucket;
    try {
      await deleteBucket.mutateAsync(bucket);
      addNotification({
        type: 'success',
        message: t('agents.detail.influxdb.deleteBucketSuccess').replace(
          '{name}',
          bucket,
        ),
      });
      // 삭제된 버킷이 선택 중이었다면 선택 해제.
      if (selectedBucket === bucket) setSelectedBucket(null);
      setDeletingBucket(null);
    } catch (err) {
      addNotification({
        type: 'error',
        message: errorMessage(err, t('agents.detail.influxdb.deleteBucketError')),
      });
    }
  }, [deletingBucket, deleteBucket, selectedBucket, addNotification, t]);

  // --- 핸들러: 버킷 초기화(truncate) ---
  const handleConfirmTruncate = useCallback(async () => {
    if (!truncatingBucket) return;
    const bucket = truncatingBucket;
    try {
      await truncateBucket.mutateAsync(bucket);
      addNotification({
        type: 'success',
        message: t('agents.detail.influxdb.truncateSuccess').replace(
          '{name}',
          bucket,
        ),
      });
      setTruncatingBucket(null);
      setTruncateConfirmInput('');
    } catch (err) {
      addNotification({
        type: 'error',
        message: errorMessage(err, t('agents.detail.influxdb.truncateError')),
      });
    }
  }, [truncatingBucket, truncateBucket, addNotification, t]);

  const handleCloseTruncate = useCallback(() => {
    setTruncatingBucket(null);
    setTruncateConfirmInput('');
  }, []);

  // truncate 확인 버튼은 입력값이 대상 버킷 이름과 정확히 일치할 때만 활성화한다.
  const truncateConfirmed =
    truncatingBucket !== null && truncateConfirmInput === truncatingBucket;

  // --- 렌더: agentName 누락 방어 ---
  if (!agentName) {
    return (
      <div className="p-4 text-sm text-(--color-text-muted)">
        {t('agents.detail.influxdb.noAgentName')}
      </div>
    );
  }

  return (
    <div className="space-y-4 p-4">
      {/* v3 미지원 안내 배너 */}
      {unsupported && (
        <div className="flex items-start gap-2 rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-700 dark:bg-amber-950 dark:text-amber-300">
          <Info className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span>{t('agents.detail.influxdb.unsupportedVersion')}</span>
        </div>
      )}

      {/* 로드 에러(미지원 이외) 안내 */}
      {bucketsQuery.isError && !unsupported && (
        <div className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-800 dark:bg-red-950 dark:text-red-300">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span>
            {errorMessage(
              bucketsQuery.error,
              t('agents.detail.influxdb.loadBucketsError'),
            )}
          </span>
        </div>
      )}

      {/* 버킷 생성 폼 (미지원 서버에서는 숨김) */}
      {!unsupported && (
        <section className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-3">
          <h4 className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-(--color-text-muted)">
            <Plus className="h-3.5 w-3.5" aria-hidden="true" />
            {t('agents.detail.influxdb.createBucketTitle')}
          </h4>
          <div className="flex flex-wrap items-end gap-2">
            <div className="flex-1 min-w-[160px]">
              <label
                htmlFor="influx-new-bucket-name"
                className="mb-1 block text-xs font-medium text-(--color-text-secondary)"
              >
                {t('agents.detail.influxdb.bucketName')}
              </label>
              <input
                id="influx-new-bucket-name"
                type="text"
                value={newBucketName}
                onChange={(e) => setNewBucketName(e.target.value)}
                placeholder={t('agents.detail.influxdb.bucketNamePlaceholder')}
                className="block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
              />
            </div>
            <div className="min-w-[140px]">
              <label
                htmlFor="influx-new-bucket-retention"
                className="mb-1 block text-xs font-medium text-(--color-text-secondary)"
              >
                {t('agents.detail.influxdb.retention')}
              </label>
              <select
                id="influx-new-bucket-retention"
                value={newRetention}
                onChange={(e) => setNewRetention(Number(e.target.value))}
                className="block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                {RETENTION_OPTIONS.map((opt) => (
                  <option key={opt.seconds} value={opt.seconds}>
                    {t(opt.labelKey)}
                  </option>
                ))}
              </select>
            </div>
            <button
              type="button"
              onClick={handleCreateBucket}
              disabled={!newBucketName.trim() || createBucket.isPending}
              className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              <Plus className="h-3.5 w-3.5" aria-hidden="true" />
              {createBucket.isPending
                ? t('agents.detail.influxdb.creating')
                : t('agents.detail.influxdb.create')}
            </button>
          </div>
        </section>
      )}

      {/* 버킷 목록 */}
      <section>
        <div className="mb-2 flex items-center justify-between">
          <h4 className="flex items-center gap-1.5 text-sm font-medium text-(--color-text-primary)">
            <Database className="h-4 w-4" aria-hidden="true" />
            {t('agents.detail.influxdb.bucketsTitle')}
            {buckets.length > 0 && (
              <span className="text-xs font-normal text-(--color-text-muted)">
                ({buckets.length})
              </span>
            )}
          </h4>
          <button
            type="button"
            onClick={() => void bucketsQuery.refetch()}
            disabled={bucketsQuery.isFetching}
            className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-2.5 py-1 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-50"
          >
            <RefreshCw
              className={cn(
                'h-3.5 w-3.5',
                bucketsQuery.isFetching && 'animate-spin',
              )}
              aria-hidden="true"
            />
            {t('agents.detail.influxdb.refresh')}
          </button>
        </div>

        {bucketsQuery.isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 3 }).map((_, i) => (
              <div
                key={i}
                className="h-10 animate-pulse rounded bg-(--color-bg-elevated)"
              />
            ))}
          </div>
        ) : buckets.length === 0 ? (
          !unsupported &&
          !bucketsQuery.isError && (
            <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-6 text-center">
              <Database
                className="mx-auto h-8 w-8 text-(--color-border-strong)"
                aria-hidden="true"
              />
              <p className="mt-2 text-sm text-(--color-text-muted)">
                {t('agents.detail.influxdb.noBuckets')}
              </p>
            </div>
          )
        ) : (
          <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-(--color-border-default) bg-(--color-bg-elevated)">
                  <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">
                    {t('agents.detail.influxdb.colName')}
                  </th>
                  <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">
                    {t('agents.detail.influxdb.colRetention')}
                  </th>
                  <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">
                    {t('agents.detail.influxdb.colId')}
                  </th>
                  <th className="px-3 py-2 text-right font-medium text-(--color-text-muted)">
                    {t('agents.detail.influxdb.colActions')}
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-(--color-border-default)">
                {buckets.map((bucket: InfluxBucket) => {
                  const isSelected = selectedBucket === bucket.name;
                  return (
                    <tr
                      key={bucket.id || bucket.name}
                      className={cn(
                        'cursor-pointer transition-colors hover:bg-(--color-bg-elevated)',
                        isSelected && 'bg-blue-50/60 dark:bg-blue-950/30',
                      )}
                      onClick={() =>
                        setSelectedBucket(isSelected ? null : bucket.name)
                      }
                      aria-selected={isSelected}
                    >
                      <td className="px-3 py-2 font-medium text-(--color-text-primary)">
                        {bucket.name}
                      </td>
                      <td className="px-3 py-2 text-(--color-text-secondary)">
                        {formatRetention(
                          bucket.retentionSeconds,
                          t('agents.detail.influxdb.retentionInfinite'),
                        )}
                      </td>
                      <td className="px-3 py-2 font-mono text-(--color-text-muted)">
                        {bucket.id || '-'}
                      </td>
                      <td className="px-3 py-2 text-right">
                        <span className="inline-flex items-center gap-1">
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              setTruncatingBucket(bucket.name);
                              setTruncateConfirmInput('');
                            }}
                            className="inline-flex items-center gap-0.5 rounded p-1 text-(--color-text-muted) transition-colors hover:bg-amber-50 hover:text-amber-600 dark:hover:bg-amber-900/20 dark:hover:text-amber-400"
                            title={t('agents.detail.influxdb.truncateTooltip')}
                            aria-label={t('agents.detail.influxdb.truncateAriaLabel').replace('{name}', bucket.name)}
                          >
                            <RefreshCw className="h-3.5 w-3.5" aria-hidden="true" />
                          </button>
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              setDeletingBucket(bucket.name);
                            }}
                            className="inline-flex items-center gap-0.5 rounded p-1 text-(--color-text-muted) transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                            title={t('agents.detail.influxdb.deleteBucketTooltip')}
                            aria-label={t('agents.detail.influxdb.deleteBucketAriaLabel').replace('{name}', bucket.name)}
                          >
                            <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                          </button>
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* Measurement 목록 (버킷 선택 시) */}
      {selectedBucket && (
        <MeasurementsSection
          agentName={agentName}
          bucket={selectedBucket}
        />
      )}

      {/* 버킷 삭제 확인 다이얼로그 */}
      <ConfirmDialog
        isOpen={deletingBucket !== null}
        onClose={() => setDeletingBucket(null)}
        onConfirm={handleConfirmDeleteBucket}
        title={t('agents.detail.influxdb.deleteBucketConfirmTitle')}
        message={t('agents.detail.influxdb.deleteBucketConfirmMessage').replace(
          '{name}',
          deletingBucket ?? '',
        )}
        confirmLabel={t('agents.detail.influxdb.delete')}
        variant="danger"
        isSubmitting={deleteBucket.isPending}
      />

      {/* 버킷 초기화(truncate) 강한 확인 다이얼로그 */}
      <ConfirmDialog
        isOpen={truncatingBucket !== null}
        onClose={handleCloseTruncate}
        onConfirm={handleConfirmTruncate}
        title={t('agents.detail.influxdb.truncateConfirmTitle')}
        message={
          <div className="space-y-3">
            <div className="flex items-start gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-red-700 dark:border-red-800 dark:bg-red-950 dark:text-red-300">
              <AlertTriangle
                className="mt-0.5 h-4 w-4 shrink-0"
                aria-hidden="true"
              />
              <span className="text-sm">
                {t('agents.detail.influxdb.truncateWarning').replace(
                  '{name}',
                  truncatingBucket ?? '',
                )}
              </span>
            </div>
            <div>
              <label
                htmlFor="influx-truncate-confirm"
                className="mb-1 block text-xs font-medium text-(--color-text-secondary)"
              >
                {t('agents.detail.influxdb.truncateConfirmPrompt').replace(
                  '{name}',
                  truncatingBucket ?? '',
                )}
              </label>
              <input
                id="influx-truncate-confirm"
                type="text"
                value={truncateConfirmInput}
                onChange={(e) => setTruncateConfirmInput(e.target.value)}
                autoComplete="off"
                className="block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 text-sm text-(--color-text-primary) focus:border-red-500 focus:outline-none focus:ring-2 focus:ring-red-500"
              />
            </div>
            {!truncateConfirmed && truncateConfirmInput.length > 0 && (
              <p className="text-xs text-red-500">
                {t('agents.detail.influxdb.truncateConfirmMismatch')}
              </p>
            )}
          </div>
        }
        confirmLabel={t('agents.detail.influxdb.truncate')}
        variant="danger"
        // 입력값이 버킷 이름과 일치하지 않으면 확인을 막기 위해 제출 상태로 비활성화한다.
        isSubmitting={truncateBucket.isPending || !truncateConfirmed}
      />
    </div>
  );
}

// ---- Measurement 섹션 ----

/**
 * 선택된 버킷의 measurement 목록과 삭제 어포던스를 렌더한다.
 *
 * measurement 삭제는 확인 다이얼로그를 거친다.
 */
function MeasurementsSection({
  agentName,
  bucket,
}: {
  agentName: string;
  bucket: string;
}) {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const measurementsQuery = useMeasurements(agentName, bucket);
  const deleteMeasurement = useDeleteMeasurement(agentName);

  const [deletingMeasurement, setDeletingMeasurement] = useState<string | null>(
    null,
  );

  const measurements = useMemo(
    () => measurementsQuery.data ?? [],
    [measurementsQuery.data],
  );

  const handleConfirmDelete = useCallback(async () => {
    if (!deletingMeasurement) return;
    const measurement = deletingMeasurement;
    try {
      await deleteMeasurement.mutateAsync({ bucket, measurement });
      addNotification({
        type: 'success',
        message: t('agents.detail.influxdb.deleteMeasurementSuccess').replace(
          '{name}',
          measurement,
        ),
      });
      setDeletingMeasurement(null);
    } catch (err) {
      addNotification({
        type: 'error',
        message:
          err instanceof APIError && err.message
            ? err.message
            : t('agents.detail.influxdb.deleteMeasurementError'),
      });
    }
  }, [deletingMeasurement, deleteMeasurement, bucket, addNotification, t]);

  return (
    <section className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-3">
      <h4 className="mb-2 flex items-center gap-1.5 text-sm font-medium text-(--color-text-primary)">
        <Table2 className="h-4 w-4" aria-hidden="true" />
        {t('agents.detail.influxdb.measurementsTitle').replace('{name}', bucket)}
        {measurements.length > 0 && (
          <span className="text-xs font-normal text-(--color-text-muted)">
            ({measurements.length})
          </span>
        )}
      </h4>

      {measurementsQuery.isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 2 }).map((_, i) => (
            <div
              key={i}
              className="h-8 animate-pulse rounded bg-(--color-bg-elevated)"
            />
          ))}
        </div>
      ) : measurementsQuery.isError ? (
        <div className="flex items-start gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-800 dark:bg-red-950 dark:text-red-300">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span>
            {measurementsQuery.error instanceof APIError &&
            measurementsQuery.error.message
              ? measurementsQuery.error.message
              : t('agents.detail.influxdb.loadMeasurementsError')}
          </span>
        </div>
      ) : measurements.length === 0 ? (
        <p className="py-3 text-center text-xs text-(--color-text-muted)">
          {t('agents.detail.influxdb.noMeasurements')}
        </p>
      ) : (
        <ul className="divide-y divide-(--color-border-default) overflow-hidden rounded-md border border-(--color-border-default)">
          {measurements.map((m) => (
            <li
              key={m}
              className="flex items-center justify-between px-3 py-2 text-xs hover:bg-(--color-bg-elevated)"
            >
              <span className="font-mono text-(--color-text-primary)">{m}</span>
              <button
                type="button"
                onClick={() => setDeletingMeasurement(m)}
                className="inline-flex items-center gap-0.5 rounded p-1 text-(--color-text-muted) transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                title={t('agents.detail.influxdb.deleteMeasurementTooltip')}
                aria-label={t('agents.detail.influxdb.deleteMeasurementAriaLabel').replace('{name}', m)}
              >
                <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
              </button>
            </li>
          ))}
        </ul>
      )}

      {/* measurement 삭제 확인 다이얼로그 */}
      <ConfirmDialog
        isOpen={deletingMeasurement !== null}
        onClose={() => setDeletingMeasurement(null)}
        onConfirm={handleConfirmDelete}
        title={t('agents.detail.influxdb.deleteMeasurementConfirmTitle')}
        message={t(
          'agents.detail.influxdb.deleteMeasurementConfirmMessage',
        )
          .replace('{name}', deletingMeasurement ?? '')
          .replace('{bucket}', bucket)}
        confirmLabel={t('agents.detail.influxdb.delete')}
        variant="danger"
        isSubmitting={deleteMeasurement.isPending}
      />
    </section>
  );
}
