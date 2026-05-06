// SPEC-WEB-006 v0.1.0 (M2, M3) — System Version Card.
//
// xflowd 시스템 버전 정보를 한 장의 카드 형태로 시각화한다.
//
// 핵심 표시 요소:
//   - 현재 버전 (v0.3.0) — 큰 mono 폰트로 강조
//   - Build commit (`abc1234`) + build_date (UTC 표기)
//   - Go runtime 버전
//   - 채널 배지 (stable / beta / nightly) — 색상 구분
//   - 업데이트 가능 여부 인디케이터:
//        - `update_available=true`  → 노란색 dot + "업데이트 가능" + latest_version
//        - `update_available=false` → 녹색 ✓ + "최신 버전입니다"
//   - 마지막 확인 시각 (상대 시간: "방금 전", "5분 전")
//   - "Check for updates" 트리거 버튼
//   - 선택적 "업데이트 시작" 버튼 (Phase D 의 UpdateDialog 와 결합 예정)
//
// 본 컴포넌트는 순수 표시(presentational) 컴포넌트로, 데이터 페칭/뮤테이션은
// 부모 (SystemStatusPage) 가 처리한다. 따라서 hook 의존성이 없고 단위
// 테스트가 결정적이다.
//
// @spec SPEC-WEB-006 v0.1.0 (M2, M3)

import { CheckCircle2, RefreshCw } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import type { Channel, VersionInfo } from '@/services/api/systemUpdate';

// ─────────────────────────────────────────────────────────────────────
// Props
// ─────────────────────────────────────────────────────────────────────

export interface SystemVersionCardProps {
  /** 현재 버전 정보 (백엔드 GET /system/version 응답). */
  version: VersionInfo;
  /**
   * 마지막으로 버전 정보를 확인한 시각 (TanStack Query 의 `dataUpdatedAt`).
   * undefined 면 "확인 안 됨" 으로 표시한다.
   */
  lastCheckedAt?: Date;
  /** "Check for updates" 버튼 클릭 핸들러. */
  onCheck: () => void;
  /** mutation 진행 중 여부. true 면 Check 버튼 비활성화. */
  isChecking?: boolean;
  /**
   * "업데이트 시작" 버튼 핸들러 (선택).
   *
   * Phase C 에서는 prop 으로만 전달되며 Phase D 가 UpdateDialog 를 결합한다.
   * 미제공이면 update_available=true 라도 시작 버튼이 숨겨진다.
   */
  onUpdate?: () => void;
}

// ─────────────────────────────────────────────────────────────────────
// 채널 배지 색상 (시각 구분 + 텍스트 의미 보존)
// ─────────────────────────────────────────────────────────────────────

const CHANNEL_BADGE_CLASS: Record<Channel, string> = {
  stable:
    'bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200',
  beta: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
  nightly:
    'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200',
};

const CHANNEL_LABEL: Record<Channel, string> = {
  stable: 'Stable',
  beta: 'Beta',
  nightly: 'Nightly',
};

// ─────────────────────────────────────────────────────────────────────
// 헬퍼: build_date / lastCheckedAt 포맷팅
// ─────────────────────────────────────────────────────────────────────

/**
 * ISO 8601 build_date 를 사람이 읽기 좋은 UTC 표기로 변환한다.
 * 잘못된 입력은 원문 그대로 반환한다 (백엔드 신뢰).
 */
function formatBuildDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return iso;
  }
  // UTC 기준 yyyy-MM-dd HH:mm 형식 — 빌드 시각은 timezone-agnostic 정보로 다룬다.
  const y = d.getUTCFullYear();
  const m = String(d.getUTCMonth() + 1).padStart(2, '0');
  const day = String(d.getUTCDate()).padStart(2, '0');
  const h = String(d.getUTCHours()).padStart(2, '0');
  const min = String(d.getUTCMinutes()).padStart(2, '0');
  return `${y}-${m}-${day} ${h}:${min} (UTC)`;
}

/**
 * lastCheckedAt 을 한국어 상대 시간으로 변환한다.
 *
 * 임계값:
 *   - undefined          → "확인 안 됨"
 *   - 60초 미만          → "방금 전"
 *   - 60분 미만          → "{n}분 전"
 *   - 24시간 미만        → "{n}시간 전"
 *   - 그 외              → "{n}일 전"
 */
function formatLastChecked(at: Date | undefined): string {
  if (!at) {
    return '확인 안 됨';
  }
  const diffSec = Math.floor((Date.now() - at.getTime()) / 1000);
  if (diffSec < 60) return '방금 전';
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}분 전`;
  const diffHour = Math.floor(diffMin / 60);
  if (diffHour < 24) return `${diffHour}시간 전`;
  const diffDay = Math.floor(diffHour / 24);
  return `${diffDay}일 전`;
}

// ─────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────

export function SystemVersionCard({
  version,
  lastCheckedAt,
  onCheck,
  isChecking = false,
  onUpdate,
}: SystemVersionCardProps) {
  const showStartButton =
    version.update_available && typeof onUpdate === 'function';

  return (
    <section
      data-testid="system-version-card"
      className={cn(
        'rounded-lg border border-(--color-border) bg-(--color-bg-elevated) p-6 shadow-sm',
      )}
    >
      <header className="mb-4 flex items-baseline justify-between gap-2">
        <h2 className="text-lg font-semibold text-(--color-text-primary)">
          xflowd 시스템 정보
        </h2>
        <span
          data-testid="system-version-channel"
          className={cn(
            'rounded px-2 py-0.5 text-xs font-mono whitespace-nowrap',
            CHANNEL_BADGE_CLASS[version.channel],
          )}
          title={`업데이트 채널: ${version.channel}`}
        >
          {CHANNEL_LABEL[version.channel]}
        </span>
      </header>

      {/* 현재 버전 (강조) */}
      <div className="mb-4">
        <div className="text-xs uppercase tracking-wide text-(--color-text-muted)">
          현재 버전
        </div>
        <div
          data-testid="system-version-current"
          className="mt-1 font-mono text-3xl font-semibold text-(--color-text-primary)"
        >
          {version.version}
        </div>
      </div>

      {/* 메타 정보 그리드 */}
      <dl className="mb-4 grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
        <div>
          <dt className="text-xs uppercase tracking-wide text-(--color-text-muted)">
            Commit
          </dt>
          <dd
            data-testid="system-version-commit"
            className="mt-0.5 font-mono text-(--color-text-primary)"
          >
            {version.commit}
          </dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wide text-(--color-text-muted)">
            Build Date
          </dt>
          <dd
            data-testid="system-version-build-date"
            className="mt-0.5 font-mono text-(--color-text-primary)"
          >
            {formatBuildDate(version.build_date)}
          </dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wide text-(--color-text-muted)">
            Go Runtime
          </dt>
          <dd
            data-testid="system-version-go"
            className="mt-0.5 font-mono text-(--color-text-primary)"
          >
            {version.go_version}
          </dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wide text-(--color-text-muted)">
            마지막 확인
          </dt>
          <dd
            data-testid="system-last-checked"
            className="mt-0.5 text-(--color-text-primary)"
          >
            {formatLastChecked(lastCheckedAt)}
          </dd>
        </div>
      </dl>

      {/* 업데이트 상태 인디케이터 */}
      <div
        data-testid="system-update-status"
        className={cn(
          'mb-4 flex items-center gap-2 rounded-md border px-3 py-2 text-sm',
          version.update_available
            ? 'border-yellow-300 bg-yellow-50 text-yellow-900 dark:border-yellow-700 dark:bg-yellow-950 dark:text-yellow-200'
            : 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-200',
        )}
      >
        {version.update_available ? (
          <>
            <span
              className="inline-block h-2 w-2 rounded-full bg-yellow-500"
              aria-hidden="true"
            />
            <span data-testid="system-update-available">
              업데이트 가능
              {version.latest_version ? (
                <>
                  {' '}
                  <span className="font-mono">→ {version.latest_version}</span>
                </>
              ) : null}
            </span>
          </>
        ) : (
          <>
            <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
            <span>최신 버전입니다</span>
          </>
        )}
      </div>

      {/* 액션 버튼들 */}
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          data-testid="system-check-button"
          onClick={onCheck}
          disabled={isChecking}
          className={cn(
            'inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-colors',
            'border-(--color-border) bg-(--color-bg-base) text-(--color-text-primary)',
            'hover:bg-(--color-bg-hover)',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          <RefreshCw
            className={cn('h-3.5 w-3.5', isChecking && 'animate-spin')}
            aria-hidden="true"
          />
          {isChecking ? '확인 중...' : '업데이트 확인'}
        </button>

        {showStartButton ? (
          <button
            type="button"
            data-testid="system-update-start-button"
            onClick={onUpdate}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
              'bg-yellow-500 text-white hover:bg-yellow-600',
              'dark:bg-yellow-600 dark:hover:bg-yellow-700',
            )}
          >
            업데이트 시작
          </button>
        ) : null}
      </div>
    </section>
  );
}
