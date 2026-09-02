// 모니터링 항목 어휘 검증 테스트.
//
// 패널 설정(`config.items`)이 담는 키를 걸러 내는 계층이라, 여기가 뚫리면
// 알 수 없는 키가 패널 렌더까지 흘러간다.

import { describe, it, expect } from 'vitest';

import {
  DEFAULT_LAYOUT,
  MONITOR_SECTIONS_WITH_ITEMS,
  VALID_ITEM_KEYS,
  sanitizeSectionItems,
} from './monitoringLayout';

describe('sanitizeSectionItems', () => {
  it('배열이 아니면 null (호출부가 기본값으로 떨어진다)', () => {
    expect(sanitizeSectionItems('metrics', undefined)).toBeNull();
    expect(sanitizeSectionItems('metrics', 'cpu')).toBeNull();
    expect(sanitizeSectionItems('metrics', null)).toBeNull();
  });

  it('알 수 없는 항목 키는 버린다', () => {
    expect(sanitizeSectionItems('metrics', ['cpu', 'bogus', 'memory'])).toEqual(['cpu', 'memory']);
    expect(sanitizeSectionItems('network', ['rxBytes', 'nope'])).toEqual(['rxBytes']);
  });

  it('중복 항목은 하나만 남긴다', () => {
    expect(sanitizeSectionItems('stats', ['uptime', 'uptime', 'goRoutines'])).toEqual([
      'uptime',
      'goRoutines',
    ]);
  });

  it('입력 순서를 유지한다 (패널 배치가 흔들리지 않도록)', () => {
    expect(sanitizeSectionItems('metrics', ['errorRate', 'cpu'])).toEqual(['errorRate', 'cpu']);
  });

  it('문자열이 아닌 항목은 무시한다', () => {
    expect(sanitizeSectionItems('stats', [1, null, 'uptime', {}])).toEqual(['uptime']);
  });

  it('빈 배열은 "모두 껐다"는 정상 상태로 존중한다', () => {
    // null 로 떨어뜨리면 항목 삭제가 새로고침마다 취소된다.
    expect(sanitizeSectionItems('metrics', [])).toEqual([]);
  });

  it('섹션마다 어휘가 분리되어 있다', () => {
    // 메트릭 키를 네트워크 섹션에 넣어도 통과하면 안 된다.
    expect(sanitizeSectionItems('network', ['cpu'])).toEqual([]);
    expect(sanitizeSectionItems('metrics', ['rxBytes'])).toEqual([]);
  });
});

describe('DEFAULT_LAYOUT', () => {
  it('모든 섹션의 기본 항목이 유효한 키다', () => {
    for (const section of MONITOR_SECTIONS_WITH_ITEMS) {
      const valid = new Set<string>(VALID_ITEM_KEYS[section]);
      for (const key of DEFAULT_LAYOUT[section]) {
        expect(valid.has(key), `${section}.${key} 는 유효한 키여야 한다`).toBe(true);
      }
    }
  });

  it('모든 섹션에 기본 항목이 하나 이상 있다', () => {
    // 기본이 비어 있으면 패널을 추가한 직후 빈 화면이 나온다.
    for (const section of MONITOR_SECTIONS_WITH_ITEMS) {
      expect(DEFAULT_LAYOUT[section].length, `${section} 기본 항목`).toBeGreaterThan(0);
    }
  });
});
