// sysmetrics 패널 배선 테스트 (SPEC-SYSMETRICS-PANEL-001 M5).
//
// 카탈로그·기본 config·설정 읽기가 서로 어긋나지 않는지 본다. 이 셋이 갈라지면
// 패널은 추가되지만 화면이 비고, 원인이 어디인지 알 수 없다.

import { describe, expect, it } from 'vitest';

import { useUIStore, type PanelType } from '@/stores/uiStore';

import { STORAGE_ITEMS, readAgentRef, readItems, readTargets } from './sysMetricsPanelConfig';
import {
  DEFAULT_SYSTEM_FIELDS,
  SYSTEM_FIELDS,
  normalizeSystemItems,
} from './sysMetricsFields';

/**
 * 패널을 추가하고 그 config 를 돌려준다.
 *
 * `createDefaultPanel` 은 모듈 비공개다. 테스트를 위해 내보내면 프로덕션 API 가
 * 테스트 때문에 넓어지므로, 기존 uiStore 테스트와 같이 스토어 액션으로 간접 검증한다.
 */
function addAndRead(type: PanelType): Record<string, unknown> {
  useUIStore.getState().addPanel(type);
  const s = useUIStore.getState();
  const page = s.dashboardPages.find((p) => p.id === s.activeDashboardId)!;
  return page.panels[page.panels.length - 1]!.config as Record<string, unknown>;
}

describe('기본 config', () => {
  it('세 패널 모두 에이전트 키를 비워 둔 채 만들어진다', () => {
    // 키가 아예 없으면 설정 화면이 어떤 필드를 편집해야 할지 알 수 없다.
    for (const type of ['sysmetrics-system', 'sysmetrics-network', 'sysmetrics-storage'] as const) {
      const config = addAndRead(type);
      expect(config).toHaveProperty('agent_id', '');
      expect(config).toHaveProperty('agent_name', '');
    }
  });

  it('대상 목록의 기본값은 빈 배열(=종합)이다', () => {
    // 기본을 "전체 개별"로 두면 마운트가 10개인 호스트에서 첫 화면부터 읽을 수 없다.
    expect(addAndRead('sysmetrics-network')).toHaveProperty('interfaces', []);
    expect(addAndRead('sysmetrics-storage')).toHaveProperty('mountpoints', []);
  });

  it('기본 표시 항목은 패널 컴포넌트의 기본값과 같다', () => {
    // 설정 화면과 실제 렌더가 다른 기본값을 쓰면 "껐는데 살아난다"가 된다.
    expect(addAndRead('sysmetrics-storage').items).toEqual(STORAGE_ITEMS);
  });

  it('시스템 패널의 기본 항목은 값 키다', () => {
    // 그룹 키를 쓰면 설정·미리보기가 값 카탈로그와 대조에 실패해 빈 목록이 된다 —
    // 미리보기가 빈 채로 나오던 원인이었다.
    const items = addAndRead('sysmetrics-system').items as string[];
    expect(items).toEqual(DEFAULT_SYSTEM_FIELDS);
    for (const key of items) {
      expect(key).toContain('.');
    }
  });
});

describe('설정 읽기', () => {
  it('agent_id 가 정본이고 agent_name 은 표시용이다', () => {
    expect(readAgentRef({ agent_id: 'a1', agent_name: 'sys' })).toEqual({
      agentId: 'a1',
      agentName: 'sys',
    });
    expect(readAgentRef(undefined)).toEqual({ agentId: '', agentName: '' });
  });

  it('대상 목록에서 문자열이 아닌 값은 버린다', () => {
    expect(readTargets({ interfaces: ['en0', 42, '', null] }, 'interfaces')).toEqual(['en0']);
  });

  it('대상 키가 없으면 빈 배열(=종합)이다', () => {
    expect(readTargets({}, 'mountpoints')).toEqual([]);
  });

  it('items 가 없으면 기본 항목을 쓰고, 빈 배열은 존중한다', () => {
    expect(readItems(undefined, STORAGE_ITEMS, STORAGE_ITEMS)).toEqual(STORAGE_ITEMS);
    expect(readItems({ items: [] }, STORAGE_ITEMS, STORAGE_ITEMS)).toEqual([]);
  });

  it('모르는 항목 키는 버린다', () => {
    // 패널 유형을 바꾸며 다른 어휘의 항목이 남아 있을 수 있다.
    expect(readItems({ items: ['usage', 'bogus'] }, STORAGE_ITEMS, STORAGE_ITEMS)).toEqual(['usage']);
  });
});

describe('미리보기가 그릴 칸 (회귀 가드)', () => {
  // 미리보기는 저장된 items 를 값 카탈로그와 대조해 칸을 만든다. 옛 그룹 키를
  // 옮기지 않으면 교집합이 비어 미리보기가 빈 채로 나온다 — 실제로 그랬다.
  it('기본 config 의 items 가 값 카탈로그와 대조된다', () => {
    const items = addAndRead('sysmetrics-system').items as string[];
    const matched = SYSTEM_FIELDS.filter((f) => items.includes(f.key));

    expect(matched.length).toBe(items.length);
    expect(matched.length).toBeGreaterThan(0);
  });

  it('옛 그룹 키도 값 키로 옮겨져 대조된다', () => {
    const normalized = normalizeSystemItems(['cpu', 'memory', 'diskIo', 'network'])!;
    const matched = SYSTEM_FIELDS.filter((f) => normalized.includes(f.key));

    expect(matched.length).toBe(normalized.length);
    expect(normalized).toContain('cpu.usage_percent');
  });

  it('값 키를 담은 items 는 그대로 통과한다', () => {
    expect(normalizeSystemItems(['memory.used_bytes'])).toEqual(['memory.used_bytes']);
  });

  it('모르는 키는 버린다', () => {
    expect(normalizeSystemItems(['bogus', 'cpu.usage_percent'])).toEqual(['cpu.usage_percent']);
  });

  it('items 가 배열이 아니면 undefined (기본값을 쓰라는 뜻)', () => {
    expect(normalizeSystemItems(undefined)).toBeUndefined();
    expect(normalizeSystemItems('cpu')).toBeUndefined();
  });
});
