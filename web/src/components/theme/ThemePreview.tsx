// 테마 미리보기 — 고르는 색을 눈으로 보는 자리.
//
// **편집 중인 프리셋을 그린다.** 화면에 적용 중인 것이 아니다. 편집기는 탭으로 고른
// `target` 의 오버라이드를 고치는데 `useTheme` 은 `resolvedPreset` 만 적용하므로,
// 낮에 앉아 야간 팔레트를 고치는 것은 눈을 감고 칠하는 것이다. 그 자리를 메운다.
//
// 화면을 넷으로 나눈다 — 대시보드 · 플로우 · 목록 · 스케줄. 한 화면만 보여 주면
// "대시보드에서는 어떻게 보이나" 에 답할 수 없고, 넷을 한 판에 늘어놓으면 각각이
// 알아볼 수 없게 작아진다.
//
// 조각은 **가짜 마크업**이지 실제 컴포넌트가 아니다. 실물을 끌어오면 편집기 스토어와
// 대시보드 훅이 딸려 오고, 미리보기 하나 때문에 설정 화면에 앱 절반이 적재된다.
// 같은 것은 토큰 이름이고 다른 것은 구조다.
//
// @spec SPEC-THEME-001 §결정 3 · REQ-04 · REQ-05 · 불변식 I3 · I4 · I7

import { useState, type CSSProperties, type ReactNode } from 'react';

import { resolveTokens, type PresetId, type ThemeTokens } from '@/lib/theme/tokens';
import { cn } from '@/lib/utils/cn';

import {
  PREVIEW_PARTS,
  PREVIEW_TABS,
  areaForToken,
  partsUsingToken,
  type PreviewArea,
} from './themePreviewParts';

export interface ThemePreviewProps {
  /** 편집 중인 프리셋. 화면에 적용 중인 것이 **아니다**. */
  target: PresetId;
  /** 그 프리셋의 사용자 오버라이드. */
  overrides: ThemeTokens;
  /** 표에서 고른 토큰 — 그 토큰을 쓰는 조각에 윤곽선을 두른다. */
  highlightToken?: string | undefined;
  /**
   * 지금 골라 둔 조각. 커서가 표를 떠나도 표시가 남아 있어야, 무엇을 고쳤는지
   * 보인다.
   */
  selectedPart?: string | undefined;
  /** 조각을 눌렀을 때. 표를 그 토큰 행으로 데려간다. */
  onPickPart?: (partId: string) => void;
}

/** 토큰 값을 컨테이너 인라인 CSS 변수로 편다. */
function tokenStyle(target: PresetId, overrides: ThemeTokens): CSSProperties {
  const resolved = resolveTokens(target, overrides);
  const style: Record<string, string> = {};
  for (const [cssVar, value] of Object.entries(resolved)) style[cssVar] = value;
  return style as CSSProperties;
}

/** `var()` 참조 — 컨테이너가 덮어쓴 값을 읽는다. */
function v(token: string): string {
  return `var(--color-${token})`;
}

export function ThemePreview({
  target,
  overrides,
  highlightToken,
  selectedPart,
  onPickPart,
}: ThemePreviewProps): React.ReactElement {
  const [picked, setPicked] = useState<Exclude<PreviewArea, 'always'>>('dashboard');

  // 표에서 짚은 색이 지금 탭에 없으면 그 색이 있는 탭으로 따라간다. 따라가지 않으면
  // 강조가 아무 데도 나타나지 않고, 사용자는 "이 색은 아무 데도 안 쓰이나" 로 읽는다.
  const followed = highlightToken === undefined ? undefined : areaForToken(highlightToken);
  const area = followed ?? picked;

  const lit = highlightToken === undefined ? [] : partsUsingToken(highlightToken);

  /**
   * 조각 하나의 공통 껍데기. 강조와 클릭이 여기 붙는다.
   *
   * `<button>` 이 아니라 `role="button"` 인 것은 조각이 **겹쳐 놓이기** 때문이다 —
   * 노드와 연결선은 캔버스 **안**에 있고, `<button>` 안의 `<button>` 은 유효하지 않은
   * 마크업이다. 안쪽 조각은 클릭을 삼켜 바깥 조각으로 번지지 않게 한다.
   */
  const part = (id: string, className: string, style: CSSProperties, children: ReactNode) => {
    const label = PREVIEW_PARTS.find((p) => p.id === id)?.label ?? id;
    const pick = (e: { stopPropagation: () => void }): void => {
      e.stopPropagation();
      onPickPart?.(id);
    };
    return (
      <div
        role="button"
        tabIndex={0}
        data-part={id}
        data-testid={`theme-preview-${id}`}
        data-lit={lit.includes(id) ? 'true' : 'false'}
        aria-label={label}
        onClick={pick}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            pick(e);
          }
        }}
        data-selected={selectedPart === id ? 'true' : 'false'}
        className={cn(
          'relative cursor-pointer text-left transition-shadow',
          // 강조(표를 스쳐 갈 때)는 실선, 선택(눌러 둔 것)은 점선이다. 둘이 같은
          // 모양이면 "지금 무엇이 고정되어 있는가" 를 구분할 수 없다.
          lit.includes(id) && 'outline outline-2 outline-offset-2 outline-blue-500',
          !lit.includes(id) &&
            selectedPart === id &&
            'outline outline-2 outline-offset-2 outline-dashed outline-blue-400',
          className,
        )}
        style={style}
      >
        {children}
      </div>
    );
  };

  /** 상태 점 + 글자. */
  const dot = (token: string, label: string) => (
    <span key={token} className="flex items-center gap-1 text-[10px]">
      <span className="h-2 w-2 rounded-full" style={{ background: v(token) }} />
      <span style={{ color: v('text-muted') }}>{label}</span>
    </span>
  );

  return (
    <div
      data-testid="theme-preview"
      data-target={target}
      data-area={area}
      // `.dark` 를 조건부로 붙인다. 이것이 없으면 토큰은 야간인데 `dark:` 유틸리티는
      // 주간으로 그려져, 방금 고친 결함과 **같은 어긋남**이 미리보기 안에 재현된다.
      className={cn('rounded-lg border p-4', target === 'night' && 'dark')}
      style={{ ...tokenStyle(target, overrides), borderColor: v('border-default') }}
    >
      {/* 화면 고르기 */}
      <div className="mb-3 flex flex-wrap gap-1">
        {PREVIEW_TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            data-testid={`theme-preview-tab-${tab.id}`}
            aria-pressed={area === tab.id}
            onClick={() => setPicked(tab.id)}
            className="rounded-md border px-2 py-1 text-[11px] font-medium transition-colors"
            style={
              area === tab.id
                ? {
                    background: v('interactive-muted'),
                    borderColor: v('interactive-primary'),
                    color: v('interactive-active'),
                  }
                : {
                    background: 'transparent',
                    borderColor: v('border-default'),
                    color: v('text-secondary'),
                  }
            }
          >
            {tab.label}
          </button>
        ))}
      </div>

      <div className="flex flex-col gap-3" style={{ background: v('bg-primary') }}>
        {/* 앱 셸 — 어느 화면에나 있다 */}
        {part(
          'shell',
          'flex items-stretch gap-1.5 rounded-md p-1.5',
          { background: v('bg-primary') },
          <>
            <span className="w-12 rounded-sm" style={{ background: v('bg-secondary') }} />
            <span
              className="flex-1 rounded-sm px-2.5 py-1.5 text-xs font-semibold"
              style={{
                background: v('bg-secondary'),
                color: v('text-primary'),
                borderBottom: `1px solid ${v('border-subtle')}`,
              }}
            >
              XFlow
            </span>
          </>,
        )}

        {/* --- 대시보드 --- */}
        {area === 'dashboard' && (
          <div className="flex gap-2">
            {part(
              'dashboard-stat',
              'flex-1 rounded-md border p-3',
              { background: v('bg-surface'), borderColor: v('border-default') },
              <>
                <span className="block text-[10px]" style={{ color: v('text-secondary') }}>
                  온도
                </span>
                <span className="block text-lg font-semibold" style={{ color: v('text-primary') }}>
                  42.5 °C
                </span>
                <span className="block text-[10px]" style={{ color: v('text-secondary') }}>
                  ▲ 1.2
                </span>
              </>,
            )}
            {part(
              'dashboard-chart',
              'flex-1 rounded-md border p-3',
              { background: v('bg-surface'), borderColor: v('border-default') },
              <>
                <span className="mb-1.5 block text-[10px]" style={{ color: v('text-muted') }}>
                  처리량
                </span>
                <span className="flex h-10 items-end gap-1">
                  {[40, 65, 30, 80, 55, 70].map((h, i) => (
                    <span
                      key={i}
                      className="flex-1 rounded-sm"
                      style={{ height: `${h}%`, background: v('interactive-primary') }}
                    />
                  ))}
                </span>
              </>,
            )}
          </div>
        )}

        {/* --- 플로우 편집기 --- */}
        {area === 'flow' &&
          part(
            'canvas',
            'relative flex h-32 items-center gap-3 overflow-hidden rounded-md p-3',
            {
              background: v('bg-secondary'),
              // 점격자. 실제 캔버스와 같은 토큰을 쓴다.
              backgroundImage: `radial-gradient(${v('flow-dot')} 1px, transparent 1px)`,
              backgroundSize: '12px 12px',
            },
            <>
              <span
                className="absolute inset-x-10 inset-y-3 rounded-md border-2 border-dashed"
                style={{ borderColor: `color-mix(in srgb, ${v('flow-area')} 70%, transparent)` }}
              />
              {part(
                'node',
                'relative z-10 flex items-center gap-2 rounded-md border px-2.5 py-2',
                { background: v('bg-surface'), borderColor: v('border-default') },
                <>
                  <span className="h-6 w-6 rounded-md" style={{ background: v('bg-sunken') }} />
                  <span className="flex flex-col leading-none">
                    <span className="text-xs font-medium" style={{ color: v('text-primary') }}>
                      switch
                    </span>
                    <span className="text-[10px]" style={{ color: v('text-muted') }}>
                      switch
                    </span>
                  </span>
                </>,
              )}
              {part(
                'edge',
                'relative z-10 h-0.5 w-10 rounded-full',
                { background: v('flow-edge') },
                null,
              )}
              <span
                className="relative z-10 h-4 w-4 rounded-full"
                style={{ background: v('bg-surface'), border: `2px solid ${v('border-default')}` }}
              />
            </>,
          )}

        {/* --- 에이전트 · 디바이스 목록 --- */}
        {area === 'list' && (
          <div className="overflow-hidden rounded-md">
            {part(
              'list-header',
              'flex justify-between px-2.5 py-1.5 text-[10px] font-medium uppercase tracking-wider',
              {
                background: v('bg-primary'),
                color: v('text-muted'),
                borderBottom: `1px solid ${v('border-default')}`,
              },
              <>
                <span>이름</span>
                <span>상태</span>
              </>,
            )}
            {part(
              'list-rows',
              'flex flex-col',
              { background: v('bg-surface') },
              <>
                {(
                  [
                    ['modbus-01', 'status-running', '실행 중', false],
                    ['chirpstack-02', 'status-stopped', '정지', true],
                    ['hvac-03', 'status-error', '오류', false],
                  ] as const
                ).map(([name, token, label, hover]) => (
                  <span
                    key={name}
                    className="flex items-center justify-between px-2.5 py-2 text-[11px]"
                    style={{
                      // 한 행만 호버 상태로 그린다 — 부유 배경이 어디 쓰이는지 보여야 한다.
                      background: hover ? v('bg-elevated') : 'transparent',
                      borderTop: `1px solid ${v('border-subtle')}`,
                    }}
                  >
                    <span style={{ color: v('text-primary') }}>{name}</span>
                    <span className="flex items-center gap-1">
                      <span className="h-2 w-2 rounded-full" style={{ background: v(token) }} />
                      <span style={{ color: v('text-secondary') }}>{label}</span>
                    </span>
                  </span>
                ))}
              </>,
            )}
          </div>
        )}

        {/* --- 스케줄 --- */}
        {area === 'schedule' &&
          part(
            'schedule-rows',
            'flex flex-col overflow-hidden rounded-md border',
            { background: v('bg-surface'), borderColor: v('border-default') },
            <>
              {(
                [
                  ['야간 절전', true, '매일 22:00', '냉방 정지'],
                  ['주말 점검', false, '토 09:00', '상태 조회'],
                ] as const
              ).map(([name, enabled, plan, action], i) => (
                <span
                  key={name}
                  className="flex items-center gap-2 px-2.5 py-2 text-[10px]"
                  style={i === 0 ? undefined : { borderTop: `1px solid ${v('border-default')}` }}
                >
                  <span className="flex-1 font-medium" style={{ color: v('text-primary') }}>
                    {name}
                  </span>
                  <span
                    className="rounded-full px-1.5 py-0.5 font-medium"
                    style={
                      enabled
                        ? { background: v('interactive-muted'), color: v('interactive-active') }
                        : { background: v('bg-sunken'), color: v('text-muted') }
                    }
                  >
                    {enabled ? '활성' : '비활성'}
                  </span>
                  <span className="w-16 text-right" style={{ color: v('text-secondary') }}>
                    {plan}
                  </span>
                  <span className="w-14 text-right" style={{ color: v('text-secondary') }}>
                    {action}
                  </span>
                </span>
              ))}
              <span
                className="flex items-center gap-1 px-2.5 py-1.5 text-[10px]"
                style={{ borderTop: `1px solid ${v('border-default')}` }}
              >
                <span
                  className="h-2 w-2 rounded-full"
                  style={{ background: v('status-warning') }}
                />
                <span style={{ color: v('text-muted') }}>다음 실행까지 3시간</span>
              </span>
            </>,
          )}

        {/* 단추 · 상태 — 어느 화면에나 있다 */}
        {part(
          'controls',
          'flex flex-wrap items-center gap-2 rounded-md p-1.5',
          { background: v('bg-primary') },
          <>
            <span
              className="rounded px-2 py-1 text-[10px] font-medium"
              style={{ background: v('interactive-primary'), color: v('text-inverse') }}
            >
              기본
            </span>
            <span
              className="rounded px-2 py-1 text-[10px]"
              style={{ background: v('interactive-hover'), color: v('text-inverse') }}
            >
              호버
            </span>
            <span
              className="rounded px-2 py-1 text-[10px]"
              style={{ background: v('interactive-muted'), color: v('interactive-active') }}
            >
              선택
            </span>
            <span
              className="rounded border px-2 py-1 text-[10px]"
              style={{ borderColor: v('border-strong'), color: v('text-secondary') }}
            >
              보조
            </span>
            {(
              [
                ['status-running', '실행'],
                ['status-stopped', '정지'],
                ['status-error', '오류'],
                ['status-warning', '주의'],
                ['status-info', '안내'],
              ] as const
            ).map(([token, label]) => dot(token, label))}
          </>,
        )}
      </div>
    </div>
  );
}
