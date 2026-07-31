// SPEC-TRIGGER-SCHED-001 — facilityScheduleUtils 순수 유틸 검증.
// M4(TARGET 셀렉터), M5(ACTION 2축), 테이블 컬럼 파생(PLAN/유효기간/PRIO 정렬),
// 규칙↔스케줄 직렬화, 검증(REQ-03-05) 을 UI 없이 검증한다.

import { describe, it, expect } from 'vitest';

import {
  buildActionCommand,
  buildControlPayload,
  buildScheduleFromDraft,
  buildTargetSelector,
  emptyDraft,
  extractTiming,
  formatActionLabel,
  formatPlanSummary,
  formatTargetDescription,
  formatValidity,
  isValidAction,
  isValidFanSpeed,
  parseActionCommand,
  parseRule,
  parseRules,
  parseTargetSelector,
  ruleToDraft,
  sortRulesByPriority,
  targetKindLabel,
  toggleEnabledAt,
  upsertScheduleAt,
  validateRuleDraft,
  type RuleDraft,
  type SerializedSchedule,
} from './facilityScheduleUtils';

describe('M4 — TARGET 셀렉터 조립 (RD-6)', () => {
  it('전체 → { line } 셀렉터 (호선 전체, 전역 all 미도입)', () => {
    expect(buildTargetSelector({ kind: 'all', value: '2' })).toEqual({ line: '2' });
  });

  it('그룹 → { group_id } (byName 이면 { group_name })', () => {
    expect(buildTargetSelector({ kind: 'group', value: 'custom:hall' })).toEqual({ group_id: 'custom:hall' });
    expect(buildTargetSelector({ kind: 'group', value: '대합실', byName: true })).toEqual({ group_name: '대합실' });
  });

  it('개별 → { device_id } (byName 이면 { device_name })', () => {
    expect(buildTargetSelector({ kind: 'device', value: 'GN:P1:2' })).toEqual({ device_id: 'GN:P1:2' });
    expect(buildTargetSelector({ kind: 'device', value: '강남#2', byName: true })).toEqual({ device_name: '강남#2' });
  });

  it('전역 "all" 키는 어떤 조립에도 나오지 않는다', () => {
    const all = ['all', 'group', 'device'].flatMap((k) =>
      Object.keys(buildTargetSelector({ kind: k as 'all', value: 'x' })),
    );
    expect(all).not.toContain('all');
  });

  it('parseTargetSelector 는 우선순위대로 역파싱한다 (device > group > line)', () => {
    expect(parseTargetSelector({ device_id: 'd1' })).toEqual({ kind: 'device', value: 'd1' });
    expect(parseTargetSelector({ device_name: '강남#2' })).toEqual({ kind: 'device', value: '강남#2', byName: true });
    expect(parseTargetSelector({ group_id: 'g1' })).toEqual({ kind: 'group', value: 'g1' });
    expect(parseTargetSelector({ group_name: '대합실' })).toEqual({ kind: 'group', value: '대합실', byName: true });
    expect(parseTargetSelector({ line: '2' })).toEqual({ kind: 'all', value: '2' });
    expect(parseTargetSelector({})).toBeNull();
    expect(parseTargetSelector(undefined)).toBeNull();
  });

  it('targetKindLabel / formatTargetDescription 한글 라벨', () => {
    expect(targetKindLabel('all')).toBe('전체');
    expect(targetKindLabel('group')).toBe('그룹');
    expect(targetKindLabel('device')).toBe('개별');
    expect(formatTargetDescription({ kind: 'all', value: '2' })).toBe('2호선 전체');
    expect(formatTargetDescription({ kind: 'device', value: 'd1' }, '강남 대합실 #2')).toBe('강남 대합실 #2');
  });
});

describe('M5 — ACTION 2축 조립 (RD-4, 모드 축 없음)', () => {
  it('전원만 → set_power + params.power', () => {
    expect(buildActionCommand({ power: true, fanSpeed: null })).toEqual({
      command: 'set_power',
      params: { power: true },
    });
    expect(buildActionCommand({ power: false, fanSpeed: null })).toEqual({
      command: 'set_power',
      params: { power: false },
    });
  });

  it('풍량만 → set_fan_speed + params.fan_speed', () => {
    expect(buildActionCommand({ power: null, fanSpeed: 2 })).toEqual({
      command: 'set_fan_speed',
      params: { fan_speed: 2 },
    });
  });

  it('전원+풍량 → set_multiple + params.{power,fan_speed}', () => {
    expect(buildActionCommand({ power: true, fanSpeed: 2 })).toEqual({
      command: 'set_multiple',
      params: { power: true, fan_speed: 2 },
    });
  });

  it('무지정/풍량 범위 밖은 조립되지 않는다 (null)', () => {
    expect(buildActionCommand({ power: null, fanSpeed: null })).toBeNull();
    expect(buildActionCommand({ power: null, fanSpeed: 0 })).toBeNull();
    expect(buildActionCommand({ power: null, fanSpeed: 4 })).toBeNull();
    expect(isValidFanSpeed(0)).toBe(false);
    expect(isValidFanSpeed(3)).toBe(true);
    expect(isValidFanSpeed(null)).toBe(true);
  });

  it('어떤 조립에도 params 에 mode(Auto/Sleep) 키가 없다', () => {
    for (const spec of [
      { power: true, fanSpeed: null },
      { power: null, fanSpeed: 1 },
      { power: false, fanSpeed: 3 },
    ]) {
      const cmd = buildActionCommand(spec)!;
      expect(cmd.params).not.toHaveProperty('mode');
      expect(JSON.stringify(cmd)).not.toContain('mode');
    }
  });

  it('parseActionCommand 는 params 에서 2축을 역파싱한다(범위 밖은 무변경)', () => {
    expect(parseActionCommand({ command: 'set_power', params: { power: true } })).toEqual({
      power: true,
      fanSpeed: null,
    });
    expect(parseActionCommand({ params: { fan_speed: 2 } })).toEqual({ power: null, fanSpeed: 2 });
    expect(parseActionCommand({ params: { power: false, fan_speed: 9 } })).toEqual({
      power: false,
      fanSpeed: null,
    });
  });

  it('formatActionLabel 은 모드를 표기하지 않는다', () => {
    expect(formatActionLabel({ power: true, fanSpeed: null })).toBe('전원 ON');
    expect(formatActionLabel({ power: false, fanSpeed: null })).toBe('전원 OFF');
    expect(formatActionLabel({ power: null, fanSpeed: 1 })).toBe('풍량 1');
    expect(formatActionLabel({ power: true, fanSpeed: 2 })).toBe('전원 ON · 풍량 2');
    expect(formatActionLabel({ power: null, fanSpeed: null })).toBe('-');
  });

  it('buildControlPayload 은 TARGET + ACTION 을 결합한다', () => {
    expect(buildControlPayload({ kind: 'all', value: '2' }, { power: false, fanSpeed: null })).toEqual({
      command: 'set_power',
      line: '2',
      params: { power: false },
    });
    expect(buildControlPayload({ kind: 'group', value: 'g1' }, { power: true, fanSpeed: 1 })).toEqual({
      command: 'set_multiple',
      group_id: 'g1',
      params: { power: true, fan_speed: 1 },
    });
    // 유효하지 않은 ACTION → null
    expect(buildControlPayload({ kind: 'all', value: '2' }, { power: null, fanSpeed: null })).toBeNull();
  });
});

describe('M2 — 테이블 컬럼 파생 (PLAN/유효기간/PRIO 정렬)', () => {
  it('formatValidity: 빈 valid_to → 무기한', () => {
    expect(formatValidity('2026-01-01', '')).toBe('2026-01-01 ~ 무기한');
    expect(formatValidity('', '')).toBe('무기한');
    expect(formatValidity('', '2026-12-31')).toBe('~ 2026-12-31');
    expect(formatValidity('2026-01-01', '2026-06-30')).toBe('2026-01-01 ~ 2026-06-30');
  });

  it('formatPlanSummary: weekly/once/interval/monthly', () => {
    expect(formatPlanSummary({ type: 'weekly', days: ['mon', 'wed', 'fri', 'tue', 'thu'], times: ['05:30'] })).toBe(
      '주간 · 월화수목금 05:30',
    );
    expect(formatPlanSummary({ type: 'once', value: '2026-07-22T02:00:00' })).toContain('단발 · 2026-07-22');
    expect(formatPlanSummary({ type: 'interval', value: '5s' })).toBe('주기 · 5s');
    expect(formatPlanSummary({ type: 'monthly', day: 'last', times: ['23:59'] })).toBe('월간 · 말일 23:59');
    expect(formatPlanSummary(undefined)).toBe('-');
  });

  it('sortRulesByPriority: 오름차순 + 안정 정렬(동률 원순서 유지)', () => {
    const rules = parseRules([
      { type: 'interval', value: '1s', name: 'A', priority: 2 },
      { type: 'interval', value: '1s', name: 'B', priority: 0 },
      { type: 'interval', value: '1s', name: 'C', priority: 1 },
      { type: 'interval', value: '1s', name: 'D', priority: 0 },
    ]);
    const asc = sortRulesByPriority(rules);
    expect(asc.map((r) => r.name)).toEqual(['B', 'D', 'C', 'A']); // 0,0(안정),1,2
    const desc = sortRulesByPriority(rules, true);
    expect(desc.map((r) => r.name)).toEqual(['A', 'C', 'B', 'D']);
  });
});

describe('규칙 파싱/직렬화 + enabled 하위호환', () => {
  it('parseRule: enabled 부재/nil → true, false 만 비활성', () => {
    expect(parseRule({ type: 'interval', value: '1s' }, 0).enabled).toBe(true);
    expect(parseRule({ type: 'interval', value: '1s', enabled: false }, 0).enabled).toBe(false);
    expect(parseRule({ type: 'interval', value: '1s', enabled: true }, 0).enabled).toBe(true);
  });

  it('parseRule: 확장 메타 + payload 를 파싱한다', () => {
    const r = parseRule(
      {
        type: 'weekly',
        days: ['mon'],
        times: ['05:30'],
        name: '평일 가동',
        valid_from: '2026-01-01',
        valid_to: '',
        priority: 3,
        enabled: true,
        payload: { command: 'set_power', line: '2', params: { power: true } },
      },
      0,
    );
    expect(r.name).toBe('평일 가동');
    expect(r.validFrom).toBe('2026-01-01');
    expect(r.validTo).toBe('');
    expect(r.priority).toBe(3);
    expect(r.target).toEqual({ kind: 'all', value: '2' });
    expect(r.action).toEqual({ power: true, fanSpeed: null });
  });

  it('extractTiming: 타이밍 키만 남기고 메타/payload 제거', () => {
    expect(
      extractTiming({ type: 'weekly', days: ['mon'], times: ['05:30'], name: 'X', priority: 1, payload: {} }),
    ).toEqual({ type: 'weekly', days: ['mon'], times: ['05:30'] });
  });

  it('buildScheduleFromDraft: 초안 → 스케줄 원소(메타+payload 병합), base 미지의 키 보존', () => {
    const draft: RuleDraft = {
      name: '주말 정지',
      validFrom: '',
      validTo: '2026-12-31',
      priority: 1,
      enabled: true,
      schedule: { type: 'weekly', days: ['sat', 'sun'], times: ['23:00'] },
      target: { kind: 'all', value: '2' },
      action: { power: false, fanSpeed: null },
    };
    const out = buildScheduleFromDraft(draft, { source_ch_size: 64 } as SerializedSchedule)!;
    expect(out).toMatchObject({
      type: 'weekly',
      days: ['sat', 'sun'],
      times: ['23:00'],
      name: '주말 정지',
      valid_from: '',
      valid_to: '2026-12-31',
      priority: 1,
      enabled: true,
      payload: { command: 'set_power', line: '2', params: { power: false } },
      source_ch_size: 64,
    });
  });

  it('buildScheduleFromDraft: 유효하지 않은 ACTION → null', () => {
    const draft: RuleDraft = {
      ...emptyDraft(),
      name: 'x',
      schedule: { type: 'interval', value: '1s' },
      target: { kind: 'all', value: '2' },
      action: { power: null, fanSpeed: null },
    };
    expect(buildScheduleFromDraft(draft)).toBeNull();
  });

  it('ruleToDraft 왕복: parse → draft → schedule 동형', () => {
    const original: SerializedSchedule = {
      type: 'weekly',
      days: ['mon'],
      times: ['05:30'],
      name: '평일',
      valid_from: '2026-01-01',
      valid_to: '',
      priority: 2,
      enabled: true,
      payload: { command: 'set_multiple', group_id: 'g1', params: { power: true, fan_speed: 2 } },
    };
    const draft = ruleToDraft(parseRule(original, 0));
    const rebuilt = buildScheduleFromDraft(draft)!;
    expect(rebuilt.name).toBe('평일');
    expect(rebuilt.priority).toBe(2);
    expect(rebuilt.payload).toEqual({ command: 'set_multiple', group_id: 'g1', params: { power: true, fan_speed: 2 } });
  });
});

describe('검증 (REQ-SCHED-03-05 / AC-8)', () => {
  it('필수값(name/PLAN/TARGET/ACTION) 누락 시 검증 실패', () => {
    const base: RuleDraft = {
      name: 'ok',
      validFrom: '',
      validTo: '',
      priority: 0,
      enabled: true,
      schedule: { type: 'interval', value: '1s' },
      target: { kind: 'all', value: '2' },
      action: { power: true, fanSpeed: null },
    };
    expect(validateRuleDraft(base).ok).toBe(true);
    expect(validateRuleDraft({ ...base, name: '  ' }).errors.name).toBe(true);
    expect(validateRuleDraft({ ...base, schedule: undefined }).errors.plan).toBe(true);
    expect(validateRuleDraft({ ...base, target: { kind: 'all', value: '' } }).errors.target).toBe(true);
    expect(validateRuleDraft({ ...base, action: { power: null, fanSpeed: null } }).errors.action).toBe(true);
    expect(isValidAction({ power: true, fanSpeed: null })).toBe(true);
  });
});

describe('배열 변형 (토글/upsert)', () => {
  it('toggleEnabledAt: 지정 인덱스만 enabled 반전(부재→true 기준)', () => {
    const arr: SerializedSchedule[] = [
      { type: 'interval', value: '1s' }, // enabled 부재 → true
      { type: 'interval', value: '2s', enabled: false },
    ];
    const t0 = toggleEnabledAt(arr, 0);
    expect(t0[0]!.enabled).toBe(false);
    expect(t0[1]!.enabled).toBe(false); // 미변경
    const t1 = toggleEnabledAt(arr, 1);
    expect(t1[1]!.enabled).toBe(true);
  });

  it('upsertScheduleAt: index>=0 교체, index<0 추가', () => {
    const arr: SerializedSchedule[] = [{ type: 'interval', value: '1s' }];
    const edited = upsertScheduleAt(arr, 0, { type: 'cron', value: '* * * * *' });
    expect(edited).toEqual([{ type: 'cron', value: '* * * * *' }]);
    const added = upsertScheduleAt(arr, -1, { type: 'cron', value: '* * * * *' });
    expect(added).toHaveLength(2);
  });
});
