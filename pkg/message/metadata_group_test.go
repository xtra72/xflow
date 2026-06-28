package message

import (
	"reflect"
	"testing"
)

// TestMetadata_StringAPIUnchanged 는 group 기능 추가 후에도 기존 string API(Set/Get)가
// 동일하게 동작함을 검증한다 (하위 호환성).
func TestMetadata_StringAPIUnchanged(t *testing.T) {
	m := NewMetadata()
	m.Set("source", "agent-1")

	got, ok := m.Get("source")
	if !ok || got != "agent-1" {
		t.Errorf("Set 후 Get = (%q, %v), 기대값 (\"agent-1\", true)", got, ok)
	}
}

// TestMetadata_SetGroupGetGroup 는 SetGroup -> GetGroup 왕복을 검증한다.
func TestMetadata_SetGroupGetGroup(t *testing.T) {
	m := NewMetadata()
	m.SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})

	g, ok := m.GetGroup("agent")
	if !ok {
		t.Fatal("SetGroup 후 GetGroup에서 ok=false를 반환했다")
	}
	if g["type"] != "serial" || g["id"] != "node-1" {
		t.Errorf("GetGroup 값 불일치: %v", g)
	}
}

// TestMetadata_SetGroupStoresCopy 는 SetGroup이 입력 맵의 복사본을 저장하는지 검증한다.
func TestMetadata_SetGroupStoresCopy(t *testing.T) {
	m := NewMetadata()
	fields := map[string]string{"type": "serial", "id": "node-1"}
	m.SetGroup("agent", fields)

	// 원본 입력 맵 수정이 저장된 group에 영향을 주지 않아야 한다
	fields["type"] = "mutated"

	g, _ := m.GetGroup("agent")
	if g["type"] != "serial" {
		t.Errorf("SetGroup이 입력 맵 복사본을 저장하지 않았다: %v", g)
	}
}

// TestMetadata_GetGroupReturnsCopy 는 GetGroup이 복사본을 반환하는지 검증한다.
func TestMetadata_GetGroupReturnsCopy(t *testing.T) {
	m := NewMetadata()
	m.SetGroup("agent", map[string]string{"type": "serial"})

	g1, _ := m.GetGroup("agent")
	g1["type"] = "mutated"

	g2, _ := m.GetGroup("agent")
	if g2["type"] != "serial" {
		t.Errorf("GetGroup 반환값 수정이 원본에 영향을 주었다: %v", g2)
	}
}

// TestMetadata_SetGroupEmptyIsNoOpDelete 는 nil/empty fields가 no-op delete로 동작함을 검증한다.
func TestMetadata_SetGroupEmptyIsNoOpDelete(t *testing.T) {
	m := NewMetadata()
	m.SetGroup("agent", map[string]string{"type": "serial"})

	// empty 맵으로 SetGroup 하면 기존 group을 제거한다
	m.SetGroup("agent", map[string]string{})
	if _, ok := m.GetGroup("agent"); ok {
		t.Error("empty fields SetGroup 후에도 group이 존재한다")
	}

	// nil 맵으로 SetGroup 하면 아무 group도 생성하지 않는다
	m.SetGroup("device", nil)
	if _, ok := m.GetGroup("device"); ok {
		t.Error("nil fields SetGroup이 group을 생성했다")
	}
	if m.Has("device") {
		t.Error("nil fields SetGroup이 키를 생성했다")
	}
}

// TestMetadata_GetGroupOnStringIsFalse 는 string 값에 대해 GetGroup이 (nil,false)를 반환함을 검증한다.
func TestMetadata_GetGroupOnStringIsFalse(t *testing.T) {
	m := NewMetadata()
	m.Set("source", "agent-1")

	if g, ok := m.GetGroup("source"); ok || g != nil {
		t.Errorf("string 값에 대해 GetGroup = (%v, %v), 기대값 (nil, false)", g, ok)
	}
}

// TestMetadata_GetOnGroupIsFalse 는 group 값에 대해 Get이 ("", false)를 반환함을 검증한다.
func TestMetadata_GetOnGroupIsFalse(t *testing.T) {
	m := NewMetadata()
	m.SetGroup("agent", map[string]string{"type": "serial"})

	if v, ok := m.Get("agent"); ok || v != "" {
		t.Errorf("group 값에 대해 Get = (%q, %v), 기대값 (\"\", false)", v, ok)
	}
}

// TestMetadata_AllExcludesGroups 는 All()이 string 항목만 반환하고 group을 제외함을 검증한다.
func TestMetadata_AllExcludesGroups(t *testing.T) {
	m := NewMetadata()
	m.Set("node_source", "poll")
	m.Set("flat", "value")
	m.SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})
	m.SetGroup("device", map[string]string{"type": "HVACR.IDU", "id": "dev-9"})

	all := m.All()
	if len(all) != 2 {
		t.Fatalf("All() 길이 = %d, 기대값 2 (string 항목만)", len(all))
	}
	if all["node_source"] != "poll" || all["flat"] != "value" {
		t.Errorf("All() string 항목 불일치: %v", all)
	}
	if _, ok := all["agent"]; ok {
		t.Error("All()에 group 키 'agent'가 포함되었다")
	}
	if _, ok := all["device"]; ok {
		t.Error("All()에 group 키 'device'가 포함되었다")
	}
}

// TestMetadata_RawIncludesStringsAndGroups 는 Raw()가 string과 group을 모두 포함하고
// 복사본을 반환함을 검증한다.
func TestMetadata_RawIncludesStringsAndGroups(t *testing.T) {
	m := NewMetadata()
	m.Set("node_source", "poll")
	m.SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})

	raw := m.Raw()
	if len(raw) != 2 {
		t.Fatalf("Raw() 길이 = %d, 기대값 2", len(raw))
	}

	s, ok := raw["node_source"].(string)
	if !ok || s != "poll" {
		t.Errorf("Raw()[node_source] = %v, 기대값 \"poll\"", raw["node_source"])
	}

	g, ok := raw["agent"].(map[string]string)
	if !ok {
		t.Fatalf("Raw()[agent] 타입이 map[string]string이 아니다: %T", raw["agent"])
	}
	if g["type"] != "serial" || g["id"] != "node-1" {
		t.Errorf("Raw()[agent] 값 불일치: %v", g)
	}

	// Raw() 반환값의 group 수정이 원본에 영향을 주지 않아야 한다
	g["type"] = "mutated"
	orig, _ := m.GetGroup("agent")
	if orig["type"] != "serial" {
		t.Error("Raw() 반환 group 수정이 원본에 영향을 주었다")
	}
}

// TestMetadata_ClonepreservesGroups 는 Clone이 string과 group을 모두 보존하고
// 독립적인 복사본을 반환함을 검증한다.
func TestMetadata_ClonePreservesGroups(t *testing.T) {
	m := NewMetadata()
	m.Set("node_source", "poll")
	m.SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})

	cloned := m.Clone()

	// string 보존
	if v, ok := cloned.Get("node_source"); !ok || v != "poll" {
		t.Errorf("Clone 후 string 보존 실패: (%q, %v)", v, ok)
	}
	// group 보존
	g, ok := cloned.GetGroup("agent")
	if !ok || g["type"] != "serial" || g["id"] != "node-1" {
		t.Errorf("Clone 후 group 보존 실패: (%v, %v)", g, ok)
	}

	// 독립성: 복제본의 group 수정이 원본에 영향을 주지 않아야 한다
	cloned.SetGroup("agent", map[string]string{"type": "tcp", "id": "node-2"})
	origG, _ := m.GetGroup("agent")
	if origG["type"] != "serial" {
		t.Error("Clone group 수정이 원본에 영향을 주었다")
	}
}

// TestMetadata_HasAndRemoveBothKinds 는 Has/Remove가 string과 group 모두에 대해
// 동작함을 검증한다.
func TestMetadata_HasAndRemoveBothKinds(t *testing.T) {
	m := NewMetadata()
	m.Set("flat", "v")
	m.SetGroup("grp", map[string]string{"a": "b"})

	if !m.Has("flat") || !m.Has("grp") {
		t.Error("Has가 string 또는 group 키에 대해 false를 반환했다")
	}

	m.Remove("flat")
	m.Remove("grp")
	if m.Has("flat") || m.Has("grp") {
		t.Error("Remove 후에도 키가 존재한다")
	}
}

// TestMetadata_RawCopyEquality 는 Raw()가 의미적으로 동일한 복사본임을 검증한다.
func TestMetadata_RawCopyEquality(t *testing.T) {
	m := NewMetadata()
	m.Set("k", "v")
	m.SetGroup("g", map[string]string{"x": "y"})

	want := map[string]any{
		"k": "v",
		"g": map[string]string{"x": "y"},
	}
	if !reflect.DeepEqual(m.Raw(), want) {
		t.Errorf("Raw() = %v, 기대값 %v", m.Raw(), want)
	}
}
