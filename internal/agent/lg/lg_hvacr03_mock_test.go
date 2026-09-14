package lg

import (
	"context"
	"fmt"
	"sync"
)

// ---------------------------------------------------------------------------
// mock Modbus 게이트웨이
//
// SPEC-LG-HVACR-003 § 4 시험 전략. 실제 PMBUSB00A 처럼 4개 레지스터 영역을 보유하고
// FC01/02/03/04/05/06/16 에 응답한다. 하드웨어 없이 폴링·제어 전 경로를 검증한다.
// ---------------------------------------------------------------------------

// mockGateway 는 PMBUSB00A 게이트웨이를 흉내 내는 in-memory Modbus 슬레이브이다.
type mockGateway struct {
	mu sync.Mutex

	// 레지스터 저장소 (0-base 프로토콜 주소로 인덱싱)
	coils    [pmbusScanBitCount]bool
	discrete [pmbusScanBitCount]bool
	holding  [pmbusMaxUnits * pmbusHoldingBlockSize]uint16
	input    [pmbusMaxUnits * pmbusHoldingBlockSize]uint16

	// 관측 기록
	requests   [][]byte // 수신한 PDU 전체
	writeAddrs []uint16 // 쓰기 요청의 대상 주소

	// 주입 가능한 동작
	connectErr  error                                  // Connect 가 반환할 에러
	failNext    int                                    // 다음 N 회 트랜잭션을 실패시킨다
	failErr     error                                  // failNext 가 반환할 에러
	interceptor func(pdu []byte) ([]byte, error, bool) // true 면 기본 처리를 건너뛴다
	connected   bool
	closed      bool

	// lockedItems 는 잠금으로 쓰기가 무시되는 Holding 항목이다 (주소 → true).
	// 문서 §6.2-2 의 "잠금 코일이 걸린 항목은 쓰기가 조용히 무시된다"를 재현한다.
	lockedHolding map[uint16]bool
	lockedCoils   map[uint16]bool
}

func newMockGateway() *mockGateway {
	return &mockGateway{
		lockedHolding: make(map[uint16]bool),
		lockedCoils:   make(map[uint16]bool),
	}
}

// setUnitConnected 는 실내기 N 의 연결 비트를 설정한다.
func (g *mockGateway) setUnitConnected(n uint16, connected bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.discrete[pmbusScanBitIndex(n, pmbusDiscreteConnected)] = connected
}

// setDiscreteBit 는 실내기 N 의 Discrete 항목을 설정한다.
func (g *mockGateway) setDiscreteBit(n, item uint16, v bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.discrete[pmbusScanBitIndex(n, item)] = v
}

// setCoil 은 실내기 N 의 코일 항목을 설정한다.
func (g *mockGateway) setCoil(n, item uint16, v bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.coils[pmbusCoilAddr(n, item)] = v
}

// setHolding 은 실내기 N 의 Holding 항목을 설정한다.
func (g *mockGateway) setHolding(n, item, v uint16) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.holding[pmbusHoldingAddr(n, item)] = v
}

// setInput 은 실내기 N 의 Input 항목을 설정한다.
func (g *mockGateway) setInput(n, item, v uint16) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.input[pmbusInputAddr(n, item)] = v
}

// getHolding 은 실내기 N 의 Holding 항목을 읽는다.
func (g *mockGateway) getHolding(n, item uint16) uint16 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.holding[pmbusHoldingAddr(n, item)]
}

// getCoil 은 실내기 N 의 코일 항목을 읽는다.
func (g *mockGateway) getCoil(n, item uint16) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.coils[pmbusCoilAddr(n, item)]
}

// lockHolding 은 Holding 항목을 잠가 쓰기가 무시되게 한다.
func (g *mockGateway) lockHolding(n, item uint16) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lockedHolding[pmbusHoldingAddr(n, item)] = true
}

// requestCount 는 수신한 트랜잭션 수를 반환한다.
func (g *mockGateway) requestCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.requests)
}

// requestsWithFC 는 특정 function code 의 요청만 반환한다.
func (g *mockGateway) requestsWithFC(fc byte) [][]byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out [][]byte
	for _, r := range g.requests {
		if len(r) > 0 && r[0] == fc {
			cp := make([]byte, len(r))
			copy(cp, r)
			out = append(out, cp)
		}
	}
	return out
}

// resetRequests 는 관측 기록을 비운다.
func (g *mockGateway) resetRequests() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = nil
	g.writeAddrs = nil
}

// ---------------------------------------------------------------------------
// modbus.ModbusTransport 구현
// ---------------------------------------------------------------------------

func (g *mockGateway) Connect(_ context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.connectErr != nil {
		return g.connectErr
	}
	g.connected = true
	return nil
}

func (g *mockGateway) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.connected = false
	g.closed = true
	return nil
}

func (g *mockGateway) IsConnected() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.connected
}

func (g *mockGateway) SendAndReceive(_ context.Context, _ byte, pdu []byte) ([]byte, error) {
	g.mu.Lock()

	cp := make([]byte, len(pdu))
	copy(cp, pdu)
	g.requests = append(g.requests, cp)

	if g.failNext > 0 {
		g.failNext--
		err := g.failErr
		g.mu.Unlock()
		if err == nil {
			err = fmt.Errorf("mock gateway: injected failure")
		}
		return nil, err
	}

	interceptor := g.interceptor
	g.mu.Unlock()

	if interceptor != nil {
		if resp, err, handled := interceptor(cp); handled {
			return resp, err
		}
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	return g.handleLocked(cp)
}

// handleLocked 는 PDU 를 처리한다. 호출 전제: g.mu 보유.
func (g *mockGateway) handleLocked(pdu []byte) ([]byte, error) {
	if len(pdu) < 5 {
		return nil, fmt.Errorf("mock gateway: short pdu")
	}
	fc := pdu[0]
	addr := uint16(pdu[1])<<8 | uint16(pdu[2])

	switch fc {
	case pmbusFCReadCoils:
		qty := uint16(pdu[3])<<8 | uint16(pdu[4])
		return buildMockBitResponse(fc, g.coils[:], addr, qty)
	case pmbusFCReadDiscreteInputs:
		qty := uint16(pdu[3])<<8 | uint16(pdu[4])
		return buildMockBitResponse(fc, g.discrete[:], addr, qty)
	case pmbusFCReadHolding:
		qty := uint16(pdu[3])<<8 | uint16(pdu[4])
		return buildMockRegResponse(fc, g.holding[:], addr, qty)
	case pmbusFCReadInput:
		qty := uint16(pdu[3])<<8 | uint16(pdu[4])
		return buildMockRegResponse(fc, g.input[:], addr, qty)

	case pmbusFCWriteSingleCoil:
		val := uint16(pdu[3])<<8 | uint16(pdu[4])
		g.writeAddrs = append(g.writeAddrs, addr)
		if int(addr) >= len(g.coils) {
			return []byte{fc | 0x80, 0x02}, nil // ILLEGAL_DATA_ADDRESS
		}
		if !g.lockedCoils[addr] {
			g.coils[addr] = val == pmbusCoilOn
		}
		// 잠금 상태여도 에코는 정상으로 돌아온다 — 무시는 조용히 일어난다.
		return append([]byte{fc}, pdu[1:5]...), nil

	case pmbusFCWriteSingleReg:
		val := uint16(pdu[3])<<8 | uint16(pdu[4])
		g.writeAddrs = append(g.writeAddrs, addr)
		if int(addr) >= len(g.holding) {
			return []byte{fc | 0x80, 0x02}, nil
		}
		if !g.lockedHolding[addr] {
			g.holding[addr] = val
		}
		return append([]byte{fc}, pdu[1:5]...), nil

	case pmbusFCWriteMultipleRegs:
		qty := int(uint16(pdu[3])<<8 | uint16(pdu[4]))
		if len(pdu) < 6+qty*2 {
			return nil, fmt.Errorf("mock gateway: short fc16 pdu")
		}
		g.writeAddrs = append(g.writeAddrs, addr)
		for i := 0; i < qty; i++ {
			target := addr + uint16(i)
			if int(target) >= len(g.holding) {
				return []byte{fc | 0x80, 0x02}, nil
			}
			if g.lockedHolding[target] {
				continue
			}
			g.holding[target] = uint16(pdu[6+i*2])<<8 | uint16(pdu[7+i*2])
		}
		return []byte{fc, pdu[1], pdu[2], pdu[3], pdu[4]}, nil

	default:
		return []byte{fc | 0x80, 0x01}, nil // ILLEGAL_FUNCTION
	}
}

func buildMockBitResponse(fc byte, store []bool, addr, qty uint16) ([]byte, error) {
	if int(addr)+int(qty) > len(store) {
		return []byte{fc | 0x80, 0x02}, nil
	}
	byteCount := (int(qty) + 7) / 8
	data := make([]byte, byteCount)
	for i := 0; i < int(qty); i++ {
		if store[int(addr)+i] {
			data[i/8] |= 1 << (uint(i) % 8)
		}
	}
	return append([]byte{fc, byte(byteCount)}, data...), nil
}

func buildMockRegResponse(fc byte, store []uint16, addr, qty uint16) ([]byte, error) {
	if int(addr)+int(qty) > len(store) {
		return []byte{fc | 0x80, 0x02}, nil
	}
	data := make([]byte, 0, int(qty)*2)
	for i := 0; i < int(qty); i++ {
		v := store[int(addr)+i]
		data = append(data, byte(v>>8), byte(v))
	}
	return append([]byte{fc, byte(len(data))}, data...), nil
}
