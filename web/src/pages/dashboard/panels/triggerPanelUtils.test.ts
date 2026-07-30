// SPEC-TRIGGER-PANEL-001 — 트리거 패널 순수 유틸리티 단위 테스트.
// §D(카탈로그 inline 주입/스냅샷 비소급) + §E(FULL config, patch-then-PUT, 충돌 감지).

import { describe, it, expect } from 'vitest';

import {
  buildFullTriggerConfig,
  detectConflict,
  findNodeConfigInDefinition,
  injectCatalogSnapshot,
  patchNodeConfigInDefinition,
} from './triggerPanelUtils';

describe('injectCatalogSnapshot (§D-2 inline 주입 / §D-3 스냅샷 비소급)', () => {
  it('D-2: 배정 시 카탈로그 payload 객체가 스케줄의 payload 로 inline 복사된다', () => {
    const schedules = [{ type: 'interval', value: '5s' }];
    const catalog = { warn: { level: 3 } };

    const next = injectCatalogSnapshot(schedules, 0, catalog, 'warn');

    expect(next[0]).toEqual({ type: 'interval', value: '5s', payload: { level: 3 } });
    // 이름/카탈로그 구조는 스케줄에 남지 않는다(노드는 resolved inline payload 만 관측).
    expect('payloadRef' in next[0]!).toBe(false);
    expect('warn' in next[0]!).toBe(false);
  });

  it('D-3: 카탈로그를 이후 수정해도 이미 주입된 스케줄 payload 는 소급 변경되지 않는다', () => {
    const catalog: Record<string, Record<string, unknown>> = { warn: { level: 3 } };
    const schedules = [{ type: 'interval', value: '5s' }];

    const injected = injectCatalogSnapshot(schedules, 0, catalog, 'warn');
    // 카탈로그의 warn 을 {level:5} 로 수정(재선택 없음).
    catalog.warn = { level: 5 };

    // 기존 주입 스냅샷은 {level:3} 그대로.
    expect(injected[0]!.payload).toEqual({ level: 3 });

    // 재선택 시에만 갱신된 값을 채택.
    const reInjected = injectCatalogSnapshot(injected, 0, catalog, 'warn');
    expect(reInjected[0]!.payload).toEqual({ level: 5 });
  });

  it('깊은 복사이므로 카탈로그 객체의 중첩 값 변경도 주입 스케줄에 전파되지 않는다', () => {
    const nested = { a: { b: 1 } };
    const catalog = { c: nested };
    const injected = injectCatalogSnapshot([{ type: 'cron', value: '* * * * *' }], 0, catalog, 'c');
    nested.a.b = 999;
    expect(injected[0]!.payload).toEqual({ a: { b: 1 } });
  });

  it('존재하지 않는 이름은 원본 스케줄을 그대로 반환한다', () => {
    const schedules = [{ type: 'interval', value: '5s' }];
    expect(injectCatalogSnapshot(schedules, 0, {}, 'missing')).toBe(schedules);
  });
});

describe('buildFullTriggerConfig (§E-2 FULL config)', () => {
  it('baseConfig 의 payload/기타 키를 보존하고 schedules 를 교체한다(델타 아님)', () => {
    const base = { schedules: [{ type: 'interval', value: '1s' }], payload: { a: 1 }, source_ch_size: 64 };
    const nextSchedules = [{ type: 'interval', value: '5s', payload: { level: 3 } }];

    const full = buildFullTriggerConfig(base, nextSchedules);

    expect(full).toEqual({
      schedules: nextSchedules,
      payload: { a: 1 },
      source_ch_size: 64,
    });
  });
});

describe('patchNodeConfigInDefinition (§E-1 patch-then-PUT)', () => {
  const definition = {
    nodes: [
      { id: 'n1', type: 'custom', data: { nodeType: 'trigger', label: 'T', schedules: [{ type: 'interval', value: '1s' }] } },
      { id: 'n2', type: 'custom', data: { nodeType: 'output', label: 'O' } },
    ],
    edges: [{ id: 'e1', source: 'n1', target: 'n2' }],
    inputs: [],
    outputs: [],
  };

  it('대상 노드의 config(data) 만 패치하고 nodeType/label 은 보존한다', () => {
    const full = { schedules: [{ type: 'interval', value: '5s' }], payload: { x: 1 } };
    const patched = patchNodeConfigInDefinition(definition, 'n1', full);

    const n1 = (patched.nodes as Record<string, unknown>[])[0]!;
    expect(n1.data).toEqual({
      nodeType: 'trigger',
      label: 'T',
      schedules: [{ type: 'interval', value: '5s' }],
      payload: { x: 1 },
    });
  });

  it('다른 노드/와이어/포트는 보존한다', () => {
    const patched = patchNodeConfigInDefinition(definition, 'n1', { schedules: [] });
    expect((patched.nodes as Record<string, unknown>[])[1]).toEqual(definition.nodes[1]);
    expect(patched.edges).toEqual(definition.edges);
    expect(patched.inputs).toEqual([]);
    expect(patched.outputs).toEqual([]);
  });

  it('nodes 가 없는 정의도 안전하게 처리한다', () => {
    expect(patchNodeConfigInDefinition({}, 'n1', { schedules: [] })).toEqual({ nodes: [] });
  });
});

describe('findNodeConfigInDefinition', () => {
  it('대상 노드의 data(config) 를 반환한다', () => {
    const def = { nodes: [{ id: 'n1', data: { schedules: [1], payload: { a: 1 } } }] };
    expect(findNodeConfigInDefinition(def, 'n1')).toEqual({ schedules: [1], payload: { a: 1 } });
  });
  it('없으면 undefined', () => {
    expect(findNodeConfigInDefinition({ nodes: [] }, 'x')).toBeUndefined();
  });
});

describe('detectConflict (§E-4 동시 편집 last-write-wins 통지)', () => {
  const baseline = { schedules: [{ type: 'interval', value: '1s' }], payload: { a: 1 } };

  it('fresh 노드 config 가 baseline 과 다르면 충돌(true)', () => {
    const fresh = { nodeType: 'trigger', schedules: [{ type: 'interval', value: '9s' }], payload: { a: 1 } };
    expect(detectConflict(fresh, baseline)).toBe(true);
  });

  it('schedules/payload 가 동일하면 충돌 아님(false)', () => {
    const fresh = { nodeType: 'trigger', label: 'x', schedules: [{ type: 'interval', value: '1s' }], payload: { a: 1 } };
    expect(detectConflict(fresh, baseline)).toBe(false);
  });

  it('fresh 가 없으면(노드 삭제 등) 충돌로 보지 않는다', () => {
    expect(detectConflict(undefined, baseline)).toBe(false);
  });
});
