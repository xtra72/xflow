// SPEC-WEB-006 v0.1.0 (M2, M3) — SystemVersionCard 컴포넌트 테스트.
//
// 시스템 버전 정보 카드의 순수 렌더링 + 사용자 상호작용 사양 테스트.
//
// 테스트 범위:
//   - VersionInfo 의 모든 필드 (version, commit, build_date, go_version, channel) 렌더링
//   - update_available true/false 분기 시각화
//   - lastCheckedAt 상대 시간 포맷팅 (방금 전, N분 전, 미확인)
//   - Channel 배지 (stable/beta/nightly) 색상 구분
//   - onCheck / onUpdate 콜백 트리거 + 비활성화 상태
//
// @spec SPEC-WEB-006 v0.1.0 (M2, M3)

import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { VersionInfo } from '@/services/api/systemUpdate';

// i18n: t() 를 ko.json 키 해석으로 모킹해 한국어 단언을 유지한다.
vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<
    string,
    unknown
  >;
  const resolve = (key: string): string => {
    const v = key.split('.').reduce<unknown>(
      (o, p) =>
        o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined,
      ko,
    );
    return typeof v === 'string' ? v : key;
  };
  return {
    useTranslation: () => ({
      t: resolve,
      locale: 'ko' as const,
      setLocale: () => {},
    }),
  };
});

import { SystemVersionCard } from './SystemVersionCard';

// ─────────────────────────────────────────────────────────────────────
// Test fixtures
// ─────────────────────────────────────────────────────────────────────

function makeVersion(overrides: Partial<VersionInfo> = {}): VersionInfo {
  return {
    version: 'v0.3.0',
    commit: 'abc1234',
    build_date: '2026-04-30T12:00:00Z',
    go_version: 'go1.25.0',
    channel: 'stable',
    update_available: false,
    latest_version: null,
    // SPEC-WEB-007 추가 필드.
    os: 'linux',
    arch: 'amd64',
    hostname: 'xflow-node-01',
    mode: 'server',
    uptime_seconds: 3600,
    ...overrides,
  };
}

// ─────────────────────────────────────────────────────────────────────
// 1. 모든 버전 필드 렌더링 (최신 상태)
// ─────────────────────────────────────────────────────────────────────

describe('SystemVersionCard — 최신 상태', () => {
  it('VersionInfo 의 모든 필드를 렌더링한다', () => {
    const version = makeVersion();
    render(
      <SystemVersionCard
        version={version}
        onCheck={vi.fn()}
        isChecking={false}
      />,
    );

    expect(screen.getByText(/xflowd 시스템 정보/)).toBeInTheDocument();
    expect(screen.getByTestId('system-version-current')).toHaveTextContent(
      'v0.3.0',
    );
    expect(screen.getByTestId('system-version-commit')).toHaveTextContent(
      'abc1234',
    );
    expect(screen.getByTestId('system-version-go')).toHaveTextContent(
      'go1.25.0',
    );
    expect(screen.getByTestId('system-version-channel')).toHaveTextContent(
      /stable/i,
    );
  });

  it('update_available=false → 최신 버전 인디케이터를 표시한다', () => {
    render(
      <SystemVersionCard
        version={makeVersion({ update_available: false })}
        onCheck={vi.fn()}
      />,
    );

    expect(screen.getByTestId('system-update-status')).toHaveTextContent(
      /최신 버전입니다/,
    );
    // 업데이트 가능 인디케이터는 없어야 함
    expect(
      screen.queryByTestId('system-update-available'),
    ).not.toBeInTheDocument();
  });
});

// ─────────────────────────────────────────────────────────────────────
// 2. 업데이트 가능 분기
// ─────────────────────────────────────────────────────────────────────

describe('SystemVersionCard — 업데이트 가능', () => {
  it('update_available=true → 업데이트 가능 + latest_version 을 표시한다', () => {
    render(
      <SystemVersionCard
        version={makeVersion({
          update_available: true,
          latest_version: 'v0.4.0',
        })}
        onCheck={vi.fn()}
      />,
    );

    expect(screen.getByTestId('system-update-available')).toHaveTextContent(
      /업데이트 가능/,
    );
    expect(screen.getByTestId('system-update-available')).toHaveTextContent(
      'v0.4.0',
    );
  });

  it('latest_version=null + update_available=true 일 때도 안전하게 렌더링한다', () => {
    // 백엔드가 inconsistent 응답을 줘도 크래시 금지.
    render(
      <SystemVersionCard
        version={makeVersion({
          update_available: true,
          latest_version: null,
        })}
        onCheck={vi.fn()}
      />,
    );

    expect(screen.getByTestId('system-update-available')).toBeInTheDocument();
  });
});

// ─────────────────────────────────────────────────────────────────────
// 3. Channel 배지
// ─────────────────────────────────────────────────────────────────────

describe('SystemVersionCard — Channel 배지', () => {
  it.each([
    ['stable', /stable/i],
    ['beta', /beta/i],
    ['nightly', /nightly/i],
  ] as const)('channel=%s → 배지에 %s 가 표시된다', (channel, expected) => {
    render(
      <SystemVersionCard
        version={makeVersion({ channel })}
        onCheck={vi.fn()}
      />,
    );
    expect(screen.getByTestId('system-version-channel')).toHaveTextContent(
      expected,
    );
  });
});

// ─────────────────────────────────────────────────────────────────────
// 4. build_date 포맷팅
// ─────────────────────────────────────────────────────────────────────

describe('SystemVersionCard — build_date 포맷팅', () => {
  it('build_date 를 사용자 친화 포맷으로 표시한다 (UTC 명시)', () => {
    render(
      <SystemVersionCard
        version={makeVersion({ build_date: '2026-04-30T12:00:00Z' })}
        onCheck={vi.fn()}
      />,
    );

    const buildDateEl = screen.getByTestId('system-version-build-date');
    // 정확한 timezone 변환 결과는 환경마다 다르므로, 핵심 토큰만 검증.
    // 2026-04-30 (UTC) 는 KST 21:00, UTC 12:00 — 어떤 환경이든 "2026" 은 들어간다.
    expect(buildDateEl.textContent).toMatch(/2026/);
    expect(buildDateEl.textContent).toMatch(/UTC/);
  });
});

// ─────────────────────────────────────────────────────────────────────
// 5-6. lastCheckedAt 상대 시간
// ─────────────────────────────────────────────────────────────────────

describe('SystemVersionCard — lastCheckedAt', () => {
  beforeEach(() => {
    // 고정된 "현재 시각" 설정으로 상대 시간 계산을 결정적으로 만든다.
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-05-01T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('lastCheckedAt=undefined → "확인 안 됨" 으로 표시한다', () => {
    render(
      <SystemVersionCard
        version={makeVersion()}
        onCheck={vi.fn()}
        lastCheckedAt={undefined}
      />,
    );

    expect(screen.getByTestId('system-last-checked')).toHaveTextContent(
      /확인 안 됨/,
    );
  });

  it('lastCheckedAt=10초 전 → "방금 전" 으로 표시한다', () => {
    const tenSecondsAgo = new Date('2026-05-01T11:59:50Z');
    render(
      <SystemVersionCard
        version={makeVersion()}
        onCheck={vi.fn()}
        lastCheckedAt={tenSecondsAgo}
      />,
    );

    expect(screen.getByTestId('system-last-checked')).toHaveTextContent(
      /방금 전/,
    );
  });

  it('lastCheckedAt=5분 전 → "5분 전" 으로 표시한다', () => {
    const fiveMinAgo = new Date('2026-05-01T11:55:00Z');
    render(
      <SystemVersionCard
        version={makeVersion()}
        onCheck={vi.fn()}
        lastCheckedAt={fiveMinAgo}
      />,
    );

    expect(screen.getByTestId('system-last-checked')).toHaveTextContent(
      /5분 전/,
    );
  });

  it('lastCheckedAt=3시간 전 → "3시간 전" 으로 표시한다', () => {
    const threeHoursAgo = new Date('2026-05-01T09:00:00Z');
    render(
      <SystemVersionCard
        version={makeVersion()}
        onCheck={vi.fn()}
        lastCheckedAt={threeHoursAgo}
      />,
    );

    expect(screen.getByTestId('system-last-checked')).toHaveTextContent(
      /3시간 전/,
    );
  });

  it('lastCheckedAt=2일 전 → "2일 전" 으로 표시한다', () => {
    const twoDaysAgo = new Date('2026-04-29T12:00:00Z');
    render(
      <SystemVersionCard
        version={makeVersion()}
        onCheck={vi.fn()}
        lastCheckedAt={twoDaysAgo}
      />,
    );

    expect(screen.getByTestId('system-last-checked')).toHaveTextContent(
      /2일 전/,
    );
  });

  it('build_date 가 잘못된 ISO 문자열이면 원문을 그대로 표시한다', () => {
    render(
      <SystemVersionCard
        version={makeVersion({ build_date: 'not-a-date' })}
        onCheck={vi.fn()}
      />,
    );

    expect(screen.getByTestId('system-version-build-date')).toHaveTextContent(
      'not-a-date',
    );
  });
});

// ─────────────────────────────────────────────────────────────────────
// 7-8. Check 버튼 동작
// ─────────────────────────────────────────────────────────────────────

describe('SystemVersionCard — Check 버튼', () => {
  it('Check 버튼 클릭 시 onCheck 콜백을 호출한다', () => {
    const onCheck = vi.fn();
    render(
      <SystemVersionCard
        version={makeVersion()}
        onCheck={onCheck}
        isChecking={false}
      />,
    );

    fireEvent.click(screen.getByTestId('system-check-button'));
    expect(onCheck).toHaveBeenCalledTimes(1);
  });

  it('isChecking=true 일 때 Check 버튼이 비활성화된다', () => {
    render(
      <SystemVersionCard
        version={makeVersion()}
        onCheck={vi.fn()}
        isChecking={true}
      />,
    );

    const btn = screen.getByTestId('system-check-button');
    expect(btn).toBeDisabled();
  });
});

// ─────────────────────────────────────────────────────────────────────
// 9-10. 업데이트 시작 버튼
// ─────────────────────────────────────────────────────────────────────

describe('SystemVersionCard — 업데이트 시작 버튼', () => {
  it('update_available=true + onUpdate 제공 → 시작 버튼이 보인다', () => {
    const onUpdate = vi.fn();
    render(
      <SystemVersionCard
        version={makeVersion({
          update_available: true,
          latest_version: 'v0.4.0',
        })}
        onCheck={vi.fn()}
        onUpdate={onUpdate}
      />,
    );

    const startBtn = screen.getByTestId('system-update-start-button');
    expect(startBtn).toBeInTheDocument();
    fireEvent.click(startBtn);
    expect(onUpdate).toHaveBeenCalledTimes(1);
  });

  it('update_available=true + onUpdate 미제공 → 시작 버튼이 숨겨진다', () => {
    render(
      <SystemVersionCard
        version={makeVersion({
          update_available: true,
          latest_version: 'v0.4.0',
        })}
        onCheck={vi.fn()}
        // onUpdate 미제공
      />,
    );

    expect(
      screen.queryByTestId('system-update-start-button'),
    ).not.toBeInTheDocument();
  });

  it('update_available=false → 시작 버튼은 onUpdate 가 있어도 숨겨진다', () => {
    render(
      <SystemVersionCard
        version={makeVersion({ update_available: false })}
        onCheck={vi.fn()}
        onUpdate={vi.fn()}
      />,
    );

    expect(
      screen.queryByTestId('system-update-start-button'),
    ).not.toBeInTheDocument();
  });
});

// ─────────────────────────────────────────────────────────────────────
// 11-13. SPEC-UPDATE-002 v0.1.0 (M8) — 채널 클릭 (admin only)
// ─────────────────────────────────────────────────────────────────────

describe('SystemVersionCard — 채널 클릭 (SPEC-UPDATE-002 M8)', () => {
  it('isAdmin=false + onChannelClick 제공 → 채널 배지는 클릭 불가 (button 미사용)', () => {
    const onChannelClick = vi.fn();
    render(
      <SystemVersionCard
        version={makeVersion()}
        onCheck={vi.fn()}
        isAdmin={false}
        onChannelClick={onChannelClick}
      />,
    );

    const channelEl = screen.getByTestId('system-version-channel');
    // 비-admin 은 button 으로 렌더되지 않아야 한다 (단순 span).
    expect(channelEl.tagName.toLowerCase()).toBe('span');
    fireEvent.click(channelEl);
    expect(onChannelClick).not.toHaveBeenCalled();
  });

  it('isAdmin=true + onChannelClick 제공 → 채널 배지가 button + aria-label 로 노출된다', () => {
    const onChannelClick = vi.fn();
    render(
      <SystemVersionCard
        version={makeVersion()}
        onCheck={vi.fn()}
        isAdmin={true}
        onChannelClick={onChannelClick}
      />,
    );

    const channelBtn = screen.getByTestId('system-version-channel');
    expect(channelBtn.tagName.toLowerCase()).toBe('button');
    expect(channelBtn).toHaveAttribute('aria-label');
    expect(channelBtn.getAttribute('aria-label')).toMatch(/채널 변경/);
  });

  it('isAdmin=true + onChannelClick 제공 → 클릭 시 onChannelClick 가 호출된다', () => {
    const onChannelClick = vi.fn();
    render(
      <SystemVersionCard
        version={makeVersion()}
        onCheck={vi.fn()}
        isAdmin={true}
        onChannelClick={onChannelClick}
      />,
    );

    fireEvent.click(screen.getByTestId('system-version-channel'));
    expect(onChannelClick).toHaveBeenCalledTimes(1);
  });

  it('isAdmin=true + onChannelClick 미제공 → 채널 배지는 클릭 불가 (단순 span)', () => {
    render(
      <SystemVersionCard
        version={makeVersion()}
        onCheck={vi.fn()}
        isAdmin={true}
      />,
    );

    const channelEl = screen.getByTestId('system-version-channel');
    expect(channelEl.tagName.toLowerCase()).toBe('span');
  });
});
