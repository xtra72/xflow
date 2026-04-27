package message

import (
	"encoding/json"
	"fmt"
	"time"
)

// jsonMessage 는 JSON 직렬화/역직렬화에 사용되는 중간 구조체이다.
type jsonMessage struct {
	ID             string             `json:"id"`
	Timestamp      time.Time          `json:"timestamp"`
	Payload        map[string]any     `json:"payload"`
	Metadata       map[string]string  `json:"metadata"`
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
		ID:             m.id,
		Timestamp:      m.timestamp,
		Payload:        m.payload.ToMap(),
		Metadata:       m.metadata.All(),
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
	metadata := NewMetadata()
	for k, v := range jm.Metadata {
		metadata.Set(k, v)
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
