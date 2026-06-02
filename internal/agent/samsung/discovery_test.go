package samsung

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 테스트용 mock 트랜스포트
// ---------------------------------------------------------------------------

// timeoutError 는 net.Error 인터페이스를 만족하는 타임아웃 에러이다.
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

// mockDiscoveryTransport 는 탐색 테스트용 mock 트랜스포트이다.
type mockDiscoveryTransport struct {
	responses [][]byte // Receive 호출 시 순서대로 반환할 응답 프레임
	idx       int      // 현재 응답 인덱스
	sent      [][]byte // Send 로 전송된 데이터 캡처
	mu        sync.Mutex
	sendErr   error // Send 에서 반환할 에러 (nil 이면 성공)
}

func (m *mockDiscoveryTransport) Open() error     { return nil }
func (m *mockDiscoveryTransport) Close() error    { return nil }
func (m *mockDiscoveryTransport) Available() bool { return true }

func (m *mockDiscoveryTransport) Send(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, append([]byte(nil), data...))
	return nil
}

func (m *mockDiscoveryTransport) Receive(buf []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.idx >= len(m.responses) {
		return 0, &timeoutError{}
	}
	n := copy(buf, m.responses[m.idx])
	m.idx++
	return n, nil
}

// encodeTestResponse 는 테스트용 NASA 응답 프레임을 생성하는 헬퍼이다.
func encodeTestResponse(t *testing.T, src, dst NasaAddress, cmd uint16, seq byte, sets []NasaMessageSet) []byte {
	t.Helper()
	proto := NewNasaProtocol()
	msg := &NasaMessage{
		SourceAddr:  src,
		DestAddr:    dst,
		CommandCode: cmd,
		SequenceNum: seq,
		MessageSets: sets,
	}
	frame, err := proto.Encode(msg)
	if err != nil {
		t.Fatalf("encodeTestResponse() Encode 실패: %v", err)
	}
	return frame
}

// ===========================================================================
// isTimeoutErr 테스트
// ===========================================================================

func TestIsTimeoutErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "타임아웃 에러는 true 반환",
			err:  &timeoutError{},
			want: true,
		},
		{
			name: "일반 에러는 false 반환",
			err:  errors.New("some error"),
			want: false,
		},
		{
			name: "nil 에러는 false 반환",
			err:  nil,
			want: false,
		},
		{
			name: "래핑된 타임아웃 에러도 true 반환",
			err:  fmt.Errorf("wrapped: %w", &timeoutError{}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTimeoutErr(tt.err)
			if got != tt.want {
				t.Errorf("isTimeoutErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// ===========================================================================
// readResponses 테스트
// ===========================================================================

func TestReadResponses_Timeout(t *testing.T) {
	// 응답이 없는 mock 트랜스포트로 타임아웃까지 읽기
	transport := &mockDiscoveryTransport{
		responses: nil, // 응답 없음
	}
	proto := NewNasaProtocol()

	messages, err := readResponses(transport, proto, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("readResponses() error = %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("readResponses() 메시지 수 = %d, want 0", len(messages))
	}
}

func TestReadResponses_CollectsMessages(t *testing.T) {
	// 두 개의 유효한 응답 프레임을 반환하는 mock
	resp1 := encodeTestResponse(t,
		NasaAddress{0x10, 0x00, 0x00}, AddrController,
		CmdStandbyResponse, 0x01,
		[]NasaMessageSet{{Index: MsgAddrInfo, Value: []byte{0x10, 0x00, 0x00, 0x00}}},
	)
	resp2 := encodeTestResponse(t,
		NasaAddress{0x10, 0x01, 0x00}, AddrController,
		CmdStandbyResponse, 0x02,
		[]NasaMessageSet{{Index: MsgAddrInfo, Value: []byte{0x10, 0x01, 0x00, 0x00}}},
	)

	transport := &mockDiscoveryTransport{
		responses: [][]byte{resp1, resp2},
	}
	proto := NewNasaProtocol()

	messages, err := readResponses(transport, proto, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("readResponses() error = %v", err)
	}
	if len(messages) != 2 {
		t.Errorf("readResponses() 메시지 수 = %d, want 2", len(messages))
	}
}

func TestReadResponses_SkipsMalformedFrames(t *testing.T) {
	// 잘못된 프레임 + 유효한 프레임
	malformed := []byte{0xFF, 0x00, 0x01, 0x02} // 유효하지 않은 데이터
	valid := encodeTestResponse(t,
		NasaAddress{0x20, 0x00, 0x01}, AddrController,
		CmdNormalRequest, 0x01,
		[]NasaMessageSet{{Index: MsgAddrInfo, Value: []byte{0x20, 0x00, 0x01, 0x00}}},
	)

	transport := &mockDiscoveryTransport{
		responses: [][]byte{malformed, valid},
	}
	proto := NewNasaProtocol()

	messages, err := readResponses(transport, proto, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("readResponses() error = %v", err)
	}
	// 잘못된 프레임은 건너뛰고 유효한 프레임만 수집
	if len(messages) != 1 {
		t.Errorf("readResponses() 메시지 수 = %d, want 1", len(messages))
	}
}

// ===========================================================================
// DiscoverOutdoors 테스트
// ===========================================================================

func TestDiscoverOutdoors_Success(t *testing.T) {
	// 실외기 1대가 C001 응답을 보내는 시나리오
	resp := encodeTestResponse(t,
		NasaAddress{0x10, 0x00, 0x00}, AddrController,
		CmdStandbyResponse, 0x01,
		[]NasaMessageSet{{Index: MsgAddrInfo, Value: []byte{0x10, 0x00, 0x00, 0x00}}},
	)

	transport := &mockDiscoveryTransport{
		responses: [][]byte{resp},
	}
	proto := NewNasaProtocol()
	var seqNum byte = 0x01

	results, err := DiscoverOutdoors(transport, proto, &seqNum, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("DiscoverOutdoors() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("DiscoverOutdoors() 결과 수 = %d, want 1", len(results))
	}

	r := results[0]
	if r.Address != (NasaAddress{0x10, 0x00, 0x00}) {
		t.Errorf("결과 주소 = %v, want 10 00 00", r.Address)
	}
	if r.DeviceType != "HVACR.ODU" {
		t.Errorf("결과 타입 = %q, want \"outdoor\"", r.DeviceType)
	}
	if r.Ready != false {
		t.Errorf("결과 Ready = %v, want false", r.Ready)
	}

	// 시퀀스 번호가 증가했는지 확인
	if seqNum != 0x02 {
		t.Errorf("seqNum = 0x%02X, want 0x02", seqNum)
	}

	// Send 가 호출되었는지 확인
	if len(transport.sent) != 1 {
		t.Fatalf("Send 호출 수 = %d, want 1", len(transport.sent))
	}

	// 전송된 프레임이 C001 명령인지 확인
	sentMsg, err := proto.Decode(transport.sent[0])
	if err != nil {
		t.Fatalf("전송된 프레임 디코딩 실패: %v", err)
	}
	if sentMsg.CommandCode != CmdStandbyRequest {
		t.Errorf("전송된 CMD = 0x%04X, want 0x%04X (CmdStandbyRequest)", sentMsg.CommandCode, CmdStandbyRequest)
	}
	if sentMsg.DestAddr != (NasaAddress{0xB0, 0xFF, 0x10}) {
		t.Errorf("전송된 DA = %v, want B0 FF 10", sentMsg.DestAddr)
	}
}

func TestDiscoverOutdoors_NoResponse(t *testing.T) {
	// 응답이 없는 시나리오 (타임아웃)
	transport := &mockDiscoveryTransport{
		responses: nil,
	}
	proto := NewNasaProtocol()
	var seqNum byte = 0x01

	results, err := DiscoverOutdoors(transport, proto, &seqNum, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("DiscoverOutdoors() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("DiscoverOutdoors() 결과 수 = %d, want 0", len(results))
	}

	// 시퀀스 번호는 여전히 증가해야 함
	if seqNum != 0x02 {
		t.Errorf("seqNum = 0x%02X, want 0x02", seqNum)
	}
}

func TestDiscoverOutdoors_TransportError(t *testing.T) {
	// Send 에서 에러가 발생하는 시나리오
	transport := &mockDiscoveryTransport{
		sendErr: errors.New("transport send failed"),
	}
	proto := NewNasaProtocol()
	var seqNum byte = 0x01

	_, err := DiscoverOutdoors(transport, proto, &seqNum, 50*time.Millisecond)
	if err == nil {
		t.Fatal("DiscoverOutdoors() 에러가 예상되었으나 nil 반환")
	}
}

func TestDiscoverOutdoors_FiltersNonOutdoorResponses(t *testing.T) {
	// 실외기와 실내기 응답이 섞인 시나리오 - 실외기만 반환해야 함
	outdoorResp := encodeTestResponse(t,
		NasaAddress{0x10, 0x00, 0x00}, AddrController,
		CmdStandbyResponse, 0x01,
		[]NasaMessageSet{{Index: MsgAddrInfo, Value: []byte{0x10, 0x00, 0x00, 0x00}}},
	)
	indoorResp := encodeTestResponse(t,
		NasaAddress{0x20, 0x00, 0x01}, AddrController,
		CmdStandbyResponse, 0x02,
		[]NasaMessageSet{{Index: MsgAddrInfo, Value: []byte{0x20, 0x00, 0x01, 0x00}}},
	)

	transport := &mockDiscoveryTransport{
		responses: [][]byte{outdoorResp, indoorResp},
	}
	proto := NewNasaProtocol()
	var seqNum byte = 0x01

	results, err := DiscoverOutdoors(transport, proto, &seqNum, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("DiscoverOutdoors() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("DiscoverOutdoors() 결과 수 = %d, want 1 (실외기만)", len(results))
	}
	if results[0].DeviceType != "HVACR.ODU" {
		t.Errorf("결과 타입 = %q, want \"outdoor\"", results[0].DeviceType)
	}
}

// ===========================================================================
// DiscoverIndoors 테스트
// ===========================================================================

func TestDiscoverIndoors_Success(t *testing.T) {
	// 실내기 1대가 C011 응답을 보내는 시나리오
	resp := encodeTestResponse(t,
		NasaAddress{0x20, 0x00, 0x01}, AddrController,
		CmdNormalRequest, 0x01,
		[]NasaMessageSet{{Index: MsgAddrInfo, Value: []byte{0x20, 0x00, 0x01, 0x00}}},
	)

	transport := &mockDiscoveryTransport{
		responses: [][]byte{resp},
	}
	proto := NewNasaProtocol()
	var seqNum byte = 0x01

	results, err := DiscoverIndoors(transport, proto, &seqNum, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("DiscoverIndoors() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("DiscoverIndoors() 결과 수 = %d, want 1", len(results))
	}

	r := results[0]
	if r.Address != (NasaAddress{0x20, 0x00, 0x01}) {
		t.Errorf("결과 주소 = %v, want 20 00 01", r.Address)
	}
	if r.DeviceType != "HVACR.IDU" {
		t.Errorf("결과 타입 = %q, want \"indoor\"", r.DeviceType)
	}

	// 시퀀스 번호가 증가했는지 확인
	if seqNum != 0x02 {
		t.Errorf("seqNum = 0x%02X, want 0x02", seqNum)
	}

	// 전송된 프레임이 C011 명령인지 확인
	if len(transport.sent) != 1 {
		t.Fatalf("Send 호출 수 = %d, want 1", len(transport.sent))
	}
	sentMsg, err := proto.Decode(transport.sent[0])
	if err != nil {
		t.Fatalf("전송된 프레임 디코딩 실패: %v", err)
	}
	if sentMsg.CommandCode != CmdNormalRequest {
		t.Errorf("전송된 CMD = 0x%04X, want 0x%04X (CmdNormalRequest)", sentMsg.CommandCode, CmdNormalRequest)
	}
	if sentMsg.DestAddr != AddrBroadcastIndoor {
		t.Errorf("전송된 DA = %v, want %v (AddrBroadcastIndoor)", sentMsg.DestAddr, AddrBroadcastIndoor)
	}
}

func TestDiscoverIndoors_NoResponse(t *testing.T) {
	// 응답이 없는 시나리오
	transport := &mockDiscoveryTransport{
		responses: nil,
	}
	proto := NewNasaProtocol()
	var seqNum byte = 0x01

	results, err := DiscoverIndoors(transport, proto, &seqNum, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("DiscoverIndoors() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("DiscoverIndoors() 결과 수 = %d, want 0", len(results))
	}
}

func TestDiscoverIndoors_MultipleUnits(t *testing.T) {
	// 여러 실내기가 응답하는 시나리오
	resp1 := encodeTestResponse(t,
		NasaAddress{0x20, 0x00, 0x01}, AddrController,
		CmdNormalRequest, 0x01,
		[]NasaMessageSet{{Index: MsgAddrInfo, Value: []byte{0x20, 0x00, 0x01, 0x00}}},
	)
	resp2 := encodeTestResponse(t,
		NasaAddress{0x20, 0x00, 0x02}, AddrController,
		CmdNormalRequest, 0x02,
		[]NasaMessageSet{{Index: MsgAddrInfo, Value: []byte{0x20, 0x00, 0x02, 0x00}}},
	)
	resp3 := encodeTestResponse(t,
		NasaAddress{0x20, 0x01, 0x01}, AddrController,
		CmdNormalRequest, 0x03,
		[]NasaMessageSet{{Index: MsgAddrInfo, Value: []byte{0x20, 0x01, 0x01, 0x00}}},
	)

	transport := &mockDiscoveryTransport{
		responses: [][]byte{resp1, resp2, resp3},
	}
	proto := NewNasaProtocol()
	var seqNum byte = 0x01

	results, err := DiscoverIndoors(transport, proto, &seqNum, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("DiscoverIndoors() error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("DiscoverIndoors() 결과 수 = %d, want 3", len(results))
	}

	// 각 결과의 주소 확인
	expectedAddrs := []NasaAddress{
		{0x20, 0x00, 0x01},
		{0x20, 0x00, 0x02},
		{0x20, 0x01, 0x01},
	}
	for i, r := range results {
		if r.Address != expectedAddrs[i] {
			t.Errorf("results[%d].Address = %v, want %v", i, r.Address, expectedAddrs[i])
		}
		if r.DeviceType != "HVACR.IDU" {
			t.Errorf("results[%d].DeviceType = %q, want \"indoor\"", i, r.DeviceType)
		}
	}
}

func TestDiscoverIndoors_TransportError(t *testing.T) {
	// Send 에서 에러가 발생하는 시나리오
	transport := &mockDiscoveryTransport{
		sendErr: errors.New("transport send failed"),
	}
	proto := NewNasaProtocol()
	var seqNum byte = 0x01

	_, err := DiscoverIndoors(transport, proto, &seqNum, 50*time.Millisecond)
	if err == nil {
		t.Fatal("DiscoverIndoors() 에러가 예상되었으나 nil 반환")
	}
}

func TestDiscoverIndoors_FiltersNonIndoorResponses(t *testing.T) {
	// 실내기와 실외기 응답이 섞인 시나리오 - 실내기만 반환해야 함
	indoorResp := encodeTestResponse(t,
		NasaAddress{0x20, 0x00, 0x01}, AddrController,
		CmdNormalRequest, 0x01,
		[]NasaMessageSet{{Index: MsgAddrInfo, Value: []byte{0x20, 0x00, 0x01, 0x00}}},
	)
	outdoorResp := encodeTestResponse(t,
		NasaAddress{0x10, 0x00, 0x00}, AddrController,
		CmdStandbyResponse, 0x02,
		[]NasaMessageSet{{Index: MsgAddrInfo, Value: []byte{0x10, 0x00, 0x00, 0x00}}},
	)

	transport := &mockDiscoveryTransport{
		responses: [][]byte{indoorResp, outdoorResp},
	}
	proto := NewNasaProtocol()
	var seqNum byte = 0x01

	results, err := DiscoverIndoors(transport, proto, &seqNum, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("DiscoverIndoors() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("DiscoverIndoors() 결과 수 = %d, want 1 (실내기만)", len(results))
	}
	if results[0].DeviceType != "HVACR.IDU" {
		t.Errorf("결과 타입 = %q, want \"indoor\"", results[0].DeviceType)
	}
}

// ===========================================================================
// DiscoveryResult 타입 검증
// ===========================================================================

func TestDiscoveryResult_Fields(t *testing.T) {
	// DiscoveryResult 구조체의 필드가 올바르게 설정되는지 확인
	r := DiscoveryResult{
		Address:    NasaAddress{0x10, 0x00, 0x00},
		DeviceType: "HVACR.ODU",
		Ready:      true,
	}

	if r.Address != (NasaAddress{0x10, 0x00, 0x00}) {
		t.Errorf("Address = %v, want 10 00 00", r.Address)
	}
	if r.DeviceType != "HVACR.ODU" {
		t.Errorf("DeviceType = %q, want \"outdoor\"", r.DeviceType)
	}
	if r.Ready != true {
		t.Errorf("Ready = %v, want true", r.Ready)
	}
}

// ===========================================================================
// DiscoveryCallbacks 인터페이스 검증
// ===========================================================================

// mockCallbacks 는 DiscoveryCallbacks 인터페이스를 구현하는 mock 이다.
type mockCallbacks struct {
	discovered []DiscoveryResult
	errors     []error
	complete   []DiscoveryResult
}

func (m *mockCallbacks) OnDeviceDiscovered(result DiscoveryResult) {
	m.discovered = append(m.discovered, result)
}
func (m *mockCallbacks) OnDiscoveryError(err error)                    { m.errors = append(m.errors, err) }
func (m *mockCallbacks) OnDiscoveryComplete(results []DiscoveryResult) { m.complete = results }

func TestDiscoveryCallbacks_Interface(t *testing.T) {
	// DiscoveryCallbacks 인터페이스 구현 확인 (컴파일 타임 검증)
	var _ DiscoveryCallbacks = &mockCallbacks{}

	cb := &mockCallbacks{}
	cb.OnDeviceDiscovered(DiscoveryResult{Address: NasaAddress{0x10, 0x00, 0x00}, DeviceType: "HVACR.ODU"})
	cb.OnDiscoveryError(errors.New("test error"))
	cb.OnDiscoveryComplete([]DiscoveryResult{{Address: NasaAddress{0x10, 0x00, 0x00}}})

	if len(cb.discovered) != 1 {
		t.Errorf("discovered 수 = %d, want 1", len(cb.discovered))
	}
	if len(cb.errors) != 1 {
		t.Errorf("errors 수 = %d, want 1", len(cb.errors))
	}
	if len(cb.complete) != 1 {
		t.Errorf("complete 수 = %d, want 1", len(cb.complete))
	}
}
