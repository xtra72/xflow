package message

// 시스템 메타데이터 키 상수
const (
	// MetaKeySource 는 메시지 발신 소스를 나타내는 키이다.
	MetaKeySource = "_source"
	// MetaKeyFlowID 는 흐름 식별자를 나타내는 키이다.
	MetaKeyFlowID = "_flowID"
	// MetaKeyNodeID 는 노드 식별자를 나타내는 키이다.
	MetaKeyNodeID = "_nodeID"
	// MetaKeyTTL 은 메시지 수명을 나타내는 키이다.
	MetaKeyTTL = "_ttl"
	// MetaKeyCorrelationID 는 상관관계 식별자를 나타내는 키이다.
	MetaKeyCorrelationID = "_correlationID"
	// MetaKeyError 는 에러 메시지를 나타내는 키이다.
	MetaKeyError = "_error"
	// MetaKeyErrorNodeID 는 에러가 발생한 노드 식별자를 나타내는 키이다.
	MetaKeyErrorNodeID = "_errorNodeID"
)

// Metadata 는 메시지 메타데이터 접근을 위한 인터페이스이다.
//
// 값 계약 (value contract):
// 메타데이터의 각 값은 정확히 두 종류 중 하나이다.
//   - string: flat(평면) 키. 기존 string API(Set/Get/All)가 다루는 대상.
//   - map[string]string: nested group(중첩 그룹). SetGroup/GetGroup이 다루는 대상.
//
// 이 두 종류만 허용되며, 다른 타입은 저장되지 않는다.
//
// 하위 호환성:
//   - Set/Get/All 은 오직 string 값만 다룬다. group 값은 이들 API에 보이지 않는다.
//   - SetGroup/GetGroup/Raw 는 group 값을 다루기 위한 신규 API이다.
type Metadata interface {
	// Get 은 키에 해당하는 문자열 값을 반환한다.
	// 키가 없거나, 값이 group(map[string]string)인 경우 ("", false)를 반환한다.
	Get(key string) (string, bool)
	// Set 은 문자열 키-값 쌍을 설정한다. 이미 존재하면 덮어쓴다.
	// (기존 키가 group이었더라도 string으로 덮어쓴다.)
	Set(key string, value string)
	// Has 는 키가 존재하는지 여부를 반환한다 (string/group 종류 무관).
	Has(key string) bool
	// Remove 는 키를 삭제한다 (string/group 종류 무관).
	// 존재하지 않는 키에 대해서는 아무 작업도 수행하지 않는다.
	Remove(key string)
	// All 은 string 값을 가진 항목만의 깊은 복사본을 반환한다.
	// group 값은 제외된다. (모든 기존 logic 소비자는 flat 문자열만 사용하므로
	// 이 동작으로 하위 호환성이 유지된다.)
	All() map[string]string
	// SetGroup 은 nested group 값을 설정한다. fields의 복사본을 저장한다.
	// fields가 nil 또는 비어있으면 no-op delete로 동작한다 (해당 키 제거,
	// 빈 객체를 만들지 않는다).
	SetGroup(key string, fields map[string]string)
	// GetGroup 은 값이 group이면 그 복사본과 true를, 그렇지 않으면 (nil, false)를 반환한다.
	GetGroup(key string) (map[string]string, bool)
	// Raw 는 전체 메타데이터(string 값 + group 복사본)의 복사본을 반환한다.
	// 반환 맵의 값은 string 또는 map[string]string 이다.
	Raw() map[string]any
	// Clone 은 독립적인 복사본을 반환한다 (string과 group 모두 보존).
	Clone() Metadata
}

// mapMetadata 는 map[string]any 기반의 Metadata 구현이다.
// 저장되는 값은 string(flat) 또는 map[string]string(group) 둘 중 하나이다.
type mapMetadata struct {
	data map[string]any
}

// NewMetadata 는 비어있는 새 Metadata 인스턴스를 생성하여 반환한다.
func NewMetadata() Metadata {
	return &mapMetadata{
		data: make(map[string]any),
	}
}

func (m *mapMetadata) Get(key string) (string, bool) {
	v, ok := m.data[key]
	if !ok {
		return "", false
	}
	// string 값만 반환한다. group 값에 대해서는 ("", false).
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, true
}

func (m *mapMetadata) Set(key string, value string) {
	m.data[key] = value
}

func (m *mapMetadata) Has(key string) bool {
	_, ok := m.data[key]
	return ok
}

func (m *mapMetadata) Remove(key string) {
	delete(m.data, key)
}

func (m *mapMetadata) All() map[string]string {
	// string 값을 가진 항목만 깊은 복사본으로 반환한다 (group 제외).
	cp := make(map[string]string, len(m.data))
	for k, v := range m.data {
		if s, ok := v.(string); ok {
			cp[k] = s
		}
	}
	return cp
}

func (m *mapMetadata) SetGroup(key string, fields map[string]string) {
	// nil/empty fields 는 no-op delete (빈 객체 생성 방지).
	if len(fields) == 0 {
		delete(m.data, key)
		return
	}
	cp := make(map[string]string, len(fields))
	for k, v := range fields {
		cp[k] = v
	}
	m.data[key] = cp
}

func (m *mapMetadata) GetGroup(key string) (map[string]string, bool) {
	v, ok := m.data[key]
	if !ok {
		return nil, false
	}
	g, ok := v.(map[string]string)
	if !ok {
		return nil, false
	}
	cp := make(map[string]string, len(g))
	for k, val := range g {
		cp[k] = val
	}
	return cp, true
}

func (m *mapMetadata) Raw() map[string]any {
	cp := make(map[string]any, len(m.data))
	for k, v := range m.data {
		switch val := v.(type) {
		case map[string]string:
			// group 은 복사본으로 보존
			g := make(map[string]string, len(val))
			for gk, gv := range val {
				g[gk] = gv
			}
			cp[k] = g
		default:
			// string 값 (그 외 타입은 저장될 수 없음)
			cp[k] = v
		}
	}
	return cp
}

func (m *mapMetadata) Clone() Metadata {
	// Raw()를 통해 string과 group을 모두 보존하여 복제한다.
	return &mapMetadata{
		data: m.Raw(),
	}
}
