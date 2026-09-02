package samsung

// SPEC-HVACR-SYNC-001 Module 2: 서버측 Mirror 입력 (설계 2a).
//
// mirrorTransport 는 NasaTransport 인터페이스의 채널 급전형 구현체이다
// (transport_type:"mirror-mqtt"/"mirror-message", REQ-SYNC-001-02-01). MQTT 업링크 구독
// 콜백(mirror-mqtt) 또는 플로우 노드(mirror-message)가 역직렬화한
// 프레임 바이트를 Feed 로 내부 채널에 넣으면, receiveLoop 의 Receive 가 이를 반환한다.
// 기존 receiveLoop → frameScanner → Decode → handleMessage 경로가 무변경 재사용된다.
//
// Available() 은 MQTT 연결 상태를 반영한다(REQ-SYNC-001-07-04). 로컬 RS-485 가 없으므로
// Send 는 no-op 이다 — 서버 제어는 Process 단계에서 다운링크로 가로채여 로컬 트랜스포트에
// 도달하지 않는다(REQ-SYNC-001-02-03).

import (
	"io"
	"sync"
	"sync/atomic"
)

// mirrorFeedBuffer 는 Feed 채널의 버퍼 크기이다. 업링크 스트림의 짧은 버스트를
// 흡수한다. 초과 시 Feed 는 블록되지 않고 프레임을 드롭한다(수신 루프 비차단 우선).
const mirrorFeedBuffer = 1024

// mirrorTransport 는 채널 급전형 NasaTransport 이다.
type mirrorTransport struct {
	frameCh   chan []byte
	available atomic.Bool
	closeCh   chan struct{}
	closeOnce sync.Once

	mu      sync.Mutex
	pending []byte // buf 보다 큰 프레임의 잔여 바이트

	droppedFrames atomic.Int64 // 버퍼 포화로 드롭된 프레임 수
}

// 컴파일 타임 인터페이스 체크
var _ NasaTransport = (*mirrorTransport)(nil)

// newMirrorTransport 는 mirrorTransport 를 생성한다.
func newMirrorTransport() *mirrorTransport {
	return &mirrorTransport{
		frameCh: make(chan []byte, mirrorFeedBuffer),
		closeCh: make(chan struct{}),
	}
}

// Open 은 미러 입력을 사용 가능 상태로 전환한다.
func (m *mirrorTransport) Open() error {
	m.available.Store(true)
	return nil
}

// Close 는 미러 입력을 닫고 블록된 Receive 를 깨운다.
func (m *mirrorTransport) Close() error {
	m.available.Store(false)
	m.closeOnce.Do(func() { close(m.closeCh) })
	return nil
}

// Send 는 no-op 이다. 서버는 로컬 RS-485 가 없으므로 제어를 트랜스포트로 실행하지
// 않는다(REQ-SYNC-001-02-03). 제어는 Process 단계에서 다운링크로 가로채인다.
func (m *mirrorTransport) Send(_ []byte) error {
	return nil
}

// Receive 는 Feed 로 급전된 프레임 바이트를 반환한다. 프레임이 없으면 블록하며,
// Close 시 io.EOF 를 반환한다(receiveLoop 는 이후 stopCh 를 확인해 정상 종료).
func (m *mirrorTransport) Receive(buf []byte) (int, error) {
	// 이전 프레임의 잔여 바이트를 우선 배출한다.
	m.mu.Lock()
	if len(m.pending) > 0 {
		n := copy(buf, m.pending)
		m.pending = m.pending[n:]
		m.mu.Unlock()
		return n, nil
	}
	m.mu.Unlock()

	select {
	case <-m.closeCh:
		return 0, io.EOF
	case frame, ok := <-m.frameCh:
		if !ok {
			return 0, io.EOF
		}
		n := copy(buf, frame)
		if n < len(frame) {
			m.mu.Lock()
			m.pending = append(m.pending, frame[n:]...)
			m.mu.Unlock()
		}
		return n, nil
	}
}

// Available 은 MQTT 연결 상태를 반영한다.
func (m *mirrorTransport) Available() bool {
	return m.available.Load()
}

// setAvailable 은 MQTT 연결 상태 변화를 반영한다(broker 콜백에서 호출).
func (m *mirrorTransport) setAvailable(v bool) {
	m.available.Store(v)
}

// Feed 는 역직렬화된 프레임 바이트를 내부 채널에 넣는다(비차단). 버퍼 포화 시
// 프레임을 드롭하고 카운터를 증가시켜 수신 경로를 차단하지 않는다.
func (m *mirrorTransport) Feed(frame []byte) {
	cp := make([]byte, len(frame))
	copy(cp, frame)
	select {
	case <-m.closeCh:
		return
	case m.frameCh <- cp:
	default:
		m.droppedFrames.Add(1)
	}
}
