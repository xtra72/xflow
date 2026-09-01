// 값 단위 목록 — 게이지·통계·라인·바·파이·테이블이 공유하는 단일 정본.
//
// 종전에는 이 목록이 PanelSettingsDialog 안에 게이지 전용으로 박혀 있었다. 그래서
// 게이지에서만 "°C" 를 목록에서 고를 수 있었고, 통계·라인은 자유 텍스트라 같은 온도
// 시리즈가 패널마다 `°C` · `C` · `degC` 로 갈렸다. 목록을 여기 하나로 모은다.
//
// 값(`value`)이 곧 화면에 붙는 문자열이다 — 라벨은 사람이 고르기 위한 것이고,
// 저장되는 것은 언제나 `value` 다.

import { formatDecimal } from './decimalPlaces';

/** 단위 한 개. `labelKey` 가 없으면 값 자체를 라벨로 쓴다(psi · kPa 처럼 이미 통용 표기). */
export interface UnitOption {
  value: string;
  labelKey?: string;
}

/** 단위 그룹(select 의 optgroup). */
export interface UnitOptionGroup {
  labelKey: string;
  units: UnitOption[];
}

/**
 * **자동 데이터 량** 단위의 저장값.
 *
 * 다른 단위는 값이 곧 화면에 붙는 접미사인데, 이것 하나는 접미사가 아니라 **규칙**이다 —
 * 값의 크기에 따라 B · KB · MB … 로 접고 접미사를 그때 정한다. 그래서 실제 단위로는
 * 쓰일 수 없는 형태(콜론 포함)를 골라, 사용자가 직접 적은 단위와 겹치지 않게 한다.
 */
export const AUTO_BYTES_UNIT = 'auto:bytes';

/**
 * **자동 데이터 전송률** 단위의 저장값.
 *
 * 접는 규칙은 데이터 량과 같고 접미사에 `/s` 가 붙는다. 별도 단위로 두는 이유:
 * 누적 카운터를 초당 증가량으로 환산한 값(`sysMetricsSeries.deltaRate`)은 **개수가
 * 아니라 속도**인데, `B` 로 표기하면 개수로 읽힌다. 실제로 초당 11252.8 바이트가
 * `11252.80B` 로 나와 "왜 바이트에 소수점이 붙나" 로 읽히던 자리다.
 */
export const AUTO_BYTES_RATE_UNIT = 'auto:bytes/s';

/** 자동 데이터 량 단위인가. */
export function isAutoBytesUnit(unit: string | undefined): boolean {
  return unit === AUTO_BYTES_UNIT;
}

/** 자동 데이터 전송률 단위인가. */
export function isAutoBytesRateUnit(unit: string | undefined): boolean {
  return unit === AUTO_BYTES_RATE_UNIT;
}

/**
 * 자동 환산 단위가 접미사 뒤에 덧붙이는 꼬리. 자동 단위가 아니면 `null`.
 *
 * 두 자동 단위의 차이는 이 꼬리 하나뿐이라, 접는 계산을 두 벌로 두지 않는다.
 */
/**
 * 값 자체를 접는 단위인가(데이터 량 · 데이터 전송률).
 *
 * 축 눈금처럼 "자릿수를 명시했을 때만 포맷터를 건다" 는 자리에서, 자동 단위는 예외로
 * 늘 포맷터를 태워야 한다 — 접지 않으면 원값(바이트)이 그대로 눈금에 남는다.
 */
export function isAutoScaledUnit(unit: string | undefined): boolean {
  return autoScaleTail(unit) !== null;
}

function autoScaleTail(unit: string | undefined): string | null {
  if (isAutoBytesUnit(unit)) return '';
  if (isAutoBytesRateUnit(unit)) return '/s';
  return null;
}

/**
 * 데이터 량 접기 단위. 밑은 **1024** 다.
 *
 * KiB 가 아니라 KB 표기를 쓰는 이유는 이 저장소의 기존 표기와 맞추기 위해서다
 * (`lib/utils/format.formatBytes` · `pages/monitoring/networkSeries.formatBytes`).
 * 같은 화면 안에서 한쪽은 `1 KB` 다른 쪽은 `1 KiB` 로 읽히면 값이 다른 것처럼 보인다.
 */
const BYTE_SCALE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'] as const;

/** 접기 밑. */
const BYTE_SCALE_BASE = 1024;

export const UNIT_OPTIONS: UnitOptionGroup[] = [
  {
    labelKey: 'dashboard.settings.unitGroups.ratio',
    units: [
      { value: '%', labelKey: 'dashboard.settings.units.percent' },
      { value: '‰', labelKey: 'dashboard.settings.units.permille' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.temperature',
    units: [
      { value: '°C', labelKey: 'dashboard.settings.units.celsius' },
      { value: '°F', labelKey: 'dashboard.settings.units.fahrenheit' },
      { value: 'K', labelKey: 'dashboard.settings.units.kelvin' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.electric',
    units: [
      { value: 'V', labelKey: 'dashboard.settings.units.volt' },
      { value: 'A', labelKey: 'dashboard.settings.units.ampere' },
      { value: 'W', labelKey: 'dashboard.settings.units.watt' },
      { value: 'kW', labelKey: 'dashboard.settings.units.kilowatt' },
      { value: 'kWh', labelKey: 'dashboard.settings.units.kilowattHour' },
      { value: 'Ω', labelKey: 'dashboard.settings.units.ohm' },
      { value: 'Hz', labelKey: 'dashboard.settings.units.hertz' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.pressureFlow',
    units: [
      { value: 'Pa', labelKey: 'dashboard.settings.units.pascal' },
      { value: 'kPa' },
      { value: 'bar', labelKey: 'dashboard.settings.units.bar' },
      { value: 'psi' },
      { value: 'L/min', labelKey: 'dashboard.settings.units.litersPerMin' },
      { value: 'm³/h' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.speedRotation',
    units: [
      { value: 'm/s', labelKey: 'dashboard.settings.units.meterPerSec' },
      { value: 'km/h' },
      { value: 'rpm', labelKey: 'dashboard.settings.units.rpm' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.weightVolume',
    units: [
      { value: 'kg', labelKey: 'dashboard.settings.units.kilogram' },
      { value: 'L', labelKey: 'dashboard.settings.units.liter' },
      { value: 'mL', labelKey: 'dashboard.settings.units.milliliter' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.length',
    units: [
      { value: 'mm', labelKey: 'dashboard.settings.units.millimeter' },
      { value: 'cm', labelKey: 'dashboard.settings.units.centimeter' },
      { value: 'm', labelKey: 'dashboard.settings.units.meter' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.dataSize',
    units: [
      // 자동을 맨 위에 둔다 — 데이터 량은 자릿수가 크게 흔들리는 값이라 고정 단위를
      // 고르면 대개 `0.00MB` 아니면 `13421772.80KB` 중 하나가 된다.
      { value: AUTO_BYTES_UNIT, labelKey: 'dashboard.settings.units.bytesAuto' },
      { value: 'B', labelKey: 'dashboard.settings.units.byte' },
      { value: 'KB', labelKey: 'dashboard.settings.units.kilobyte' },
      { value: 'MB', labelKey: 'dashboard.settings.units.megabyte' },
      { value: 'GB', labelKey: 'dashboard.settings.units.gigabyte' },
      { value: 'TB', labelKey: 'dashboard.settings.units.terabyte' },
      { value: 'PB', labelKey: 'dashboard.settings.units.petabyte' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.dataRate',
    units: [
      { value: AUTO_BYTES_RATE_UNIT, labelKey: 'dashboard.settings.units.bytesRateAuto' },
      { value: 'B/s', labelKey: 'dashboard.settings.units.bytePerSec' },
      { value: 'KB/s', labelKey: 'dashboard.settings.units.kilobytePerSec' },
      { value: 'MB/s', labelKey: 'dashboard.settings.units.megabytePerSec' },
      { value: 'GB/s', labelKey: 'dashboard.settings.units.gigabytePerSec' },
      { value: 'TB/s', labelKey: 'dashboard.settings.units.terabytePerSec' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.etc',
    units: [
      { value: 'dB', labelKey: 'dashboard.settings.units.decibel' },
      { value: 'lux', labelKey: 'dashboard.settings.units.lux' },
      { value: 'ppm' },
      { value: '', labelKey: 'dashboard.settings.units.none' },
    ],
  },
];

/**
 * select 의 "직접 입력" 항목이 갖는 값.
 *
 * 실제 단위 문자열과 겹치지 않아야 하므로 단위로 쓰일 수 없는 형태를 쓴다.
 */
export const CUSTOM_UNIT_SENTINEL = '__custom__';

/** 목록에 있는 단위인가. 없으면 사용자가 직접 적은 값이다. */
export function isPresetUnit(value: string): boolean {
  return UNIT_OPTIONS.some((group) => group.units.some((u) => u.value === value));
}

/**
 * 값과 단위를 잇는다.
 *
 * 붙임표를 두지 않는 이유: `21.5°C` · `80%` 처럼 계측 표기는 붙여 쓰는 것이 관행이고,
 * 띄우면 좁은 타일에서 줄바꿈이 값과 단위를 갈라놓는다. 단위가 비면 값만 돌려준다.
 *
 * **자동 데이터 량은 여기서 처리하지 않는다.** 그 단위는 접미사가 아니라 값까지 바꾸는
 * 규칙이라 이미 문자열이 된 값에는 걸 수 없다 — 저장값이 그대로 붙는 사고를 막기 위해
 * 빈 단위로 취급한다. 수를 들고 있는 자리에서는 {@link scaleValueUnit} 를 쓴다.
 */
export function withUnit(text: string, unit: string | undefined): string {
  if (!unit || autoScaleTail(unit) !== null) return text;
  return `${text}${unit}`;
}

/**
 * 값을 표기 문자열과 접미사로 나눈다.
 *
 * 값과 단위를 **따로** 그리는 자리(통계 타일의 큰 숫자 + 작은 단위, 게이지 가운데
 * 숫자)가 있어 둘을 갈라 돌려준다. 이어 붙인 한 문자열이 필요하면
 * {@link formatValueWithUnit} 를 쓴다.
 *
 * 자동 데이터 량이면 값을 1024 씩 접고 그 자리의 단위를 접미사로 준다. 음수도 접는다 —
 * 증감(delta)이 음수로 오는 자리가 있고, 부호를 잃으면 늘어난 것과 줄어든 것이 같아진다.
 *
 * **자릿수는 모든 자리에서 사용자 설정을 따른다.** B 자리만 정수로 반올림하는 규칙도
 * 생각할 수 있지만(바이트는 쪼갤 수 없으므로), 한 자리만 설정을 무시하면 "자릿수를 3
 * 으로 했는데 왜 안 되지" 가 된다. 규칙은 하나이고, 조절 손잡이는 사용자에게 있다.
 */
export function scaleValueUnit(
  value: number,
  decimals: number,
  unit: string | undefined,
): { text: string; suffix: string } {
  const parts = scaleValueParts(value, unit);
  return {
    text: Number.isFinite(parts.value) ? formatDecimal(parts.value, decimals) : String(parts.value),
    suffix: parts.suffix,
  };
}

/**
 * 자동 환산을 **수치 상태로** 마친 결과.
 *
 * 포맷과 분리하는 이유: 눈금은 접은 **뒤의** 크기로 자릿수를 정해야 한다. 1536 바이트는
 * 1.5KB 이므로 "10 이하 → 한 자리" 규칙이 1536 이 아니라 1.5 에 걸려야 한다. 자릿수를
 * 먼저 받는 {@link scaleValueUnit} 로는 그 순서를 만들 수 없다.
 */
export function scaleValueParts(
  value: number,
  unit: string | undefined,
): { value: number; suffix: string } {
  const tail = autoScaleTail(unit);
  if (tail === null) return { value, suffix: unit ?? '' };
  if (!Number.isFinite(value)) return { value, suffix: '' };

  const sign = value < 0 ? -1 : 1;
  let v = Math.abs(value);
  let i = 0;
  while (v >= BYTE_SCALE_BASE && i < BYTE_SCALE_UNITS.length - 1) {
    v /= BYTE_SCALE_BASE;
    i += 1;
  }
  return { value: sign * v, suffix: `${BYTE_SCALE_UNITS[i]!}${tail}` };
}

/**
 * **눈금 값**의 자릿수 — 크기가 정한다.
 *
 * 눈금은 값 읽기가 아니라 눈금자다. 큰 수에 소수를 달면 축이 `12345.00 · 24690.00` 처럼
 * 길어져 서로 겹치고, 작은 수를 정수로 끊으면 `0 · 0 · 0` 이 되어 눈금 구실을 못 한다.
 * 그래서 자릿수를 **한 설정값이 아니라 그 눈금의 크기**로 정한다.
 *
 *   |v| ≤ 1  → 2자리 · |v| ≤ 10 → 1자리 · 그 밖 → 정수
 */
export function tickDecimals(value: number): number {
  const a = Math.abs(value);
  if (!Number.isFinite(a)) return 0;
  if (a <= 1) return 2;
  if (a <= 10) return 1;
  return 0;
}

/**
 * 눈금 하나의 표기 — 수만 남기고 자릿수는 크기가 정한다.
 *
 * **접미사는 붙이지 않는다.** 눈금은 축을 따라 여러 번 반복되므로 단위를 매번 달면
 * 좁은 자리에서 글자가 겹치고, 단위는 값 표기에서 한 번만 말하면 충분하다.
 *
 * 다만 자동 환산 단위는 **값을 접는다** — 접지 않으면 눈금이 원시 바이트(`100000`)로
 * 남아 값 표기(`10.99KB/s`)와 자릿수가 어긋난다. 접은 뒤의 크기로 자릿수를 정하므로
 * 1536 은 `1.5`(1536 이 아니라 1.5 에 규칙이 걸린다)다.
 *
 * `decimals` 를 주면 그 값이 이긴다 — 사용자가 자릿수를 직접 지정했으면 축과 값이 같은
 * 자릿수여야 한다는 기존 규약(`decimalPlaces.ts` 머리말)을 지킨다.
 */
export function formatTickValue(
  value: number,
  unit: string | undefined,
  decimals?: number,
): string {
  const parts = scaleValueParts(value, unit);
  if (!Number.isFinite(parts.value)) return `${parts.value}`;
  const dec = decimals ?? tickDecimals(parts.value);
  return formatDecimal(parts.value, dec);
}

/**
 * **축 라벨**에 적을 단위 표기.
 *
 * 자동 환산 단위는 저장값(`auto:bytes`)이 곧 표기가 아니라 규칙이다. 그대로 축 라벨에
 * 붙이면 눈금은 KB 로 접혀 있는데 라벨은 `auto:bytes` 라고 말해, 축이 무엇을 세는지
 * 알 수 없다(`0 · 34 · 68 · 103 · 137 (auto:bytes)` 로 보이던 자리).
 *
 * 그래서 축이 실제로 접은 배율의 접미사를 돌려준다 — 배율은 눈금마다 다시 정하지 않고
 * **축 전체가 하나**를 쓰므로(가장 큰 눈금이 정한다) 라벨 한 줄로 말할 수 있다.
 *
 * 기준값을 모르면(데이터가 아직 없는 축) **접기 이전의 밑단위**로 떨어진다 — 라벨을 아예
 * 지우지 않는 이유는, 값이 들어오기 전까지 축이 무엇을 세는지 말하지 않게 되기 때문이다.
 * 실시간 모드는 시작 직후가 늘 이 상태라(첫 표본은 증가량을 구할 기준점이 없다) 라벨이
 * 사라지면 "단위 설정이 안 먹는다" 로 보인다. 빈 축은 어차피 0 이므로 0 의 배율이 맞다.
 *
 * @param reference 배율을 정할 기준값. 축의 최대 눈금을 넘긴다.
 */
export function axisUnitLabel(
  unit: string | undefined,
  reference: number | undefined,
): string | undefined {
  if (!isAutoScaledUnit(unit)) return unit === '' ? undefined : unit;
  const at = reference !== undefined && Number.isFinite(reference) ? reference : 0;
  return scaleValueParts(at, unit).suffix;
}

/** {@link scaleValueUnit} 의 두 조각을 이어 붙인 문자열. 툴팁·축 눈금·표 셀에서 쓴다. */
export function formatValueWithUnit(
  value: number,
  decimals: number,
  unit: string | undefined,
): string {
  const { text, suffix } = scaleValueUnit(value, decimals, unit);
  return `${text}${suffix}`;
}
