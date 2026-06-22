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

import { formatUptime } from '@/lib/utils/formatUptime';
import {
  useSystemMetrics,
  type SystemMetrics,
} from '@/services/api/monitorService';

// ─────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────

export function SystemRuntimeCard() {
  const { data, isLoading, isError, refetch } = useSystemMetrics();

  return (
    <section
      data-testid="runtime-card"
      className="rounded-lg border border-(--color-border) bg-(--color-bg-elevated) p-6 shadow-sm"
    >
      <header className="mb-4">
        <h3 className="text-base font-semibold text-(--color-text-primary)">
          런타임 메트릭
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
  // v1 백엔드는 CPU 샘플링 미지원으로 0 고정 → "측정 미지원" 안내로 0% 오인 방지.
  const cpuUnsupported = metrics.cpu_usage_percent === 0;

  return (
    <dl className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
      <Field label="Uptime">
        <span
          data-testid="runtime-uptime"
          className="font-mono text-(--color-text-primary)"
        >
          {formatUptime(metrics.uptime_seconds)}
        </span>
      </Field>

      <Field label="CPU 사용률">
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
            title="현재 런타임은 프로세스 CPU 샘플링을 지원하지 않습니다"
          >
            (측정 미지원)
          </span>
        ) : null}
      </Field>

      <Field label="메모리 사용률">
        <span
          data-testid="runtime-memory"
          className="text-(--color-text-primary)"
        >
          {metrics.memory_usage_percent.toFixed(1)}%
        </span>
      </Field>

      <Field label="Goroutines">
        <span
          data-testid="runtime-goroutines"
          className="font-mono text-(--color-text-primary)"
        >
          {metrics.go_routines}
        </span>
      </Field>

      <Field label="Go Heap (alloc / sys)">
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
      <dt className="text-xs uppercase tracking-wide text-(--color-text-muted)">
        {label}
      </dt>
      <dd className="mt-0.5">{children}</dd>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Sub: 로딩 스켈레톤 (Identity 와 독립 — M7/AC-11)
// ─────────────────────────────────────────────────────────────────────

function RuntimeSkeleton() {
  return (
    <div
      data-testid="runtime-loading"
      className="grid grid-cols-1 gap-3 sm:grid-cols-2"
      aria-busy="true"
      aria-label="런타임 메트릭을 불러오는 중"
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
  return (
    <div
      data-testid="runtime-error"
      role="alert"
      className="rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-900 dark:border-red-800 dark:bg-red-950 dark:text-red-200"
    >
      <p className="font-medium">런타임 메트릭을 불러오지 못했습니다</p>
      <button
        type="button"
        data-testid="runtime-retry"
        onClick={onRetry}
        className="mt-3 inline-flex items-center gap-1 rounded border border-red-300 bg-white px-3 py-1 text-xs font-medium text-red-800 hover:bg-red-100 dark:border-red-700 dark:bg-red-900 dark:text-red-100 dark:hover:bg-red-800"
      >
        다시 시도
      </button>
    </div>
  );
}

export default SystemRuntimeCard;
