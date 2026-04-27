package message

import (
	"time"

	"github.com/google/uuid"
)

// Message 는 xflow 메시지의 공개 인터페이스이다.
type Message interface {
	// ID 는 메시지의 고유 식별자(UUID v4)를 반환한다.
	ID() string
	// Timestamp 는 메시지 생성 시각을 반환한다.
	Timestamp() time.Time
	// Payload 는 메시지 페이로드에 대한 접근자를 반환한다.
	Payload() Payload
	// Metadata 는 메시지 메타데이터에 대한 접근자를 반환한다.
	Metadata() Metadata
	// History 는 변경 이력 슬라이스를 반환한다. 이력 비활성화 시 nil을 반환한다.
	History() []ChangeRecord
	// HistoryEnabled 는 이력 기록 활성화 여부를 반환한다.
	HistoryEnabled() bool
	// Clone 은 새로운 UUID를 가진 독립적인 복사본을 반환한다.
	Clone() Message
	// MarshalJSON 은 메시지를 JSON 바이트로 직렬화한다.
	MarshalJSON() ([]byte, error)
}

// config 는 메시지 생성 옵션을 담는 내부 설정 구조체이다.
type config struct {
	historyEnabled bool
	maxHistory     int
	metadata       map[string]string
	payload        Payload
}

// Option 은 메시지 생성 시 적용할 옵션 함수 타입이다.
type Option func(*config)

// WithHistory 는 변경 이력 기록 활성화 여부를 설정한다.
func WithHistory(enabled bool) Option {
	return func(c *config) {
		c.historyEnabled = enabled
	}
}

// WithMaxHistory 는 최대 이력 보관 개수를 설정한다. 기본값은 100이다.
func WithMaxHistory(n int) Option {
	return func(c *config) {
		c.maxHistory = n
	}
}

// WithMetadata 는 메시지 생성 시 메타데이터 키-값 쌍을 추가한다.
func WithMetadata(key, value string) Option {
	return func(c *config) {
		if c.metadata == nil {
			c.metadata = make(map[string]string)
		}
		c.metadata[key] = value
	}
}

// WithPayload 는 메시지 생성 시 커스텀 페이로드를 주입한다.
func WithPayload(p Payload) Option {
	return func(c *config) {
		c.payload = p
	}
}

// defaultMessage 는 Message 인터페이스의 기본 구현체이다.
type defaultMessage struct {
	id             string
	timestamp      time.Time
	payload        Payload
	metadata       Metadata
	historyEnabled bool
	history        []ChangeRecord
	maxHistory     int
	nodeID         string
}

// New 는 새 Message 인스턴스를 생성하여 반환한다.
func New(opts ...Option) Message {
	// 기본 설정
	cfg := &config{
		maxHistory: 100,
	}

	// 옵션 적용
	for _, opt := range opts {
		opt(cfg)
	}

	msg := &defaultMessage{
		id:             uuid.New().String(),
		timestamp:      time.Now(),
		historyEnabled: cfg.historyEnabled,
		maxHistory:     cfg.maxHistory,
	}

	// 페이로드 설정
	if cfg.payload != nil {
		msg.payload = cfg.payload
	} else {
		msg.payload = NewPayload()
	}

	// 메타데이터 설정
	msg.metadata = NewMetadata()
	if cfg.metadata != nil {
		for k, v := range cfg.metadata {
			msg.metadata.Set(k, v)
		}
	}

	// 이력 래핑
	if cfg.historyEnabled {
		msg.history = make([]ChangeRecord, 0)
		msg.payload = newHistoryPayload(msg.payload, &msg.history, msg.nodeID, msg.maxHistory)
		msg.metadata = newHistoryMetadata(msg.metadata, &msg.history, msg.nodeID, msg.maxHistory)
	}

	return msg
}

func (m *defaultMessage) ID() string {
	return m.id
}

func (m *defaultMessage) Timestamp() time.Time {
	return m.timestamp
}

func (m *defaultMessage) Payload() Payload {
	return m.payload
}

func (m *defaultMessage) Metadata() Metadata {
	return m.metadata
}

func (m *defaultMessage) History() []ChangeRecord {
	if !m.historyEnabled {
		return nil
	}
	// 이력의 복사본 반환
	cp := make([]ChangeRecord, len(m.history))
	copy(cp, m.history)
	return cp
}

func (m *defaultMessage) HistoryEnabled() bool {
	return m.historyEnabled
}

// Clone 은 새 UUID를 가진 독립적인 메시지 복사본을 생성한다.
func (m *defaultMessage) Clone() Message {
	cloned := &defaultMessage{
		id:             uuid.New().String(),
		timestamp:      m.timestamp,
		historyEnabled: m.historyEnabled,
		maxHistory:     m.maxHistory,
		nodeID:         m.nodeID,
	}

	// 페이로드와 메타데이터 깊은 복사
	clonedPayload := m.payload.Clone()
	clonedMetadata := m.metadata.Clone()

	cloned.payload = clonedPayload
	cloned.metadata = clonedMetadata

	// 이력 활성화 시 새로운 빈 이력으로 다시 래핑
	if m.historyEnabled {
		cloned.history = make([]ChangeRecord, 0)
		cloned.payload = newHistoryPayload(cloned.payload, &cloned.history, cloned.nodeID, cloned.maxHistory)
		cloned.metadata = newHistoryMetadata(cloned.metadata, &cloned.history, cloned.nodeID, cloned.maxHistory)
	}

	return cloned
}
