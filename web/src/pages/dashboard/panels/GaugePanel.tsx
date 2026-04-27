// 게이지 차트 패널 컴포넌트.
// 7가지 게이지 타입을 SVG로 렌더링한다.
// simple(도넛), half(반원), multi-ring(동심원), needle(원형 니들),
// needle-rainbow(레인보우), vertical-bar(세로 바), half-rainbow(5단계 등급).

import { useCallback, useEffect, useState } from 'react';
import { Gauge as GaugeIcon } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { post } from '@/services/api/client';

import { getByPath } from './charts/chartChannelTypes';
import { ConnectionStatusIcon } from './charts/ConnectionStatusIcon';
import { toNumber } from './charts/chartChannelUtils';
import { useChartChannel } from './charts/useChartChannel';

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
  return list.find((d) => d?.sourceType === 'store' && !!d.storeAgent && !!d.storeKey);
}

/** Store 최신 값 폴링 훅 (5초 주기) */
function useStoreLatestValue(
  source: GaugeDataSource | undefined,
): number | undefined {
  const [value, setValue] = useState<number | undefined>(undefined);

  const fetchValue = useCallback(async () => {
    if (!source?.storeAgent || !source?.storeKey) return;
    try {
      const resp = await post<{
        entries: Array<{ value: unknown; timestamp: number }>;
      }>(`/store/${encodeURIComponent(source.storeAgent)}/query`, {
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
  }, [source?.storeAgent, source?.storeKey, source?.storeNamespace, source?.displayField]);

  useEffect(() => {
    if (!source?.storeAgent || !source?.storeKey) {
      setValue(undefined);
      return;
    }
    fetchValue();
    const id = window.setInterval(fetchValue, 5000);
    return () => window.clearInterval(id);
  }, [source?.storeAgent, source?.storeKey, fetchValue]);

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
  return { value, min, max, unit, gaugeType, thresholds, values };
}

// ---- 게이지 렌더러 ----

/** 1. Simple Gauge (도넛형) — 360° 도넛 */
function SimpleGauge({ value, min, max, unit, thresholds, hasValue }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const cx = 100, cy = 100, outerR = 90, innerR = 72;
  const valueAngle = ratio * 360;
  const color = thresholds.length > 0
    ? getThresholdColor(value, thresholds, '#5B8FB9')
    : '#5B8FB9';

  return (
    <svg viewBox="0 0 200 200" className="h-full w-full">
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
function HalfGauge({ value, min, max, unit, thresholds, hasValue }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const cx = 120, cy = 100, outerR = 80, innerR = 62;
  const startAngle = 270; // 9시(왼쪽) 시작
  const totalAngle = 180; // → 3시(오른쪽) 끝
  const valueAngle = ratio * totalAngle;
  const color = thresholds.length > 0
    ? getThresholdColor(value, thresholds, '#5B8FB9')
    : '#5B8FB9';

  return (
    <svg viewBox="0 0 240 140" className="h-full w-full">
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
function NeedleGauge({ value, min, max, unit, thresholds, hasValue }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const cx = 100, cy = 100, r = 80;
  const needleAngle = ratio * 360;
  const needleEnd = polarToCartesian(cx, cy, r - 14, needleAngle);
  const color = thresholds.length > 0
    ? getThresholdColor(value, thresholds, '#EF4444')
    : '#EF4444';
  const ticks = Array.from({ length: 11 }, (_, i) => i);

  return (
    <svg viewBox="0 0 200 200" className="h-full w-full">
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
      {/* 니들 — 값이 있을 때만 */}
      {hasValue && (
        <>
          <line x1={cx} y1={cy} x2={needleEnd.x} y2={needleEnd.y}
            stroke="#1E293B" strokeWidth={2} strokeLinecap="round" />
          <circle cx={cx} cy={cy} r={5} fill="#1E293B" />
        </>
      )}
      {!hasValue && (
        <circle cx={cx} cy={cy} r={4} fill="#9CA3AF" />
      )}
      {/* 값 배지 */}
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
      {/* 니들 — 값이 있을 때만 표시 */}
      {hasValue && (
        <>
          <polygon
            points={`${needleTip.x},${needleTip.y} ${needleBase1.x},${needleBase1.y} ${needleBase2.x},${needleBase2.y}`}
            fill="#1A1A1A"
          />
          <circle cx={cx} cy={cy} r={6} fill="#1A1A1A" />
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
      {/* 니들 — 값이 있을 때만 */}
      {hasValue ? (
        <>
          <polygon
            points={`${needleTip.x},${needleTip.y} ${needleBase1.x},${needleBase1.y} ${needleBase2.x},${needleBase2.y}`}
            fill="#1E293B"
          />
          <circle cx={cx} cy={cy} r={6} fill="#1E293B" />
        </>
      ) : (
        <circle cx={cx} cy={cy} r={4} fill="#9CA3AF" />
      )}
      <circle cx={cx} cy={cy} r={3.5} fill="#FFFFFF" />
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

// ---- 메인 컴포넌트 ----

/** 게이지 차트 패널 */
export default function GaugePanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: GaugePanelProps) {
  // chart-emitter 바인딩이 있으면 실시간 구독
  const chartSource = pickChartEmitterSource(config);
  const { entries, status } = useChartChannel(chartSource?.channelName, { maxPoints: 1 });

  // store 바인딩이 있으면 폴링 구독
  const storeSource = pickStoreSource(config);
  const storeValue = useStoreLatestValue(storeSource);

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

  // 우선순위: chart-emitter > store > static
  const liveValue = chartLiveValue ?? storeValue;
  const hasBinding = !!chartSource || !!storeSource;

  const parsedBase = parseConfig(config);
  const hasValue = !hasBinding || liveValue !== undefined;
  const parsed = liveValue !== undefined
    ? { ...parsedBase, value: liveValue }
    : parsedBase;
  const { gaugeType } = parsed;

  const renderGauge = () => {
    switch (gaugeType) {
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
  };

  return (
    <div className={cn(
      'relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4',
      'ring-1 ring-(--color-border-default)',
    )}>
      {/* chart-emitter 구독 중일 때만 연결 상태 아이콘 표시 */}
      {chartSource && (
        <div className="absolute right-3 top-3 z-10">
          <ConnectionStatusIcon status={status} />
        </div>
      )}
      {/* 헤더 */}
      <div className="mb-1 flex shrink-0 items-center gap-2 pr-6">
        <GaugeIcon className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
        <span className="truncate text-sm font-semibold text-(--color-text-primary)">{title}</span>
      </div>
      {/* 게이지 SVG */}
      <div className="flex min-h-0 flex-1 items-center justify-center">
        {renderGauge()}
      </div>
    </div>
  );
}

