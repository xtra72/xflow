package message

import "time"

// ChangeRecord 는 메시지 변경 이력의 개별 항목을 나타내는 값 타입이다.
type ChangeRecord struct {
	Target    string    // "payload" 또는 "metadata"
	Operation string    // "add", "set", "delete", "remove"
	Key       string    // 변경된 키
	OldValue  any       // 변경 전 값 (없으면 nil)
	NewValue  any       // 변경 후 값 (삭제 시 nil)
	NodeID    string    // 변경을 수행한 노드 식별자
	Timestamp time.Time // 변경 시각
}

// historyRecorder 는 변경 이력을 기록하는 공통 구조체이다.
type historyRecorder struct {
	history    *[]ChangeRecord // 공유 이력 슬라이스 포인터
	nodeID     string          // 현재 노드 식별자
	maxHistory int             // 최대 이력 개수
}

// record 는 변경 레코드를 이력에 추가한다. 최대 개수 초과 시 FIFO로 가장 오래된 항목을 제거한다.
func (h *historyRecorder) record(target, operation, key string, oldValue, newValue any) {
	rec := ChangeRecord{
		Target:    target,
		Operation: operation,
		Key:       key,
		OldValue:  oldValue,
		NewValue:  newValue,
		NodeID:    h.nodeID,
		Timestamp: time.Now(),
	}
	*h.history = append(*h.history, rec)

	// FIFO: 최대 개수 초과 시 가장 오래된 항목 제거
	if len(*h.history) > h.maxHistory {
		*h.history = (*h.history)[len(*h.history)-h.maxHistory:]
	}
}

// historyPayload 는 Payload를 래핑하여 변경 이력을 기록하는 데코레이터이다.
type historyPayload struct {
	inner Payload
	historyRecorder
}

// newHistoryPayload 는 이력 기록이 가능한 Payload 데코레이터를 생성한다.
func newHistoryPayload(inner Payload, history *[]ChangeRecord, nodeID string, maxHistory int) *historyPayload {
	return &historyPayload{
		inner: inner,
		historyRecorder: historyRecorder{
			history:    history,
			nodeID:     nodeID,
			maxHistory: maxHistory,
		},
	}
}

func (hp *historyPayload) Add(key string, value any) error {
	// 기존 값 조회 (이력 기록용)
	oldVal, _ := hp.inner.Get(key)
	err := hp.inner.Add(key, value)
	if err != nil {
		return err
	}
	hp.record("payload", "add", key, oldVal, value)
	return nil
}

func (hp *historyPayload) Set(key string, value any) {
	oldVal, _ := hp.inner.Get(key)
	hp.inner.Set(key, value)
	hp.record("payload", "set", key, oldVal, value)
}

func (hp *historyPayload) Delete(key string) {
	oldVal, _ := hp.inner.Get(key)
	hp.inner.Delete(key)
	hp.record("payload", "delete", key, oldVal, nil)
}

func (hp *historyPayload) Get(key string) (any, bool) {
	return hp.inner.Get(key)
}

func (hp *historyPayload) GetPath(jsonpath string) (any, error) {
	return hp.inner.GetPath(jsonpath)
}

func (hp *historyPayload) Keys() []string {
	return hp.inner.Keys()
}

func (hp *historyPayload) ToMap() map[string]any {
	return hp.inner.ToMap()
}

func (hp *historyPayload) ToJSON() ([]byte, error) {
	return hp.inner.ToJSON()
}

// Clone 은 내부 Payload의 복제본을 반환한다 (래핑되지 않은 상태).
func (hp *historyPayload) Clone() Payload {
	return hp.inner.Clone()
}

// historyMetadata 는 Metadata를 래핑하여 변경 이력을 기록하는 데코레이터이다.
type historyMetadata struct {
	inner Metadata
	historyRecorder
}

// newHistoryMetadata 는 이력 기록이 가능한 Metadata 데코레이터를 생성한다.
func newHistoryMetadata(inner Metadata, history *[]ChangeRecord, nodeID string, maxHistory int) *historyMetadata {
	return &historyMetadata{
		inner: inner,
		historyRecorder: historyRecorder{
			history:    history,
			nodeID:     nodeID,
			maxHistory: maxHistory,
		},
	}
}

func (hm *historyMetadata) Set(key string, value string) {
	oldVal, _ := hm.inner.Get(key)
	hm.inner.Set(key, value)
	var oldAny any
	if oldVal != "" {
		oldAny = oldVal
	}
	hm.record("metadata", "set", key, oldAny, value)
}

func (hm *historyMetadata) Remove(key string) {
	oldVal, exists := hm.inner.Get(key)
	hm.inner.Remove(key)
	var oldAny any
	if exists {
		oldAny = oldVal
	}
	hm.record("metadata", "remove", key, oldAny, nil)
}

func (hm *historyMetadata) Get(key string) (string, bool) {
	return hm.inner.Get(key)
}

func (hm *historyMetadata) Has(key string) bool {
	return hm.inner.Has(key)
}

func (hm *historyMetadata) All() map[string]string {
	return hm.inner.All()
}

// Clone 은 내부 Metadata의 복제본을 반환한다 (래핑되지 않은 상태).
func (hm *historyMetadata) Clone() Metadata {
	return hm.inner.Clone()
}
