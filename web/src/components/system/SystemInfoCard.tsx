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
//   - hostname, OS/Arch("linux/amd64"), version, commit(단축 해시),
//     build_date(브라우저 로컬 형식), go_version, remote 모드(한글 라벨)
//   - version/commit/hostname/OS·Arch 는 monospaced 칩 (M9)
//
// Identity 와 Runtime 은 각자 별도 query 를 쓰므로 한쪽 실패가 다른 쪽을 막지
// 않는다 (M7). 본 카드는 로딩/에러/성공을 자체적으로 분기한다.
//
// @spec SPEC-WEB-007 v0.1.0 (M2, M7, M8, M9)

import { ExternalLink } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useSystemVersion, type VersionInfo } from '@/services/api/systemUpdate';

// ─────────────────────────────────────────────────────────────────────
// 모드 한글 라벨 매핑 (M2, M9 / AC-5)
// ─────────────────────────────────────────────────────────────────────

const MODE_LABEL: Record<VersionInfo['mode'], string> = {
  server: '관리 서버',
  client: '클라이언트 노드',
  disabled: '독립 실행 (standalone)',
};

// GitHub commit 링크 베이스 (AC-14). 단축 해시여도 GitHub 이 전체 해시로 리다이렉트한다.
const COMMIT_URL_BASE = 'https://github.com/xtra72/xflow/commit/';

// monospaced 칩 공통 클래스 (MetadataChips 패턴 차용).
const CHIP_CLASS =
  'inline-block rounded px-1.5 py-0.5 font-mono text-xs ' +
  'bg-(--color-bg-base) text-(--color-text-primary) border border-(--color-border)';

// ─────────────────────────────────────────────────────────────────────
// 헬퍼: build_date → 브라우저 로컬 형식
// ─────────────────────────────────────────────────────────────────────

/**
 * ISO 8601 build_date 를 브라우저 로컬 타임존 형식으로 변환한다 (M2).
 * 잘못된 입력은 원문 그대로 반환한다 (백엔드 신뢰).
 *
 * SystemVersionCard 는 빌드 시각을 UTC 로 다루지만, 본 Identity 카드는 운영자가
 * "자기 인스턴스" 를 보는 맥락이므로 로컬 타임존 표기가 더 직관적이다.
 */
function formatLocalDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return iso;
  }
  return d.toLocaleString();
}

// ─────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────

export function SystemInfoCard() {
  const { data, isLoading, isError, refetch } = useSystemVersion();

  return (
    <section
      data-testid="sysinfo-card"
      className="rounded-lg border border-(--color-border) bg-(--color-bg-elevated) p-6 shadow-sm"
    >
      <header className="mb-4 flex items-baseline justify-between gap-2">
        <h3 className="text-base font-semibold text-(--color-text-primary)">
          인스턴스 정보
        </h3>
        {/* self 명시 라벨 — 원격 노드 목록과 혼동 방지 (M8/AC-12). */}
        <span
          data-testid="sysinfo-self-badge"
          className="rounded bg-(--color-bg-base) px-2 py-0.5 text-xs font-medium text-(--color-text-muted)"
          title="현재 접속 중인 인스턴스 자신"
        >
          이 인스턴스 (self)
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
  return (
    <>
      <dl className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
        <Field label="Hostname">
          <span data-testid="sysinfo-hostname" className={CHIP_CLASS}>
            {version.hostname}
          </span>
        </Field>

        <Field label="OS / Arch">
          <span data-testid="sysinfo-os-arch" className={CHIP_CLASS}>
            {version.os}/{version.arch}
          </span>
        </Field>

        <Field label="버전">
          <span data-testid="sysinfo-version" className={CHIP_CLASS}>
            {version.version}
          </span>
        </Field>

        <Field label="Commit">
          {/* AC-14: commit 클릭 시 GitHub commit 페이지를 새 탭으로 연다. */}
          <a
            data-testid="sysinfo-commit-link"
            href={`${COMMIT_URL_BASE}${version.commit}`}
            target="_blank"
            rel="noopener noreferrer"
            className={cn(
              CHIP_CLASS,
              'inline-flex items-center gap-1 hover:brightness-110',
            )}
            title="GitHub 에서 이 커밋 보기"
          >
            <span data-testid="sysinfo-commit" className="font-mono">
              {version.commit}
            </span>
            <ExternalLink className="h-3 w-3" aria-hidden="true" />
          </a>
        </Field>

        <Field label="Build Date">
          <span
            data-testid="sysinfo-build-date"
            className="text-(--color-text-primary)"
          >
            {formatLocalDate(version.build_date)}
          </span>
        </Field>

        <Field label="Go Runtime">
          <span
            data-testid="sysinfo-go-version"
            className="font-mono text-(--color-text-primary)"
          >
            {version.go_version}
          </span>
        </Field>

        <Field label="원격 모드">
          <span
            data-testid="sysinfo-mode"
            className="text-(--color-text-primary)"
          >
            {MODE_LABEL[version.mode]}
          </span>
        </Field>
      </dl>

      {/* M8/AC-12 (Optional): server 모드면 원격 노드 뷰로 안내. */}
      {version.mode === 'server' ? (
        <p
          data-testid="sysinfo-remote-hint"
          className="mt-4 text-xs text-(--color-text-muted)"
        >
          원격 노드 목록은 대시보드 노드 뷰에서 확인하세요.
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
      <dt className="text-xs uppercase tracking-wide text-(--color-text-muted)">
        {label}
      </dt>
      <dd className="mt-0.5">{children}</dd>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────
// Sub: 로딩 스켈레톤 (Runtime 과 독립 — M7/AC-11)
// ─────────────────────────────────────────────────────────────────────

function IdentitySkeleton() {
  return (
    <div
      data-testid="sysinfo-loading"
      className="grid grid-cols-1 gap-3 sm:grid-cols-2"
      aria-busy="true"
      aria-label="인스턴스 정보를 불러오는 중"
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
  return (
    <div
      data-testid="sysinfo-error"
      role="alert"
      className="rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-900 dark:border-red-800 dark:bg-red-950 dark:text-red-200"
    >
      <p className="font-medium">인스턴스 정보를 불러오지 못했습니다</p>
      <button
        type="button"
        data-testid="sysinfo-retry"
        onClick={onRetry}
        className="mt-3 inline-flex items-center gap-1 rounded border border-red-300 bg-white px-3 py-1 text-xs font-medium text-red-800 hover:bg-red-100 dark:border-red-700 dark:bg-red-900 dark:text-red-100 dark:hover:bg-red-800"
      >
        다시 시도
      </button>
    </div>
  );
}

export default SystemInfoCard;
