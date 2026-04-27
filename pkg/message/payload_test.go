package message

import (
	"encoding/json"
	"sort"
	"testing"
)

// TestPayload_AddNewKey 는 새 키에 대한 Add가 성공하는지 검증한다.
func TestPayload_AddNewKey(t *testing.T) {
	p := NewPayload()
	if err := p.Add("name", "John"); err != nil {
		t.Errorf("새 키 Add에서 에러 발생: %v", err)
	}
	got, ok := p.Get("name")
	if !ok || got != "John" {
		t.Errorf("Add 후 Get = (%v, %v), 기대값 (\"John\", true)", got, ok)
	}
}

// TestPayload_AddDuplicateKey 는 중복 키에 대해 ErrKeyExists를 반환하는지 검증한다.
func TestPayload_AddDuplicateKey(t *testing.T) {
	p := NewPayload()
	_ = p.Add("name", "John")

	err := p.Add("name", "Jane")
	if err != ErrKeyExists {
		t.Errorf("중복 키 Add 에러 = %v, 기대값 ErrKeyExists", err)
	}

	// 기존 값이 변경되지 않아야 한다
	got, _ := p.Get("name")
	if got != "John" {
		t.Errorf("중복 Add 후 값이 변경되었다: %v", got)
	}
}

// TestPayload_SetNewAndExisting 은 Set이 새 키와 기존 키 모두에 동작하는지 검증한다.
func TestPayload_SetNewAndExisting(t *testing.T) {
	p := NewPayload()

	// 새 키 설정
	p.Set("name", "John")
	got, ok := p.Get("name")
	if !ok || got != "John" {
		t.Errorf("Set 후 Get = (%v, %v), 기대값 (\"John\", true)", got, ok)
	}

	// 기존 키 덮어쓰기
	p.Set("name", "Jane")
	got, ok = p.Get("name")
	if !ok || got != "Jane" {
		t.Errorf("Set 덮어쓰기 후 Get = (%v, %v), 기대값 (\"Jane\", true)", got, ok)
	}
}

// TestPayload_Delete 는 키 삭제와 존재하지 않는 키 삭제를 검증한다.
func TestPayload_Delete(t *testing.T) {
	p := NewPayload()
	p.Set("k1", "v1")
	p.Set("k2", "v2")

	p.Delete("k1")
	_, ok := p.Get("k1")
	if ok {
		t.Error("Delete 후에도 k1이 존재한다")
	}

	// 존재하지 않는 키 삭제는 패닉 없이 수행되어야 한다
	p.Delete("nonexistent")

	// k2는 영향받지 않아야 한다
	_, ok = p.Get("k2")
	if !ok {
		t.Error("다른 키 삭제 후 k2가 사라졌다")
	}
}

// TestPayload_GetMissing 은 존재하지 않는 키에 대해 (nil, false)를 반환하는지 검증한다.
func TestPayload_GetMissing(t *testing.T) {
	p := NewPayload()
	got, ok := p.Get("missing")
	if ok {
		t.Error("존재하지 않는 키에 대해 ok=true를 반환했다")
	}
	if got != nil {
		t.Errorf("존재하지 않는 키에 대해 nil이 아닌 %v를 반환했다", got)
	}
}

// TestPayload_Keys 는 정렬된 키 목록을 반환하는지 검증한다.
func TestPayload_Keys(t *testing.T) {
	p := NewPayload()
	p.Set("banana", 1)
	p.Set("apple", 2)
	p.Set("cherry", 3)

	keys := p.Keys()
	expected := []string{"apple", "banana", "cherry"}

	if len(keys) != len(expected) {
		t.Fatalf("Keys() 길이 = %d, 기대값 %d", len(keys), len(expected))
	}
	for i, k := range keys {
		if k != expected[i] {
			t.Errorf("Keys()[%d] = %q, 기대값 %q", i, k, expected[i])
		}
	}
}

// TestPayload_KeysEmpty 는 빈 페이로드에서 빈 슬라이스를 반환하는지 검증한다.
func TestPayload_KeysEmpty(t *testing.T) {
	p := NewPayload()
	keys := p.Keys()
	if len(keys) != 0 {
		t.Errorf("빈 페이로드의 Keys() 길이 = %d, 기대값 0", len(keys))
	}
}

// TestPayload_ToMapDeepCopy 는 ToMap()이 깊은 복사본을 반환하는지 검증한다.
func TestPayload_ToMapDeepCopy(t *testing.T) {
	p := NewPayload()
	p.Set("name", "John")
	p.Set("nested", map[string]any{"inner": "value"})

	m := p.ToMap()
	if m["name"] != "John" {
		t.Errorf("ToMap()[\"name\"] = %v, 기대값 \"John\"", m["name"])
	}

	// 반환된 맵 수정이 원본에 영향을 주지 않아야 한다
	m["name"] = "modified"
	m["new"] = "added"

	got, _ := p.Get("name")
	if got != "John" {
		t.Error("ToMap() 반환값 수정이 원본에 영향을 주었다")
	}
	_, ok := p.Get("new")
	if ok {
		t.Error("ToMap() 반환값에 키 추가가 원본에 영향을 주었다")
	}
}

// TestPayload_ToJSON 은 JSON 직렬화를 검증한다.
func TestPayload_ToJSON(t *testing.T) {
	p := NewPayload()
	p.Set("name", "John")
	p.Set("age", float64(30))

	data, err := p.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON 에러: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("ToJSON 결과 파싱 에러: %v", err)
	}
	if result["name"] != "John" {
		t.Errorf("JSON name = %v, 기대값 \"John\"", result["name"])
	}
	if result["age"] != float64(30) {
		t.Errorf("JSON age = %v, 기대값 30", result["age"])
	}
}

// TestPayload_GetPath 는 GetPath가 내부 evaluatePath를 올바르게 호출하는지 검증한다.
func TestPayload_GetPath(t *testing.T) {
	p := NewPayload()
	p.Set("user", map[string]any{"name": "John"})

	got, err := p.GetPath("$.user.name")
	if err != nil {
		t.Fatalf("GetPath 에러: %v", err)
	}
	if got != "John" {
		t.Errorf("GetPath(\"$.user.name\") = %v, 기대값 \"John\"", got)
	}

	_, err = p.GetPath("$.missing")
	if err != ErrPathNotFound {
		t.Errorf("존재하지 않는 경로 에러 = %v, 기대값 ErrPathNotFound", err)
	}
}

// TestPayload_Clone 은 Clone()이 독립적인 복사본을 반환하는지 검증한다.
func TestPayload_Clone(t *testing.T) {
	p := NewPayload()
	p.Set("name", "John")
	p.Set("items", []any{"a", "b"})

	cloned := p.Clone()

	// 복제본에서 값 확인
	got, ok := cloned.Get("name")
	if !ok || got != "John" {
		t.Errorf("Clone 후 name = (%v, %v), 기대값 (\"John\", true)", got, ok)
	}

	// 복제본 수정이 원본에 영향을 주지 않아야 한다
	cloned.Set("name", "Jane")
	orig, _ := p.Get("name")
	if orig != "John" {
		t.Error("Clone 수정이 원본에 영향을 주었다")
	}
}

// TestPayload_NewPayloadEmpty 는 인자 없는 NewPayload가 빈 페이로드를 생성하는지 검증한다.
func TestPayload_NewPayloadEmpty(t *testing.T) {
	p := NewPayload()
	keys := p.Keys()
	if len(keys) != 0 {
		t.Errorf("NewPayload() 키 길이 = %d, 기대값 0", len(keys))
	}
}

// TestPayload_NewPayloadWithData 는 초기 데이터가 있는 NewPayload를 검증한다.
func TestPayload_NewPayloadWithData(t *testing.T) {
	initial := map[string]any{
		"name": "John",
		"age":  30,
	}
	p := NewPayload(initial)

	got, ok := p.Get("name")
	if !ok || got != "John" {
		t.Errorf("NewPayload(data) 후 name = (%v, %v), 기대값 (\"John\", true)", got, ok)
	}

	keys := p.Keys()
	sort.Strings(keys)
	if len(keys) != 2 || keys[0] != "age" || keys[1] != "name" {
		t.Errorf("NewPayload(data) 키 = %v, 기대값 [\"age\", \"name\"]", keys)
	}

	// 원본 맵 수정이 페이로드에 영향을 주지 않아야 한다
	initial["name"] = "modified"
	got2, _ := p.Get("name")
	if got2 != "John" {
		t.Error("원본 맵 수정이 페이로드에 영향을 주었다")
	}
}
