// sysmetrics 패널 3종의 공통 크롬.
//
// 세 패널은 같은 상태 분기를 겪는다 — 에이전트가 없거나, 멈췄거나, 아직 표본이
// 없거나, 그 지표를 수집하지 않는다. 분기 화면을 패널마다 따로 쓰면 문구가 조금씩
// 갈라지고, 사용자는 같은 상황을 세 가지 표현으로 만나게 된다.

import type { ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

import { usePanelTitleStyle, usePanelTitleVisible } from '../../panelChromeContext';
import type { SysMetricsItemOptions, TileAlign, TileTextSize, TileTextWeight } from './sysMetricsItemOptions';
import type { SysMetricsPanelState } from './useSysMetricsSnapshot';

// --- 타일 표시 매핑 ---
//
// 설정값(작게·보통·크게…)을 실제 클래스로 옮긴다. 한 곳에 모아 두어야 타일과
// 미리보기가 같은 크기를 낸다.

/** 가로 정렬 → 클래스 */
const ALIGN_CLASS: Record<TileAlign, string> = {
  left: 'items-start text-left',
  center: 'items-center text-center',
  right: 'items-end text-right',
};

/** 글자 크기 → 클래스 */
const SIZE_CLASS: Record<TileTextSize, string> = {
  sm: 'text-xs',
  md: 'text-sm',
  lg: 'text-lg',
  xl: 'text-xl',
  '2xl': 'text-3xl',
};

/** 글자 굵기 → 클래스 */
const WEIGHT_CLASS: Record<TileTextWeight, string> = {
  normal: 'font-normal',
  medium: 'font-medium',
  bold: 'font-bold',
};

interface SysMetricsPanelShellProps {
  panelId: string;
  title: string;
  icon: LucideIcon;
  /** 헤더 악센트 색 (없으면 기본색) */
  headerColor?: string;
  /** 훅이 판정한 상태 */
  state: SysMetricsPanelState;
  /** 바인딩된 에이전트 이름 (안내 문구에 쓴다) */
  agentName?: string;
  children: ReactNode;
}

/** 상태 → 안내 문구 i18n 키. ready 는 안내가 없다. */
const STATE_MESSAGE_KEY: Record<Exclude<SysMetricsPanelState, 'ready'>, string> = {
  loading: 'sysmetrics.state.loading',
  unavailable: 'sysmetrics.state.unavailable',
  incompatible: 'sysmetrics.state.incompatible',
  no_sample: 'sysmetrics.state.noSample',
  // 멈춘 에이전트는 마지막 표본과 함께 그린다 — 안내는 배너로만 얹는다.
  stopped: 'sysmetrics.state.stopped',
};

/**
 * 패널 껍데기.
 *
 * `stopped` 는 본문을 가리지 않는다. 마지막으로 본 값을 지우면 "멈췄다"와
 * "0 이 되었다"가 화면에서 구별되지 않기 때문이다. 나머지 비정상 상태는 그릴 값
 * 자체가 없으므로 본문을 대신한다.
 */
export function SysMetricsPanelShell({
  panelId,
  title,
  icon: Icon,
  headerColor,
  state,
  agentName,
  children,
}: SysMetricsPanelShellProps) {
  const { t } = useTranslation();
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();

  const blocked = state !== 'ready' && state !== 'stopped';

  return (
    <div
      data-panel-id={panelId}
      className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow"
    >
      {showTitle && (
        <div className="mb-3 flex shrink-0 items-center gap-2">
          <Icon
            className="h-4 w-4 shrink-0 text-(--color-text-muted)"
            style={headerColor ? { color: headerColor } : undefined}
          />
          <span
            className="truncate text-sm font-medium text-(--color-text-primary)"
            style={{ ...(headerColor ? { color: headerColor } : undefined), ...titleStyle }}
          >
            {title}
          </span>
          {agentName && (
            <span className="truncate text-xs text-(--color-text-muted)">{agentName}</span>
          )}
        </div>
      )}

      {state === 'stopped' && (
        <p
          data-testid="sysmetrics-panel-stopped"
          className="mb-2 shrink-0 rounded bg-(--color-bg-sunken) px-2 py-1 text-xs text-(--color-text-muted)"
        >
          {t(STATE_MESSAGE_KEY.stopped)}
        </p>
      )}

      {blocked ? (
        <p
          data-testid="sysmetrics-panel-state"
          data-state={state}
          className="flex flex-1 items-center justify-center text-xs text-(--color-text-muted)"
        >
          {t(STATE_MESSAGE_KEY[state])}
        </p>
      ) : (
        children
      )}
    </div>
  );
}

// --- 지표 타일 ---

/** 대상 하나의 값 (타일이 대상 여럿을 보여줄 때) */
export interface MetricTileRow {
  name: string;
  color: string;
  value?: string;
}

interface MetricTileProps {
  /**
   * 표시 옵션. 없으면 종전 모양(좌측 정렬·값 xl 굵게)으로 그린다 —
   * 옵션을 넘기지 않는 호출부(수집 꺼짐 폴백 등)를 위한 기본값이다.
   */
  options?: SysMetricsItemOptions;
  label: string;
  /** 표시할 값. 수집이 꺼져 있으면 undefined 로 넘긴다. */
  value?: string;
  /**
   * 대상별 값 목록. 주어지면 `value` 대신 이 목록을 줄마다 보여준다.
   *
   * 대상을 둘 이상 고른 패널에서 첫 대상만 그리면 나머지가 조용히 사라진다.
   */
  rows?: MetricTileRow[];
  /** 보조 설명 (범위·대상 이름 등) */
  hint?: string;
  labelColor?: string;
  valueColor?: string;
  /** 수집이 꺼진 항목인지 — 값 0 과 구별해 표시한다. */
  disabled?: boolean;
  testId?: string;
}

/**
 * 지표 한 칸.
 *
 * `disabled` 는 "수집하지 않는다"이고 값이 `-` 인 것은 "아직 값이 없다"이다. 둘을
 * 같게 그리면 설정을 잘못한 것인지 기다리면 되는 것인지 알 수 없다.
 */
export function MetricTile({
  options,
  label,
  value,
  rows,
  hint,
  labelColor,
  valueColor,
  disabled,
  testId,
}: MetricTileProps) {
  const { t } = useTranslation();

  return (
    <div
      data-testid={testId}
      data-disabled={disabled ? 'true' : undefined}
      className={[
        'flex min-w-0 flex-col justify-center rounded-lg bg-(--color-bg-surface) p-3 shadow',
        ALIGN_CLASS[options?.align ?? 'left'],
      ].join(' ')}
    >
      <p
        className={[
          'w-full truncate text-(--color-text-muted)',
          SIZE_CLASS[options?.labelSize ?? 'sm'],
          WEIGHT_CLASS[options?.labelWeight ?? 'normal'],
        ].join(' ')}
        style={labelColor ? { color: labelColor } : undefined}
      >
        {label}
      </p>
      {disabled ? (
        <p className="mt-1 truncate text-sm text-(--color-text-muted)">
          {t('sysmetrics.state.notCollected')}
        </p>
      ) : rows && rows.length > 0 ? (
        <ul className="mt-1 space-y-0.5" data-testid="sysmetrics-tile-rows" data-rows={rows.length}>
          {rows.map((row) => (
            <li key={row.name} className="flex items-baseline justify-between gap-2 text-sm">
              <span className="flex min-w-0 items-center gap-1 text-(--color-text-muted)">
                <span
                  className="inline-block h-1.5 w-1.5 shrink-0 rounded-full"
                  style={{ backgroundColor: row.color }}
                />
                <span className="truncate">{row.name}</span>
              </span>
              <span
                className="shrink-0 font-semibold text-(--color-text-primary)"
                style={valueColor ? { color: valueColor } : undefined}
              >
                {row.value ?? '-'}
              </span>
            </li>
          ))}
        </ul>
      ) : (
        <p
          data-testid={testId ? `${testId}-value` : undefined}
          className={[
            'mt-1 w-full truncate font-bold text-(--color-text-primary)',
            SIZE_CLASS[options?.valueSize ?? 'xl'],
          ].join(' ')}
          style={valueColor ? { color: valueColor } : undefined}
        >
          {value ?? '-'}
        </p>
      )}
      {hint && <p className="truncate text-xs text-(--color-text-muted)">{hint}</p>}
    </div>
  );
}
