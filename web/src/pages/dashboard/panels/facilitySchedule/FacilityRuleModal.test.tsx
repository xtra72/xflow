// SPEC-TRIGGER-SCHED-001 M3~M5 — FacilityRuleModal.
// 생성/편집 왕복, 필수값 검증(AC-8), TARGET 피커(M4/AC-9), ACTION 편집기(M5/AC-10), 취소.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

// TriggerScheduleEditor 는 스텁으로 대체(모달 폼 로직에 집중). value/onChange 계약만 재현.
vi.mock('@/components/property/TriggerScheduleEditor', () => ({
  TriggerScheduleEditor: ({
    value,
    onChange,
  }: {
    value: unknown;
    onChange: (v: unknown) => void;
  }) => (
    <div>
      <span data-testid="plan-count">{Array.isArray(value) ? value.length : 0}</span>
      <button type="button" data-testid="plan-set" onClick={() => onChange([{ type: 'cron', value: '0 5 * * *' }])}>
        set
      </button>
      <button type="button" data-testid="plan-clear" onClick={() => onChange([])}>
        clear
      </button>
    </div>
  ),
}));

// 열거 훅: agentId 유무에 따라 옵션을 제공하도록 상태 기반 mock.
const enumState = vi.hoisted(() => ({
  stations: [] as Array<{ line: string }>,
  devices: [] as Array<{ device_id: string; name: string }>,
  groups: [] as Array<{ id: string; name: string; member_count: number }>,
}));
vi.mock('@/hooks/useStation', () => ({
  useStations: () => ({ data: enumState.stations }),
  useXsfmDevices: () => ({ data: enumState.devices }),
}));
vi.mock('@/hooks/useGroups', () => ({ useGroups: () => ({ data: enumState.groups }) }));

import FacilityRuleModal from './FacilityRuleModal';
import { emptyDraft, ruleToDraft, parseRule } from './facilityScheduleUtils';

beforeEach(() => {
  enumState.stations = [];
  enumState.devices = [];
  enumState.groups = [];
});

describe('M3 — 생성 왕복 (AC-5)', () => {
  it('필수값 입력 후 저장 시 onSave 에 조립된 초안을 전달한다 (free-form TARGET)', () => {
    const onSave = vi.fn();
    render(<FacilityRuleModal initial={emptyDraft()} agentId="" onSave={onSave} onCancel={() => {}} />);

    fireEvent.change(screen.getByTestId('fr-name'), { target: { value: '평일 정규 가동' } });
    // TARGET: 기본 kind=all, free-form 값 입력.
    fireEvent.change(screen.getByTestId('fr-target-value'), { target: { value: '2' } });
    // ACTION: emptyDraft 기본 power=ON. 저장.
    fireEvent.click(screen.getByTestId('facility-rule-save'));

    expect(onSave).toHaveBeenCalledTimes(1);
    const draft = onSave.mock.calls[0]![0];
    expect(draft.name).toBe('평일 정규 가동');
    expect(draft.target).toEqual({ kind: 'all', value: '2' });
    expect(draft.action).toEqual({ power: true, fanSpeed: null });
    expect(draft.schedule?.type).toBe('weekly');
  });
});

describe('M3 — 편집 왕복 (AC-6)', () => {
  it('기존 규칙 값이 프리필되고 수정 후 저장 시 반영된다', () => {
    const rule = parseRule(
      {
        type: 'weekly',
        days: ['mon'],
        times: ['05:30'],
        name: '평일',
        valid_from: '2026-01-01',
        valid_to: '',
        priority: 2,
        enabled: true,
        payload: { command: 'set_power', line: '2', params: { power: false } },
      },
      3,
    );
    const onSave = vi.fn();
    render(<FacilityRuleModal initial={ruleToDraft(rule)} agentId="" onSave={onSave} onCancel={() => {}} />);

    // 프리필 확인
    expect((screen.getByTestId('fr-name') as HTMLInputElement).value).toBe('평일');
    expect((screen.getByTestId('fr-priority') as HTMLInputElement).value).toBe('2');
    expect((screen.getByTestId('fr-target-value') as HTMLInputElement).value).toBe('2');
    expect((screen.getByTestId('fr-action-power') as HTMLSelectElement).value).toBe('off');

    // 이름 수정 + 저장
    fireEvent.change(screen.getByTestId('fr-name'), { target: { value: '평일 수정' } });
    fireEvent.click(screen.getByTestId('facility-rule-save'));

    expect(onSave).toHaveBeenCalledTimes(1);
    expect(onSave.mock.calls[0]![0].name).toBe('평일 수정');
  });
});

describe('M3 — 필수값 검증 (AC-8)', () => {
  it('이름 누락 시 저장이 거부되고 오류가 표시된다', () => {
    const onSave = vi.fn();
    const draft = { ...emptyDraft(), target: { kind: 'all' as const, value: '2' } };
    render(<FacilityRuleModal initial={draft} agentId="" onSave={onSave} onCancel={() => {}} />);

    fireEvent.click(screen.getByTestId('facility-rule-save'));
    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByTestId('fr-error-name')).toBeInTheDocument();
  });

  it('PLAN 누락 시 저장이 거부된다', () => {
    const onSave = vi.fn();
    const draft = { ...emptyDraft(), name: 'x', target: { kind: 'all' as const, value: '2' } };
    render(<FacilityRuleModal initial={draft} agentId="" onSave={onSave} onCancel={() => {}} />);

    fireEvent.click(screen.getByTestId('plan-clear')); // PLAN 비움
    fireEvent.click(screen.getByTestId('facility-rule-save'));
    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByTestId('fr-error-plan')).toBeInTheDocument();
  });

  it('TARGET 값 누락 시 저장이 거부된다', () => {
    const onSave = vi.fn();
    const draft = { ...emptyDraft(), name: 'x', target: { kind: 'all' as const, value: '' } };
    render(<FacilityRuleModal initial={draft} agentId="" onSave={onSave} onCancel={() => {}} />);
    fireEvent.click(screen.getByTestId('facility-rule-save'));
    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByTestId('fr-error-target')).toBeInTheDocument();
  });

  it('ACTION 무지정 시 저장이 거부된다', () => {
    const onSave = vi.fn();
    const draft = {
      ...emptyDraft(),
      name: 'x',
      target: { kind: 'all' as const, value: '2' },
      action: { power: null, fanSpeed: null },
    };
    render(<FacilityRuleModal initial={draft} agentId="" onSave={onSave} onCancel={() => {}} />);
    fireEvent.click(screen.getByTestId('facility-rule-save'));
    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByTestId('fr-error-action')).toBeInTheDocument();
  });
});

describe('M4 — TARGET 피커 (AC-9)', () => {
  it('agentId 있으면 종류별 열거 옵션(라인/그룹/기기)을 제시한다', () => {
    enumState.stations = [{ line: '1' }, { line: '2' }, { line: '2' }];
    enumState.groups = [{ id: 'custom:hall', name: '대합실', member_count: 5 }];
    enumState.devices = [{ device_id: 'GN:P1:2', name: '강남#2' }];
    const onSave = vi.fn();
    render(<FacilityRuleModal initial={emptyDraft()} agentId="agent-1" onSave={onSave} onCancel={() => {}} />);

    // 기본 kind=all → 호선 옵션(distinct)
    let sel = screen.getByTestId('fr-target-value') as HTMLSelectElement;
    const values = Array.from(sel.options).map((o) => o.value);
    expect(values).toContain('1');
    expect(values).toContain('2');
    expect(values.filter((v) => v === '2')).toHaveLength(1); // distinct

    // 그룹으로 전환 → group.id 옵션
    fireEvent.click(screen.getByTestId('fr-target-kind-group'));
    sel = screen.getByTestId('fr-target-value') as HTMLSelectElement;
    expect(Array.from(sel.options).map((o) => o.value)).toContain('custom:hall');

    // 개별로 전환 → device_id 옵션 선택 → draft.target 반영
    fireEvent.click(screen.getByTestId('fr-target-kind-device'));
    fireEvent.change(screen.getByTestId('fr-target-value'), { target: { value: 'GN:P1:2' } });
    fireEvent.change(screen.getByTestId('fr-name'), { target: { value: 'n' } });
    fireEvent.click(screen.getByTestId('facility-rule-save'));
    expect(onSave.mock.calls[0]![0].target).toEqual({ kind: 'device', value: 'GN:P1:2', byName: false });
  });

  it('free-form 에서 이름 셀렉터 토글 시 byName=true', () => {
    const onSave = vi.fn();
    const draft = { ...emptyDraft(), name: 'n', target: { kind: 'device' as const, value: '강남#2' } };
    render(<FacilityRuleModal initial={draft} agentId="" onSave={onSave} onCancel={() => {}} />);
    fireEvent.click(screen.getByTestId('fr-target-byname'));
    fireEvent.click(screen.getByTestId('facility-rule-save'));
    expect(onSave.mock.calls[0]![0].target).toEqual({ kind: 'device', value: '강남#2', byName: true });
  });
});

describe('M5 — ACTION 편집기 (AC-10)', () => {
  it('전원+풍량 선택 시 draft.action 이 2축을 담는다(모드 없음)', () => {
    const onSave = vi.fn();
    const draft = { ...emptyDraft(), name: 'n', target: { kind: 'all' as const, value: '2' } };
    render(<FacilityRuleModal initial={draft} agentId="" onSave={onSave} onCancel={() => {}} />);

    fireEvent.change(screen.getByTestId('fr-action-power'), { target: { value: 'off' } });
    fireEvent.change(screen.getByTestId('fr-action-fan'), { target: { value: '2' } });
    fireEvent.click(screen.getByTestId('facility-rule-save'));

    expect(onSave.mock.calls[0]![0].action).toEqual({ power: false, fanSpeed: 2 });
    // 편집기 UI 에 모드 컨트롤이 없다.
    expect(screen.queryByText(/Auto|Sleep|모드/)).toBeNull();
  });
});

describe('취소 (AC / REQ-03-07)', () => {
  it('취소 버튼 클릭 시 onCancel 호출, onSave 미호출', () => {
    const onSave = vi.fn();
    const onCancel = vi.fn();
    render(<FacilityRuleModal initial={emptyDraft()} agentId="" onSave={onSave} onCancel={onCancel} />);
    fireEvent.click(screen.getByTestId('facility-rule-cancel-btn'));
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onSave).not.toHaveBeenCalled();
  });
});
