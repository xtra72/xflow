package modbus

import (
	"context"
	"sync"
)

// ---------------------------------------------------------------------------
// sharedTransport — 세션 공유 트랜스포트 (참조 카운팅, F3 / REQ-MODBUS-008-03)
// ---------------------------------------------------------------------------
//
// share_session 이 활성일 때 동일 엔드포인트(TCP (host,port) / RTU serial_port)를 대상으로
// 하는 여러 디바이스가 하나의 하부 트랜스포트/연결을 공유한다. 이 래퍼는 그 연결의
// connect/close 소유권을 참조 카운팅으로 관리한다:
//
//   - buildDevices 가 그룹의 각 디바이스마다 addRef() 를 1회 호출하여 참조 수를 세운다.
//   - Connect 는 하부에 위임한다. 하부(TCP/RTU)의 Connect 는 멱등이므로 여러 디바이스가
//     각자 Connect 를 호출해도 실제 연결은 한 번만 수립된다.
//   - Close 는 참조 수를 1 감소시키고, 마지막 참조(0)일 때만 하부 Close 를 수행한다
//     (마지막-참조 close, AC-06(b)/AC-08). 따라서 공유 디바이스 하나가 제거·정지되어도
//     다른 디바이스가 아직 참조 중이면 연결은 유지된다.
//   - SendAndReceive/IsConnected 는 하부에 위임한다. 접근 직렬화는 하부가 담당한다:
//     TCP 는 자체 mu, RTU 는 turnaround mutex(transport_rtu.go)로 SendAndReceive 전체를
//     직렬화하므로, 동일 연결을 쓰는 여러 디바이스의 요청이 안전하게 직렬화된다(AC-05).
//
// 스레드 안전성: 참조 수(refs)/닫힘 여부(closed)는 mu 로 보호한다. SendAndReceive 등
// 데이터 경로는 하부 트랜스포트가 직접 직렬화하므로 이 래퍼에 추가 락이 필요 없다(-race).
type sharedTransport struct {
	inner  ModbusTransport
	mu     sync.Mutex
	refs   int  // 활성 참조(디바이스) 수. addRef 로 증가, Close 로 감소.
	closed bool // 하부가 이미 실제 close 되었는지(중복 close 방지).
}

// 컴파일 타임 인터페이스 체크
var _ ModbusTransport = (*sharedTransport)(nil)

// newSharedTransport 는 하부 트랜스포트를 감싸는 참조 카운팅 공유 래퍼를 생성한다(refs=0).
func newSharedTransport(inner ModbusTransport) *sharedTransport {
	return &sharedTransport{inner: inner}
}

// addRef 는 공유 참조 수를 1 증가시킨다. buildDevices 가 그룹의 디바이스마다 1회 호출한다.
func (t *sharedTransport) addRef() {
	t.mu.Lock()
	t.refs++
	t.mu.Unlock()
}

// refCount 는 현재 활성 참조 수를 반환한다(테스트/관측용).
func (t *sharedTransport) refCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.refs
}

// setObs 는 프레임 로그 관측성 배선을 하부 트랜스포트에 위임 주입한다(F4, opt-in).
// 하부가 setObs 를 지원하지 않으면(예: mock) no-op 이다.
func (t *sharedTransport) setObs(o *clientObs) {
	if setter, ok := t.inner.(interface{ setObs(*clientObs) }); ok {
		setter.setObs(o)
	}
}

// Connect 는 하부 트랜스포트에 연결을 위임한다(하부 Connect 는 멱등).
func (t *sharedTransport) Connect(ctx context.Context) error {
	return t.inner.Connect(ctx)
}

// SendAndReceive 는 하부 트랜스포트에 위임한다. 직렬화는 하부(TCP mu / RTU turnaround mutex)가
// 담당하므로 동일 연결을 공유하는 여러 디바이스의 동시 요청이 안전하게 직렬화된다(AC-05).
func (t *sharedTransport) SendAndReceive(ctx context.Context, unitID byte, pdu []byte) ([]byte, error) {
	return t.inner.SendAndReceive(ctx, unitID, pdu)
}

// Close 는 참조 수를 1 감소시키고, 마지막 참조일 때만 하부 Close 를 수행한다(마지막-참조 close).
// 이미 하부가 닫혔거나 참조가 남아 있으면 하부 Close 를 호출하지 않아, 한 디바이스의 제거가
// 동일 연결을 쓰는 다른 디바이스의 트랜잭션을 중단시키지 않는다(AC-06(b)/AC-08).
func (t *sharedTransport) Close() error {
	t.mu.Lock()
	if t.refs > 0 {
		t.refs--
	}
	// 아직 참조가 남아 있거나 이미 닫혔으면 하부를 건드리지 않는다.
	if t.refs > 0 || t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()
	return t.inner.Close()
}

// releaseRef 는 참조 수를 1 감소시키고, 마지막 참조(0)이며 아직 닫히지 않았다면 닫아야 할
// 하부 트랜스포트를 반환한다(그 외에는 nil). Close() 와 동일한 마지막-참조 규칙을 공유하되,
// 블로킹 I/O 인 실제 하부 Close() 호출을 이 래퍼 밖으로 위임한다: 참조 감소(래퍼 상태 변경)는
// 호출자의 상위 락(a.mu) 하에서 즉시 끝내되, 반환된 하부 트랜스포트의 Close() 는 락 해제 후
// 실행하도록 하여 in-flight SendAndReceive 완료 대기가 상위 락을 장시간 붙잡지 않게 한다
// (remove_device 지연 close, add 경로의 락-밖 Connect 와 대칭).
func (t *sharedTransport) releaseRef() ModbusTransport {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.refs > 0 {
		t.refs--
	}
	// 아직 참조가 남아 있거나 이미 닫혔으면 하부를 닫지 않는다(마지막-참조 close 규칙 동일).
	if t.refs > 0 || t.closed {
		return nil
	}
	t.closed = true
	return t.inner
}

// IsConnected 는 하부 트랜스포트의 연결 상태를 반환한다.
func (t *sharedTransport) IsConnected() bool {
	return t.inner.IsConnected()
}
