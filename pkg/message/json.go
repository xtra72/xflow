package message

import (
	"encoding/json"
	"fmt"
	"time"
)

// jsonMessage 는 JSON 직렬화/역직렬화에 사용되는 중간 구조체이다.
type jsonMessage struct {
	ID        string         `json:"id"`
	Type      string         `json:"type,omitempty"` // v0.12.0: top-level 메시지 타입
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload"`
	// Metadata 는 string(flat) 또는 map[string]string(group) 값을 가진다.
	// nested group 보존을 위해 map[string]any 로 직렬화한다.
	Metadata       map[string]any     `json:"metadata"`
	HistoryEnabled bool               `json:"history_enabled"`
	History        []jsonChangeRecord `json:"history"`
}

// jsonChangeRecord 는 ChangeRecord의 JSON 직렬화 형태이다.
type jsonChangeRecord struct {
	Target    string    `json:"target"`
	Operation string    `json:"operation"`
	Key       string    `json:"key"`
	OldValue  any       `json:"old_value"`
	NewValue  any       `json:"new_value"`
	NodeID    string    `json:"node_id"`
	Timestamp time.Time `json:"timestamp"`
}

// MarshalJSON 은 defaultMessage를 JSON 바이트로 직렬화한다.
func (m *defaultMessage) MarshalJSON() ([]byte, error) {
	jm := jsonMessage{
		ID:        m.id,
		Type:      m.msgType, // v0.12.0
		Timestamp: m.timestamp,
		Payload:   m.payload.ToMap(),
		// Raw() 로 직렬화하여 nested group 은 JSON 객체로, flat 키는 문자열로 표현한다.
		Metadata:       m.metadata.Raw(),
		HistoryEnabled: m.historyEnabled,
	}

	if m.historyEnabled && m.history != nil {
		jm.History = make([]jsonChangeRecord, len(m.history))
		for i, rec := range m.history {
			jm.History[i] = jsonChangeRecord{
				Target:    rec.Target,
				Operation: rec.Operation,
				Key:       rec.Key,
				OldValue:  rec.OldValue,
				NewValue:  rec.NewValue,
				NodeID:    rec.NodeID,
				Timestamp: rec.Timestamp,
			}
		}
	} else {
		jm.History = nil
	}

	return json.Marshal(jm)
}

// FromJSON 은 JSON 바이트를 Message로 역직렬화한다.
func FromJSON(data []byte) (Message, error) {
	var jm jsonMessage
	if err := json.Unmarshal(data, &jm); err != nil {
		return nil, fmt.Errorf("JSON 역직렬화 실패: %w", err)
	}

	// ID 필수 검증
	if jm.ID == "" {
		return nil, fmt.Errorf("JSON에 id 필드가 없다")
	}

	// 페이로드 복원
	var payload Payload
	if jm.Payload != nil {
		payload = NewPayload(jm.Payload)
	} else {
		payload = NewPayload()
	}

	// 메타데이터 복원
	// 재구성 규칙:
	//   - JSON 문자열 → string 값 (Set)
	//   - JSON 객체(map) → group 값 (SetGroup). 객체 내 비문자열 필드는
	//     fmt.Sprint 로 문자열 강제 변환하여 데이터 손실을 방지한다.
	//   - 그 외(숫자/불리언/배열/null) → 문자열 표현으로 저장(Set)하여 데이터 손실을 방지한다.
	metadata := NewMetadata()
	for k, v := range jm.Metadata {
		restoreMetadataEntry(metadata, k, v)
	}

	// 이력 복원
	var history []ChangeRecord
	if jm.History != nil {
		history = make([]ChangeRecord, len(jm.History))
		for i, jr := range jm.History {
			history[i] = ChangeRecord{
				Target:    jr.Target,
				Operation: jr.Operation,
				Key:       jr.Key,
				OldValue:  jr.OldValue,
				NewValue:  jr.NewValue,
				NodeID:    jr.NodeID,
				Timestamp: jr.Timestamp,
			}
		}
	}

	msg := &defaultMessage{
		id:             jm.ID,
		msgType:        jm.Type, // v0.12.0
		timestamp:      jm.Timestamp,
		payload:        payload,
		metadata:       metadata,
		historyEnabled: jm.HistoryEnabled,
		history:        history,
		maxHistory:     100, // 기본값
	}

	// 이력 활성화 시 래핑
	if msg.historyEnabled {
		if msg.history == nil {
			msg.history = make([]ChangeRecord, 0)
		}
		msg.payload = newHistoryPayload(msg.payload, &msg.history, msg.nodeID, msg.maxHistory)
		msg.metadata = newHistoryMetadata(msg.metadata, &msg.history, msg.nodeID, msg.maxHistory)
	}

	return msg, nil
}

// restoreMetadataEntry 는 JSON 메타데이터 항목 하나를 Metadata 에 복원한다.
//
// 규칙:
//   - string → Set (flat 문자열 값)
//   - map[string]any (JSON 객체) → SetGroup. 객체 필드 값은 문자열로 강제 변환한다
//     (이미 문자열이면 그대로, 아니면 fmt.Sprint 로 변환). 빈 객체는 SetGroup 의
//     no-op delete 규칙에 따라 저장되지 않는다.
//   - 그 외(숫자/불리언/배열/null) → fmt.Sprint 로 문자열 표현을 만들어 Set
//     (데이터 손실 방지). nil 은 빈 문자열로 저장한다.
func restoreMetadataEntry(metadata Metadata, key string, value any) {
	switch v := value.(type) {
	case string:
		metadata.Set(key, v)
	case map[string]any:
		fields := make(map[string]string, len(v))
		for fk, fv := range v {
			if s, ok := fv.(string); ok {
				fields[fk] = s
			} else if fv == nil {
				fields[fk] = ""
			} else {
				fields[fk] = fmt.Sprint(fv)
			}
		}
		// 빈 객체면 SetGroup 의 no-op delete 규칙이 적용된다.
		metadata.SetGroup(key, fields)
	case nil:
		metadata.Set(key, "")
	default:
		metadata.Set(key, fmt.Sprint(v))
	}
}
