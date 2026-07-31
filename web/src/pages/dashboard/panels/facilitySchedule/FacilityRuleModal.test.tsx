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
// 역사/위치 필드는 개별(device) 테이블 조인(이름/라인/역사/위치)을 검증하기 위해 선택적으로 채운다.
const enumState = vi.hoisted(() => ({
  stations: [] as Array<{
    line: string;
    station?: string;
    display_name?: string;
    places?: Array<{ place: string; display_name: string }>;
  }>,
  devices: [] as Array<{
    device_id: string;
    name: string;
    station?: string;
    place?: string;
    online?: boolean;
  }>,
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
  it('agentId 있으면 전체/그룹은 열거 select, 개별은 이름/라인/역사/위치 테이블을 제시한다', () => {
    enumState.stations = [
      { station: 'GN', line: '1', display_name: '강남역', places: [{ place: 'P1', display_name: '1번 승강장' }] },
      { station: 'HD', line: '2', display_name: '홍대입구', places: [{ place: 'P1', display_name: '대합실' }] },
      { station: 'HD2', line: '2', display_name: '홍대2', places: [] },
    ];
    enumState.groups = [{ id: 'custom:hall', name: '대합실', member_count: 5 }];
    enumState.devices = [{ device_id: 'GN:P1:2', name: '강남#2', station: 'GN', place: 'P1', online: true }];
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

    // 개별로 전환 → 테이블(라인/역사/위치 조인). select 는 없다.
    fireEvent.click(screen.getByTestId('fr-target-kind-device'));
    expect(screen.queryByTestId('fr-target-value')).toBeNull();
    expect(screen.getByTestId('fr-target-device-table')).toBeInTheDocument();
    const row = screen.getByTestId('fr-target-device-row-GN:P1:2');
    expect(row).toHaveTextContent('강남#2'); // 이름
    expect(row).toHaveTextContent('1'); // 라인(역사 GN → line 1)
    expect(row).toHaveTextContent('강남역'); // 역사 표시명
    expect(row).toHaveTextContent('1번 승강장'); // 위치 표시명

    // 행 클릭으로 선택 → draft.target 반영(device_id, byName:false — 저장 값 의미 불변)
    fireEvent.click(row);
    fireEvent.change(screen.getByTestId('fr-name'), { target: { value: 'n' } });
    fireEvent.click(screen.getByTestId('facility-rule-save'));
    expect(onSave.mock.calls[0]![0].target).toEqual({ kind: 'device', value: 'GN:P1:2', byName: false });
  });

  it('개별 테이블 검색이 이름/라인/역사/위치 부분일치로 행을 필터링한다', () => {
    enumState.stations = [
      { station: 'GN', line: '1', display_name: '강남역', places: [{ place: 'P1', display_name: '1번 승강장' }] },
      { station: 'HD', line: '2', display_name: '홍대입구', places: [{ place: 'P1', display_name: '대합실' }] },
    ];
    enumState.devices = [
      { device_id: 'GN:P1:2', name: '강남#2', station: 'GN', place: 'P1', online: true },
      { device_id: 'HD:P1:1', name: '홍대#1', station: 'HD', place: 'P1', online: false },
    ];
    render(<FacilityRuleModal initial={{ ...emptyDraft(), target: { kind: 'device', value: '' } }} agentId="agent-1" onSave={vi.fn()} onCancel={() => {}} />);

    // 두 행 모두 노출
    expect(screen.getByTestId('fr-target-device-row-GN:P1:2')).toBeInTheDocument();
    expect(screen.getByTestId('fr-target-device-row-HD:P1:1')).toBeInTheDocument();

    // 역사 표시명으로 필터 → 강남 행만
    fireEvent.change(screen.getByTestId('fr-target-device-search'), { target: { value: '홍대' } });
    expect(screen.queryByTestId('fr-target-device-row-GN:P1:2')).toBeNull();
    expect(screen.getByTestId('fr-target-device-row-HD:P1:1')).toBeInTheDocument();
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

describe('에이전트 선택(선택 기능) — 규칙 추가 시 TARGET 미표시 수정(Bug 1)', () => {
  it('agents 미제공 시 에이전트 셀렉터가 없다(패널 경로 불변)', () => {
    render(<FacilityRuleModal initial={emptyDraft()} agentId="" onSave={vi.fn()} onCancel={() => {}} />);
    expect(screen.queryByTestId('fr-agent-select')).toBeNull();
  });

  it('agents 제공 + 에이전트 선택 시 TARGET 열거가 활성화되고 onAgentChange 가 호출된다', () => {
    enumState.stations = [{ line: '2' }];
    const onAgentChange = vi.fn();
    render(
      <FacilityRuleModal
        initial={emptyDraft()}
        agentId=""
        agents={[{ id: 'ag-x', name: '설비X' }]}
        selectedAgentId=""
        onAgentChange={onAgentChange}
        onSave={vi.fn()}
        onCancel={() => {}}
      />,
    );
    // 선택 전에는 free-form 입력(비열거). 셀렉터는 존재.
    expect(screen.getByTestId('fr-agent-select')).toBeInTheDocument();
    // 에이전트 선택 → TARGET 값이 열거 select 로 전환(호선 2 옵션 노출).
    fireEvent.change(screen.getByTestId('fr-agent-select'), { target: { value: 'ag-x' } });
    expect(onAgentChange).toHaveBeenCalledWith('ag-x');
    const sel = screen.getByTestId('fr-target-value') as HTMLSelectElement;
    expect(Array.from(sel.options).map((o) => o.value)).toContain('2');
  });
});

describe('대상 노드 선택(선택 기능) — 스케줄 뷰 CREATE 모드', () => {
  const nodeProps = (derivedAgentIds: string[]) => ({
    agents: [{ id: 'ag-x', name: '설비X' }],
    selectedAgentId: '',
    onAgentChange: vi.fn(),
    nodes: [{ flowId: 'f1', nodeId: 'n1', label: 'F1 / N1', derivedAgentIds }],
    selectedNodeKey: '',
    onNodeChange: vi.fn(),
  });

  it('nodes 미제공 시 대상 노드 셀렉터가 없다(패널/행 편집 경로 불변)', () => {
    render(<FacilityRuleModal initial={emptyDraft()} agentId="" onSave={vi.fn()} onCancel={() => {}} />);
    expect(screen.queryByTestId('fr-node-select')).toBeNull();
  });

  it('nodes 제공 시 본문 최상단에 대상 노드 셀렉터를 노출한다(라벨 = 플로우/노드)', () => {
    render(
      <FacilityRuleModal
        initial={emptyDraft()}
        agentId=""
        onSave={vi.fn()}
        onCancel={() => {}}
        {...nodeProps(['ag-x'])}
      />,
    );
    const sel = screen.getByTestId('fr-node-select') as HTMLSelectElement;
    expect(sel).toBeInTheDocument();
    expect(Array.from(sel.options).map((o) => o.textContent)).toContain('F1 / N1');
  });

  it('단일 유도 노드 선택 시 에이전트가 프리필되고 TARGET 열거가 활성화된다', () => {
    enumState.stations = [{ line: '2' }];
    const props = nodeProps(['ag-x']);
    render(
      <FacilityRuleModal
        initial={emptyDraft()}
        agentId=""
        onSave={vi.fn()}
        onCancel={() => {}}
        {...props}
      />,
    );

    // 노드 선택 → onNodeChange + 유도 에이전트 프리필(onAgentChange 로 통지).
    fireEvent.change(screen.getByTestId('fr-node-select'), { target: { value: 'f1:n1' } });
    expect(props.onNodeChange).toHaveBeenCalledWith('f1:n1');
    expect(props.onAgentChange).toHaveBeenCalledWith('ag-x');

    // 에이전트 셀렉터가 유도 값으로 프리필 + TARGET 이 열거 select 로 전환(호선 2).
    expect((screen.getByTestId('fr-agent-select') as HTMLSelectElement).value).toBe('ag-x');
    const target = screen.getByTestId('fr-target-value') as HTMLSelectElement;
    expect(Array.from(target.options).map((o) => o.value)).toContain('2');
  });

  it('모호(복수 유도) 노드 선택 시 에이전트는 프리필되지 않는다(수동 선택 폴백)', () => {
    const props = nodeProps(['ag-a', 'ag-b']);
    render(
      <FacilityRuleModal
        initial={emptyDraft()}
        agentId=""
        onSave={vi.fn()}
        onCancel={() => {}}
        {...props}
      />,
    );
    fireEvent.change(screen.getByTestId('fr-node-select'), { target: { value: 'f1:n1' } });
    expect(props.onNodeChange).toHaveBeenCalledWith('f1:n1');
    expect(props.onAgentChange).not.toHaveBeenCalled();
    expect((screen.getByTestId('fr-agent-select') as HTMLSelectElement).value).toBe('');
  });

  it('대상 노드 미선택 시 저장이 거부되고 오류가 표시된다(노드 필수)', () => {
    const onSave = vi.fn();
    const draft = { ...emptyDraft(), name: 'n', target: { kind: 'all' as const, value: '2' } };
    render(
      <FacilityRuleModal
        initial={draft}
        agentId=""
        onSave={onSave}
        onCancel={() => {}}
        {...nodeProps(['ag-x'])}
      />,
    );
    // 노드 미선택 상태로 저장 시도 → 거부 + fr-error-node.
    fireEvent.click(screen.getByTestId('facility-rule-save'));
    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByTestId('fr-error-node')).toBeInTheDocument();
  });

  it('노드 선택 후에는 저장이 허용된다(다른 필드 유효 시 onSave 호출)', () => {
    const onSave = vi.fn();
    const draft = { ...emptyDraft(), name: 'n', target: { kind: 'all' as const, value: '2' } };
    render(
      <FacilityRuleModal
        initial={draft}
        agentId=""
        onSave={onSave}
        onCancel={() => {}}
        {...nodeProps([])}
      />,
    );
    fireEvent.change(screen.getByTestId('fr-node-select'), { target: { value: 'f1:n1' } });
    fireEvent.click(screen.getByTestId('facility-rule-save'));
    expect(onSave).toHaveBeenCalledTimes(1);
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
