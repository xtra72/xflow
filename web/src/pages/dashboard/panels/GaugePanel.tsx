// 게이지 차트 패널 컴포넌트.
// 7가지 게이지 타입을 SVG로 렌더링한다.
// simple(도넛), half(반원), multi-ring(동심원), needle(원형 니들),
// needle-rainbow(레인보우), vertical-bar(세로 바), half-rainbow(5단계 등급).
//
// SPEC-CHART-002 M4: 다른 차트 패널과 동일한 공용 Store 데이터 소스(`store_source`)를
// 읽는 **신규 경로**가 추가되었다. 신규 경로는 `data_source === 'store'` + store_source
// 활성 + `series_reduce` 지정이 모두 성립할 때만 진입하며(§2.9 [S1]), 그 외에는 기존
// `config.dataSources[]` 레거시 경로가 한 픽셀도 바뀌지 않는다.
//
// SPEC-CHART-002 M5: 두 경로의 **우선순위 판정**을 `charts/gaugeLegacyBinding.ts` 의
// 순수 함수(`resolveGaugeValueSource`)로 옮겼다. 이 컴포넌트는 판정 결과로 경로를 고를
// 뿐 조건식을 갖지 않는다 — 값 해석 경로가 7지점에 분산되어 있어 조건을 인라인으로
// 두면 조용한 회귀를 만들기 때문이다(plan.md §4). 판정 진리표 7행은
// `gaugeLegacyBinding.test.ts` 가 전수로 잠근다.

import { useCallback, useEffect, useMemo, useState, type ReactElement } from 'react';
import { Gauge as GaugeIcon } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { post } from '@/services/api/client';
import { useAgents } from '@/hooks/useAgent';

import {
  getByPath,
  type SeriesReduceFunc,
} from './charts/chartChannelTypes';
import { ConnectionStatusIcon } from './charts/ConnectionStatusIcon';
import { toNumber } from './charts/chartChannelUtils';
import {
  gaugeValueSourceFlags,
  resolveGaugeValueSource,
} from './charts/gaugeLegacyBinding';
import { reduceAllSeries, type ReducedSeries } from './charts/seriesReduce';
import { SeriesTileGrid } from './charts/SeriesTileGrid';
import { useChartChannel } from './charts/useChartChannel';
import { usePanelSeriesData } from './charts/usePanelSeriesData';
import { resolveStoreAgentName } from './charts/storeAgentResolve';
import { usePanelTitleVisible } from '../panelChromeContext';

// ---- 타입 정의 ----

export type GaugeType =
  | 'simple'
  | 'half'
  | 'multi-ring'
  | 'needle'
  | 'needle-rainbow'
  | 'vertical-bar'
  | 'half-rainbow';

interface GaugePanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

interface ThresholdEntry {
  name: string;
  color: string;
  from: number;
  to: number;
}

/** GaugeSection 에서 저장하는 데이터 소스 바인딩 형상 (PanelSettingsDialog 와 동일) */
interface GaugeDataSource {
  sourceType: 'resource' | 'flow' | 'chart-emitter' | 'store';
  resource?: string;
  flowId?: string;
  dataField?: string;
  channelName?: string;
  displayField?: string;
  /** store 소스 전용: Store 에이전트의 안정적 ID(정본). @spec SPEC-WEB-006 */
  storeAgentId?: string;
  /** store 소스 전용: Store 에이전트 이름(표시/폴백). @spec SPEC-WEB-006 */
  storeAgent?: string;
  storeKey?: string;
  storeNamespace?: string;
}

function pickChartEmitterSource(config: Record<string, unknown>): GaugeDataSource | undefined {
  const list = config.dataSources as GaugeDataSource[] | undefined;
  if (!Array.isArray(list)) return undefined;
  return list.find((d) => d?.sourceType === 'chart-emitter' && !!d.channelName);
}

function pickStoreSource(config: Record<string, unknown>): GaugeDataSource | undefined {
  const list = config.dataSources as GaugeDataSource[] | undefined;
  if (!Array.isArray(list)) return undefined;
  // SPEC-WEB-006: storeAgentId(정본) 또는 storeAgent(구 config 이름) 중 하나로 바인딩 판별.
  return list.find(
    (d) => d?.sourceType === 'store' && (!!d.storeAgentId || !!d.storeAgent) && !!d.storeKey,
  );
}

/** Store 최신 값 폴링 훅 (5초 주기) */
function useStoreLatestValue(
  source: GaugeDataSource | undefined,
): number | undefined {
  const [value, setValue] = useState<number | undefined>(undefined);

  // SPEC-WEB-006: 저장된 storeAgentId 를 현재 에이전트 이름으로 해석해 호출한다.
  // 에이전트 이름이 바뀌어도 id 는 불변이므로 항상 현재 이름으로 조회된다.
  // 구 config(storeAgentId 부재)는 저장된 storeAgent 이름을 그대로 사용(하위호환).
  const { data: agentsResult } = useAgents();
  const resolvedAgentName = resolveStoreAgentName(
    source?.storeAgentId,
    source?.storeAgent ?? '',
    agentsResult?.data,
  );

  const fetchValue = useCallback(async () => {
    if (!resolvedAgentName || !source?.storeKey) return;
    try {
      const resp = await post<{
        entries: Array<{ value: unknown; timestamp: number }>;
      }>(`/store/${encodeURIComponent(resolvedAgentName)}/query`, {
        key: source.storeKey,
        mode: 'latest',
        namespace: source.storeNamespace ?? 'default',
      });
      if (resp.entries && resp.entries.length > 0) {
        const field = source.displayField ?? 'value';
        const raw = field === 'value'
          ? resp.entries[0]!.value
          : undefined;
        const n = toNumber(raw);
        setValue(Number.isFinite(n) ? n : undefined);
      }
    } catch {
      // 조회 실패 시 이전 값 유지
    }
  }, [resolvedAgentName, source?.storeKey, source?.storeNamespace, source?.displayField]);

  useEffect(() => {
    if (!resolvedAgentName || !source?.storeKey) {
      setValue(undefined);
      return;
    }
    fetchValue();
    const id = window.setInterval(fetchValue, 5000);
    return () => window.clearInterval(id);
  }, [resolvedAgentName, source?.storeKey, fetchValue]);

  return value;
}

// ---- 헬퍼 함수 ----

/** 각도(deg)를 라디안으로 변환 */
const toRad = (deg: number) => (deg * Math.PI) / 180;

/** 극좌표 → 직교좌표 (SVG 좌표계, 12시 방향 = 0도) */
function polarToCartesian(cx: number, cy: number, r: number, angleDeg: number) {
  const rad = toRad(angleDeg - 90);
  return { x: cx + r * Math.cos(rad), y: cy + r * Math.sin(rad) };
}

/** 호(arc) SVG path — startAngle→endAngle 시계방향(화면 기준) */
function describeArc(
  cx: number,
  cy: number,
  r: number,
  startAngle: number,
  endAngle: number,
): string {
  const s = polarToCartesian(cx, cy, r, startAngle);
  const e = polarToCartesian(cx, cy, r, endAngle);
  const span = ((endAngle - startAngle) % 360 + 360) % 360;
  const largeArc = span > 180 ? 1 : 0;
  return `M ${s.x},${s.y} A ${r},${r} 0 ${largeArc},1 ${e.x},${e.y}`;
}

/**
 * 파이 sector path — 중심에서 외곽으로 채워진 부채꼴.
 * 360° 전체 (start === end) 인 경우 단일 원으로 처리.
 *
 * 사용처: 게이지 내부 임계값 영역 (옵션) 시각화.
 */
function describeSector(
  cx: number,
  cy: number,
  r: number,
  startAngle: number,
  endAngle: number,
): string {
  const span = ((endAngle - startAngle) % 360 + 360) % 360;
  // 거의 완전 원 → 단일 circle path 로 단순화 (sector self-intersection 방지)
  if (span >= 359.9) {
    return `M ${cx - r},${cy} a ${r},${r} 0 1,0 ${r * 2},0 a ${r},${r} 0 1,0 ${-r * 2},0 Z`;
  }
  if (span < 0.1) return '';
  const start = polarToCartesian(cx, cy, r, startAngle);
  const end = polarToCartesian(cx, cy, r, endAngle);
  const largeArc = span > 180 ? 1 : 0;
  return `M ${cx},${cy} L ${start.x},${start.y} A ${r},${r} 0 ${largeArc},1 ${end.x},${end.y} Z`;
}

/** 도넛형 아크 path (외원 → 내원) */
function describeDonutArc(
  cx: number,
  cy: number,
  outerR: number,
  innerR: number,
  startAngle: number,
  endAngle: number,
): string {
  const outerStart = polarToCartesian(cx, cy, outerR, startAngle);
  const outerEnd = polarToCartesian(cx, cy, outerR, endAngle);
  const innerStart = polarToCartesian(cx, cy, innerR, startAngle);
  const innerEnd = polarToCartesian(cx, cy, innerR, endAngle);
  const largeArc = endAngle - startAngle > 180 ? 1 : 0;
  return [
    `M ${outerStart.x},${outerStart.y}`,
    `A ${outerR},${outerR} 0 ${largeArc},1 ${outerEnd.x},${outerEnd.y}`,
    `L ${innerEnd.x},${innerEnd.y}`,
    `A ${innerR},${innerR} 0 ${largeArc},0 ${innerStart.x},${innerStart.y}`,
    'Z',
  ].join(' ');
}

/** 값을 비율로 변환 */
function normalize(value: number, min: number, max: number) {
  if (max === min) return 0;
  return Math.max(0, Math.min(1, (value - min) / (max - min)));
}

/** 임계값에 따른 색상 결정 */
function getThresholdColor(value: number, thresholds: ThresholdEntry[], fallback: string): string {
  for (const t of thresholds) {
    if (value >= t.from && value <= t.to) return t.color;
  }
  return fallback;
}

// ---- config 파싱 ----

function parseConfig(config: Record<string, unknown>) {
  const value = (config.value as number) ?? 0;
  const min = (config.min as number) ?? 0;
  const max = (config.max as number) ?? 100;
  const unit = (config.unit as string) ?? '%';
  const gaugeType = (config.gaugeType as GaugeType) ?? 'simple';
  const thresholds = (config.thresholds as ThresholdEntry[]) ?? [];
  const values = (config.values as number[]) ?? [];
  // 옵션: 임계값 영역을 게이지 내부에 파이 sector 로 시각화.
  // 명시적으로 false 가 아닌 한 thresholds 가 1개 이상이면 기본 ON.
  // 미설정(undefined) 이면 thresholds 존재 여부로 결정한다.
  const showThresholdZones = config.showThresholdZones === false
    ? false
    : config.showThresholdZones === true || thresholds.length > 0;
  return { value, min, max, unit, gaugeType, thresholds, values, showThresholdZones };
}

// ---- 게이지 렌더러 ----

/** 1. Simple Gauge (도넛형) — 360° 도넛 */
function SimpleGauge({ value, min, max, unit, thresholds, hasValue, showThresholdZones }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const cx = 100, cy = 100, outerR = 90, innerR = 72;
  const valueAngle = ratio * 360;
  const color = thresholds.length > 0
    ? getThresholdColor(value, thresholds, '#5B8FB9')
    : '#5B8FB9';
  // 도넛 내부 (innerR 안쪽) 에 임계값 sector 표시
  const sectorR = innerR - 2;

  return (
    <svg viewBox="0 0 200 200" className="h-full w-full">
      {/* 임계값 영역 (옵션): 도넛 내부에 파이 sector */}
      {showThresholdZones && thresholds.length > 0 && thresholds.map((t, i) => {
        const startRatio = normalize(t.from, min, max);
        const endRatio = normalize(t.to, min, max);
        const path = describeSector(cx, cy, sectorR, startRatio * 360, endRatio * 360);
        if (!path) return null;
        return <path key={i} d={path} fill={t.color} opacity={0.25} />;
      })}
      <circle cx={cx} cy={cy} r={(outerR + innerR) / 2} fill="none"
        stroke="#E2E8F0" strokeWidth={outerR - innerR} />
      {hasValue && valueAngle > 0.5 && (
        <path d={describeDonutArc(cx, cy, outerR, innerR, 0, valueAngle)} fill={color} />
      )}
      <text x={cx} y={cy - 2} textAnchor="middle" dominantBaseline="central"
        className="fill-(--color-text-primary)" fontWeight={700}>
        {hasValue ? (
          <>
            <tspan fontSize={28}>{value}</tspan>
            {unit && <tspan fontSize={14} className="fill-(--color-text-muted)">{unit}</tspan>}
          </>
        ) : (
          <tspan fontSize={28}>--</tspan>
        )}
      </text>
    </svg>
  );
}

/** 2. Half-Circular Gauge (반원형) — 상단 180° */
function HalfGauge({ value, min, max, unit, thresholds, hasValue, showThresholdZones }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const cx = 120, cy = 100, outerR = 80, innerR = 62;
  const startAngle = 270; // 9시(왼쪽) 시작
  const totalAngle = 180; // → 3시(오른쪽) 끝
  const valueAngle = ratio * totalAngle;
  const color = thresholds.length > 0
    ? getThresholdColor(value, thresholds, '#5B8FB9')
    : '#5B8FB9';
  // 반원 내부에 sector — 트랙(도넛 띠) 안쪽 영역에 임계값 표시
  const sectorR = innerR - 2;

  return (
    <svg viewBox="0 0 240 140" className="h-full w-full">
      {/* 임계값 영역 (옵션): 반원 내부 파이 sector (180° 전체에 매핑) */}
      {showThresholdZones && thresholds.length > 0 && thresholds.map((t, i) => {
        const startRatio = normalize(t.from, min, max);
        const endRatio = normalize(t.to, min, max);
        const sa = startAngle + startRatio * totalAngle;
        const ea = startAngle + endRatio * totalAngle;
        const path = describeSector(cx, cy, sectorR, sa, ea);
        if (!path) return null;
        return <path key={i} d={path} fill={t.color} opacity={0.25} />;
      })}
      {/* 트랙 — 도넛형으로 통일 (round cap 아티팩트 제거) */}
      <path d={describeDonutArc(cx, cy, outerR, innerR, startAngle, startAngle + totalAngle)}
        fill="#E2E8F0" />
      {hasValue && valueAngle > 0.5 && (
        <path d={describeDonutArc(cx, cy, outerR, innerR, startAngle, startAngle + valueAngle)}
          fill={color} />
      )}
      <text x={cx - outerR - 4} y={cy + 12} textAnchor="end"
        className="fill-(--color-text-muted)" fontSize={9} fontWeight={500}>
        {min}
      </text>
      <text x={cx + outerR + 4} y={cy + 12} textAnchor="start"
        className="fill-(--color-text-muted)" fontSize={9} fontWeight={500}>
        {max}
      </text>
      <text x={cx} y={cy + 10} textAnchor="middle" dominantBaseline="central"
        className="fill-(--color-text-primary)" fontWeight={700}>
        {hasValue ? (
          <>
            <tspan fontSize={24}>{value}</tspan>
            {unit && <tspan fontSize={12} className="fill-(--color-text-muted)">{unit}</tspan>}
          </>
        ) : (
          <tspan fontSize={24}>--</tspan>
        )}
      </text>
    </svg>
  );
}

/** 3. Multi-Ring Gauge (동심원) — 270° 3개 링 */
function MultiRingGauge({ value, min, max, values, hasValue }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const cx = 100, cy = 100;
  const rings = [
    { val: value, color: '#2C6E8A', outerR: 92, innerR: 76 },
    { val: values[0] ?? 74, color: '#2ABFBF', outerR: 72, innerR: 56 },
    { val: values[1] ?? 82, color: '#F0A04B', outerR: 52, innerR: 36 },
  ];
  const startAngle = 225; // 270° arc: 7시 → 5시 (하단 열림)
  const totalAngle = 270;

  return (
    <svg viewBox="0 0 200 200" className="h-full w-full">
      {rings.map((ring, i) => {
        const ratio = normalize(ring.val, min, max);
        const angle = ratio * totalAngle;
        const midR = (ring.outerR + ring.innerR) / 2;
        const thickness = ring.outerR - ring.innerR;
        return (
          <g key={i}>
            {/* 트랙 */}
            <path d={describeArc(cx, cy, midR, startAngle, startAngle + totalAngle)}
              fill="none" stroke="#E2E8F0" strokeWidth={thickness} strokeLinecap="round" />
            {hasValue && angle > 0.5 && (
              <path d={describeArc(cx, cy, midR, startAngle, startAngle + angle)}
                fill="none" stroke={ring.color} strokeWidth={thickness} strokeLinecap="round" />
            )}
            <text x={cx} y={cy - 36 + i * 24} textAnchor="middle" dominantBaseline="central"
              fill="#FFFFFF" fontSize={11} fontWeight={700}>
              {hasValue ? Math.round(ring.val) : '--'}
            </text>
          </g>
        );
      })}
    </svg>
  );
}

/** 4. Circular Needle (원형 니들) — 360° + 니들 */
function NeedleGauge({ value, min, max, unit, thresholds, hasValue, showThresholdZones }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const cx = 100, cy = 100, r = 80;
  const needleAngle = ratio * 360;
  const needleEnd = polarToCartesian(cx, cy, r - 14, needleAngle);
  const color = thresholds.length > 0
    ? getThresholdColor(value, thresholds, '#EF4444')
    : '#EF4444';
  const ticks = Array.from({ length: 11 }, (_, i) => i);
  // 파이 sector 반경 — 외곽 링 안쪽으로 약간 들어가도록
  const sectorR = r - 6;

  return (
    <svg viewBox="0 0 200 200" className="h-full w-full">
      {/*
        임계값 영역 (옵션): 게이지 내부를 파이 sector 로 분할.
        - 360° 게이지에서 각 임계값의 [from, to] 구간이 sector 의 시작/끝 각도.
        - 외곽 링/눈금/니들보다 BEFORE 그려서 배경처럼 작용.
      */}
      {showThresholdZones && thresholds.length > 0 && thresholds.map((t, i) => {
        const startRatio = normalize(t.from, min, max);
        const endRatio = normalize(t.to, min, max);
        const startAngle = startRatio * 360;
        const endAngle = endRatio * 360;
        const path = describeSector(cx, cy, sectorR, startAngle, endAngle);
        if (!path) return null;
        return (
          <path key={i} d={path} fill={t.color} opacity={0.25} />
        );
      })}
      {/* 외곽 링 */}
      <circle cx={cx} cy={cy} r={r} fill="none" stroke={color} strokeWidth={10} />
      {/* 내부 원 */}
      <circle cx={cx} cy={cy} r={r - 10} fill="none" stroke="#E2E8F0" strokeWidth={1} />
      {/* 눈금 + 라벨 */}
      {ticks.map((i) => {
        const angle = (i / 10) * 360;
        const tickStart = polarToCartesian(cx, cy, r - 2, angle);
        const tickEnd = polarToCartesian(cx, cy, r - 10, angle);
        const labelPos = polarToCartesian(cx, cy, r - 22, angle);
        const tickValue = Math.round(min + ((max - min) * i) / 10);
        return (
          <g key={i}>
            <line x1={tickStart.x} y1={tickStart.y} x2={tickEnd.x} y2={tickEnd.y}
              stroke={color} strokeWidth={2} />
            <text x={labelPos.x} y={labelPos.y} textAnchor="middle" dominantBaseline="central"
              className="fill-(--color-text-muted)" fontSize={7} fontWeight={500}>
              {tickValue}
            </text>
          </g>
        );
      })}
      {/* 니들 — 값이 있을 때만. 다크모드에서도 보이도록 text-primary 사용. */}
      {hasValue && (
        <>
          <line x1={cx} y1={cy} x2={needleEnd.x} y2={needleEnd.y}
            className="stroke-(--color-text-primary)" strokeWidth={2} strokeLinecap="round" />
          <circle cx={cx} cy={cy} r={5} className="fill-(--color-text-primary)" />
        </>
      )}
      {!hasValue && (
        <circle cx={cx} cy={cy} r={4} fill="#9CA3AF" />
      )}
      {/* 값 배지 — 흰 텍스트와의 대비를 위해 항상 어두운 배경 유지 */}
      <rect x={cx - 26} y={cy + 28} width={52} height={20} rx={4} fill="#1E293B" />
      <text x={cx} y={cy + 38} textAnchor="middle" dominantBaseline="central"
        fill="#FFFFFF" fontWeight={700}>
        {hasValue ? (
          <>
            <tspan fontSize={10}>{value}</tspan>
            {unit && <tspan fontSize={7} opacity={0.7}>{unit}</tspan>}
          </>
        ) : (
          <tspan fontSize={10}>--</tspan>
        )}
      </text>
    </svg>
  );
}

/** 5. Needle Rainbow (레인보우) — 270° 속도계 스타일 */
function NeedleRainbowGauge({ value, min, max, unit, thresholds, hasValue }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const cx = 110, cy = 105, outerR = 85, innerR = 75;
  const startAngle = 225; // 7시 방향 시작 (하단 열림)
  const totalAngle = 270;
  const needleAngle = startAngle + ratio * totalAngle;

  // 기본 3구간 또는 임계값 사용
  const segments = thresholds.length >= 2
    ? thresholds.map((t) => ({
        color: t.color,
        startRatio: normalize(t.from, min, max),
        endRatio: normalize(t.to, min, max),
      }))
    : [
        { color: '#2BBDB1', startRatio: 0, endRatio: 0.5 },
        { color: '#E8943A', startRatio: 0.5, endRatio: 0.8 },
        { color: '#8B8055', startRatio: 0.8, endRatio: 1 },
      ];

  const needleTip = polarToCartesian(cx, cy, outerR - 4, needleAngle);
  const needleBase1 = polarToCartesian(cx, cy, 6, needleAngle + 90);
  const needleBase2 = polarToCartesian(cx, cy, 6, needleAngle - 90);

  // 눈금 수
  const tickCount = 10;
  const labelCount = Math.min(11, Math.ceil((max - min) / ((max - min) / 10)) + 1);

  return (
    <svg viewBox="0 0 220 210" className="h-full w-full">
      {/* 색상 아크 세그먼트 */}
      {segments.map((seg, i) => {
        const sAngle = startAngle + seg.startRatio * totalAngle;
        const eAngle = startAngle + seg.endRatio * totalAngle;
        if (eAngle - sAngle < 0.5) return null;
        const midR = (outerR + innerR) / 2;
        return (
          <path key={i}
            d={describeArc(cx, cy, midR, sAngle, eAngle)}
            fill="none" stroke={seg.color} strokeWidth={outerR - innerR} />
        );
      })}
      {/* 눈금 */}
      {Array.from({ length: tickCount * 2 + 1 }, (_, i) => {
        const angle = startAngle + (i / (tickCount * 2)) * totalAngle;
        const isMajor = i % 2 === 0;
        const s = polarToCartesian(cx, cy, outerR, angle);
        const e = polarToCartesian(cx, cy, isMajor ? innerR : innerR + 4, angle);
        return (
          <line key={i} x1={s.x} y1={s.y} x2={e.x} y2={e.y}
            stroke="#FFFFFF" strokeWidth={isMajor ? 2 : 1} />
        );
      })}
      {/* 숫자 라벨 */}
      {Array.from({ length: labelCount }, (_, i) => {
        const angle = startAngle + (i / (labelCount - 1)) * totalAngle;
        const pos = polarToCartesian(cx, cy, outerR + 14, angle);
        const labelVal = Math.round(min + ((max - min) * i) / (labelCount - 1));
        return (
          <text key={i} x={pos.x} y={pos.y} textAnchor="middle" dominantBaseline="central"
            className="fill-(--color-text-muted)" fontSize={7} fontWeight={600}>
            {labelVal}
          </text>
        );
      })}
      {/* 니들 — 값이 있을 때만 표시. 다크모드에서도 보이도록 text-primary 사용. */}
      {hasValue && (
        <>
          <polygon
            points={`${needleTip.x},${needleTip.y} ${needleBase1.x},${needleBase1.y} ${needleBase2.x},${needleBase2.y}`}
            className="fill-(--color-text-primary)"
          />
          <circle cx={cx} cy={cy} r={6} className="fill-(--color-text-primary)" />
        </>
      )}
      {!hasValue && (
        <circle cx={cx} cy={cy} r={4} fill="#9CA3AF" />
      )}
      {/* 값 텍스트 */}
      <text x={cx} y={cy + 24} textAnchor="middle" dominantBaseline="central"
        className="fill-(--color-text-primary)" fontWeight={700}>
        {hasValue ? (
          <>
            <tspan fontSize={12}>{value}</tspan>
            {unit && <tspan fontSize={8} className="fill-(--color-text-muted)">{unit}</tspan>}
          </>
        ) : (
          <tspan fontSize={12}>--</tspan>
        )}
      </text>
    </svg>
  );
}

/** 9. Vertical Bar Gauge (세로 바) */
function VerticalBarGauge({ value, min, max, unit, thresholds, hasValue }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const barW = 48, barH = 180, x = 60, y = 10;
  const fillH = Math.max(0, ratio * barH);
  const color = thresholds.length > 0
    ? getThresholdColor(value, thresholds, '#F97316')
    : '#F97316';
  // min~max 기반 5단계 눈금
  const tickCount = 5;
  const ticks = Array.from({ length: tickCount + 1 }, (_, i) =>
    Math.round(min + ((max - min) * i) / tickCount),
  );

  return (
    <svg viewBox="0 0 140 210" className="h-full w-full">
      {/* 임계값 배경 구간 — thresholds 가 있으면 구간별 색상 */}
      {thresholds.length >= 2 ? (
        thresholds.map((t, i) => {
          const fromRatio = normalize(t.from, min, max);
          const toRatio = normalize(t.to, min, max);
          const segY = y + barH - toRatio * barH;
          const segH = (toRatio - fromRatio) * barH;
          return (
            <rect key={i} x={x} y={segY} width={barW} height={Math.max(0, segH)}
              fill={t.color} opacity={0.3} />
          );
        })
      ) : (
        <rect x={x} y={y} width={barW} height={barH} rx={3} fill="#334155" />
      )}
      {/* 값 바 */}
      {hasValue && fillH > 0 && (
        <rect x={x} y={y + barH - fillH} width={barW} height={fillH} fill={color} />
      )}
      {/* Y축 눈금 */}
      {ticks.map((t) => {
        const tickRatio = normalize(t, min, max);
        const ty = y + barH - tickRatio * barH;
        return (
          <g key={t}>
            <line x1={x} y1={ty} x2={x + barW} y2={ty} stroke="#FFFFFF" strokeWidth={0.5} opacity={0.3} />
            <text x={x - 6} y={ty + 3} textAnchor="end"
              className="fill-(--color-text-muted)" fontSize={8} fontWeight={500}>
              {t}
            </text>
          </g>
        );
      })}
      <text x={x + barW / 2} y={y + barH + 16} textAnchor="middle"
        dominantBaseline="central" className="fill-(--color-text-primary)" fontWeight={700}>
        {hasValue ? (
          <>
            <tspan fontSize={13}>{value}</tspan>
            {unit && <tspan fontSize={8} className="fill-(--color-text-muted)">{unit}</tspan>}
          </>
        ) : (
          <tspan fontSize={13}>--</tspan>
        )}
      </text>
    </svg>
  );
}

/** 10. Half Rainbow 2 (5단계 등급) */
function HalfRainbowGauge({ value, min, max, thresholds, hasValue }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const cx = 120, cy = 110, outerR = 80, innerR = 56;
  const startAngle = 180;
  const totalAngle = 180;

  // 기본 5단계 또는 임계값 사용
  const segments = thresholds.length >= 2
    ? thresholds.map((t) => ({
        color: t.color,
        label: t.name,
        startRatio: normalize(t.from, min, max),
        endRatio: normalize(t.to, min, max),
      }))
    : [
        { color: '#EF4444', label: 'VERY POOR', startRatio: 0, endRatio: 0.2 },
        { color: '#F97316', label: 'POOR', startRatio: 0.2, endRatio: 0.4 },
        { color: '#EAB308', label: 'FAIR', startRatio: 0.4, endRatio: 0.6 },
        { color: '#22C55E', label: 'GOOD', startRatio: 0.6, endRatio: 0.8 },
        { color: '#3B82F6', label: 'EXCELLENT', startRatio: 0.8, endRatio: 1 },
      ];

  const needleAngle = startAngle + ratio * totalAngle;
  const needleTip = polarToCartesian(cx, cy, outerR - 4, needleAngle);
  const needleBase1 = polarToCartesian(cx, cy, 5, needleAngle + 90);
  const needleBase2 = polarToCartesian(cx, cy, 5, needleAngle - 90);

  return (
    <svg viewBox="0 0 240 150" className="h-full w-full">
      {/* 세그먼트 아크 */}
      {segments.map((seg, i) => {
        const sAngle = startAngle + seg.startRatio * totalAngle;
        const eAngle = startAngle + seg.endRatio * totalAngle;
        if (eAngle - sAngle < 0.5) return null;
        return (
          <g key={i}>
            <path d={describeDonutArc(cx, cy, outerR, innerR, sAngle, eAngle)} fill={seg.color} />
            {/* 구간 라벨 */}
            {seg.label && (() => {
              const midAngle = (sAngle + eAngle) / 2;
              const labelPos = polarToCartesian(cx, cy, (outerR + innerR) / 2, midAngle);
              return (
                <text x={labelPos.x} y={labelPos.y} textAnchor="middle" dominantBaseline="central"
                  fill="#FFFFFF" fontSize={6} fontWeight={600}>
                  {seg.label}
                </text>
              );
            })()}
          </g>
        );
      })}
      {/* 니들 — 값이 있을 때만. 다크모드에서도 보이도록 text-primary 사용. */}
      {hasValue ? (
        <>
          <polygon
            points={`${needleTip.x},${needleTip.y} ${needleBase1.x},${needleBase1.y} ${needleBase2.x},${needleBase2.y}`}
            className="fill-(--color-text-primary)"
          />
          <circle cx={cx} cy={cy} r={6} className="fill-(--color-text-primary)" />
        </>
      ) : (
        <circle cx={cx} cy={cy} r={4} fill="#9CA3AF" />
      )}
      <circle cx={cx} cy={cy} r={3.5} className="fill-(--color-bg-surface)" />
      <text x={cx} y={cy + 18} textAnchor="middle" dominantBaseline="central"
        className="fill-(--color-text-primary)" fontWeight={700}>
        {hasValue ? (
          <tspan fontSize={18}>{value}</tspan>
        ) : (
          <tspan fontSize={18}>--</tspan>
        )}
      </text>
    </svg>
  );
}

// ---- 게이지 타입 디스패치 ----

/**
 * 게이지 타입별 렌더러 선택. 단일 출력(레거시)과 다중 출력(M4)이 **같은** 디스패치를
 * 공유하도록 컴포넌트 밖으로 끌어냈다 — 두 벌로 나뉘면 게이지 타입이 하나 늘 때마다
 * 한쪽만 고치는 사고가 난다.
 */
function renderGaugeByType(
  parsed: ReturnType<typeof parseConfig>,
  hasValue: boolean,
): ReactElement {
  switch (parsed.gaugeType) {
    case 'simple':
      return <SimpleGauge {...parsed} hasValue={hasValue} />;
    case 'half':
      return <HalfGauge {...parsed} hasValue={hasValue} />;
    case 'multi-ring':
      return <MultiRingGauge {...parsed} hasValue={hasValue} />;
    case 'needle':
      return <NeedleGauge {...parsed} hasValue={hasValue} />;
    case 'needle-rainbow':
      return <NeedleRainbowGauge {...parsed} hasValue={hasValue} />;
    case 'vertical-bar':
      return <VerticalBarGauge {...parsed} hasValue={hasValue} />;
    case 'half-rainbow':
      return <HalfRainbowGauge {...parsed} hasValue={hasValue} />;
    default:
      return <SimpleGauge {...parsed} hasValue={hasValue} />;
  }
}

/**
 * 게이지 타입별 종횡비(각 렌더러의 `viewBox` 와 동일).
 *
 * 그리드 칸은 높이가 내용으로 결정되는데 게이지 SVG 는 `h-full` 이므로, 감싸는 상자에
 * 종횡비를 주지 않으면 높이가 0 으로 접힌다. 단일 출력 경로는 부모가 `flex-1` 로 높이를
 * 주므로 이 문제가 없다 — 다중 출력에서만 필요하다.
 */
const GAUGE_TILE_ASPECT: Record<GaugeType, string> = {
  simple: '200 / 200',
  half: '240 / 140',
  'multi-ring': '200 / 200',
  needle: '200 / 200',
  'needle-rainbow': '220 / 210',
  'vertical-bar': '140 / 210',
  'half-rainbow': '240 / 150',
};

/**
 * 다중 출력 게이지 1개 — 게이지 + 시리즈 이름 캡션.
 *
 * 색상 축 분리(§2.6): **호(arc) 색은 기존 `thresholds` 가 계속 소유**하고 시리즈 색은
 * 캡션 라벨에만 적용한다. 그래서 `base`(패널 공통 min/max/unit/thresholds/gaugeType)를
 * 그대로 넘기고 `value` 만 시리즈 대표값으로 갈아 끼운다.
 *
 * 대표값이 없으면(`undefined`) 슬롯은 유지하고 값 자리에 `--` 를 표시한다(§2.4) —
 * 각 렌더러가 `hasValue={false}` 에서 이미 그렇게 그린다.
 */
function GaugeTile({
  item,
  base,
}: {
  item: ReducedSeries;
  base: ReturnType<typeof parseConfig>;
}): ReactElement {
  const hasValue = item.value !== undefined && Number.isFinite(item.value);
  const parsed = hasValue ? { ...base, value: item.value! } : base;
  return (
    <>
      <div
        data-testid="gauge-tile-chart"
        className="max-h-full w-full min-w-0"
        style={{ aspectRatio: GAUGE_TILE_ASPECT[base.gaugeType] ?? '1 / 1' }}
      >
        {renderGaugeByType(parsed, hasValue)}
      </div>
      <span
        data-testid="gauge-tile-caption"
        title={item.name}
        className={cn(
          'w-full truncate text-center text-xs font-medium',
          !item.color && 'text-(--color-text-muted)',
        )}
        style={item.color ? { color: item.color } : undefined}
      >
        {item.name}
      </span>
    </>
  );
}

/**
 * 판정이 `legacy` 일 때 공용 시리즈 훅에 넘기는 빈 config.
 *
 * `data_source` 가 없으므로 계약이 `channel` 로 해석하고, 그러면 store·tsdb 훅 모두
 * 인자를 받지 못해 구독도 폴링도 일어나지 않는다 — 종전 `useStoreChartData(undefined,
 * false)` 와 같은 idle 상태다. 모듈 상수로 두어 렌더마다 새 객체가 생기지 않게 한다.
 */
const IDLE_SERIES_CONFIG: Record<string, unknown> = Object.freeze({});

// ---- 메인 컴포넌트 ----

/** 게이지 차트 패널 */
export default function GaugePanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: GaugePanelProps) {
  const showTitle = usePanelTitleVisible();

  // ---- SPEC-CHART-002 M5: 값 소스 판정 ----
  //
  // 판정 자체는 순수 함수가 소유한다(`charts/gaugeLegacyBinding.ts`). 여기서는 결과로
  // 경로를 고르기만 한다. `'store-source'` 는 세 조건의 논리곱이 성립할 때만 나온다 —
  //   1) `data_source === 'store'`   — 사용자가 토글로 명시한 상태
  //   2) `store_source` 활성          — keys 모드 시리즈 ≥ 1 또는 tag 모드 태그 ≥ 1
  //   3) `series_reduce` 지정         — 부재는 "기본값 last" 가 아니라 레거시 경로다
  // 즉 **신규 경로가 실제로 값을 낼 수 있을 때만** 레거시를 밀어낸다(§2.9 [S1] / §4.5).
  const seriesReduce = config.series_reduce as SeriesReduceFunc | undefined;
  const isStoreSourcePath =
    resolveGaugeValueSource(gaugeValueSourceFlags(config)) === 'store-source';

  // ---- 두 경로의 훅 배선 ----
  //
  // 세 훅 모두 조건 없이 항상 호출하고, 진 쪽을 `undefined` 인자로 idle 에 둔다
  // (React 훅 규칙 — `LineChartPanel` / stat / bar / pie 와 같은 형태). idle 경로에서는
  // 구독도 폴링도 일어나지 않으므로, store-source 가 이긴 게이지는 레거시
  // `mode:'latest'` 5초 폴링을 더 이상 발생시키지 않는다.
  //
  // 레거시 계산 코드는 **삭제하지 않는다** — 판정이 `'legacy'` 인 순간 그대로 되살아나며
  // 그것이 이관의 되돌리기 경로다(§4.5). 특성화 CH-11~CH-18 이 이 가지를 계속 지킨다.
  const chartSource = isStoreSourcePath ? undefined : pickChartEmitterSource(config);
  const { entries, status } = useChartChannel(chartSource?.channelName, { maxPoints: 1 });

  const storeSource = isStoreSourcePath ? undefined : pickStoreSource(config);
  const storeValue = useStoreLatestValue(storeSource);

  // 공용 시리즈 훅 하나로 Store 와 TSDB 를 모두 받는다(다른 차트 패널과 같은 배선).
  // 종전에는 `useStoreChartData` 를 직접 불러 store 만 조회했으므로, 게이지에서 TSDB 를
  // 고르면 설정 화면에는 토글이 보이는데 값이 오지 않는 상태였다.
  //
  // 게이지 고유의 레거시 우선순위(§2.9 진리표)는 **여기서 그대로 유지**한다. config 를
  // 조건 없이 넘기면 `series_reduce` 없는 store 게이지까지 폴링이 시작되어, 판정이
  // `legacy` 인데 조회는 도는 상태가 된다. 그래서 판정이 진 경우 빈 config 를 넘겨
  // 종전 `useStoreChartData(undefined, false)` 와 같은 idle 로 둔다.
  const storeChart = usePanelSeriesData(isStoreSourcePath ? config : IDLE_SERIES_CONFIG);

  // chart-emitter 최신 값
  const chartLiveValue = (() => {
    if (!chartSource || entries.length === 0) return undefined;
    const last = entries[entries.length - 1]!;
    const field = chartSource.displayField && chartSource.displayField.length > 0
      ? chartSource.displayField
      : 'value';
    const n = toNumber(getByPath(last, field));
    return Number.isFinite(n) ? n : undefined;
  })();

  // 레거시 경로 안의 우선순위: chart-emitter > store > static.
  //
  // 주의 — 이것은 **값** 우선순위이지 **바인딩** 우선순위가 아니다(특성화 CH-15). 채널이
  // 바인딩되어 있어도 그 채널이 값을 못 내면(entries 0 / 비수치) 조용히 레거시 store 값이
  // 이긴다. spec.md §1.2.4 의 "우선순위 chart-emitter > store > static" 문구는 바인딩
  // 우선순위처럼 읽히지만 실제 동작은 값 우선순위다. M5 는 이 규칙을 바꾸지 않는다.
  //
  // store-source 경로가 이긴 경우 위의 두 레거시 훅이 idle 이므로 두 값 모두 undefined 가
  // 되고, `hasBinding` 은 false 가 된다. 렌더 분기가 그 경우 `hasValue` 를 강제로 false 로
  // 넘기므로(아래 renderGaugeByType 호출) 표시 결과는 M4 와 동일하다.
  const liveValue = chartLiveValue ?? storeValue;
  const hasBinding = !!chartSource || !!storeSource;

  const parsedBase = parseConfig(config);
  const hasValue = !hasBinding || liveValue !== undefined;
  const parsed = liveValue !== undefined
    ? { ...parsedBase, value: liveValue }
    : parsedBase;

  // 시리즈 순서 그대로 대표값 1개씩. 재조회 없이 렌더 시점에만 계산된다(§2.7 [E1]).
  const reduced = useMemo<ReducedSeries[] | null>(
    () =>
      isStoreSourcePath && seriesReduce !== undefined
        ? reduceAllSeries(
            storeChart.seriesEntries,
            storeChart.seriesNames,
            storeChart.seriesStyles,
            seriesReduce,
          )
        : null,
    [
      isStoreSourcePath,
      seriesReduce,
      storeChart.seriesEntries,
      storeChart.seriesNames,
      storeChart.seriesStyles,
    ],
  );
  const showGauges = reduced !== null && reduced.length > 0;

  return (
    <div className={cn(
      'relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4',
      'ring-1 ring-(--color-border-default)',
    )}>
      {/*
        연결 상태 아이콘. Store 데이터 소스 경로에서는 조회 상태를 노출한다(M4.6).
        레거시 경로의 규칙은 그대로다 — chart-emitter 구독 중일 때만 표시하고 레거시
        store 폴링에는 표시하지 않는다(특성화 CH-17 이 잠근 동작).
      */}
      {isStoreSourcePath ? (
        <div className="absolute right-3 top-3 z-10">
          <ConnectionStatusIcon status={storeChart.status} />
        </div>
      ) : chartSource ? (
        <div className="absolute right-3 top-3 z-10">
          <ConnectionStatusIcon status={status} />
        </div>
      ) : null}
      {/* 헤더 */}
      {showTitle && (
        <div className="mb-1 flex shrink-0 items-center gap-2 pr-6">
          <GaugeIcon className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
          <span className="truncate text-sm font-semibold text-(--color-text-primary)">{title}</span>
        </div>
      )}
      {/* 게이지 SVG */}
      {showGauges ? (
        <div
          data-testid="gauge-tiles"
          className="flex min-h-0 flex-1 flex-col justify-center overflow-hidden"
        >
          <SeriesTileGrid
            items={reduced}
            limit={config.multi_output_limit as number | undefined}
            itemKey={(item, i) => `${i}:${item.name}`}
            renderItem={(item) => <GaugeTile item={item} base={parsedBase} />}
          />
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 items-center justify-center">
          {/* Store 경로인데 시리즈가 0개면 신규 경로의 빈 상태(`--`)를 보여준다(§2.4).
              레거시 값으로 몰래 되돌아가지 않는다. */}
          {renderGaugeByType(parsed, isStoreSourcePath ? false : hasValue)}
        </div>
      )}
    </div>
  );
}

