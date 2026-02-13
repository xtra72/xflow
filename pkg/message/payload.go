package message

import (
	"encoding/json"
	"sort"
)

// Payload 는 메시지 페이로드 접근을 위한 인터페이스이다.
type Payload interface {
	// Add 는 새 키에 값을 추가한다. 이미 존재하면 ErrKeyExists를 반환한다.
	Add(key string, value any) error
	// Set 은 키에 값을 설정한다. 이미 존재하면 덮어쓴다(upsert).
	Set(key string, value any)
	// Delete 는 키를 삭제한다. 존재하지 않는 키에 대해서는 아무 작업도 수행하지 않는다.
	Delete(key string)
	// Get 은 키에 해당하는 값을 반환한다. 키가 없으면 (nil, false)를 반환한다.
	Get(key string) (any, bool)
	// GetPath 는 JSONPath 표현식으로 값을 조회한다.
	GetPath(jsonpath string) (any, error)
	// Keys 는 모든 최상위 키를 정렬된 슬라이스로 반환한다.
	Keys() []string
	// ToMap 은 페이로드 데이터의 깊은 복사본을 맵으로 반환한다.
	ToMap() map[string]any
	// ToJSON 은 페이로드를 JSON 바이트로 직렬화한다.
	ToJSON() ([]byte, error)
	// Clone 은 독립적인 복사본을 반환한다.
	Clone() Payload
}

// mapPayload 는 map[string]any 기반의 Payload 구현이다.
type mapPayload struct {
	data map[string]any
}

// NewPayload 는 새 Payload 인스턴스를 생성하여 반환한다.
// 선택적으로 초기 데이터 맵을 전달할 수 있다.
func NewPayload(data ...map[string]any) Payload {
	p := &mapPayload{
		data: make(map[string]any),
	}
	if len(data) > 0 && data[0] != nil {
		for k, v := range data[0] {
			p.data[k] = v
		}
	}
	return p
}

func (p *mapPayload) Add(key string, value any) error {
	if _, exists := p.data[key]; exists {
		return ErrKeyExists
	}
	p.data[key] = value
	return nil
}

func (p *mapPayload) Set(key string, value any) {
	p.data[key] = value
}

func (p *mapPayload) Delete(key string) {
	delete(p.data, key)
}

func (p *mapPayload) Get(key string) (any, bool) {
	v, ok := p.data[key]
	return v, ok
}

func (p *mapPayload) GetPath(jsonpath string) (any, error) {
	return evaluatePath(p.data, jsonpath)
}

func (p *mapPayload) Keys() []string {
	keys := make([]string, 0, len(p.data))
	for k := range p.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (p *mapPayload) ToMap() map[string]any {
	return deepCopyMap(p.data)
}

func (p *mapPayload) ToJSON() ([]byte, error) {
	return json.Marshal(p.data)
}

func (p *mapPayload) Clone() Payload {
	return &mapPayload{
		data: deepCopyMap(p.data),
	}
}

// deepCopyMap 은 map[string]any를 재귀적으로 깊은 복사한다.
func deepCopyMap(src map[string]any) map[string]any {
	cp := make(map[string]any, len(src))
	for k, v := range src {
		cp[k] = deepCopyValue(v)
	}
	return cp
}

// deepCopyValue 는 any 타입의 값을 깊은 복사한다.
func deepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return deepCopyMap(val)
	case []any:
		cp := make([]any, len(val))
		for i, item := range val {
			cp[i] = deepCopyValue(item)
		}
		return cp
	default:
		// 기본 타입(string, float64, bool, nil 등)은 값 복사
		return v
	}
}
