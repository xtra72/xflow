package flow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// 테스트 헬퍼
// ---------------------------------------------------------------------------

// newTestFlow 는 2개의 노드와 1개의 와이어를 가진 테스트용 Flow를 생성한다.
func newTestFlow() Flow {
	nodeA := NewNodeDef("sensor", "bridge",
		WithErrorPort(),
		WithAgentRef(AgentRef{AgentName: "mqtt-broker", Direction: BridgeIn}),
		WithNodeMetadata("x", "100"),
	)
	nodeB := NewNodeDef("filter", "filter",
		WithNodeConfig("condition", "$.temp > 30"),
	)
	wire := NewWire(nodeA.ID, "out", nodeB.ID, "in")

	f := NewFlow("test-pipeline",
		WithDescription("Test flow"),
		WithFlowConfig(FlowConfig{
			TrackHistory:   true,
			MaxHistorySize: 50,
			ErrorHandling:  ErrorPropagate,
		}),
		WithFlowMetadata("author", "test"),
		WithNodes(nodeA, nodeB),
		WithWires(wire),
	)
	return f
}

// ---------------------------------------------------------------------------
// AC-25: JSON 라운드트립
// ---------------------------------------------------------------------------

func TestMarshalJSON(t *testing.T) {
	f := newTestFlow()

	df, ok := f.(*defaultFlow)
	if !ok {
		t.Fatal("Flow는 *defaultFlow 타입이어야 한다")
	}

	data, err := df.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 실패: %v", err)
	}

	// JSON이 유효한지 확인
	if !json.Valid(data) {
		t.Fatal("MarshalJSON 결과가 유효한 JSON이 아니다")
	}

	// 주요 필드가 JSON에 포함되어 있는지 확인
	s := string(data)
	if !strings.Contains(s, `"name":"test-pipeline"`) {
		t.Error("JSON에 name 필드가 포함되어야 한다")
	}
	if !strings.Contains(s, `"description":"Test flow"`) {
		t.Error("JSON에 description 필드가 포함되어야 한다")
	}
	if !strings.Contains(s, `"state":"stored"`) {
		t.Error("JSON에 state 필드가 포함되어야 한다")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	original := newTestFlow()

	df, ok := original.(*defaultFlow)
	if !ok {
		t.Fatal("Flow는 *defaultFlow 타입이어야 한다")
	}

	// 직렬화
	data, err := df.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 실패: %v", err)
	}

	// 역직렬화
	restored, err := FlowFromJSON(data)
	if err != nil {
		t.Fatalf("FlowFromJSON 실패: %v", err)
	}

	// ID, Name, Description 일치 확인
	if restored.ID() != original.ID() {
		t.Errorf("ID 불일치: got %q, want %q", restored.ID(), original.ID())
	}
	if restored.Name() != original.Name() {
		t.Errorf("Name 불일치: got %q, want %q", restored.Name(), original.Name())
	}
	if restored.Description() != original.Description() {
		t.Errorf("Description 불일치: got %q, want %q", restored.Description(), original.Description())
	}

	// State 일치 확인
	if restored.State() != original.State() {
		t.Errorf("State 불일치: got %q, want %q", restored.State(), original.State())
	}

	// Nodes 개수 일치 확인
	if len(restored.Nodes()) != len(original.Nodes()) {
		t.Fatalf("Nodes 개수 불일치: got %d, want %d", len(restored.Nodes()), len(original.Nodes()))
	}

	// NodeDef 필드 확인 (첫 번째 노드)
	origNode := original.Nodes()[0]
	restoredNode := restored.Nodes()[0]
	if restoredNode.ID != origNode.ID {
		t.Errorf("Node[0].ID 불일치: got %q, want %q", restoredNode.ID, origNode.ID)
	}
	if restoredNode.Name != origNode.Name {
		t.Errorf("Node[0].Name 불일치: got %q, want %q", restoredNode.Name, origNode.Name)
	}
	if restoredNode.Type != origNode.Type {
		t.Errorf("Node[0].Type 불일치: got %q, want %q", restoredNode.Type, origNode.Type)
	}

	// 두 번째 노드의 Config 확인
	origNode2 := original.Nodes()[1]
	restoredNode2 := restored.Nodes()[1]
	if restoredNode2.Config["condition"] != origNode2.Config["condition"] {
		t.Errorf("Node[1].Config[condition] 불일치: got %v, want %v",
			restoredNode2.Config["condition"], origNode2.Config["condition"])
	}

	// Wires 개수 일치 확인
	if len(restored.Wires()) != len(original.Wires()) {
		t.Fatalf("Wires 개수 불일치: got %d, want %d", len(restored.Wires()), len(original.Wires()))
	}

	// Wire source/target 일치 확인
	origWire := original.Wires()[0]
	restoredWire := restored.Wires()[0]
	if restoredWire.SourceNodeID != origWire.SourceNodeID {
		t.Errorf("Wire.SourceNodeID 불일치: got %q, want %q",
			restoredWire.SourceNodeID, origWire.SourceNodeID)
	}
	if restoredWire.SourcePort != origWire.SourcePort {
		t.Errorf("Wire.SourcePort 불일치: got %q, want %q",
			restoredWire.SourcePort, origWire.SourcePort)
	}
	if restoredWire.TargetNodeID != origWire.TargetNodeID {
		t.Errorf("Wire.TargetNodeID 불일치: got %q, want %q",
			restoredWire.TargetNodeID, origWire.TargetNodeID)
	}
	if restoredWire.TargetPort != origWire.TargetPort {
		t.Errorf("Wire.TargetPort 불일치: got %q, want %q",
			restoredWire.TargetPort, origWire.TargetPort)
	}

	// Config 일치 확인
	if restored.Config().TrackHistory != original.Config().TrackHistory {
		t.Errorf("Config.TrackHistory 불일치: got %v, want %v",
			restored.Config().TrackHistory, original.Config().TrackHistory)
	}
	if restored.Config().MaxHistorySize != original.Config().MaxHistorySize {
		t.Errorf("Config.MaxHistorySize 불일치: got %d, want %d",
			restored.Config().MaxHistorySize, original.Config().MaxHistorySize)
	}
	if restored.Config().ErrorHandling != original.Config().ErrorHandling {
		t.Errorf("Config.ErrorHandling 불일치: got %q, want %q",
			restored.Config().ErrorHandling, original.Config().ErrorHandling)
	}

	// Metadata 일치 확인
	if restored.Metadata()["author"] != original.Metadata()["author"] {
		t.Errorf("Metadata[author] 불일치: got %q, want %q",
			restored.Metadata()["author"], original.Metadata()["author"])
	}

	// CreatedAt/UpdatedAt 일치 확인
	if !restored.CreatedAt().Equal(original.CreatedAt()) {
		t.Errorf("CreatedAt 불일치: got %v, want %v",
			restored.CreatedAt(), original.CreatedAt())
	}
	if !restored.UpdatedAt().Equal(original.UpdatedAt()) {
		t.Errorf("UpdatedAt 불일치: got %v, want %v",
			restored.UpdatedAt(), original.UpdatedAt())
	}
}

// ---------------------------------------------------------------------------
// AC-26: YAML 라운드트립
// ---------------------------------------------------------------------------

func TestYAMLRoundTrip(t *testing.T) {
	original := newTestFlow()

	// YAML 직렬화
	yamlData, err := FlowToYAML(original)
	if err != nil {
		t.Fatalf("FlowToYAML 실패: %v", err)
	}

	// YAML이 비어있지 않은지 확인
	if len(yamlData) == 0 {
		t.Fatal("FlowToYAML 결과가 비어있다")
	}

	// YAML 역직렬화
	restored, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	// ID, Name, Description 일치 확인
	if restored.ID() != original.ID() {
		t.Errorf("ID 불일치: got %q, want %q", restored.ID(), original.ID())
	}
	if restored.Name() != original.Name() {
		t.Errorf("Name 불일치: got %q, want %q", restored.Name(), original.Name())
	}
	if restored.Description() != original.Description() {
		t.Errorf("Description 불일치: got %q, want %q", restored.Description(), original.Description())
	}

	// State 일치 확인
	if restored.State() != original.State() {
		t.Errorf("State 불일치: got %q, want %q", restored.State(), original.State())
	}

	// Nodes 개수 일치 확인
	if len(restored.Nodes()) != len(original.Nodes()) {
		t.Fatalf("Nodes 개수 불일치: got %d, want %d",
			len(restored.Nodes()), len(original.Nodes()))
	}

	// NodeDef 필드 확인
	origNode := original.Nodes()[0]
	restoredNode := restored.Nodes()[0]
	if restoredNode.ID != origNode.ID {
		t.Errorf("Node[0].ID 불일치: got %q, want %q", restoredNode.ID, origNode.ID)
	}
	if restoredNode.Name != origNode.Name {
		t.Errorf("Node[0].Name 불일치: got %q, want %q", restoredNode.Name, origNode.Name)
	}
	if restoredNode.Type != origNode.Type {
		t.Errorf("Node[0].Type 불일치: got %q, want %q", restoredNode.Type, origNode.Type)
	}

	// Wires 개수 일치 확인
	if len(restored.Wires()) != len(original.Wires()) {
		t.Fatalf("Wires 개수 불일치: got %d, want %d",
			len(restored.Wires()), len(original.Wires()))
	}

	// Wire source/target 일치 확인
	origWire := original.Wires()[0]
	restoredWire := restored.Wires()[0]
	if restoredWire.SourceNodeID != origWire.SourceNodeID {
		t.Errorf("Wire.SourceNodeID 불일치: got %q, want %q",
			restoredWire.SourceNodeID, origWire.SourceNodeID)
	}
	if restoredWire.TargetNodeID != origWire.TargetNodeID {
		t.Errorf("Wire.TargetNodeID 불일치: got %q, want %q",
			restoredWire.TargetNodeID, origWire.TargetNodeID)
	}

	// Config 일치 확인
	if restored.Config().TrackHistory != original.Config().TrackHistory {
		t.Errorf("Config.TrackHistory 불일치: got %v, want %v",
			restored.Config().TrackHistory, original.Config().TrackHistory)
	}
	if restored.Config().MaxHistorySize != original.Config().MaxHistorySize {
		t.Errorf("Config.MaxHistorySize 불일치: got %d, want %d",
			restored.Config().MaxHistorySize, original.Config().MaxHistorySize)
	}

	// Metadata 일치 확인
	if restored.Metadata()["author"] != original.Metadata()["author"] {
		t.Errorf("Metadata[author] 불일치: got %q, want %q",
			restored.Metadata()["author"], original.Metadata()["author"])
	}
}

// ---------------------------------------------------------------------------
// AC-27: 에러 케이스
// ---------------------------------------------------------------------------

func TestFlowFromJSON_InvalidJSON(t *testing.T) {
	_, err := FlowFromJSON([]byte(`{invalid json`))
	if err == nil {
		t.Error("유효하지 않은 JSON에 대해 에러를 반환해야 한다")
	}
}

func TestFlowFromJSON_MissingName(t *testing.T) {
	data := []byte(`{"id":"abc","name":"","state":"stored"}`)
	_, err := FlowFromJSON(data)
	if err != ErrFlowNameRequired {
		t.Errorf("이름이 없으면 ErrFlowNameRequired를 반환해야 한다: got %v", err)
	}
}

func TestFlowFromJSON_MissingID_AutoGenerate(t *testing.T) {
	data := []byte(`{"name":"auto-id-test","state":"stored","config":{"track_history":false,"max_history_size":0,"error_handling":"propagate"},"nodes":[],"wires":[]}`)
	f, err := FlowFromJSON(data)
	if err != nil {
		t.Fatalf("FlowFromJSON 실패: %v", err)
	}

	// ID가 자동 생성되었는지 확인
	if f.ID() == "" {
		t.Error("ID가 비어있으면 안 된다")
	}

	// 유효한 UUID인지 확인
	_, err = uuid.Parse(f.ID())
	if err != nil {
		t.Errorf("자동 생성된 ID가 유효한 UUID가 아니다: %q", f.ID())
	}
}

func TestFlowFromJSON_EmptyState_DefaultsToStored(t *testing.T) {
	data := []byte(`{"name":"state-test","state":"","config":{"track_history":false,"max_history_size":0,"error_handling":"propagate"},"nodes":[],"wires":[]}`)
	f, err := FlowFromJSON(data)
	if err != nil {
		t.Fatalf("FlowFromJSON 실패: %v", err)
	}

	if f.State() != FlowStored {
		t.Errorf("빈 상태는 FlowStored로 기본 설정되어야 한다: got %q", f.State())
	}
}

func TestFlowFromYAML_InvalidYAML(t *testing.T) {
	_, err := FlowFromYAML([]byte(`invalid: yaml: [[[`))
	if err == nil {
		t.Error("유효하지 않은 YAML에 대해 에러를 반환해야 한다")
	}
}

func TestFlowFromYAML_MissingName(t *testing.T) {
	yamlData := []byte("id: abc\nname: \"\"\nstate: stored\n")
	_, err := FlowFromYAML(yamlData)
	if err != ErrFlowNameRequired {
		t.Errorf("이름이 없으면 ErrFlowNameRequired를 반환해야 한다: got %v", err)
	}
}

// ---------------------------------------------------------------------------
// AC-28: 파일 로딩
// ---------------------------------------------------------------------------

func TestLoadFlowFromFile_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.json")

	original := newTestFlow()
	if err := SaveFlowToFile(original, path); err != nil {
		t.Fatalf("SaveFlowToFile 실패: %v", err)
	}

	loaded, err := LoadFlowFromFile(path)
	if err != nil {
		t.Fatalf("LoadFlowFromFile 실패: %v", err)
	}

	if loaded.Name() != original.Name() {
		t.Errorf("Name 불일치: got %q, want %q", loaded.Name(), original.Name())
	}
}

func TestLoadFlowFromFile_YAML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.yaml")

	original := newTestFlow()
	if err := SaveFlowToFile(original, path); err != nil {
		t.Fatalf("SaveFlowToFile 실패: %v", err)
	}

	loaded, err := LoadFlowFromFile(path)
	if err != nil {
		t.Fatalf("LoadFlowFromFile 실패: %v", err)
	}

	if loaded.Name() != original.Name() {
		t.Errorf("Name 불일치: got %q, want %q", loaded.Name(), original.Name())
	}
}

func TestLoadFlowFromFile_YML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.yml")

	original := newTestFlow()
	if err := SaveFlowToFile(original, path); err != nil {
		t.Fatalf("SaveFlowToFile 실패: %v", err)
	}

	loaded, err := LoadFlowFromFile(path)
	if err != nil {
		t.Fatalf("LoadFlowFromFile 실패: %v", err)
	}

	if loaded.Name() != original.Name() {
		t.Errorf("Name 불일치: got %q, want %q", loaded.Name(), original.Name())
	}
}

func TestLoadFlowFromFile_UnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.xml")

	// 더미 파일 생성
	if err := os.WriteFile(path, []byte("<xml/>"), 0644); err != nil {
		t.Fatalf("파일 생성 실패: %v", err)
	}

	_, err := LoadFlowFromFile(path)
	if err != ErrUnsupportedFormat {
		t.Errorf("지원하지 않는 확장자에 대해 ErrUnsupportedFormat을 반환해야 한다: got %v", err)
	}
}

func TestLoadFlowFromFile_NotFound(t *testing.T) {
	_, err := LoadFlowFromFile("/tmp/nonexistent-xflow-file.json")
	if err == nil {
		t.Error("존재하지 않는 파일에 대해 에러를 반환해야 한다")
	}
	if !os.IsNotExist(err) {
		t.Errorf("os.ErrNotExist로 래핑되어야 한다: got %v", err)
	}
}

func TestSaveFlowToFile_UnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.xml")

	f := newTestFlow()
	err := SaveFlowToFile(f, path)
	if err != ErrUnsupportedFormat {
		t.Errorf("지원하지 않는 확장자에 대해 ErrUnsupportedFormat을 반환해야 한다: got %v", err)
	}
}

// ---------------------------------------------------------------------------
// AC-29: 파일 저장/로드 라운드트립
// ---------------------------------------------------------------------------

func TestSaveAndLoadRoundTrip_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "roundtrip.json")

	original := newTestFlow()

	// 저장
	if err := SaveFlowToFile(original, path); err != nil {
		t.Fatalf("SaveFlowToFile 실패: %v", err)
	}

	// 저장된 파일이 존재하는지 확인
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("저장된 파일이 존재해야 한다")
	}

	// 로드
	loaded, err := LoadFlowFromFile(path)
	if err != nil {
		t.Fatalf("LoadFlowFromFile 실패: %v", err)
	}

	// 원본과 일치하는지 확인
	if loaded.ID() != original.ID() {
		t.Errorf("ID 불일치: got %q, want %q", loaded.ID(), original.ID())
	}
	if loaded.Name() != original.Name() {
		t.Errorf("Name 불일치: got %q, want %q", loaded.Name(), original.Name())
	}
	if loaded.Description() != original.Description() {
		t.Errorf("Description 불일치: got %q, want %q",
			loaded.Description(), original.Description())
	}
	if len(loaded.Nodes()) != len(original.Nodes()) {
		t.Errorf("Nodes 개수 불일치: got %d, want %d",
			len(loaded.Nodes()), len(original.Nodes()))
	}
	if len(loaded.Wires()) != len(original.Wires()) {
		t.Errorf("Wires 개수 불일치: got %d, want %d",
			len(loaded.Wires()), len(original.Wires()))
	}
}

func TestSaveAndLoadRoundTrip_YAML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "roundtrip.yaml")

	original := newTestFlow()

	// 저장
	if err := SaveFlowToFile(original, path); err != nil {
		t.Fatalf("SaveFlowToFile 실패: %v", err)
	}

	// 로드
	loaded, err := LoadFlowFromFile(path)
	if err != nil {
		t.Fatalf("LoadFlowFromFile 실패: %v", err)
	}

	// 원본과 일치하는지 확인
	if loaded.ID() != original.ID() {
		t.Errorf("ID 불일치: got %q, want %q", loaded.ID(), original.ID())
	}
	if loaded.Name() != original.Name() {
		t.Errorf("Name 불일치: got %q, want %q", loaded.Name(), original.Name())
	}
	if loaded.Description() != original.Description() {
		t.Errorf("Description 불일치: got %q, want %q",
			loaded.Description(), original.Description())
	}
	if len(loaded.Nodes()) != len(original.Nodes()) {
		t.Errorf("Nodes 개수 불일치: got %d, want %d",
			len(loaded.Nodes()), len(original.Nodes()))
	}
	if len(loaded.Wires()) != len(original.Wires()) {
		t.Errorf("Wires 개수 불일치: got %d, want %d",
			len(loaded.Wires()), len(original.Wires()))
	}
}

// ---------------------------------------------------------------------------
// JSON 직렬화 세부 확인
// ---------------------------------------------------------------------------

func TestMarshalJSON_PrettyOutput(t *testing.T) {
	f := newTestFlow()

	df, ok := f.(*defaultFlow)
	if !ok {
		t.Fatal("Flow는 *defaultFlow 타입이어야 한다")
	}

	data, err := df.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 실패: %v", err)
	}

	// JSON에 올바른 필드 이름이 포함되어 있는지 확인
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("JSON 파싱 실패: %v", err)
	}

	requiredFields := []string{"id", "name", "state", "config", "nodes", "wires", "created_at", "updated_at"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("JSON에 필수 필드 %q가 없다", field)
		}
	}
}

func TestSaveFlowToFile_JSON_PrettyPrinted(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "pretty.json")

	f := newTestFlow()
	if err := SaveFlowToFile(f, path); err != nil {
		t.Fatalf("SaveFlowToFile 실패: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("파일 읽기 실패: %v", err)
	}

	// 들여쓰기가 있는지 확인 (pretty-printed)
	if !strings.Contains(string(data), "\n") {
		t.Error("JSON 파일은 pretty-printed 형식이어야 한다")
	}
}

// ---------------------------------------------------------------------------
// 노드 ErrorPort, AgentRef 라운드트립 확인
// ---------------------------------------------------------------------------

func TestJSONRoundTrip_NodeErrorPort(t *testing.T) {
	original := newTestFlow()

	df := original.(*defaultFlow)
	data, err := df.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 실패: %v", err)
	}

	restored, err := FlowFromJSON(data)
	if err != nil {
		t.Fatalf("FlowFromJSON 실패: %v", err)
	}

	// 첫 번째 노드의 ErrorPort 확인
	origNode := original.Nodes()[0]
	restoredNode := restored.Nodes()[0]

	if origNode.ErrorPort == nil {
		t.Fatal("원본 노드에 ErrorPort가 있어야 한다")
	}
	if restoredNode.ErrorPort == nil {
		t.Fatal("복원된 노드에 ErrorPort가 있어야 한다")
	}
	if restoredNode.ErrorPort.Name != origNode.ErrorPort.Name {
		t.Errorf("ErrorPort.Name 불일치: got %q, want %q",
			restoredNode.ErrorPort.Name, origNode.ErrorPort.Name)
	}
}

func TestJSONRoundTrip_NodeAgentRef(t *testing.T) {
	original := newTestFlow()

	df := original.(*defaultFlow)
	data, err := df.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 실패: %v", err)
	}

	restored, err := FlowFromJSON(data)
	if err != nil {
		t.Fatalf("FlowFromJSON 실패: %v", err)
	}

	// 첫 번째 노드의 AgentRef 확인
	origNode := original.Nodes()[0]
	restoredNode := restored.Nodes()[0]

	if origNode.AgentRef == nil {
		t.Fatal("원본 노드에 AgentRef가 있어야 한다")
	}
	if restoredNode.AgentRef == nil {
		t.Fatal("복원된 노드에 AgentRef가 있어야 한다")
	}
	if restoredNode.AgentRef.AgentName != origNode.AgentRef.AgentName {
		t.Errorf("AgentRef.AgentName 불일치: got %q, want %q",
			restoredNode.AgentRef.AgentName, origNode.AgentRef.AgentName)
	}
	if restoredNode.AgentRef.Direction != origNode.AgentRef.Direction {
		t.Errorf("AgentRef.Direction 불일치: got %q, want %q",
			restoredNode.AgentRef.Direction, origNode.AgentRef.Direction)
	}
}
