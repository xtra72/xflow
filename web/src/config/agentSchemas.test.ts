// xsfm 에이전트 설정 스키마 — 상태 방출 옵션 3종 테스트 (SPEC-XSFM-AGENT-IO-001 M5, REQ-05).
//
// 검증 대상:
//   - forward_received_to_node(boolean, 기본 false)
//   - state_emit_mode(enum event/interval/both, 기본 event)
//   - state_emit_interval(duration string, 기본 60s, mode=interval|both 일 때만 표시)
// 세 필드는 백엔드 Transport.Options 키와 1:1 매핑되어야 하며, 기본값은 현행 동작과
// 바이트 동일(forward off, mode=event)이어야 한다(무회귀, REQ-04-01/REQ-05-02).

import { describe, expect, it } from 'vitest';

import { getAgentConfigDefaults, getAgentConfigSchema } from './agentSchemas';
import type { ConfigField } from '@/types/node';

/** xsfm 스키마에서 name 으로 필드 하나를 찾는다. */
function xsfmField(name: string): ConfigField | undefined {
  return getAgentConfigSchema('xsfm')?.fields.find((f) => f.name === name);
}

/**
 * AgentDetailPanel.TwoColumnConfigLayout 의 filterVisible 술어와 동형.
 * visibleWhen 조건을 현재 config 값에 대해 평가한다(패널 렌더 로직을 그대로 미러).
 */
function isVisible(field: ConfigField, data: Record<string, unknown>): boolean {
  if (!field.visibleWhen) return true;
  const actual = data[field.visibleWhen.field];
  const expected = field.visibleWhen.value;
  if (Array.isArray(expected)) return (expected as unknown[]).includes(actual);
  return actual === expected;
}

describe('xsfm 상태 방출 옵션 스키마 (SPEC-XSFM-AGENT-IO-001 M5)', () => {
  it('세 컨트롤이 xsfm 스키마에 노출된다(백엔드 Transport.Options 키와 정확히 일치)', () => {
    const forward = xsfmField('forward_received_to_node');
    const mode = xsfmField('state_emit_mode');
    const interval = xsfmField('state_emit_interval');

    expect(forward).toBeDefined();
    expect(mode).toBeDefined();
    expect(interval).toBeDefined();

    // 위젯 타입: boolean 토글 / enum select / duration string
    expect(forward!.type).toBe('boolean');
    expect(mode!.type).toBe('select');
    expect(interval!.type).toBe('string');
  });

  it('state_emit_mode enum 은 event/interval/both 이며 기본값은 event 이다', () => {
    const mode = xsfmField('state_emit_mode');
    expect(mode!.options).toEqual(['event', 'interval', 'both']);
    expect(mode!.default).toBe('event');
  });

  it('기본값은 현행 동작과 동일하다(forward off, mode=event, interval 60s — 무회귀)', () => {
    const defaults = getAgentConfigDefaults('xsfm');
    expect(defaults.forward_received_to_node).toBe(false);
    expect(defaults.state_emit_mode).toBe('event');
    expect(defaults.state_emit_interval).toBe('60s');
  });

  it('state_emit_interval 은 mode=interval|both 일 때만 표시되고 event 에서는 숨겨진다', () => {
    const interval = xsfmField('state_emit_interval')!;
    // 조건부 표시 규약이 state_emit_mode 에 종속되어야 한다.
    expect(interval.visibleWhen?.field).toBe('state_emit_mode');

    expect(isVisible(interval, { state_emit_mode: 'event' })).toBe(false);
    expect(isVisible(interval, { state_emit_mode: 'interval' })).toBe(true);
    expect(isVisible(interval, { state_emit_mode: 'both' })).toBe(true);
  });

  it('forward_received_to_node 는 항상 표시된다(state_emit_mode 와 독립, REQ-01-06)', () => {
    const forward = xsfmField('forward_received_to_node')!;
    expect(forward.visibleWhen).toBeUndefined();
    expect(isVisible(forward, { state_emit_mode: 'event' })).toBe(true);
    expect(isVisible(forward, { state_emit_mode: 'interval' })).toBe(true);
  });

  it('세 필드 모두 운영(operation) 섹션에 속한다', () => {
    expect(xsfmField('forward_received_to_node')!.section).toBe('operation');
    expect(xsfmField('state_emit_mode')!.section).toBe('operation');
    expect(xsfmField('state_emit_interval')!.section).toBe('operation');
  });
});
