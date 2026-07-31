// SPEC-SCHEDULE-VIEW-001 M6 — scheduleLogCsv.buildScheduleLogCsv 단위 테스트.
//
// 병합된 로그 행을 CSV 로 직렬화하는 순수 빌더를 검증한다: 헤더 행, 논리 행당 한 줄,
// 결과/대상 요약(ok=N/M), fire-only 처리, actor≠declared 표기, CSV 이스케이프(쉼표).
// mergeScheduleLogs 를 통해 실제 병합 결과를 입력으로 사용한다.

import { describe, expect, it } from 'vitest';

import type { ScheduleLogRecord } from '@/types/schedule';

import { buildScheduleLogCsv } from './scheduleLogCsv';
import { mergeScheduleLogs } from './scheduleLogMerge';

function rec(over: Partial<ScheduleLogRecord>): ScheduleLogRecord {
  return {
    id: 0,
    correlation_id: 'sch:0',
    record_kind: 'fire',
    schedule_id: 'sch',
    rule_name: '규칙',
    declared_agent_id: 'ag-decl',
    actor_agent_id: '',
    trigger_time: 1000,
    target: '',
    action: '',
    result: '',
    targets: [],
    reason: '',
    timestamp: 1000,
    ...over,
  };
}

describe('buildScheduleLogCsv', () => {
  it('헤더 행을 포함한다', () => {
    const csv = buildScheduleLogCsv([]);
    const [header] = csv.split('\r\n');
    expect(header).toBe('실행 시각,규칙 이름,에이전트,대상,동작,결과');
  });

  it('병합된 fire+result 를 한 줄로 직렬화하고 대상 요약을 포함한다', () => {
    const rows = mergeScheduleLogs([
      rec({
        correlation_id: 'c:1',
        record_kind: 'result',
        rule_name: '전원 규칙',
        declared_agent_id: 'ag-1',
        actor_agent_id: 'ag-1',
        action: 'set_multiple',
        target: 'group=g1',
        result: 'error',
        targets: [
          { target: 'm1', result: 'ok', reason: '' },
          { target: 'm2', result: 'error', reason: 'timeout' },
        ],
        trigger_time: 1000,
        timestamp: 1000,
      }),
      rec({ correlation_id: 'c:1', record_kind: 'fire', rule_name: '전원 규칙', trigger_time: 1000 }),
    ]);

    const lines = buildScheduleLogCsv(rows).split('\r\n');
    // 헤더 + 데이터 1행.
    expect(lines).toHaveLength(2);
    const dataLine = lines[1]!;
    expect(dataLine).toContain('전원 규칙');
    expect(dataLine).toContain('set_multiple');
    expect(dataLine).toContain('group=g1');
    // 결과 셀: error + 대상 요약.
    expect(dataLine).toContain('error (ok=1/2)');
  });

  it('fire-only 행은 결과 "결과 없음", 대상/동작 "-" 으로 표기한다', () => {
    const rows = mergeScheduleLogs([
      rec({ correlation_id: 'c:2', record_kind: 'fire', rule_name: '단순', trigger_time: 2000 }),
    ]);
    const dataLine = buildScheduleLogCsv(rows).split('\r\n')[1]!;
    // 시각,규칙,에이전트,대상(-),동작(-),결과(결과 없음).
    expect(dataLine).toContain('결과 없음');
    // 대상/동작 자리는 '-'.
    expect(dataLine).toMatch(/,-,-,결과 없음$/);
  });

  it('actor 가 declared 와 다르면 에이전트 셀에 양쪽을 표기한다', () => {
    const rows = mergeScheduleLogs([
      rec({
        correlation_id: 'c:3',
        record_kind: 'result',
        declared_agent_id: 'ag-decl',
        actor_agent_id: 'ag-actor',
        result: 'ok',
        trigger_time: 3000,
        timestamp: 3000,
      }),
    ]);
    const dataLine = buildScheduleLogCsv(rows).split('\r\n')[1]!;
    expect(dataLine).toContain('ag-decl (실행: ag-actor)');
  });

  it('쉼표를 포함한 필드는 큰따옴표로 이스케이프한다', () => {
    const rows = mergeScheduleLogs([
      rec({
        correlation_id: 'c:4',
        record_kind: 'fire',
        rule_name: 'A, B, C 규칙',
        trigger_time: 4000,
      }),
    ]);
    const dataLine = buildScheduleLogCsv(rows).split('\r\n')[1]!;
    expect(dataLine).toContain('"A, B, C 규칙"');
  });
});
