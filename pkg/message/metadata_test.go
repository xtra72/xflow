package message

import "testing"

// TestMetadata_GetMissing 은 존재하지 않는 키에 대해 ("", false)를 반환하는지 검증한다.
func TestMetadata_GetMissing(t *testing.T) {
	m := NewMetadata()
	val, ok := m.Get("nonexistent")
	if ok {
		t.Error("존재하지 않는 키에 대해 ok=true를 반환했다")
	}
	if val != "" {
		t.Errorf("존재하지 않는 키에 대해 빈 문자열이 아닌 %q를 반환했다", val)
	}
}

// TestMetadata_SetAndGet 은 Set 후 Get으로 값을 올바르게 조회하는지 검증한다.
func TestMetadata_SetAndGet(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "일반 키", key: "source", value: "agent-1"},
		{name: "시스템 키", key: MetaKeySource, value: "system"},
		{name: "빈 값", key: "empty", value: ""},
		{name: "유니코드 값", key: "greeting", value: "안녕하세요"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMetadata()
			m.Set(tt.key, tt.value)

			got, ok := m.Get(tt.key)
			if !ok {
				t.Errorf("Set(%q, %q) 후 Get에서 ok=false를 반환했다", tt.key, tt.value)
			}
			if got != tt.value {
				t.Errorf("Set(%q, %q) 후 Get = %q, 기대값 %q", tt.key, tt.value, got, tt.value)
			}
		})
	}
}

// TestMetadata_Has 는 Has가 존재하는 키에 true, 없는 키에 false를 반환하는지 검증한다.
func TestMetadata_Has(t *testing.T) {
	m := NewMetadata()

	if m.Has("key1") {
		t.Error("빈 메타데이터에서 Has(\"key1\")이 true를 반환했다")
	}

	m.Set("key1", "val1")
	if !m.Has("key1") {
		t.Error("Set 후 Has(\"key1\")이 false를 반환했다")
	}

	if m.Has("key2") {
		t.Error("설정하지 않은 키에 대해 Has(\"key2\")가 true를 반환했다")
	}
}

// TestMetadata_Remove 는 키 삭제와 존재하지 않는 키 삭제(no-op)를 검증한다.
func TestMetadata_Remove(t *testing.T) {
	m := NewMetadata()
	m.Set("key1", "val1")
	m.Set("key2", "val2")

	m.Remove("key1")
	if m.Has("key1") {
		t.Error("Remove 후에도 key1이 존재한다")
	}

	// 존재하지 않는 키 삭제는 패닉 없이 수행되어야 한다
	m.Remove("nonexistent")

	// key2는 영향받지 않아야 한다
	if !m.Has("key2") {
		t.Error("다른 키 삭제 후 key2가 사라졌다")
	}
}

// TestMetadata_AllDeepCopy 는 All()이 깊은 복사본을 반환하는지 검증한다.
func TestMetadata_AllDeepCopy(t *testing.T) {
	m := NewMetadata()
	m.Set("k1", "v1")
	m.Set("k2", "v2")

	all := m.All()
	if len(all) != 2 {
		t.Fatalf("All() 길이 = %d, 기대값 2", len(all))
	}
	if all["k1"] != "v1" || all["k2"] != "v2" {
		t.Errorf("All() 값 불일치: %v", all)
	}

	// 반환된 맵 수정이 원본에 영향을 주지 않아야 한다
	all["k1"] = "modified"
	all["k3"] = "new"

	got, ok := m.Get("k1")
	if !ok || got != "v1" {
		t.Error("All() 반환값 수정이 원본 메타데이터에 영향을 주었다")
	}
	if m.Has("k3") {
		t.Error("All() 반환값에 키 추가가 원본 메타데이터에 영향을 주었다")
	}
}

// TestMetadata_Clone 은 Clone()이 독립적인 복사본을 반환하는지 검증한다.
func TestMetadata_Clone(t *testing.T) {
	m := NewMetadata()
	m.Set("k1", "v1")
	m.Set("k2", "v2")

	cloned := m.Clone()

	// 복제본에서 값 확인
	v, ok := cloned.Get("k1")
	if !ok || v != "v1" {
		t.Errorf("Clone 후 k1 = (%q, %v), 기대값 (\"v1\", true)", v, ok)
	}

	// 복제본 수정이 원본에 영향을 주지 않아야 한다
	cloned.Set("k1", "modified")
	cloned.Set("k3", "new")

	orig, ok := m.Get("k1")
	if !ok || orig != "v1" {
		t.Error("Clone 수정이 원본에 영향을 주었다")
	}
	if m.Has("k3") {
		t.Error("Clone에 추가한 키가 원본에 영향을 주었다")
	}
}

// TestMetadata_SystemKeyConstants 는 시스템 키 상수 값을 검증한다.
func TestMetadata_SystemKeyConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{name: "MetaKeySource", constant: MetaKeySource, expected: "_source"},
		{name: "MetaKeyFlowID", constant: MetaKeyFlowID, expected: "_flowID"},
		{name: "MetaKeyNodeID", constant: MetaKeyNodeID, expected: "_nodeID"},
		{name: "MetaKeyTTL", constant: MetaKeyTTL, expected: "_ttl"},
		{name: "MetaKeyCorrelationID", constant: MetaKeyCorrelationID, expected: "_correlationID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, 기대값 %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}
