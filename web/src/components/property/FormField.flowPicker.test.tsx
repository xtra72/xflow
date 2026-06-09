// FormField flow_picker — 타깃 인지(target-aware) 서브플로우 picker 테스트
// (SPEC-REMOTE-001).
//
// 시나리오 A(원격 노드 내부 서브플로우): 원격 노드의 플로우를 편집할 때 flow_picker
// 는 "그 노드"의 플로우를 나열해야 한다(매니저 로컬 플로우 아님). 저장되는 flow_id
// 는 그 노드에 존재하는 플로우를 가리켜 배포 시 노드에서 네이티브로 해석된다.
//
// 검증:
//   - 원격 TargetProvider 컨텍스트: picker 가 원격 노드 플로우 목록을 나열하고,
//     자기참조(currentFlowId)를 제외한다.
//   - 로컬 컨텍스트(Provider 없음 → 기본 로컬 타깃): picker 가 로컬 플로우를
//     나열한다(기존 동작 회귀 없음).
//   - 선택 시 { flow_id, flow_name } 복합 객체를 onChange 로 반환한다(원격이면 노드
//     플로우 id).

import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';

import type { ResourceTarget } from '@/lib/remote/target';
import type { FlowInfo } from '@/types/flow';
import type { ConfigField } from '@/types/node';

// --- 모킹: 타깃 인지 플로우 목록 ---
// target 인자에 따라 로컬/원격 플로우를 돌려주는 스파이. 각 테스트가 구현을 주입한다.
const useFlowsTargetMock =
  vi.fn<(target: ResourceTarget) => { data: { data: FlowInfo[]; total: number } | undefined; isLoading: boolean }>();
vi.mock('@/hooks/useResourceTargets', () => ({
  useFlowsTarget: (target: ResourceTarget) => useFlowsTargetMock(target),
}));

// --- 모킹: 에디터 스토어 currentFlowId(자기참조 제외용) ---
let currentFlowIdValue: string | null = null;
vi.mock('@/stores/editorStore', () => ({
  useEditorStore: (selector: (s: { currentFlowId: string | null }) => unknown) =>
    selector({ currentFlowId: currentFlowIdValue }),
}));

// agent_select 경로는 본 테스트와 무관하므로 useAgents 는 빈 결과로 스텁한다.
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] }, isLoading: false }),
}));

// --- 모킹: 원격 훅(그룹 RU 노드 선택기 의존성) ---
// 본 파일은 "동일노드 원격 편집" + "로컬 회귀(노드 선택기 미노출)" 시나리오만 다루므로
// 기본값으로 노드 선택기가 노출되지 않도록(비-server / 노드 없음) 둔다. 그룹 RU 의
// 로컬→원격 노드 선택 자체는 FlowPickerInput.test.tsx 에서 별도 검증한다.
const useManagedNodesMock = vi.hoisted(() => vi.fn());
const useRemoteModeMock = vi.hoisted(() => vi.fn());
const useNodeLiveListMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useRemote', () => ({
  useManagedNodes: (...a: unknown[]) => useManagedNodesMock(...a),
  useRemoteMode: () => useRemoteModeMock(),
  useNodeLiveList: (...a: unknown[]) => useNodeLiveListMock(...a),
}));

// TargetProvider/useTargetContext 는 실제 구현을 사용한다(컨텍스트 전파 검증 목적).
import { FormField } from './FormField';
import { I18nProvider } from '@/lib/i18n';
import { TargetProvider } from '@/lib/remote/TargetContext';

const REMOTE_TARGET: ResourceTarget = { type: 'remote', instanceId: 'node-a' };

const flow = (id: string, name: string): FlowInfo => ({
  id,
  name,
  status: 'running',
  node_count: 0,
});

const flowPickerField: ConfigField = {
  name: 'flow_id',
  type: 'flow_picker',
  label: '참조 플로우',
};

function listResult(flows: FlowInfo[]) {
  return { data: { data: flows, total: flows.length }, isLoading: false };
}

beforeEach(() => {
  useFlowsTargetMock.mockReset();
  currentFlowIdValue = null;
  // 기본: 노드 선택기 미노출(로컬 회귀/동일노드 원격 편집 시나리오).
  useManagedNodesMock.mockReset().mockReturnValue({ data: [] });
  useRemoteModeMock.mockReset().mockReturnValue({ data: { mode: 'disabled' } });
  useNodeLiveListMock.mockReset().mockReturnValue({ data: [], isLoading: false });
});

/** I18nProvider 로 감싸 렌더한다(FlowPickerInput 이 useTranslation 사용). */
function renderWithI18n(ui: React.ReactElement) {
  return render(<I18nProvider>{ui}</I18nProvider>);
}

describe('FormField flow_picker — 원격 타깃', () => {
  it('원격 TargetProvider 컨텍스트에서 picker 는 원격 노드의 플로우를 나열한다', () => {
    const remoteFlows = [flow('rf-1', '노드 플로우 A'), flow('rf-2', '노드 플로우 B')];
    // 원격 타깃이 전달되어야만 원격 목록을 돌려주도록 구현을 검사한다.
    useFlowsTargetMock.mockImplementation((target) =>
      target.type === 'remote' && target.instanceId === 'node-a'
        ? listResult(remoteFlows)
        : listResult([flow('local-x', '로컬 플로우 X')]),
    );

    renderWithI18n(
      <TargetProvider target={REMOTE_TARGET}>
        <FormField field={flowPickerField} value="" onChange={vi.fn()} />
      </TargetProvider>,
    );

    const select = screen.getByRole('combobox');
    // 원격 노드 플로우가 옵션으로 노출된다.
    expect(within(select).getByRole('option', { name: '노드 플로우 A' })).toBeTruthy();
    expect(within(select).getByRole('option', { name: '노드 플로우 B' })).toBeTruthy();
    // 매니저 로컬 플로우는 노출되지 않는다.
    expect(within(select).queryByRole('option', { name: '로컬 플로우 X' })).toBeNull();
  });

  it('자기참조(currentFlowId)인 원격 플로우는 후보에서 제외된다', () => {
    currentFlowIdValue = 'rf-1';
    const remoteFlows = [flow('rf-1', '자기 자신'), flow('rf-2', '다른 노드 플로우')];
    useFlowsTargetMock.mockReturnValue(listResult(remoteFlows));

    renderWithI18n(
      <TargetProvider target={REMOTE_TARGET}>
        <FormField field={flowPickerField} value="" onChange={vi.fn()} />
      </TargetProvider>,
    );

    const select = screen.getByRole('combobox');
    expect(within(select).queryByRole('option', { name: '자기 자신' })).toBeNull();
    expect(within(select).getByRole('option', { name: '다른 노드 플로우' })).toBeTruthy();
  });

  it('원격 플로우 선택 시 { flow_id, flow_name } 으로 노드 플로우 id 를 반환한다', () => {
    const onChange = vi.fn();
    const remoteFlows = [flow('rf-2', '노드 플로우 B')];
    useFlowsTargetMock.mockReturnValue(listResult(remoteFlows));

    renderWithI18n(
      <TargetProvider target={REMOTE_TARGET}>
        <FormField field={flowPickerField} value="" onChange={onChange} />
      </TargetProvider>,
    );

    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'rf-2' } });
    expect(onChange).toHaveBeenCalledWith({ flow_id: 'rf-2', flow_name: '노드 플로우 B' });
  });
});

describe('FormField flow_picker — 로컬 타깃(회귀)', () => {
  it('Provider 없이(기본 로컬 타깃) picker 는 로컬 플로우를 나열한다', () => {
    const localFlows = [flow('local-1', '로컬 플로우 1'), flow('local-2', '로컬 플로우 2')];
    useFlowsTargetMock.mockImplementation((target) =>
      // 로컬 기본값(Provider 미설정)일 때만 로컬 목록을 돌려준다.
      target.type === 'local' ? listResult(localFlows) : listResult([]),
    );

    renderWithI18n(<FormField field={flowPickerField} value="" onChange={vi.fn()} />);

    const select = screen.getByRole('combobox');
    expect(within(select).getByRole('option', { name: '로컬 플로우 1' })).toBeTruthy();
    expect(within(select).getByRole('option', { name: '로컬 플로우 2' })).toBeTruthy();
  });

  it('로컬 picker 도 currentFlowId 자기참조를 제외한다(기존 동작)', () => {
    currentFlowIdValue = 'local-1';
    const localFlows = [flow('local-1', '편집 중 플로우'), flow('local-2', '로컬 플로우 2')];
    useFlowsTargetMock.mockReturnValue(listResult(localFlows));

    renderWithI18n(<FormField field={flowPickerField} value="" onChange={vi.fn()} />);

    const select = screen.getByRole('combobox');
    expect(within(select).queryByRole('option', { name: '편집 중 플로우' })).toBeNull();
    expect(within(select).getByRole('option', { name: '로컬 플로우 2' })).toBeTruthy();
  });
});
