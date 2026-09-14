// 테마 미리보기 — 고르는 색을 눈으로 보는 자리.
//
// **편집 중인 프리셋을 그린다.** 화면에 적용 중인 것이 아니다. 편집기는 탭으로 고른
// `target` 의 오버라이드를 고치는데 `useTheme` 은 `resolvedPreset` 만 적용하므로,
// 낮에 앉아 야간 팔레트를 고치는 것은 눈을 감고 칠하는 것이다. 그 자리를 메운다.
//
// 조각은 **가짜 마크업**이지 실제 컴포넌트가 아니다. 실제 `CustomNode` 를 끌어오면
// `ReactFlowProvider` · 스토어 · 훅이 딸려 오고, 미리보기 하나 때문에 설정 화면에
// 편집기 전체가 적재된다. 같은 것은 토큰 이름이고 다른 것은 구조다.
//
// @spec SPEC-THEME-001 §결정 3 · REQ-04 · REQ-05 · 불변식 I3 · I4 · I7 (M5)

import type { CSSProperties } from 'react';

import { resolveTokens, type PresetId, type ThemeTokens } from '@/lib/theme/tokens';
import { cn } from '@/lib/utils/cn';

import { PREVIEW_PARTS, partsUsingToken } from './themePreviewParts';

export interface ThemePreviewProps {
  /** 편집 중인 프리셋. 화면에 적용 중인 것이 **아니다**. */
  target: PresetId;
  /** 그 프리셋의 사용자 오버라이드. */
  overrides: ThemeTokens;
  /** 표에서 고른 토큰 — 그 토큰을 쓰는 조각에 윤곽선을 두른다. */
  highlightToken?: string | undefined;
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
  onPickPart,
}: ThemePreviewProps): React.ReactElement {
  const lit = highlightToken === undefined ? [] : partsUsingToken(highlightToken);

  /**
   * 조각 하나의 공통 껍데기. 강조와 클릭이 여기 붙는다.
   *
   * `<button>` 이 아니라 `role="button"` 인 것은 조각이 **겹쳐 놓이기** 때문이다 —
   * 노드와 연결선은 캔버스 **안**에 있고, `<button>` 안의 `<button>` 은 유효하지 않은
   * 마크업이다. 안쪽 조각은 클릭을 삼켜 바깥 조각으로 번지지 않게 한다.
   */
  const part = (id: string, className: string, style: CSSProperties, children: React.ReactNode) => {
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
        className={cn(
          'relative cursor-pointer text-left transition-shadow',
          lit.includes(id) && 'outline outline-2 outline-offset-2 outline-blue-500',
          className,
        )}
        style={style}
      >
        {children}
      </div>
    );
  };

  return (
    <div
      data-testid="theme-preview"
      data-target={target}
      // `.dark` 를 조건부로 붙인다. 이것이 없으면 토큰은 야간인데 `dark:` 유틸리티는
      // 주간으로 그려져, 방금 고친 결함과 **같은 어긋남**이 미리보기 안에 재현된다.
      className={cn('rounded-lg border p-4', target === 'night' && 'dark')}
      style={{ ...tokenStyle(target, overrides), borderColor: v('border-default') }}
    >
      <div className="flex flex-col gap-3" style={{ background: v('bg-primary') }}>
        {/* 앱 셸 — 사이드바 + 헤더 */}
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

        {/* 편집기 캔버스 — 점격자 + 영역 상자 + 노드 + 연결선 */}
        {part(
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
            {part('edge', 'relative z-10 h-0.5 w-10 rounded-full', { background: v('flow-edge') }, null)}
            <span
              className="relative z-10 h-4 w-4 rounded-full"
              style={{ background: v('bg-surface'), border: `2px solid ${v('border-default')}` }}
            />
          </>,
        )}

        <div className="flex gap-2">
          {/* 대시보드 패널 */}
          {part(
            'panel',
            'flex-1 rounded-md border p-3',
            { background: v('bg-surface'), borderColor: v('border-default') },
            <>
              <span className="block text-[10px]" style={{ color: v('text-secondary') }}>
                온도
              </span>
              <span className="block text-lg font-semibold" style={{ color: v('text-primary') }}>
                42.5 °C
              </span>
            </>,
          )}

          {/* 목록 표 */}
          {part(
            'table',
            'flex-1 overflow-hidden rounded-md',
            { background: v('bg-elevated') },
            <>
              <span
                className="flex justify-between px-2 py-1.5 text-[10px]"
                style={{ background: v('bg-sunken'), color: v('text-secondary') }}
              >
                <span>이름</span>
                <span>값</span>
              </span>
              <span
                className="flex justify-between px-2 py-1.5 text-[10px]"
                style={{ color: v('text-muted'), borderTop: `1px solid ${v('border-subtle')}` }}
              >
                <span>node-1</span>
                <span>12</span>
              </span>
            </>,
          )}
        </div>

        {/* 단추 · 상태 */}
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
            ).map(([token, label]) => (
              <span key={token} className="flex items-center gap-1 text-[10px]">
                <span className="h-2 w-2 rounded-full" style={{ background: v(token) }} />
                <span style={{ color: v('text-muted') }}>{label}</span>
              </span>
            ))}
          </>,
        )}
      </div>
    </div>
  );
}
