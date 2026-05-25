package century

import (
	"fmt"
	"sync"
	"time"
)

// CenturyDevice 는 Century 회선상 관측된 단일 indoor unit 디바이스이다 (REQ-CENTURY-013).
//
// 키는 sub_dev_id 이며, 자동 발견된 디바이스는 Source="auto", 설정으로 사전 등록된 디바이스는
// Source="config" 로 표기한다. State 는 마지막으로 수신된 디코딩 메시지의 스냅샷을 보관한다.
type CenturyDevice struct {
	// SubDevID 는 디바이스의 sub_dev_id (payload[0]) 이다.
	SubDevID byte

	// Label 은 사용자에게 표시되는 디바이스 이름이다 (기본 "indoor-<hex>").
	Label string

	// Source 는 디바이스 등록 출처를 표시한다 ("auto" / "config").
	Source string

	// Online 은 마지막 통신 이후 OfflineTimeout 내인지 여부이다.
	Online bool

	// LastSeen 은 마지막으로 디바이스의 frame 이 관측된 시각이다.
	LastSeen time.Time

	// ErrorCount 는 누적 디코딩 에러 횟수이다 (REQ-CENTURY-024 의 통계).
	ErrorCount int

	// State 는 디코딩된 최신 메시지의 스냅샷이다.
	State *CenturyDeviceState

	mu sync.RWMutex
}

// CenturyDeviceState 는 디바이스의 마지막 알려진 디코딩 메시지를 register 별로 보관한다 (REQ-CENTURY-015).
//
// 각 필드는 nil 일 수 있으며, 해당 register 의 메시지를 아직 한 번도 받지 않았음을 의미한다.
// LastFrameAt 은 이 디바이스의 모든 register 에 걸친 가장 최근 frame 의 수신 시각이다.
type CenturyDeviceState struct {
	Reg02       *Reg02Decoded
	Reg03       *Reg03Decoded
	Reg04Read   *Reg04ReadDecoded
	Reg04Write  *Reg04WriteDecoded
	LastFrameAt time.Time
}

// NewCenturyDevice 는 주어진 sub_dev_id 의 새 CenturyDevice 를 생성한다.
//
// source 는 "auto" 또는 "config" 이어야 한다.
// 생성 직후 Online=true 로 설정되며, LastSeen 은 now 로 초기화된다.
func NewCenturyDevice(subDevID byte, source string, now time.Time) *CenturyDevice {
	return &CenturyDevice{
		SubDevID: subDevID,
		Label:    fmt.Sprintf("indoor-%02x", subDevID),
		Source:   source,
		Online:   true,
		LastSeen: now,
		State:    &CenturyDeviceState{},
	}
}

// Touch 는 디바이스의 LastSeen 을 갱신하고 Online 을 true 로 설정한다.
//
// 호출 패턴: captureLoop 이 디바이스 frame 을 수신할 때마다 호출한다.
func (d *CenturyDevice) Touch(now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.LastSeen = now
	d.Online = true
}

// IsStale 은 디바이스가 stale 상태 (LastSeen 으로부터 timeout 초과) 인지 여부를 반환한다.
//
// 경계 조건: now - LastSeen > timeout 이면 stale.
// now - LastSeen == timeout 는 stale 이 아니다 (REQ-CENTURY-014 의 "초과" 의미).
func (d *CenturyDevice) IsStale(now time.Time, timeout time.Duration) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return now.Sub(d.LastSeen) > timeout
}

// Update 는 디코딩된 메시지를 device state 에 통합한다 (REQ-CENTURY-013).
//
// decoded 는 Decode() 가 반환하는 5가지 타입 중 하나이다:
//   - *Reg02Decoded, *Reg03Decoded, *Reg04ReadDecoded, *Reg04WriteDecoded
//   - *ACKDecoded (state 변경 없이 LastFrameAt 만 갱신)
//
// 인식되지 않는 타입은 무시한다. 호출자는 Touch 도 함께 호출해야 한다.
func (d *CenturyDevice) Update(decoded any, frameAt time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch m := decoded.(type) {
	case *Reg02Decoded:
		d.State.Reg02 = m
	case *Reg03Decoded:
		d.State.Reg03 = m
	case *Reg04ReadDecoded:
		d.State.Reg04Read = m
	case *Reg04WriteDecoded:
		d.State.Reg04Write = m
	case *ACKDecoded:
		// ACK 는 register 별 슬롯에 영향 없음 — LastFrameAt 만 갱신.
	}
	d.State.LastFrameAt = frameAt
}

// IncrementError 는 디바이스의 ErrorCount 를 1 증가시킨다.
func (d *CenturyDevice) IncrementError() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ErrorCount++
}

// CenturyDeviceSnapshot 은 CenturyDevice 의 lock-free 읽기 전용 스냅샷이다.
//
// State.Reg02 등 포인터 필드는 원본과 공유된다 (디코딩 메시지는 immutable 로 가정).
// 호출자는 sub-필드를 수정해서는 안 된다.
type CenturyDeviceSnapshot struct {
	SubDevID   byte
	Label      string
	Source     string
	Online     bool
	LastSeen   time.Time
	ErrorCount int
	State      *CenturyDeviceState
}

// Snapshot 은 현재 디바이스 상태의 읽기 전용 사본을 반환한다.
func (d *CenturyDevice) Snapshot() CenturyDeviceSnapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()
	stateCopy := *d.State
	return CenturyDeviceSnapshot{
		SubDevID:   d.SubDevID,
		Label:      d.Label,
		Source:     d.Source,
		Online:     d.Online,
		LastSeen:   d.LastSeen,
		ErrorCount: d.ErrorCount,
		State:      &stateCopy,
	}
}
