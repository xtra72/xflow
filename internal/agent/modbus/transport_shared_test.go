package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// M3 — F3 공유 트랜스포트 참조 카운팅 + 직렬화 (REQ-MODBUS-008-03, AC-05/AC-06(b))
// ---------------------------------------------------------------------------

// TestSharedTransport_RefCount_LastReferenceClose 는 참조 카운팅 마지막-참조 close 를 검증한다.
// 두 디바이스가 공유하는 연결에서, 하나가 close 되어도(참조 잔존) 하부 연결은 유지되고,
// 마지막 참조가 close 될 때만 실제 하부 close 가 수행된다(AC-06(b)/AC-08).
func TestSharedTransport_RefCount_LastReferenceClose(t *testing.T) {
	inner := &mockModbusTransport{connected: true}
	st := newSharedTransport(inner)

	// 두 디바이스가 참조를 획득(그룹 크기 2).
	st.addRef()
	st.addRef()
	require.Equal(t, 2, st.refCount())

	// 첫 close: 마지막 참조가 아니므로 하부는 닫히지 않는다.
	require.NoError(t, st.Close())
	assert.Equal(t, 1, st.refCount())
	assert.Equal(t, 0, inner.closeCnt, "마지막 참조가 아니면 하부 close 가 호출되지 않아야 한다")
	assert.True(t, inner.IsConnected(), "잔여 참조가 있으면 연결이 유지되어야 한다")

	// 두 번째(마지막) close: 하부가 실제로 닫힌다.
	require.NoError(t, st.Close())
	assert.Equal(t, 0, st.refCount())
	assert.Equal(t, 1, inner.closeCnt, "마지막 참조 close 에서만 하부 close 가 1회 수행되어야 한다")
	assert.False(t, inner.IsConnected())

	// 과다 close: no-op(하부 close 중복 호출 없음).
	require.NoError(t, st.Close())
	assert.Equal(t, 1, inner.closeCnt, "과다 close 는 하부 close 를 반복하지 않아야 한다")
}

// TestSharedTransport_RemovedDeviceDoesNotAbortOthers 는 공유 연결을 쓰는 한 디바이스의 제거
// (close)가 동일 연결을 쓰는 다른 디바이스의 트랜잭션을 중단시키지 않음을 검증한다(AC-06(b)).
func TestSharedTransport_RemovedDeviceDoesNotAbortOthers(t *testing.T) {
	inner := &mockModbusTransport{connected: true, response: buildFC03Response(0, 1, 2)}
	st := newSharedTransport(inner)
	st.addRef() // devA
	st.addRef() // devB

	// devA 제거를 모사(마지막 참조 아님) → 하부 연결 유지.
	require.NoError(t, st.Close())
	require.True(t, st.IsConnected(), "잔여 참조가 있으면 공유 연결은 유지되어야 한다")

	// devB 의 트랜잭션은 정상 수행되어야 한다(중단 없음).
	resp, err := st.SendAndReceive(context.Background(), 1, []byte{FC03ReadHoldingRegisters, 0, 0, 0, 2})
	require.NoError(t, err)
	assert.NotEmpty(t, resp)
}

// TestSharedTransport_ConnectDelegatesIdempotent 는 여러 디바이스가 각자 Connect 를 호출해도
// 하부 Connect 는 멱등하게 위임됨을 검증한다.
func TestSharedTransport_ConnectDelegatesIdempotent(t *testing.T) {
	inner := &mockModbusTransport{}
	st := newSharedTransport(inner)
	st.addRef()
	st.addRef()

	require.NoError(t, st.Connect(context.Background()))
	require.NoError(t, st.Connect(context.Background()))
	assert.True(t, st.IsConnected())
	// mock 은 매 Connect 호출을 카운트하지만 실제 연결 상태는 멱등하게 유지된다.
	assert.GreaterOrEqual(t, inner.connectCnt, 1)
}

// concurrencyTrackingTransport 는 SendAndReceive 의 최대 동시 진입 수를 추적하는 테스트용
// 트랜스포트이다. 하부가 SendAndReceive 전체를 직렬화하는지(turnaround mutex 준거) 검증한다.
type concurrencyTrackingTransport struct {
	mu         sync.Mutex // SendAndReceive 를 직렬화하는 턴어라운드 락(하부 트랜스포트 모사)
	inFlight   atomic.Int32
	maxInFlile atomic.Int32
	calls      atomic.Int32
	connected  atomic.Bool
}

func (c *concurrencyTrackingTransport) Connect(_ context.Context) error {
	c.connected.Store(true)
	return nil
}

func (c *concurrencyTrackingTransport) SendAndReceive(_ context.Context, _ byte, _ []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cur := c.inFlight.Add(1)
	// 관측된 최대 동시 진입 수 갱신.
	for {
		prev := c.maxInFlile.Load()
		if cur <= prev || c.maxInFlile.CompareAndSwap(prev, cur) {
			break
		}
	}
	c.calls.Add(1)
	c.inFlight.Add(-1)
	return []byte{0x03, 0x02, 0x00, 0x00}, nil
}

func (c *concurrencyTrackingTransport) Close() error      { c.connected.Store(false); return nil }
func (c *concurrencyTrackingTransport) IsConnected() bool { return c.connected.Load() }

// TestSharedTransport_SendAndReceiveSerialized 는 공유 트랜스포트를 통한 동시 요청이 하부의
// 직렬화(turnaround mutex 준거)로 인해 한 번에 하나씩만 수행됨을 검증한다(AC-05, -race).
func TestSharedTransport_SendAndReceiveSerialized(t *testing.T) {
	inner := &concurrencyTrackingTransport{}
	inner.connected.Store(true)
	st := newSharedTransport(inner)
	st.addRef()
	st.addRef()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = st.SendAndReceive(context.Background(), 1, []byte{FC03ReadHoldingRegisters, 0, 0, 0, 1})
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(goroutines), inner.calls.Load(), "모든 요청이 수행되어야 한다")
	assert.Equal(t, int32(1), inner.maxInFlile.Load(), "공유 연결 접근은 한 번에 하나로 직렬화되어야 한다(AC-05)")
}

// ---------------------------------------------------------------------------
// AC-05 end-to-end — 실제 하부 트랜스포트(ModbusTCPTransport)로 직렬화 검증
// ---------------------------------------------------------------------------
//
// TestSharedTransport_SendAndReceiveSerialized 는 mock(자체 mu 로 직렬화)로 위임을 증명하지만,
// 실제 트랜스포트 경계에서 두 디바이스가 하나의 하부 연결을 동시에 구동해도 접근이 직렬화됨을
// 보이지는 못한다. 이 테스트는 net.Pipe 로 구동되는 진짜 ModbusTCPTransport 를 공유 래퍼로 감싸,
// 여러 goroutine(2 디바이스 부하)이 동시에 SendAndReceive 를 호출할 때:
//   - 실제 하부 conn 경계에서 관측된 최대 동시 진입이 1(트랜스포트 t.mu 직렬화)임을,
//   - 모든 응답이 자기 호출자에게 정확히 라우팅됨(크로스토크/응답 뒤섞임 없음)을 검증한다.
// (하드웨어 불필요, -race 통과.)

// concurrencyTrackingConn 은 net.Conn 을 감싸 Read/Write 접근의 동시 진입 최대치를 추적한다.
// 실제 ModbusTCPTransport 가 SendAndReceive 전체를 t.mu 로 직렬화하면 이 하부 conn 경계에서는
// 어떤 두 goroutine 도 동시에 I/O 하지 않으므로 최대 동시 진입이 1로 관측된다.
type concurrencyTrackingConn struct {
	net.Conn
	inFlight atomic.Int32
	maxSeen  atomic.Int32
}

func (c *concurrencyTrackingConn) track() {
	cur := c.inFlight.Add(1)
	for {
		prev := c.maxSeen.Load()
		if cur <= prev || c.maxSeen.CompareAndSwap(prev, cur) {
			break
		}
	}
}

func (c *concurrencyTrackingConn) Read(b []byte) (int, error) {
	c.track()
	defer c.inFlight.Add(-1)
	return c.Conn.Read(b)
}

func (c *concurrencyTrackingConn) Write(b []byte) (int, error) {
	c.track()
	defer c.inFlight.Add(-1)
	return c.Conn.Write(b)
}

// buildEchoUnitResponse 는 요청 unitID 를 응답 데이터(1 레지스터)에 실어 회신하는 FC03 응답
// ADU 를 만든다. 호출자는 ADU-stripped 응답 PDU 의 마지막 바이트로 자신의 요청이 올바르게
// 라우팅됐는지(크로스토크 없음) 검증할 수 있다.
func buildEchoUnitResponse(txID uint16, unitID byte) []byte {
	byteCount := byte(2)                 // 1 레지스터(2바이트)
	length := uint16(3 + int(byteCount)) // unitID(1) + FC(1) + byteCount(1) + data(2)
	resp := make([]byte, 0, MBAPHeaderSize+4)
	resp = append(resp,
		byte(txID>>8), byte(txID), // Transaction ID (에코)
		0x00, 0x00, // Protocol ID
		byte(length>>8), byte(length), // Length
		unitID,                   // Unit ID
		FC03ReadHoldingRegisters, // Function Code
		byteCount,                // Byte Count
		0x00, unitID,             // Data: 마지막 바이트에 요청 unitID 를 에코하여 라우팅 검증
	)
	return resp
}

// TestSharedTransport_RealTransport_SerializedEndToEnd 는 실제 ModbusTCPTransport 를 공유하는
// 두 디바이스의 동시 요청이 하부 conn 경계에서 직렬화되고(최대 동시 진입 1) 각 응답이 정확히
// 라우팅됨을 검증한다(AC-05 end-to-end, -race).
func TestSharedTransport_RealTransport_SerializedEndToEnd(t *testing.T) {
	logger := discardLogger()
	tr := NewModbusTCPTransport("10.9.9.9", 502, 2*time.Second, logger)

	server, rawClient := net.Pipe()
	trackedClient := &concurrencyTrackingConn{Conn: rawClient}
	// 실제 트랜스포트가 이 추적 conn 을 통해 I/O 하도록 주입(연결 상태를 연결됨으로 고정).
	tr.conn = trackedClient
	tr.connected = true
	defer rawClient.Close()
	defer server.Close()

	const requests = 32
	// 서버 읽기가 영원히 막히지 않도록 데드라인을 건다(실패 경로 안전망).
	_ = server.SetDeadline(time.Now().Add(10 * time.Second))

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		for i := 0; i < requests; i++ {
			hdr := make([]byte, MBAPHeaderSize)
			if _, err := io.ReadFull(server, hdr); err != nil {
				return
			}
			length := binary.BigEndian.Uint16(hdr[4:6])
			pduBuf := make([]byte, int(length)-1) // UnitID 는 헤더에 포함
			if _, err := io.ReadFull(server, pduBuf); err != nil {
				return
			}
			// 응답 지연으로 동시 진입 탐지 창을 넓힌다(직렬화가 깨지면 conn 경계 중첩이 드러남).
			time.Sleep(time.Millisecond)
			txID := binary.BigEndian.Uint16(hdr[0:2])
			unitID := hdr[6]
			if _, err := server.Write(buildEchoUnitResponse(txID, unitID)); err != nil {
				return
			}
		}
	}()

	// 2개 디바이스가 하나의 실제 하부 트랜스포트를 공유한다.
	st := newSharedTransport(tr)
	st.addRef() // devA
	st.addRef() // devB

	var wg sync.WaitGroup
	errs := make(chan error, requests)
	wg.Add(requests)
	for i := 0; i < requests; i++ {
		unitID := byte(1 + (i % 2)) // devA=unit1, devB=unit2 교대 부하
		go func(uid byte) {
			defer wg.Done()
			reqPDU := buildReadPDU(FC03ReadHoldingRegisters, 0, 1)
			respPDU, err := st.SendAndReceive(context.Background(), uid, reqPDU)
			if err != nil {
				errs <- fmt.Errorf("unit %d: %w", uid, err)
				return
			}
			// respPDU = [FC=0x03][byteCount=0x02][0x00][unitID]. 마지막 바이트가 자기 요청 unitID 여야 한다.
			if len(respPDU) != 4 || respPDU[3] != uid {
				errs <- fmt.Errorf("unit %d: 응답 라우팅 불일치(크로스토크): %v", uid, respPDU)
			}
		}(unitID)
	}
	wg.Wait()
	close(errs)
	<-serverDone

	for err := range errs {
		t.Errorf("동시 요청 실패: %v", err)
	}
	assert.Equal(t, int32(1), trackedClient.maxSeen.Load(),
		"실제 하부 트랜스포트 경계에서 접근은 한 번에 하나로 직렬화되어야 한다(AC-05)")
}
