// SPEC-WEB-007 v0.1.0 (M2, M7, M8, M9) — System Info Card (Identity).
//
// "이 인스턴스 (self)" 의 정적 식별 정보를 한 장의 카드로 표시한다. 원격 노드
// 목록(NodeDashboard) 과 혼동되지 않도록 self 임을 명시 라벨링한다 (M8/AC-12).
//
// 데이터 소스: useSystemVersion() (SPEC-WEB-006 의 GET /system/version 폴링 훅).
//   SPEC-WEB-007 (Phase B) 이 VersionInfo 에 os/arch/hostname/mode/uptime_seconds
//   필드를 추가했다. 본 카드는 식별 정보(Identity)만 소비하며, runtime 메트릭은
//   별도 SystemRuntimeCard 가 담당한다 (영역별 독립 — M7/AC-11).
//
// 표시 항목 (M2):
//   - hostname, OS/Arch("linux/amd64"), version, remote 모드(한글 라벨)
//   - 값은 칩/박스 없이 plain 텍스트로 표시한다. 원격 관리(NodeDashboard) 개요
//     카드의 InfoItem 룩앤필과 시각적으로 통일한다 (식별자성 값만 font-mono 유지).
//
// Identity 와 Runtime 은 각자 별도 query 를 쓰므로 한쪽 실패가 다른 쪽을 막지
// 않는다 (M7). 본 카드는 로딩/에러/성공을 자체적으로 분기한다.
//
// @spec SPEC-WEB-007 v0.1.0 (M2, M7, M8, M9)

import { Server } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { useSystemVersion, type VersionInfo } from '@/services/api/systemUpdate';

// ─────────────────────────────────────────────────────────────────────
// 모드 라벨 i18n 키 매핑 (M2, M9 / AC-5)
// 렌더 시 t(key) 로 변환한다.
// ─────────────────────────────────────────────────────────────────────

const MODE_LABEL_KEY: Record<VersionInfo['mode'], string> = {
  server: 'system.info.modeServer',
  client: 'system.info.modeClient',
  disabled: 'system.info.modeDisabled',
};

// ─────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────

export function SystemInfoCard() {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch } = useSystemVersion();

  return (
    <section
      data-testid="sysinfo-card"
      className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
    >
      {/* 원격 관리 노드 카드와 동일한 헤더 룩앤필: 아이콘+제목 좌측 그룹 / self 배지 우측. */}
      <header className="mb-3 flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Server
            className="h-4 w-4 text-(--color-text-muted)"
            aria-hidden="true"
          />
          <h3 className="text-sm font-semibold text-(--color-text-primary)">
            {t('system.info.cardTitle')}
          </h3>
        </div>
        {/* self 명시 라벨 — 원격 노드 목록과 혼동 방지 (M8/AC-12). */}
        <span
          data-testid="sysinfo-self-badge"
          className="rounded bg-(--color-bg-base) px-2 py-0.5 text-xs font-medium text-(--color-text-muted)"
          title={t('system.info.selfBadgeTitle')}
        >
          {t('system.info.selfBadge')}
        </span>
      </header>

      {isLoading ? <IdentitySkeleton /> : null}

      {isError ? (
        <IdentityError onRetry={() => refetch()} /> // M7/AC-11: Runtime 과 독립.
      ) : null}

      {!isLoading && !isError && data ? <IdentityBody version={data} /> : null}
    </section>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Sub: 성공 본문
// ─────────────────────────────────────────────────────────────────────

function IdentityBody({ version }: { version: VersionInfo }) {
  const { t } = useTranslation();
  return (
    <>
      <dl className="grid grid-cols-2 gap-x-6 gap-y-2.5 sm:grid-cols-3">
        {/* 식별자성 값(hostname/os·arch/version/commit)은 칩 없이 plain 텍스트 +
            가독을 위한 font-mono 만 유지한다 (레퍼런스 InfoItem 톤). */}
        <Field label={t('system.info.hostname')}>
          <span
            data-testid="sysinfo-hostname"
            className="font-mono text-(--color-text-primary)"
          >
            {version.hostname}
          </span>
        </Field>

        <Field label={t('system.info.osArch')}>
          <span
            data-testid="sysinfo-os-arch"
            className="font-mono text-(--color-text-primary)"
          >
            {version.os}/{version.arch}
          </span>
        </Field>

        <Field label={t('system.info.version')}>
          <span
            data-testid="sysinfo-version"
            className="font-mono text-(--color-text-primary)"
          >
            {version.version}
          </span>
        </Field>

        <Field label={t('system.info.remoteMode')}>
          <span
            data-testid="sysinfo-mode"
            className="text-(--color-text-primary)"
          >
            {t(MODE_LABEL_KEY[version.mode])}
          </span>
        </Field>
      </dl>

      {/* M8/AC-12 (Optional): server 모드면 원격 노드 뷰로 안내. */}
      {version.mode === 'server' ? (
        <p
          data-testid="sysinfo-remote-hint"
          className="mt-4 text-xs text-(--color-text-muted)"
        >
          {t('system.info.remoteHint')}
        </p>
      ) : null}
    </>
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
// Sub: 로딩 스켈레톤 (Runtime 과 독립 — M7/AC-11)
// ─────────────────────────────────────────────────────────────────────

function IdentitySkeleton() {
  const { t } = useTranslation();
  return (
    <div
      data-testid="sysinfo-loading"
      className="grid grid-cols-2 gap-x-6 gap-y-2.5 sm:grid-cols-3"
      aria-busy="true"
      aria-label={t('system.info.loadingAria')}
    >
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="space-y-1">
          <div className="h-3 w-1/3 animate-pulse rounded bg-(--color-bg-hover)" />
          <div className="h-5 w-2/3 animate-pulse rounded bg-(--color-bg-hover)" />
        </div>
      ))}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Sub: 에러 + 다시 시도 (Runtime 과 독립 — M7/AC-11)
// ─────────────────────────────────────────────────────────────────────

function IdentityError({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      data-testid="sysinfo-error"
      role="alert"
      className="rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-900 dark:border-red-800 dark:bg-red-950 dark:text-red-200"
    >
      <p className="font-medium">{t('system.info.loadError')}</p>
      <button
        type="button"
        data-testid="sysinfo-retry"
        onClick={onRetry}
        className="mt-3 inline-flex items-center gap-1 rounded border border-red-300 bg-(--color-bg-surface) px-3 py-1 text-xs font-medium text-red-800 hover:bg-red-100 dark:border-red-700 dark:bg-red-900 dark:text-red-100 dark:hover:bg-red-800"
      >
        {t('system.info.retry')}
      </button>
    </div>
  );
}

export default SystemInfoCard;
