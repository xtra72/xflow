// 노드 타입 정규화 유틸.
//
// 12개 HVAC 노드 TYPE 식별자가 `_` → `-` 로 정규화되었다(canonical).
// 백엔드는 두 표기를 모두 해석하지만(엔진 alias), 프론트엔드의 스키마/메타
// 조회는 canonical `-` 키만 보유한다. 저장된 플로우가 옛 `_` 타입을 들고
// 있어도 스키마/포트/설명이 정상 해석되도록, 조회 직전에 이 함수로 정규화한다.
//
// 매핑 대상이 아닌 타입은 그대로 반환한다.

/** 옛 `_` HVAC 타입 → canonical `-` 타입 매핑 (12종). */
const HVAC_TYPE_ALIASES: Readonly<Record<string, string>> = {
  samsung_hvacr01: 'samsung-hvacr01',
  samsung_hvacr01_status: 'samsung-hvacr01-status',
  samsung_hvacr01_control: 'samsung-hvacr01-control',
  lg_hvacr01: 'lg-hvacr01',
  lg_hvacr01_status: 'lg-hvacr01-status',
  lg_hvacr01_control: 'lg-hvacr01-control',
  lg_hvacr02: 'lg-hvacr02',
  lg_hvacr02_status: 'lg-hvacr02-status',
  lg_hvacr02_control: 'lg-hvacr02-control',
  century_hvacr01: 'century-hvacr01',
  century_hvacr01_status: 'century-hvacr01-status',
  century_hvacr01_control: 'century-hvacr01-control',
};

/**
 * 노드 타입 식별자를 canonical 표기로 정규화한다.
 *
 * 12개 옛 `_` HVAC 타입은 대응하는 `-` 표기로 변환하고, 그 외 타입은
 * 변경 없이 그대로 반환한다. 빈 문자열/이미 canonical 인 값도 안전하다.
 */
export function normalizeNodeType(type: string): string {
  return HVAC_TYPE_ALIASES[type] ?? type;
}
