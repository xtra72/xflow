package flow

import (
	"encoding/json"
	"testing"
)

// ===========================================================================
// SPEC-SUBFLOW-001 Milestone 1: 플로우 레벨 입출력 포트 모델 테스트
//
// 본 테스트는 그룹 A(REQ-SUBFLOW-A01, A03, A05)의 백엔드 모델/영속 부분을
// 검증한다. 플로우 레벨 포트는 노드 포트와 동일한 flow.Port 타입을 재사용하되
// (ID/Name/Direction), 플로우 정의 최상위에 저장되는 플로우 레벨 엔티티이다.
// 방향은 입력 목록 → PortInput, 출력 목록 → PortOutput 으로 강제한다.
// ===========================================================================

// REQ-SUBFLOW-A01: 신규 플로우는 입출력 포트가 비어 있어야 한다(빈 슬라이스, nil 아님).
func TestFlow_Inputs_Outputs_DefaultEmpty(t *testing.T) {
	f := NewFlow("empty-ports-flow")

	if got := f.Inputs(); got == nil {
		t.Fatalf("Inputs() 가 nil 이면 안 된다 (빈 슬라이스여야 함)")
	} else if len(got) != 0 {
		t.Errorf("기본 Inputs 길이 = %d, want 0", len(got))
	}

	if got := f.Outputs(); got == nil {
		t.Fatalf("Outputs() 가 nil 이면 안 된다 (빈 슬라이스여야 함)")
	} else if len(got) != 0 {
		t.Errorf("기본 Outputs 길이 = %d, want 0", len(got))
	}
}

// REQ-SUBFLOW-A01/A02: SetInputs/SetOutputs 로 플로우 포트를 설정·조회할 수 있어야 한다.
// 설정 시 방향이 각 목록(input/output)에 맞게 강제된다.
func TestFlow_SetInputs_SetOutputs(t *testing.T) {
	f := NewFlow("set-ports-flow")

	in := []Port{
		{ID: "in-1", Name: "in1"},
		{ID: "in-2", Name: "in2"},
	}
	out := []Port{
		{ID: "out-1", Name: "out1"},
	}

	f.SetInputs(in)
	f.SetOutputs(out)

	gotIn := f.Inputs()
	if len(gotIn) != 2 {
		t.Fatalf("Inputs 길이 = %d, want 2", len(gotIn))
	}
	for _, p := range gotIn {
		if p.Direction != PortInput {
			t.Errorf("입력 포트 %q 방향 = %q, want %q", p.Name, p.Direction, PortInput)
		}
	}
	if gotIn[0].ID != "in-1" || gotIn[0].Name != "in1" {
		t.Errorf("입력 포트[0] = %+v, want id=in-1 name=in1", gotIn[0])
	}

	gotOut := f.Outputs()
	if len(gotOut) != 1 {
		t.Fatalf("Outputs 길이 = %d, want 1", len(gotOut))
	}
	if gotOut[0].Direction != PortOutput {
		t.Errorf("출력 포트 방향 = %q, want %q", gotOut[0].Direction, PortOutput)
	}
	if gotOut[0].ID != "out-1" || gotOut[0].Name != "out1" {
		t.Errorf("출력 포트[0] = %+v, want id=out-1 name=out1", gotOut[0])
	}
}

// Inputs()/Outputs() 는 방어적 복사본을 반환해야 한다(외부 변경이 내부에 반영되지 않음).
func TestFlow_Inputs_Outputs_DefensiveCopy(t *testing.T) {
	f := NewFlow("defensive-flow")
	f.SetInputs([]Port{{ID: "in-1", Name: "in1"}})

	got := f.Inputs()
	got[0].Name = "mutated"

	if f.Inputs()[0].Name != "in1" {
		t.Errorf("Inputs() 반환값 변경이 내부 상태에 반영됨 — 방어적 복사 위반: %q", f.Inputs()[0].Name)
	}
}

// REQ-SUBFLOW-A03: 포트 이름을 변경해도 id 가 유지되어 식별자 연속성이 보존된다.
func TestFlow_RenamePort_PreservesID(t *testing.T) {
	f := NewFlow("rename-flow")
	f.SetInputs([]Port{{ID: "stable-id", Name: "oldName"}})

	// 이름만 바꾼 새 목록으로 교체(같은 id 유지)
	f.SetInputs([]Port{{ID: "stable-id", Name: "newName"}})

	got := f.Inputs()
	if len(got) != 1 {
		t.Fatalf("Inputs 길이 = %d, want 1", len(got))
	}
	if got[0].ID != "stable-id" {
		t.Errorf("포트 id = %q, want stable-id (이름 변경 후에도 불변)", got[0].ID)
	}
	if got[0].Name != "newName" {
		t.Errorf("포트 name = %q, want newName", got[0].Name)
	}
}

// REQ-SUBFLOW-A01/A02: AddInput/AddOutput/RemoveInput/RemoveOutput 변이자가
// 기존 AddNode/RemoveNode 스타일을 따른다.
func TestFlow_AddRemovePort(t *testing.T) {
	f := NewFlow("addremove-flow")

	f.AddInput(Port{ID: "in-1", Name: "in1"})
	f.AddInput(Port{ID: "in-2", Name: "in2"})
	f.AddOutput(Port{ID: "out-1", Name: "out1"})

	if len(f.Inputs()) != 2 {
		t.Fatalf("AddInput 후 Inputs 길이 = %d, want 2", len(f.Inputs()))
	}
	if f.Inputs()[1].Direction != PortInput {
		t.Errorf("AddInput 방향 = %q, want %q", f.Inputs()[1].Direction, PortInput)
	}
	if len(f.Outputs()) != 1 {
		t.Fatalf("AddOutput 후 Outputs 길이 = %d, want 1", len(f.Outputs()))
	}

	if !f.RemoveInput("in-1") {
		t.Errorf("RemoveInput(in-1) 이 false 반환 — 존재하는 포트여야 함")
	}
	if len(f.Inputs()) != 1 || f.Inputs()[0].ID != "in-2" {
		t.Errorf("RemoveInput 후 남은 입력 = %+v, want [in-2]", f.Inputs())
	}
	if f.RemoveInput("nonexistent") {
		t.Errorf("RemoveInput(nonexistent) 이 true 반환 — 없는 포트는 false 여야 함")
	}

	if !f.RemoveOutput("out-1") {
		t.Errorf("RemoveOutput(out-1) 이 false 반환")
	}
	if len(f.Outputs()) != 0 {
		t.Errorf("RemoveOutput 후 Outputs 길이 = %d, want 0", len(f.Outputs()))
	}
}

// WithFlowInputPorts/WithFlowOutputPorts 옵션이 NewFlow 옵션 패턴으로 동작한다.
// 이름은 노드 레벨 WithInputPorts 와의 충돌을 피하기 위해 WithFlow 접두사를 쓴다.
func TestNewFlow_WithFlowPortsOptions(t *testing.T) {
	f := NewFlow("opt-flow",
		WithFlowInputPorts(Port{ID: "in-1", Name: "in1"}),
		WithFlowOutputPorts(
			Port{ID: "out-1", Name: "out1"},
			Port{ID: "out-2", Name: "out2"},
		),
	)

	if len(f.Inputs()) != 1 {
		t.Fatalf("WithFlowInputPorts: Inputs 길이 = %d, want 1", len(f.Inputs()))
	}
	if f.Inputs()[0].Direction != PortInput {
		t.Errorf("WithFlowInputPorts 방향 = %q, want %q", f.Inputs()[0].Direction, PortInput)
	}
	if len(f.Outputs()) != 2 {
		t.Fatalf("WithFlowOutputPorts: Outputs 길이 = %d, want 2", len(f.Outputs()))
	}
	if f.Outputs()[1].Name != "out2" {
		t.Errorf("WithFlowOutputPorts[1].Name = %q, want out2", f.Outputs()[1].Name)
	}
}

// REQ-SUBFLOW-A05/A07: JSON 직렬화/역직렬화 라운드트립에서 플로우 포트(id/name/방향)가
// 정의 최상위 inputs/outputs 로 보존되어야 한다.
func TestFlow_PortRoundTrip_JSON(t *testing.T) {
	f := NewFlow("roundtrip-flow",
		WithFlowInputPorts(Port{ID: "in-1", Name: "in1"}),
		WithFlowOutputPorts(Port{ID: "out-1", Name: "out1"}),
	)

	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("Marshal 실패: %v", err)
	}

	// 최상위에 inputs/outputs 키가 존재하는지 확인
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("raw unmarshal 실패: %v", err)
	}
	if _, ok := raw["inputs"]; !ok {
		t.Errorf("직렬화 JSON 최상위에 inputs 키가 없음")
	}
	if _, ok := raw["outputs"]; !ok {
		t.Errorf("직렬화 JSON 최상위에 outputs 키가 없음")
	}

	// 역직렬화 후 포트 보존 확인
	reloaded, err := FlowFromJSON(data)
	if err != nil {
		t.Fatalf("FlowFromJSON 실패: %v", err)
	}
	if len(reloaded.Inputs()) != 1 {
		t.Fatalf("재로드 Inputs 길이 = %d, want 1", len(reloaded.Inputs()))
	}
	if reloaded.Inputs()[0].ID != "in-1" || reloaded.Inputs()[0].Name != "in1" {
		t.Errorf("재로드 입력 포트 = %+v, want id=in-1 name=in1", reloaded.Inputs()[0])
	}
	if reloaded.Inputs()[0].Direction != PortInput {
		t.Errorf("재로드 입력 포트 방향 = %q, want %q", reloaded.Inputs()[0].Direction, PortInput)
	}
	if len(reloaded.Outputs()) != 1 {
		t.Fatalf("재로드 Outputs 길이 = %d, want 1", len(reloaded.Outputs()))
	}
	if reloaded.Outputs()[0].ID != "out-1" || reloaded.Outputs()[0].Direction != PortOutput {
		t.Errorf("재로드 출력 포트 = %+v, want id=out-1 direction=output", reloaded.Outputs()[0])
	}
}

// REQ-SUBFLOW-F02: inputs/outputs 가 없는 기존 정의는 빈 슬라이스로 로드되어야 한다(회귀 0).
func TestFlow_PortBackwardCompat_AbsentMeansEmpty(t *testing.T) {
	data := []byte(`{"name":"legacy-flow","nodes":[],"wires":[]}`)

	f, err := FlowFromJSON(data)
	if err != nil {
		t.Fatalf("FlowFromJSON 실패: %v", err)
	}
	if got := f.Inputs(); got == nil || len(got) != 0 {
		t.Errorf("inputs 누락 시 Inputs() = %+v, want 빈 슬라이스", got)
	}
	if got := f.Outputs(); got == nil || len(got) != 0 {
		t.Errorf("outputs 누락 시 Outputs() = %+v, want 빈 슬라이스", got)
	}
}
