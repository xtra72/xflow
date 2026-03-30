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
// 노드 Errors, AgentRef 라운드트립 확인
// ---------------------------------------------------------------------------

func TestJSONRoundTrip_NodeErrors(t *testing.T) {
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

	// 첫 번째 노드의 Errors 확인
	origNode := original.Nodes()[0]
	restoredNode := restored.Nodes()[0]

	if len(origNode.Errors) == 0 {
		t.Fatal("원본 노드에 Errors가 있어야 한다")
	}
	if len(restoredNode.Errors) == 0 {
		t.Fatal("복원된 노드에 Errors가 있어야 한다")
	}
	if restoredNode.Errors[0].Name != origNode.Errors[0].Name {
		t.Errorf("Errors[0].Name 불일치: got %q, want %q",
			restoredNode.Errors[0].Name, origNode.Errors[0].Name)
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

// ---------------------------------------------------------------------------
// 노드/포트 ID 자동 생성
// ---------------------------------------------------------------------------

func TestNormalizeNodeDefaults_NodeIDIsUUID(t *testing.T) {
	nodes := []NodeDef{
		{Name: "my-node", Type: "transform"},
	}
	nameToID := normalizeNodeDefaults(nodes)

	// ID가 UUID 형식이어야 한다
	if _, err := uuid.Parse(nodes[0].ID); err != nil {
		t.Errorf("ID가 UUID 형식이어야 한다: got %q, err: %v", nodes[0].ID, err)
	}
	// name→ID 매핑이 반환되어야 한다
	if nameToID["my-node"] != nodes[0].ID {
		t.Errorf("nameToID 매핑 불일치: got %q, want %q", nameToID["my-node"], nodes[0].ID)
	}
}

func TestNormalizeNodeDefaults_PortIDFromNodeAndPortName(t *testing.T) {
	nodes := []NodeDef{
		{
			Name: "extractor",
			Type: "transform",
			Inputs: []Port{
				{Name: "in", Direction: PortInput},
			},
			Outputs: []Port{
				{Name: "out", Direction: PortOutput},
			},
			Errors: []Port{{Name: "error", Direction: PortError}},
		},
	}
	normalizeNodeDefaults(nodes)

	if nodes[0].Inputs[0].ID != "extractor.in" {
		t.Errorf("입력 포트 ID: got %q, want %q", nodes[0].Inputs[0].ID, "extractor.in")
	}
	if nodes[0].Outputs[0].ID != "extractor.out" {
		t.Errorf("출력 포트 ID: got %q, want %q", nodes[0].Outputs[0].ID, "extractor.out")
	}
	if nodes[0].Errors[0].ID != "extractor.error" {
		t.Errorf("에러 포트 ID: got %q, want %q", nodes[0].Errors[0].ID, "extractor.error")
	}
}

func TestNormalizeNodeDefaults_PreserveExistingIDs(t *testing.T) {
	nodes := []NodeDef{
		{
			ID:   "custom-id",
			Name: "my-node",
			Type: "filter",
			Inputs: []Port{
				{ID: "custom-port-id", Name: "in", Direction: PortInput},
			},
			Outputs: []Port{
				{Name: "out", Direction: PortOutput},
			},
		},
	}
	normalizeNodeDefaults(nodes)

	if nodes[0].ID != "custom-id" {
		t.Errorf("기존 노드 ID가 유지되어야 한다: got %q, want %q", nodes[0].ID, "custom-id")
	}
	if nodes[0].Inputs[0].ID != "custom-port-id" {
		t.Errorf("기존 포트 ID가 유지되어야 한다: got %q, want %q", nodes[0].Inputs[0].ID, "custom-port-id")
	}
	// ID가 없는 포트는 자동 생성
	if nodes[0].Outputs[0].ID != "my-node.out" {
		t.Errorf("빈 포트 ID가 자동 생성되어야 한다: got %q, want %q", nodes[0].Outputs[0].ID, "my-node.out")
	}
}

func TestFlowFromYAML_NodeIDAutoGeneration(t *testing.T) {
	yamlData := []byte(`
name: "test-flow"
nodes:
  - name: "sensor-receiver"
    type: "bridge"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
    errors:
      - name: "error"
        direction: "error"
  - id: "explicit-id"
    name: "field-extractor"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - id: "explicit-port-id"
        name: "out"
        direction: "output"
wires:
  - source_node_id: "sensor-receiver"
    source_port: "out"
    target_node_id: "explicit-id"
    target_port: "in"
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	nodes := f.Nodes()
	if len(nodes) != 2 {
		t.Fatalf("노드 개수: got %d, want 2", len(nodes))
	}

	// 첫 번째 노드: ID가 UUID로 자동 생성
	n0 := nodes[0]
	if _, err := uuid.Parse(n0.ID); err != nil {
		t.Errorf("노드 ID가 UUID 형식이어야 한다: got %q, err: %v", n0.ID, err)
	}
	if n0.Inputs[0].ID != "sensor-receiver.in" {
		t.Errorf("입력 포트 ID 자동 생성: got %q, want %q", n0.Inputs[0].ID, "sensor-receiver.in")
	}
	if n0.Outputs[0].ID != "sensor-receiver.out" {
		t.Errorf("출력 포트 ID 자동 생성: got %q, want %q", n0.Outputs[0].ID, "sensor-receiver.out")
	}
	if len(n0.Errors) == 0 {
		t.Fatal("에러 포트가 있어야 한다")
	}
	if n0.Errors[0].ID != "sensor-receiver.error" {
		t.Errorf("에러 포트 ID 자동 생성: got %q, want %q", n0.Errors[0].ID, "sensor-receiver.error")
	}

	// 두 번째 노드: 기존 ID 유지
	n1 := nodes[1]
	if n1.ID != "explicit-id" {
		t.Errorf("명시적 노드 ID 유지: got %q, want %q", n1.ID, "explicit-id")
	}
	if n1.Inputs[0].ID != "field-extractor.in" {
		t.Errorf("포트 ID 자동 생성 (명시적 노드): got %q, want %q", n1.Inputs[0].ID, "field-extractor.in")
	}
	if n1.Outputs[0].ID != "explicit-port-id" {
		t.Errorf("명시적 포트 ID 유지: got %q, want %q", n1.Outputs[0].ID, "explicit-port-id")
	}

	// 와이어의 SourceNodeID가 노드의 UUID로 해석되어야 한다
	wires := f.Wires()
	if len(wires) != 1 {
		t.Fatalf("와이어 개수: got %d, want 1", len(wires))
	}
	if wires[0].SourceNodeID != n0.ID {
		t.Errorf("와이어 SourceNodeID가 노드 UUID로 해석되어야 한다: got %q, want %q", wires[0].SourceNodeID, n0.ID)
	}
	if wires[0].TargetNodeID != "explicit-id" {
		t.Errorf("와이어 TargetNodeID는 명시적 ID 유지: got %q, want %q", wires[0].TargetNodeID, "explicit-id")
	}
}

// ---------------------------------------------------------------------------
// Wire source/target 단축 문법
// ---------------------------------------------------------------------------

func TestFlowFromYAML_WireShorthand(t *testing.T) {
	yamlData := []byte(`
name: "test-flow"
nodes:
  - name: "sensor"
    type: "bridge"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
  - name: "filter"
    type: "filter"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
wires:
  - source: "sensor:out"
    target: "filter:in"
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	nodes := f.Nodes()
	wires := f.Wires()
	if len(wires) != 1 {
		t.Fatalf("와이어 개수: got %d, want 1", len(wires))
	}

	w := wires[0]
	// 와이어의 SourceNodeID/TargetNodeID는 노드 UUID로 해석되어야 한다
	if w.SourceNodeID != nodes[0].ID {
		t.Errorf("SourceNodeID가 노드 UUID와 일치해야 한다: got %q, want %q", w.SourceNodeID, nodes[0].ID)
	}
	if w.SourcePort != "out" {
		t.Errorf("SourcePort: got %q, want %q", w.SourcePort, "out")
	}
	if w.TargetNodeID != nodes[1].ID {
		t.Errorf("TargetNodeID가 노드 UUID와 일치해야 한다: got %q, want %q", w.TargetNodeID, nodes[1].ID)
	}
	if w.TargetPort != "in" {
		t.Errorf("TargetPort: got %q, want %q", w.TargetPort, "in")
	}
}

func TestFlowFromYAML_WireShorthandPreserveExplicit(t *testing.T) {
	yamlData := []byte(`
name: "test-flow"
nodes:
  - name: "a"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
  - name: "b"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
wires:
  - source_node_id: "a"
    source_port: "out"
    source: "ignored:ignored"
    target: "b:in"
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	nodes := f.Nodes()
	w := f.Wires()[0]

	// source_node_id가 명시적이지만, "a"는 노드 이름이므로 UUID로 해석된다
	if w.SourceNodeID != nodes[0].ID {
		t.Errorf("source_node_id가 노드 UUID로 해석되어야 한다: got %q, want %q", w.SourceNodeID, nodes[0].ID)
	}
	if w.SourcePort != "out" {
		t.Errorf("명시적 source_port 유지: got %q, want %q", w.SourcePort, "out")
	}

	// target은 단축 문법 적용 → 노드 UUID로 해석
	if w.TargetNodeID != nodes[1].ID {
		t.Errorf("TargetNodeID가 노드 UUID로 해석되어야 한다: got %q, want %q", w.TargetNodeID, nodes[1].ID)
	}
	if w.TargetPort != "in" {
		t.Errorf("TargetPort: got %q, want %q", w.TargetPort, "in")
	}
}

func TestFlowFromJSON_WireShorthand(t *testing.T) {
	jsonData := []byte(`{
		"name": "test-flow",
		"nodes": [
			{"name": "a", "type": "transform", "inputs": [{"name": "in", "direction": "input"}], "outputs": [{"name": "out", "direction": "output"}]},
			{"name": "b", "type": "transform", "inputs": [{"name": "in", "direction": "input"}], "outputs": [{"name": "out", "direction": "output"}]}
		],
		"wires": [
			{"source": "a:out", "target": "b:in"}
		]
	}`)

	f, err := FlowFromJSON(jsonData)
	if err != nil {
		t.Fatalf("FlowFromJSON 실패: %v", err)
	}

	nodes := f.Nodes()
	w := f.Wires()[0]
	// 노드 이름 기반 참조가 UUID로 해석되어야 한다
	if w.SourceNodeID != nodes[0].ID {
		t.Errorf("SourceNodeID가 노드 UUID와 일치해야 한다: got %q, want %q", w.SourceNodeID, nodes[0].ID)
	}
	if w.SourcePort != "out" {
		t.Errorf("SourcePort: got %q, want %q", w.SourcePort, "out")
	}
	if w.TargetNodeID != nodes[1].ID {
		t.Errorf("TargetNodeID가 노드 UUID와 일치해야 한다: got %q, want %q", w.TargetNodeID, nodes[1].ID)
	}
	if w.TargetPort != "in" {
		t.Errorf("TargetPort: got %q, want %q", w.TargetPort, "in")
	}
}

func TestNormalizeWireDefaults_ModeDefaultBypass(t *testing.T) {
	yamlData := []byte(`
name: test-flow
nodes:
  - name: "a"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
  - name: "b"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
wires:
  - source: "a:out"
    target: "b:in"
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	w := f.Wires()[0]
	if w.Mode != WireBypass {
		t.Errorf("Mode: got %q, want %q", w.Mode, WireBypass)
	}
	if w.BufferSize != 0 {
		t.Errorf("BufferSize: got %d, want 0", w.BufferSize)
	}
	if w.TTL != 0 {
		t.Errorf("TTL: got %v, want 0", w.TTL)
	}
}

func TestNormalizeWireDefaults_PreserveExplicitMode(t *testing.T) {
	yamlData := []byte(`
name: test-flow
nodes:
  - name: "a"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
  - name: "b"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
wires:
  - source: "a:out"
    target: "b:in"
    mode: "buffer"
    buffer_size: 50
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	w := f.Wires()[0]
	if w.Mode != WireBuffer {
		t.Errorf("Mode: got %q, want %q", w.Mode, WireBuffer)
	}
	if w.BufferSize != 50 {
		t.Errorf("BufferSize: got %d, want 50", w.BufferSize)
	}
}

func TestNormalizeWireDefaults_IDAutoGeneration(t *testing.T) {
	yamlData := []byte(`
name: my-flow
nodes:
  - name: "a"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
  - name: "b"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
  - name: "c"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
wires:
  - source: "a:out"
    target: "b:in"
  - id: "custom-wire"
    source: "b:out"
    target: "c:in"
  - source: "a:out"
    target: "c:in"
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	wires := f.Wires()
	if len(wires) != 3 {
		t.Fatalf("Wires 수: got %d, want 3", len(wires))
	}

	// ID가 없는 와이어 → <flow_name>.wire-<index>
	if wires[0].ID != "my-flow.wire-0" {
		t.Errorf("wires[0].ID: got %q, want %q", wires[0].ID, "my-flow.wire-0")
	}

	// 명시적 ID는 보존
	if wires[1].ID != "custom-wire" {
		t.Errorf("wires[1].ID: got %q, want %q", wires[1].ID, "custom-wire")
	}

	// 세 번째 와이어도 자동 생성
	if wires[2].ID != "my-flow.wire-2" {
		t.Errorf("wires[2].ID: got %q, want %q", wires[2].ID, "my-flow.wire-2")
	}
}

// ---------------------------------------------------------------------------
// 포트 단축 문법 (문자열 배열)
// ---------------------------------------------------------------------------

func TestFlowFromYAML_PortShorthand(t *testing.T) {
	yamlData := []byte(`
name: "test-flow"
nodes:
  - name: "processor"
    type: "transform"
    inputs:
      - "in"
    outputs:
      - "out"
    errors:
      - "error"
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	n := f.Nodes()[0]

	if len(n.Inputs) != 1 {
		t.Fatalf("Inputs 개수: got %d, want 1", len(n.Inputs))
	}
	if n.Inputs[0].Name != "in" {
		t.Errorf("Inputs[0].Name: got %q, want %q", n.Inputs[0].Name, "in")
	}
	if n.Inputs[0].Direction != PortInput {
		t.Errorf("Inputs[0].Direction: got %q, want %q", n.Inputs[0].Direction, PortInput)
	}

	if len(n.Outputs) != 1 {
		t.Fatalf("Outputs 개수: got %d, want 1", len(n.Outputs))
	}
	if n.Outputs[0].Name != "out" {
		t.Errorf("Outputs[0].Name: got %q, want %q", n.Outputs[0].Name, "out")
	}
	if n.Outputs[0].Direction != PortOutput {
		t.Errorf("Outputs[0].Direction: got %q, want %q", n.Outputs[0].Direction, PortOutput)
	}

	if len(n.Errors) != 1 {
		t.Fatalf("Errors 개수: got %d, want 1", len(n.Errors))
	}
	if n.Errors[0].Name != "error" {
		t.Errorf("Errors[0].Name: got %q, want %q", n.Errors[0].Name, "error")
	}
	if n.Errors[0].Direction != PortError {
		t.Errorf("Errors[0].Direction: got %q, want %q", n.Errors[0].Direction, PortError)
	}
}

func TestFlowFromYAML_PortShorthandMultiple(t *testing.T) {
	yamlData := []byte(`
name: "test-flow"
nodes:
  - name: "merger"
    type: "transform"
    inputs:
      - "in1"
      - "in2"
      - "in3"
    outputs:
      - "out"
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	n := f.Nodes()[0]
	if len(n.Inputs) != 3 {
		t.Fatalf("Inputs 개수: got %d, want 3", len(n.Inputs))
	}

	expectedNames := []string{"in1", "in2", "in3"}
	for i, expected := range expectedNames {
		if n.Inputs[i].Name != expected {
			t.Errorf("Inputs[%d].Name: got %q, want %q", i, n.Inputs[i].Name, expected)
		}
		if n.Inputs[i].ID != "merger."+expected {
			t.Errorf("Inputs[%d].ID: got %q, want %q", i, n.Inputs[i].ID, "merger."+expected)
		}
	}
}

func TestFlowFromYAML_PortShorthandMixed(t *testing.T) {
	yamlData := []byte(`
name: "test-flow"
nodes:
  - name: "mixer"
    type: "transform"
    inputs:
      - "simple-in"
      - name: "detailed-in"
        id: "custom-port-id"
    outputs:
      - "out"
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	n := f.Nodes()[0]
	if len(n.Inputs) != 2 {
		t.Fatalf("Inputs 개수: got %d, want 2", len(n.Inputs))
	}

	// 문자열 단축 포트
	if n.Inputs[0].Name != "simple-in" {
		t.Errorf("Inputs[0].Name: got %q, want %q", n.Inputs[0].Name, "simple-in")
	}
	if n.Inputs[0].ID != "mixer.simple-in" {
		t.Errorf("Inputs[0].ID: got %q, want %q", n.Inputs[0].ID, "mixer.simple-in")
	}

	// 맵 형식 포트 (기존 형식)
	if n.Inputs[1].Name != "detailed-in" {
		t.Errorf("Inputs[1].Name: got %q, want %q", n.Inputs[1].Name, "detailed-in")
	}
	if n.Inputs[1].ID != "custom-port-id" {
		t.Errorf("Inputs[1].ID: got %q, want %q", n.Inputs[1].ID, "custom-port-id")
	}
}

func TestFlowFromJSON_PortShorthand(t *testing.T) {
	jsonData := []byte(`{
		"name": "test-flow",
		"nodes": [
			{
				"name": "node-a",
				"type": "transform",
				"inputs": ["in"],
				"outputs": ["out"],
				"errors": ["error"]
			}
		],
		"wires": []
	}`)

	f, err := FlowFromJSON(jsonData)
	if err != nil {
		t.Fatalf("FlowFromJSON 실패: %v", err)
	}

	n := f.Nodes()[0]
	if n.Inputs[0].Name != "in" {
		t.Errorf("Inputs[0].Name: got %q, want %q", n.Inputs[0].Name, "in")
	}
	if n.Outputs[0].Name != "out" {
		t.Errorf("Outputs[0].Name: got %q, want %q", n.Outputs[0].Name, "out")
	}
	if n.Errors[0].Name != "error" {
		t.Errorf("Errors[0].Name: got %q, want %q", n.Errors[0].Name, "error")
	}
}

// ---------------------------------------------------------------------------
// AC-10~13: FlowConfig.LogOutput 직렬화 검증
// ---------------------------------------------------------------------------

// AC-10: LogOutput이 설정되면 JSON에 log_output 필드가 포함된다.
func TestFlowConfig_LogOutput_JSONSerialization(t *testing.T) {
	f := NewFlow("log-output-test",
		WithFlowConfig(FlowConfig{
			TrackHistory: true,
			LogOutput:    "/var/log/xflow/mqtt.log",
		}),
	)

	df := f.(*defaultFlow)
	data, err := df.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 실패: %v", err)
	}

	s := string(data)
	if !strings.Contains(s, `"log_output":"/var/log/xflow/mqtt.log"`) {
		t.Errorf("JSON에 log_output 필드가 포함되어야 한다: %s", s)
	}
}

// AC-11: LogOutput이 비어있으면 JSON에 log_output 필드가 생략된다 (omitempty).
func TestFlowConfig_LogOutput_JSONOmitempty(t *testing.T) {
	f := NewFlow("log-output-omit-test",
		WithFlowConfig(FlowConfig{
			TrackHistory: true,
			LogOutput:    "",
		}),
	)

	df := f.(*defaultFlow)
	data, err := df.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 실패: %v", err)
	}

	s := string(data)
	if strings.Contains(s, `"log_output"`) {
		t.Errorf("빈 LogOutput은 JSON에서 생략되어야 한다: %s", s)
	}
}

// AC-12: YAML에서 log_output과 log_level이 올바르게 파싱된다.
func TestFlowConfig_LogOutput_YAMLRoundTrip(t *testing.T) {
	yamlData := []byte(`
name: "yaml-log-test"
config:
  track_history: true
  max_history_size: 100
  error_handling: "propagate"
  log_level: "info"
  log_output: "/var/log/xflow/mqtt.log"
nodes: []
wires: []
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	if f.Config().LogOutput != "/var/log/xflow/mqtt.log" {
		t.Errorf("LogOutput 불일치: got %q, want %q",
			f.Config().LogOutput, "/var/log/xflow/mqtt.log")
	}
	if f.Config().LogLevel != "info" {
		t.Errorf("LogLevel 불일치: got %q, want %q",
			f.Config().LogLevel, "info")
	}
}

// AC-13: log_output이 없는 YAML(구 형식)도 정상 파싱된다 (하위 호환성).
func TestFlowConfig_LogOutput_YAMLBackwardCompat(t *testing.T) {
	yamlData := []byte(`
name: "backward-compat-test"
config:
  track_history: true
  max_history_size: 50
  error_handling: "propagate"
  log_level: "debug"
nodes: []
wires: []
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	if f.Config().LogOutput != "" {
		t.Errorf("log_output이 없으면 빈 문자열이어야 한다: got %q", f.Config().LogOutput)
	}
	if f.Config().LogLevel != "debug" {
		t.Errorf("LogLevel 불일치: got %q, want %q",
			f.Config().LogLevel, "debug")
	}
	if f.Config().TrackHistory != true {
		t.Error("TrackHistory는 true여야 한다")
	}
	if f.Config().MaxHistorySize != 50 {
		t.Errorf("MaxHistorySize 불일치: got %d, want 50", f.Config().MaxHistorySize)
	}
}

// LogOutput이 JSON 라운드트립에서 보존되는지 검증한다.
func TestFlowConfig_LogOutput_JSONRoundTrip(t *testing.T) {
	original := NewFlow("json-roundtrip-log-test",
		WithFlowConfig(FlowConfig{
			TrackHistory: true,
			LogOutput:    "stdout+/var/log/xflow/node.log",
		}),
	)

	df := original.(*defaultFlow)
	data, err := df.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 실패: %v", err)
	}

	restored, err := FlowFromJSON(data)
	if err != nil {
		t.Fatalf("FlowFromJSON 실패: %v", err)
	}

	if restored.Config().LogOutput != "stdout+/var/log/xflow/node.log" {
		t.Errorf("LogOutput 라운드트립 불일치: got %q, want %q",
			restored.Config().LogOutput, "stdout+/var/log/xflow/node.log")
	}
}

func TestNormalizeNodeDefaults_PortDirectionAutoSet(t *testing.T) {
	yamlData := []byte(`
name: test-flow
nodes:
  - name: "processor"
    type: "transform"
    inputs:
      - name: "in"
    outputs:
      - name: "out"
    errors:
      - name: "error"
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	n := f.Nodes()[0]

	// inputs → PortInput
	if n.Inputs[0].Direction != PortInput {
		t.Errorf("Inputs[0].Direction: got %q, want %q", n.Inputs[0].Direction, PortInput)
	}

	// outputs → PortOutput
	if n.Outputs[0].Direction != PortOutput {
		t.Errorf("Outputs[0].Direction: got %q, want %q", n.Outputs[0].Direction, PortOutput)
	}

	// errors → PortError
	if n.Errors[0].Direction != PortError {
		t.Errorf("Errors[0].Direction: got %q, want %q", n.Errors[0].Direction, PortError)
	}
}

// TestNormalizeEdgesToWires_BasicFromTo 는 edges 키의 from/to 단축 문법이
// wires 의 source_node_id/target_node_id 로 정규화되는지 검증한다.
func TestNormalizeEdgesToWires_BasicFromTo(t *testing.T) {
	yamlData := []byte(`
name: edge-test
nodes:
  - name: "source-node"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
  - name: "target-node"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
edges:
  - from: "source-node"
    to: "target-node"
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	if len(f.Wires()) != 1 {
		t.Fatalf("Wires 수: got %d, want 1", len(f.Wires()))
	}

	w := f.Wires()[0]
	// from → source_node_id (노드 이름이 UUID로 해석됨)
	if w.SourcePort != "out" {
		t.Errorf("SourcePort: got %q, want %q", w.SourcePort, "out")
	}
	if w.TargetPort != "in" {
		t.Errorf("TargetPort: got %q, want %q", w.TargetPort, "in")
	}
}

// TestNormalizeEdgesToWires_WiresHasPriority 는 wires 와 edges 가 동시에 존재할 때
// wires 가 우선되고 edges 가 무시되는지 검증한다.
func TestNormalizeEdgesToWires_WiresHasPriority(t *testing.T) {
	yamlData := []byte(`
name: priority-test
nodes:
  - name: "a"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
  - name: "b"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
  - name: "c"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
wires:
  - source: "a:out"
    target: "b:in"
edges:
  - from: "a"
    to: "c"
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	// wires 가 우선이므로 1개만
	if len(f.Wires()) != 1 {
		t.Fatalf("Wires 수: got %d, want 1 (wires 우선)", len(f.Wires()))
	}
}

// TestNormalizeEdgesToWires_ExplicitPorts 는 edges 에서 source_node_id/target_node_id 를
// 명시적으로 지정한 경우 from/to 가 무시되는지 검증한다.
func TestNormalizeEdgesToWires_ExplicitPorts(t *testing.T) {
	yamlData := []byte(`
name: explicit-test
nodes:
  - name: "a"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
  - name: "b"
    type: "transform"
    inputs:
      - name: "in"
        direction: "input"
    outputs:
      - name: "out"
        direction: "output"
edges:
  - from: "ignored"
    source_node_id: "a"
    source_port: "out"
    to: "also-ignored"
    target_node_id: "b"
    target_port: "in"
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	if len(f.Wires()) != 1 {
		t.Fatalf("Wires 수: got %d, want 1", len(f.Wires()))
	}

	w := f.Wires()[0]
	if w.SourcePort != "out" {
		t.Errorf("SourcePort: got %q, want %q", w.SourcePort, "out")
	}
	if w.TargetPort != "in" {
		t.Errorf("TargetPort: got %q, want %q", w.TargetPort, "in")
	}
}

// TestNormalizeEdgesToWires_NodePortFormat 은 edges 에서 "node:port" 형식의
// from/to 값이 올바르게 node_id와 port로 분리되는지 검증한다.
func TestNormalizeEdgesToWires_NodePortFormat(t *testing.T) {
	yamlData := []byte(`
name: nodeport-test
nodes:
  - name: "source"
    type: "bridge"
    inputs:
      - "in"
    outputs:
      - "out"
    agent_ref:
      agent_name: "test-agent"
      direction: "in"
  - name: "sink"
    type: "bridge"
    inputs:
      - "in"
    outputs:
      - "out"
    agent_ref:
      agent_name: "test-sink"
      direction: "out"
edges:
  - from: "source:out"
    to: "sink:in"
`)

	f, err := FlowFromYAML(yamlData)
	if err != nil {
		t.Fatalf("FlowFromYAML 실패: %v", err)
	}

	if len(f.Wires()) != 1 {
		t.Fatalf("Wires 수: got %d, want 1", len(f.Wires()))
	}

	w := f.Wires()[0]
	// source_node_id 는 name-to-ID 매핑 후 UUID 이므로 포트만 검증
	if w.SourcePort != "out" {
		t.Errorf("SourcePort: got %q, want %q", w.SourcePort, "out")
	}
	if w.TargetPort != "in" {
		t.Errorf("TargetPort: got %q, want %q", w.TargetPort, "in")
	}
	// SourceNodeID 가 "source:out" 이 아닌 올바른 노드 ID 인지 확인
	if strings.Contains(w.SourceNodeID, ":") {
		t.Errorf("SourceNodeID 에 ':' 포함: %q (node:port 미분리)", w.SourceNodeID)
	}
	if strings.Contains(w.TargetNodeID, ":") {
		t.Errorf("TargetNodeID 에 ':' 포함: %q (node:port 미분리)", w.TargetNodeID)
	}
}

// TestSplitNodePort 는 splitNodePort 헬퍼 함수를 단위 테스트한다.
func TestSplitNodePort(t *testing.T) {
	tests := []struct {
		input       string
		defaultPort string
		wantNode    string
		wantPort    string
	}{
		{"nasa-source:out", "default", "nasa-source", "out"},
		{"trace-logger:in", "default", "trace-logger", "in"},
		{"simple-node", "out", "simple-node", "out"},
		{"node-with-dash:error", "out", "node-with-dash", "error"},
		{" spaced : port ", "out", "spaced", "port"},
		{"", "out", "", "out"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotNode, gotPort := splitNodePort(tt.input, tt.defaultPort)
			if gotNode != tt.wantNode {
				t.Errorf("node: got %q, want %q", gotNode, tt.wantNode)
			}
			if gotPort != tt.wantPort {
				t.Errorf("port: got %q, want %q", gotPort, tt.wantPort)
			}
		})
	}
}

// TestNormalizeConfigAgentRef 는 config.agent_ref 문자열이 노드 레벨 agent_ref 구조체로
// 승격되는지 검증한다. mqtt-publisher, mqtt-subscriber 등 에이전트 참조 노드가
// YAML 에서 config.agent_ref: "agent-name" 형식을 사용할 때 적용된다.
func TestNormalizeConfigAgentRef(t *testing.T) {
	t.Run("config.agent_ref 문자열이 노드 레벨 agent_ref 로 승격", func(t *testing.T) {
		yamlData := []byte(`
name: agent-ref-test
nodes:
  - name: "mqtt-pub"
    type: "mqtt-publisher"
    config:
      agent_ref: "mqtt-broker"
      default_topic: "test/topic"
    outputs:
      - "out"
wires: []
`)
		f, err := FlowFromYAML(yamlData)
		if err != nil {
			t.Fatalf("FlowFromYAML 실패: %v", err)
		}

		nodes := f.Nodes()
		if len(nodes) != 1 {
			t.Fatalf("노드 수: got %d, want 1", len(nodes))
		}

		node := nodes[0]
		// AgentRef 가 승격되어야 한다
		if node.AgentRef == nil {
			t.Fatal("config.agent_ref 가 NodeDef.AgentRef 로 승격되지 않았다")
		}
		if node.AgentRef.AgentName != "mqtt-broker" {
			t.Errorf("AgentRef.AgentName = %q, want %q", node.AgentRef.AgentName, "mqtt-broker")
		}
		// config 에서 agent_ref 가 제거되어야 한다 (중복 방지)
		if _, exists := node.Config["agent_ref"]; exists {
			t.Error("config 에 agent_ref 가 남아있다 (중복)")
		}
		// 다른 config 값은 보존되어야 한다
		if node.Config["default_topic"] != "test/topic" {
			t.Errorf("config.default_topic = %v, want %q", node.Config["default_topic"], "test/topic")
		}
	})

	t.Run("노드 레벨 agent_ref 가 있으면 config.agent_ref 무시", func(t *testing.T) {
		yamlData := []byte(`
name: bridge-test
nodes:
  - name: "bridge-in"
    type: "bridge"
    agent_ref:
      agent_name: "mqtt-broker"
      direction: "in"
    config:
      agent_ref: "should-be-ignored"
    inputs:
      - "in"
    outputs:
      - "out"
wires: []
`)
		f, err := FlowFromYAML(yamlData)
		if err != nil {
			t.Fatalf("FlowFromYAML 실패: %v", err)
		}

		node := f.Nodes()[0]
		// 노드 레벨 agent_ref 가 유지되어야 한다
		if node.AgentRef == nil {
			t.Fatal("노드 레벨 AgentRef 가 nil")
		}
		if node.AgentRef.AgentName != "mqtt-broker" {
			t.Errorf("AgentRef.AgentName = %q, want %q", node.AgentRef.AgentName, "mqtt-broker")
		}
		if node.AgentRef.Direction != BridgeIn {
			t.Errorf("AgentRef.Direction = %q, want %q", node.AgentRef.Direction, BridgeIn)
		}
	})

	t.Run("config.agent_ref 가 없으면 변경 없음", func(t *testing.T) {
		yamlData := []byte(`
name: no-ref-test
nodes:
  - name: "transform"
    type: "transform"
    config:
      expression: "msg.payload"
    inputs:
      - "in"
    outputs:
      - "out"
wires: []
`)
		f, err := FlowFromYAML(yamlData)
		if err != nil {
			t.Fatalf("FlowFromYAML 실패: %v", err)
		}

		node := f.Nodes()[0]
		if node.AgentRef != nil {
			t.Error("agent_ref 가 없는 노드에 AgentRef 가 생성되었다")
		}
	})
}
