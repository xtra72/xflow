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
)

// Metadata 는 메시지 메타데이터 접근을 위한 인터페이스이다.
type Metadata interface {
	// Get 은 키에 해당하는 값을 반환한다. 키가 없으면 ("", false)를 반환한다.
	Get(key string) (string, bool)
	// Set 은 키-값 쌍을 설정한다. 이미 존재하면 덮어쓴다.
	Set(key string, value string)
	// Has 는 키가 존재하는지 여부를 반환한다.
	Has(key string) bool
	// Remove 는 키를 삭제한다. 존재하지 않는 키에 대해서는 아무 작업도 수행하지 않는다.
	Remove(key string)
	// All 은 모든 키-값 쌍의 깊은 복사본을 반환한다.
	All() map[string]string
	// Clone 은 독립적인 복사본을 반환한다.
	Clone() Metadata
}

// mapMetadata 는 map[string]string 기반의 Metadata 구현이다.
type mapMetadata struct {
	data map[string]string
}

// NewMetadata 는 비어있는 새 Metadata 인스턴스를 생성하여 반환한다.
func NewMetadata() Metadata {
	return &mapMetadata{
		data: make(map[string]string),
	}
}

func (m *mapMetadata) Get(key string) (string, bool) {
	v, ok := m.data[key]
	return v, ok
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
	// 깊은 복사본 반환
	cp := make(map[string]string, len(m.data))
	for k, v := range m.data {
		cp[k] = v
	}
	return cp
}

func (m *mapMetadata) Clone() Metadata {
	return &mapMetadata{
		data: m.All(),
	}
}
