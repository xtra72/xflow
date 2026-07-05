// SPEC-WEB-007 v0.1.0 (M3, M7, M9) — System Runtime Card.
//
// "이 인스턴스 (self)" 의 런타임 메트릭을 한 장의 카드로 표시한다. Identity 와는
// 별도 query(useSystemMetrics, 5초 폴링) 를 쓰므로 한쪽 실패가 다른 쪽을 막지
// 않는다 (M7/AC-11).
//
// 표시 항목 (M3):
//   - uptime: uptime_seconds → "3d 4h 12m" 가독 형식 (formatUptime 유틸, M9)
//   - CPU %: cpu_usage_percent (소수점 1자리 + %). 0 이면 "측정 미지원" 보조 안내
//            (v1 백엔드는 CPU 샘플링 미지원 — 0% 오인 방지, AC-7)
//   - 메모리 %: memory_usage_percent (소수점 1자리 + %)
//   - goroutines: go_routines
//   - Go heap: go_mem_alloc_mb / go_mem_sys_mb (소수점 1자리 + MB, M9)
//
// @spec SPEC-WEB-007 v0.1.0 (M3, M7, M9)

import { Activity } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { formatUptime } from '@/lib/utils/formatUptime';
import {
  useSystemMetrics,
  type SystemMetrics,
} from '@/services/api/monitorService';

// ─────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────

export function SystemRuntimeCard() {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch } = useSystemMetrics();

  return (
    <section
      data-testid="runtime-card"
      className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
    >
      {/* 원격 관리 노드 카드와 동일한 헤더 룩앤필: 아이콘 + 제목. */}
      <header className="mb-3 flex items-center gap-2">
        <Activity
          className="h-4 w-4 text-(--color-text-muted)"
          aria-hidden="true"
        />
        <h3 className="text-sm font-semibold text-(--color-text-primary)">
          {t('system.runtime.cardTitle')}
        </h3>
      </header>

      {isLoading ? <RuntimeSkeleton /> : null}

      {isError ? (
        <RuntimeError onRetry={() => refetch()} /> // M7/AC-11: Identity 와 독립.
      ) : null}

      {!isLoading && !isError && data ? <RuntimeBody metrics={data} /> : null}
    </section>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Sub: 성공 본문
// ─────────────────────────────────────────────────────────────────────

function RuntimeBody({ metrics }: { metrics: SystemMetrics }) {
  const { t } = useTranslation();
  // v1 백엔드는 CPU 샘플링 미지원으로 0 고정 → "측정 미지원" 안내로 0% 오인 방지.
  const cpuUnsupported = metrics.cpu_usage_percent === 0;

  return (
    <dl className="grid grid-cols-2 gap-x-6 gap-y-2.5 sm:grid-cols-3">
      <Field label={t('system.runtime.uptime')}>
        <span
          data-testid="runtime-uptime"
          className="font-mono text-(--color-text-primary)"
        >
          {formatUptime(metrics.uptime_seconds)}
        </span>
      </Field>

      <Field label={t('system.runtime.cpu')}>
        <span
          data-testid="runtime-cpu"
          className="text-(--color-text-primary)"
        >
          {metrics.cpu_usage_percent.toFixed(1)}%
        </span>
        {cpuUnsupported ? (
          <span
            data-testid="runtime-cpu-unsupported"
            className="ml-2 text-xs text-(--color-text-muted)"
            title={t('system.runtime.cpuUnsupportedTitle')}
          >
            {t('system.runtime.cpuUnsupported')}
          </span>
        ) : null}
      </Field>

      <Field label={t('system.runtime.memory')}>
        <span
          data-testid="runtime-memory"
          className="text-(--color-text-primary)"
        >
          {metrics.memory_usage_percent.toFixed(1)}%
        </span>
      </Field>

      <Field label={t('system.runtime.goroutines')}>
        <span
          data-testid="runtime-goroutines"
          className="font-mono text-(--color-text-primary)"
        >
          {metrics.go_routines}
        </span>
      </Field>

      <Field label={t('system.runtime.goHeap')}>
        <span
          data-testid="runtime-heap"
          className="font-mono text-(--color-text-primary)"
        >
          {metrics.go_mem_alloc_mb.toFixed(1)} / {metrics.go_mem_sys_mb.toFixed(1)} MB
        </span>
      </Field>
    </dl>
  );
}

/** label + value 쌍을 표시하는 dt/dd 묶음. */
function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
        {label}
      </dt>
      <dd className="mt-1 break-all text-sm text-(--color-text-primary)">
        {children}
      </dd>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Sub: 로딩 스켈레톤 (Identity 와 독립 — M7/AC-11)
// ─────────────────────────────────────────────────────────────────────

function RuntimeSkeleton() {
  const { t } = useTranslation();
  return (
    <div
      data-testid="runtime-loading"
      className="grid grid-cols-2 gap-x-6 gap-y-2.5 sm:grid-cols-3"
      aria-busy="true"
      aria-label={t('system.runtime.loadingAria')}
    >
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="space-y-1">
          <div className="h-3 w-1/3 animate-pulse rounded bg-(--color-bg-hover)" />
          <div className="h-5 w-1/2 animate-pulse rounded bg-(--color-bg-hover)" />
        </div>
      ))}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Sub: 에러 + 다시 시도 (Identity 와 독립 — M7/AC-11)
// ─────────────────────────────────────────────────────────────────────

function RuntimeError({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      data-testid="runtime-error"
      role="alert"
      className="rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-900 dark:border-red-800 dark:bg-red-950 dark:text-red-200"
    >
      <p className="font-medium">{t('system.runtime.loadError')}</p>
      <button
        type="button"
        data-testid="runtime-retry"
        onClick={onRetry}
        className="mt-3 inline-flex items-center gap-1 rounded border border-red-300 bg-white px-3 py-1 text-xs font-medium text-red-800 hover:bg-red-100 dark:border-red-700 dark:bg-red-900 dark:text-red-100 dark:hover:bg-red-800"
      >
        {t('system.runtime.retry')}
      </button>
    </div>
  );
}

export default SystemRuntimeCard;
