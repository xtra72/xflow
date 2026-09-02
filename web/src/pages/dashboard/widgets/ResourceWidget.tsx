// 프로세스 리소스 위젯.
// 모니터링 메트릭(CPU, 메모리, 처리량, 에러율)을 미니 시계열 차트와 함께 표시한다.

import { useEffect, useRef } from 'react';
import { AlertTriangle, Cpu, HardDrive, Zap } from 'lucide-react';
import { Area, AreaChart, ResponsiveContainer } from 'recharts';

import { useMetricsTarget } from '@/hooks/useMetricsTarget';
import { useTranslation } from '@/lib/i18n';
import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { useUIStore } from '@/stores/uiStore';
import { usePanelTitleStyle, usePanelTitleVisible } from '../panelChromeContext';
import {
  type MetricKey,
  type PanelConfig,
} from '@/stores/uiStore';

/** 히스토리에 보관할 최대 데이터 포인트 수 */
const MAX_HISTORY = 30;

interface ResourceWidgetProps {
  metrics: Record<string, unknown> | undefined;
  /** 패널 설정 (멀티-대시보드 모델에서 전달) */
  panelConfig?: PanelConfig;
}

/**
 * 메트릭 데이터에서 숫자 값을 안전하게 추출한다.
 * 여러 키를 순서대로 시도하여 첫 번째 유효값을 반환한다.
 */
function extractNumber(metrics: Record<string, unknown>, ...keys: string[]): number | null {
  for (const key of keys) {
    const value = metrics[key];
    if (typeof value === 'number' && !Number.isNaN(value)) return value;
    if (typeof value === 'string') {
      const parsed = parseFloat(value);
      if (!Number.isNaN(parsed)) return parsed;
    }
  }
  return null;
}

/**
 * 미니 차트 포함 메트릭 카드.
 */
function MetricCard({
  icon,
  label,
  display,
  history,
  color,
  accentColor,
}: {
  icon: React.ReactNode;
  label: string;
  display: string;
  history: { v: number }[];
  color: string;
  /** 카드 전체 악센트 색상 (아이콘, 수치, 차트 통합) */
  accentColor?: string;
}) {
  const cardColor = accentColor ?? color;
  return (
    <div className="flex flex-col items-center gap-2 rounded-md border border-(--color-border-default) p-4">
      <div className="flex items-center gap-2 text-(--color-text-muted)" style={accentColor ? { color: accentColor } : undefined}>
        {icon}
        <span className="text-sm font-medium">{label}</span>
      </div>
      <span className="text-2xl font-bold text-(--color-text-primary)" style={accentColor ? { color: accentColor } : undefined}>
        {display}
      </span>
      {history.length > 1 && (
        <div className="h-10 w-full">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={history}>
              <Area
                type="monotone"
                dataKey="v"
                stroke={cardColor}
                fill={cardColor}
                fillOpacity={0.2}
                strokeWidth={2}
                dot={false}
                isAnimationActive={false}
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}

/** 프로세스 리소스 개요를 표시하는 대시보드 위젯 */
export default function ResourceWidget({ metrics: localMetrics, panelConfig }: ResourceWidgetProps) {
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();
  // 원격 대시보드 target(SPEC-REMOTE-001 M10, REQ-L05): 원격이면 노드의 메트릭
  // 스냅샷을 monitor/metrics query-action 으로 취득한다. 로컬은 prop 의 metrics 를
  // 그대로 사용해 회귀 없이 동일 렌더한다.
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const refreshMs = useUIStore((s) => s.dashboardRefreshInterval) * 1000;
  const remoteMetrics = useMetricsTarget(target, refreshMs, remote);
  const metrics = remote ? remoteMetrics.metrics : localMetrics;

  const { t } = useTranslation();

  // 패널 설정 (멀티-대시보드 패널 config에서 읽기)
  const title = panelConfig?.title ?? t('dashboard.panelTypes.resource');
  const visibleMetrics = (panelConfig?.config?.visibleMetrics as MetricKey[]) ?? ['cpu', 'memory', 'throughput', 'errorRate'];
  const panelColor = panelConfig?.config?.panelColor as string | undefined;
  const accentElements = (panelConfig?.config?.accentElements as Record<string, string | boolean>) ?? {};
  const acColor = (group: string): string | undefined => {
    if (accentElements[group] === false) return undefined;
    const val = accentElements[group];
    if (typeof val === 'string') return val;
    return panelColor;
  };

  // 메트릭 추출 (REST 필드명 + WS 필드명 양쪽 시도)
  const cpuPercent = metrics ? extractNumber(metrics, 'cpu_usage_percent', 'cpu') : null;
  const memPercent = metrics ? extractNumber(metrics, 'memory_usage_percent', 'memory') : null;
  const throughput = metrics ? extractNumber(metrics, 'throughput', 'messages_per_second') : null;
  const errorRate = metrics ? extractNumber(metrics, 'error_rate', 'error_rate_percent') : null;

  // 실 시계열 히스토리 누적
  const cpuHistory = useRef<{ v: number }[]>([]);
  const memHistory = useRef<{ v: number }[]>([]);
  const throughputHistory = useRef<{ v: number }[]>([]);
  const errorRateHistory = useRef<{ v: number }[]>([]);

  useEffect(() => {
    if (cpuPercent !== null) {
      cpuHistory.current = [...cpuHistory.current, { v: cpuPercent }].slice(-MAX_HISTORY);
    }
    if (memPercent !== null) {
      memHistory.current = [...memHistory.current, { v: memPercent }].slice(-MAX_HISTORY);
    }
    if (throughput !== null) {
      throughputHistory.current = [...throughputHistory.current, { v: throughput }].slice(-MAX_HISTORY);
    }
    if (errorRate !== null) {
      errorRateHistory.current = [...errorRateHistory.current, { v: errorRate }].slice(-MAX_HISTORY);
    }
  }, [cpuPercent, memPercent, throughput, errorRate]);

  // 그리드 열 수: config에서 읽거나 보이는 메트릭 수로 자동 결정
  const configCols = panelConfig?.config?.gridCols as number | undefined;
  const visibleCount = visibleMetrics.length;
  const gridCols = configCols ?? (visibleCount <= 2 ? visibleCount : visibleCount <= 3 ? 3 : 4);

  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-6 shadow">
      {/* 헤더: 타이틀 */}
      {showTitle && (
        <div className="mb-4 flex shrink-0 items-center justify-between">
          <h3
            className="text-lg font-semibold text-(--color-text-primary)"
            style={{ ...(acColor('header') ? { color: acColor('header')! } : undefined), ...titleStyle }}
          >
            {title}
          </h3>
        </div>
      )}
      <div
        className="min-h-0 flex-1 grid gap-4 overflow-y-auto"
        style={{ gridTemplateColumns: `repeat(${gridCols}, minmax(0, 1fr))` }}
      >
        {visibleMetrics.includes('cpu') && (
          <MetricCard
            icon={<Cpu className="h-4 w-4" />}
            label={t('dashboard.cpuUsage')}
            display={cpuPercent !== null ? `${cpuPercent.toFixed(1)}%` : '-'}
            history={cpuHistory.current}
            color="#3b82f6"
            accentColor={acColor('cpu')}
          />
        )}
        {visibleMetrics.includes('memory') && (
          <MetricCard
            icon={<HardDrive className="h-4 w-4" />}
            label={t('dashboard.memoryUsage')}
            display={memPercent !== null ? `${memPercent.toFixed(1)}%` : '-'}
            history={memHistory.current}
            color="#8b5cf6"
            accentColor={acColor('memory')}
          />
        )}
        {visibleMetrics.includes('throughput') && (
          <MetricCard
            icon={<Zap className="h-4 w-4" />}
            label={t('dashboard.widget.throughputLabel')}
            display={throughput !== null ? throughput.toLocaleString() : '-'}
            history={throughputHistory.current}
            color="#10b981"
            accentColor={acColor('throughput')}
          />
        )}
        {visibleMetrics.includes('errorRate') && (
          <MetricCard
            icon={<AlertTriangle className="h-4 w-4" />}
            label={t('dashboard.settings.metricLabels.errorRate')}
            display={errorRate !== null ? `${errorRate.toFixed(1)}%` : '-'}
            history={errorRateHistory.current}
            color="#ef4444"
            accentColor={acColor('errorRate')}
          />
        )}
      </div>
    </div>
  );
}
