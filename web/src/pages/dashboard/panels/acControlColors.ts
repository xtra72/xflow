// 에어컨 제어 패널 컬러 스킴 (값 색상 / 제어 버튼 / 풍량)
// PanelSettingsDialog 의 스타일 섹션과 AcControlPanel 양쪽에서 공유한다.
//
// 스키마 설계 방침:
// - 모든 필드는 선택값(undefined 가능) — 미설정 시 기존 토큰/기본값 사용
// - 값 범위(ranges)는 0개 이상; min/max 둘 다 미지정이면 모든 값에 적용
// - 제어 버튼 selectedMode 가 'unified' 면 selectedColor 단일 적용,
//   'individual' 이면 perButton 매핑이 우선
// - 풍량은 perLevel 매핑만 사용; 비활성 단계는 unselected 적용
//
// 모든 입력은 사용자 정의 가능하므로 절대 throw 하지 않는다 (방어적 처리).

import type { AcMode, FanSpeed } from './acControlTypes';

/** 값 색상 — 현재 값(온도) 표시에 적용 */
export interface ValueColorRange {
  /** 임계값 이름 (예: "정상", "주의", "위험"). 라벨 용도. */
  name?: string;
  /** 하한 (inclusive). undefined 이면 -∞ */
  min?: number;
  /** 상한 (exclusive). undefined 이면 +∞ */
  max?: number;
  /** 적용 색상 (CSS color 문자열) */
  color: string;
}

export interface ValueColorConfig {
  /** 기본 컬러 — 모든 구간에 해당하지 않는 값에 적용 */
  default?: string;
  /** 구간 컬러 (0개 이상). 위에서부터 처음 매치되는 항목 사용 */
  ranges?: ValueColorRange[];
  /** 컬러 지정 모드 — 개별/연속. UI 토글용. */
  mode?: 'individual' | 'continuous';
}

/** 제어 버튼 컬러 — 운전 모드 버튼 그룹에 적용 */
export interface ControlButtonColorConfig {
  /** 미선택 상태 색상 (배경/링) */
  unselected?: string;
  /** 선택 상태 컬러링 모드 */
  selectedMode?: 'unified' | 'individual';
  /** unified 모드일 때 사용되는 단일 선택 색상 */
  selectedColor?: string;
  /** individual 모드일 때 모드별 색상 매핑 */
  perButton?: Partial<Record<AcMode, string>>;
}

/** 풍량 컬러 — 풍량 단계 버튼 그룹에 적용 */
export interface FanLevelColorConfig {
  /** 미선택 상태 색상 */
  unselected?: string;
  /** 단계별 색상 매핑 */
  perLevel?: Partial<Record<FanSpeed, string>>;
}

// ---- 헬퍼 ----

/**
 * 값 색상을 해석한다.
 * 우선순위: 매치되는 첫 ranges → default → undefined (호출자가 폴백 토큰 적용)
 */
export function resolveValueColor(
  value: number | undefined,
  config: ValueColorConfig | undefined,
): string | undefined {
  if (!config) return undefined;
  if (typeof value === 'number' && Array.isArray(config.ranges)) {
    for (const r of config.ranges) {
      if (!r || typeof r.color !== 'string') continue;
      const minOk = r.min === undefined || value >= r.min;
      const maxOk = r.max === undefined || value < r.max;
      if (minOk && maxOk) return r.color;
    }
  }
  return config.default;
}

/**
 * 제어 버튼 색상을 해석한다.
 * @param mode  현재 활성 모드
 * @param target 대상 버튼의 모드 키
 * @param config 컬러 설정
 * @returns { active: boolean, color?: string } — color 미지정 시 호출자가 기본 토큰 적용
 */
export function resolveControlButtonColor(
  mode: AcMode,
  target: AcMode,
  config: ControlButtonColorConfig | undefined,
): { active: boolean; color: string | undefined } {
  const active = mode === target;
  if (!config) return { active, color: undefined };
  if (!active) return { active, color: config.unselected };
  if (config.selectedMode === 'individual') {
    return { active, color: config.perButton?.[target] ?? config.selectedColor };
  }
  return { active, color: config.selectedColor };
}

/**
 * 풍량 단계 색상을 해석한다.
 */
export function resolveFanLevelColor(
  current: FanSpeed,
  target: FanSpeed,
  config: FanLevelColorConfig | undefined,
): { active: boolean; color: string | undefined } {
  const active = current === target;
  if (!config) return { active, color: undefined };
  if (!active) return { active, color: config.unselected };
  return { active, color: config.perLevel?.[target] };
}

/** config (Record<string,unknown>) 에서 안전하게 ValueColorConfig 추출 */
export function readValueColorConfig(
  config: Record<string, unknown> | undefined,
): ValueColorConfig | undefined {
  const v = config?.['valueColor'];
  if (!v || typeof v !== 'object') return undefined;
  return v as ValueColorConfig;
}

/** config (Record<string,unknown>) 에서 안전하게 ControlButtonColorConfig 추출 */
export function readControlButtonColorConfig(
  config: Record<string, unknown> | undefined,
): ControlButtonColorConfig | undefined {
  const v = config?.['controlButtonColor'];
  if (!v || typeof v !== 'object') return undefined;
  return v as ControlButtonColorConfig;
}

/** config (Record<string,unknown>) 에서 안전하게 FanLevelColorConfig 추출 */
export function readFanLevelColorConfig(
  config: Record<string, unknown> | undefined,
): FanLevelColorConfig | undefined {
  const v = config?.['fanLevelColor'];
  if (!v || typeof v !== 'object') return undefined;
  return v as FanLevelColorConfig;
}
