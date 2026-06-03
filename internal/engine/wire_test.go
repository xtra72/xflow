package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

func TestCreateRuntimeWires_Bypass(t *testing.T) {
	// WireBypass 모드에서는 unbuffered 채널이 생성되어야 한다.
	wires := []flow.Wire{
		{
			ID:           "w1",
			SourceNodeID: "n1",
			SourcePort:   "out",
			TargetNodeID: "n2",
			TargetPort:   "in",
			Mode:         flow.WireBypass,
		},
	}

	runtimeWires, err := CreateRuntimeWires(wires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runtimeWires) != 1 {
		t.Fatalf("expected 1 wire, got %d", len(runtimeWires))
	}

	rw := runtimeWires[0]
	if rw.ID != "w1" {
		t.Errorf("expected ID 'w1', got %q", rw.ID)
	}
	if rw.SourceNodeID != "n1" {
		t.Errorf("expected SourceNodeID 'n1', got %q", rw.SourceNodeID)
	}
	if rw.TargetNodeID != "n2" {
		t.Errorf("expected TargetNodeID 'n2', got %q", rw.TargetNodeID)
	}
	if rw.Mode != flow.WireBypass {
		t.Errorf("expected WireBypass mode, got %q", rw.Mode)
	}
	if cap(rw.Ch) != 0 {
		t.Errorf("expected unbuffered channel (cap 0), got %d", cap(rw.Ch))
	}
}

func TestCreateRuntimeWires_Buffer(t *testing.T) {
	// WireBuffer 모드에서는 buffered 채널이 생성되어야 한다.
	wires := []flow.Wire{
		{
			ID:           "w1",
			SourceNodeID: "n1",
			SourcePort:   "out",
			TargetNodeID: "n2",
			TargetPort:   "in",
			Mode:         flow.WireBuffer,
			BufferSize:   10,
		},
	}

	runtimeWires, err := CreateRuntimeWires(wires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runtimeWires) != 1 {
		t.Fatalf("expected 1 wire, got %d", len(runtimeWires))
	}

	rw := runtimeWires[0]
	if cap(rw.Ch) != 10 {
		t.Errorf("expected buffered channel with cap 10, got %d", cap(rw.Ch))
	}
}

func TestCreateRuntimeWires_BufferMinSize(t *testing.T) {
	// WireBuffer 모드에서 BufferSize가 0 이하이면 최소 1로 설정되어야 한다.
	wires := []flow.Wire{
		{
			ID:           "w1",
			SourceNodeID: "n1",
			SourcePort:   "out",
			TargetNodeID: "n2",
			TargetPort:   "in",
			Mode:         flow.WireBuffer,
			BufferSize:   0,
		},
	}

	runtimeWires, err := CreateRuntimeWires(wires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rw := runtimeWires[0]
	if cap(rw.Ch) < 1 {
		t.Errorf("expected buffered channel with at least cap 1, got %d", cap(rw.Ch))
	}
}

func TestCreateRuntimeWires_Multiple(t *testing.T) {
	// 여러 Wire가 올바르게 생성되는지 검증한다.
	wires := []flow.Wire{
		{ID: "w1", SourceNodeID: "n1", SourcePort: "out", TargetNodeID: "n2", TargetPort: "in", Mode: flow.WireBypass},
		{ID: "w2", SourceNodeID: "n2", SourcePort: "out", TargetNodeID: "n3", TargetPort: "in", Mode: flow.WireBuffer, BufferSize: 5},
	}

	runtimeWires, err := CreateRuntimeWires(wires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runtimeWires) != 2 {
		t.Fatalf("expected 2 wires, got %d", len(runtimeWires))
	}

	if cap(runtimeWires[0].Ch) != 0 {
		t.Errorf("wire[0] expected unbuffered, got cap %d", cap(runtimeWires[0].Ch))
	}
	if cap(runtimeWires[1].Ch) != 5 {
		t.Errorf("wire[1] expected cap 5, got %d", cap(runtimeWires[1].Ch))
	}
}

func TestCreateRuntimeWires_TTL(t *testing.T) {
	// TTL이 올바르게 전달되는지 검증한다.
	wires := []flow.Wire{
		{
			ID:           "w1",
			SourceNodeID: "n1",
			SourcePort:   "out",
			TargetNodeID: "n2",
			TargetPort:   "in",
			Mode:         flow.WireBypass,
			TTL:          5 * time.Second,
		},
	}

	runtimeWires, err := CreateRuntimeWires(wires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runtimeWires[0].TTL != 5*time.Second {
		t.Errorf("expected TTL 5s, got %v", runtimeWires[0].TTL)
	}
}

func TestRuntimeWire_Send(t *testing.T) {
	// 정상적으로 메시지를 전송할 수 있는지 검증한다.
	rw := &RuntimeWire{
		ID:   "w1",
		Ch:   make(chan message.Message, 1),
		Mode: flow.WireBuffer,
	}

	msg := message.New()
	ctx := context.Background()

	err := rw.Send(ctx, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	received := <-rw.Ch
	if received.ID() != msg.ID() {
		t.Errorf("expected message ID %q, got %q", msg.ID(), received.ID())
	}
}

// TestRuntimeWire_Send_BufferBlocksWhenFull 는 WireBuffer 모드에서 버퍼가 가득
// 차면(cap 만큼 채워지면) 다음 Send 가 수신이 발생할 때까지 블록되고, 수신으로
// 슬롯이 비면 풀려서 메시지 손실 없이 전달되는지 검증한다.
func TestRuntimeWire_Send_BufferBlocksWhenFull(t *testing.T) {
	rw := &RuntimeWire{
		ID:   "w1",
		Ch:   make(chan message.Message, 2), // cap=2
		Mode: flow.WireBuffer,
	}
	ctx := context.Background()

	msg1 := message.New()
	msg2 := message.New()
	msg3 := message.New()

	// cap 까지 두 건은 즉시(non-blocking) 성공해야 한다.
	if err := rw.Send(ctx, msg1); err != nil {
		t.Fatalf("첫 번째 Send 실패: %v", err)
	}
	if err := rw.Send(ctx, msg2); err != nil {
		t.Fatalf("두 번째 Send 실패: %v", err)
	}

	// 세 번째 Send 는 버퍼가 가득 차서 블록되어야 한다.
	sendDone := make(chan error, 1)
	go func() {
		sendDone <- rw.Send(ctx, msg3)
	}()

	select {
	case err := <-sendDone:
		t.Fatalf("버퍼가 가득 찼는데 세 번째 Send 가 블록되지 않고 즉시 반환됨: err=%v", err)
	case <-time.After(50 * time.Millisecond):
		// 기대 동작: 아직 블록 중
	}

	// 한 건을 수신하여 슬롯을 비우면 블록된 Send 가 풀려야 한다.
	got1 := <-rw.Ch
	if got1.ID() != msg1.ID() {
		t.Errorf("첫 수신 메시지 ID = %q, want %q", got1.ID(), msg1.ID())
	}

	select {
	case err := <-sendDone:
		if err != nil {
			t.Fatalf("슬롯이 비었는데 세 번째 Send 가 에러 반환: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("슬롯이 비었는데 세 번째 Send 가 풀리지 않음")
	}

	// 손실 없이 msg2, msg3 가 순서대로 남아 있어야 한다.
	got2 := <-rw.Ch
	got3 := <-rw.Ch
	if got2.ID() != msg2.ID() {
		t.Errorf("두 번째 수신 메시지 ID = %q, want %q", got2.ID(), msg2.ID())
	}
	if got3.ID() != msg3.ID() {
		t.Errorf("세 번째 수신 메시지 ID = %q, want %q", got3.ID(), msg3.ID())
	}
}

func TestRuntimeWire_Send_ContextCancelled(t *testing.T) {
	// 컨텍스트가 취소되면 전송이 실패해야 한다.
	rw := &RuntimeWire{
		ID:   "w1",
		Ch:   make(chan message.Message), // unbuffered - 수신자가 없으면 블록
		Mode: flow.WireBypass,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 즉시 취소

	msg := message.New()
	err := rw.Send(ctx, msg)
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
}

func TestRuntimeWire_Send_ClosedChannel(t *testing.T) {
	// 닫힌 채널에 전송하면 ErrChannelClosed를 반환해야 한다.
	rw := &RuntimeWire{
		ID:   "w1",
		Ch:   make(chan message.Message, 1),
		Mode: flow.WireBuffer,
	}
	rw.Close()

	msg := message.New()
	ctx := context.Background()

	err := rw.Send(ctx, msg)
	if err == nil {
		t.Fatal("expected error on closed channel, got nil")
	}
	if !errors.Is(err, ErrChannelClosed) {
		t.Errorf("expected ErrChannelClosed, got %v", err)
	}
}

func TestRuntimeWire_Close(t *testing.T) {
	// Close() 후 IsClosed()가 true를 반환해야 한다.
	rw := &RuntimeWire{
		ID: "w1",
		Ch: make(chan message.Message),
	}

	if rw.IsClosed() {
		t.Fatal("wire should not be closed initially")
	}

	rw.Close()

	if !rw.IsClosed() {
		t.Fatal("wire should be closed after Close()")
	}
}

func TestRuntimeWire_Close_Idempotent(t *testing.T) {
	// Close()를 여러 번 호출해도 패닉이 발생하지 않아야 한다.
	rw := &RuntimeWire{
		ID: "w1",
		Ch: make(chan message.Message),
	}

	rw.Close()
	// 두 번째 Close()에서 패닉이 발생하지 않아야 한다.
	rw.Close()

	if !rw.IsClosed() {
		t.Fatal("wire should be closed")
	}
}

func TestCreateRuntimeWires_Empty(t *testing.T) {
	// 빈 와이어 슬라이스에서도 에러 없이 빈 결과를 반환해야 한다.
	runtimeWires, err := CreateRuntimeWires(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runtimeWires) != 0 {
		t.Errorf("expected 0 wires, got %d", len(runtimeWires))
	}
}

func TestCreateRuntimeWires_DropOldest(t *testing.T) {
	// WireDropOldest 모드에서 buffered 채널이 생성되어야 한다.
	wires := []flow.Wire{
		{ID: "w1", SourceNodeID: "n1", SourcePort: "out", TargetNodeID: "n2", TargetPort: "in", Mode: flow.WireDropOldest, BufferSize: 32},
	}
	runtimeWires, err := CreateRuntimeWires(wires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap(runtimeWires[0].Ch) != 32 {
		t.Errorf("expected buffer capacity 32, got %d", cap(runtimeWires[0].Ch))
	}
}

func TestCreateRuntimeWires_DropOldest_DefaultSize(t *testing.T) {
	// WireDropOldest 모드에서 BufferSize가 0이면 기본값 64가 적용되어야 한다.
	wires := []flow.Wire{
		{ID: "w1", SourceNodeID: "n1", SourcePort: "out", TargetNodeID: "n2", TargetPort: "in", Mode: flow.WireDropOldest, BufferSize: 0},
	}
	runtimeWires, err := CreateRuntimeWires(wires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap(runtimeWires[0].Ch) != 64 {
		t.Errorf("expected default buffer capacity 64, got %d", cap(runtimeWires[0].Ch))
	}
}

func TestRuntimeWire_Send_DropOldest_NonBlocking(t *testing.T) {
	// drop_oldest 모드에서 버퍼가 가득 차도 Send가 블로킹하지 않아야 한다.
	rw := &RuntimeWire{
		ID:           "w1",
		SourceNodeID: "src",
		SourcePort:   "out",
		TargetNodeID: "tgt",
		TargetPort:   "in",
		Ch:           make(chan message.Message, 2),
		Mode:         flow.WireDropOldest,
	}

	ctx := context.Background()
	msg1 := message.New()
	msg2 := message.New()
	msg3 := message.New()

	// 버퍼 채움 (2개)
	if err := rw.Send(ctx, msg1); err != nil {
		t.Fatalf("send msg1: %v", err)
	}
	if err := rw.Send(ctx, msg2); err != nil {
		t.Fatalf("send msg2: %v", err)
	}

	// 3번째 메시지: 가장 오래된 msg1이 드랍되고 msg3이 들어가야 함
	if err := rw.Send(ctx, msg3); err != nil {
		t.Fatalf("send msg3 (drop_oldest): %v", err)
	}

	// 드랍 카운터 검증
	if rw.Dropped() != 1 {
		t.Errorf("expected 1 dropped, got %d", rw.Dropped())
	}

	// 채널에서 꺼낸 메시지: msg2, msg3 순서 (msg1은 드랍됨)
	got1 := <-rw.Ch
	if got1.ID() != msg2.ID() {
		t.Errorf("expected msg2 (oldest surviving), got %q", got1.ID())
	}
	got2 := <-rw.Ch
	if got2.ID() != msg3.ID() {
		t.Errorf("expected msg3 (newest), got %q", got2.ID())
	}
}

func TestRuntimeWire_Send_DropOldest_NotFull(t *testing.T) {
	// 버퍼가 가득 차지 않으면 드랍 없이 정상 전송되어야 한다.
	rw := &RuntimeWire{
		ID:   "w1",
		Ch:   make(chan message.Message, 4),
		Mode: flow.WireDropOldest,
	}

	ctx := context.Background()
	msg := message.New()
	if err := rw.Send(ctx, msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rw.Dropped() != 0 {
		t.Errorf("expected 0 dropped, got %d", rw.Dropped())
	}
}

func TestCreateRuntimeWires_NameAndType(t *testing.T) {
	// Name과 Type이 RuntimeWire로 올바르게 복사되는지 검증한다.
	wires := []flow.Wire{
		{
			ID:           "w1",
			Name:         "sensor.out_to_processor.in",
			Type:         flow.WireSimple,
			SourceNodeID: "n1",
			SourcePort:   "out",
			TargetNodeID: "n2",
			TargetPort:   "in",
			Mode:         flow.WireBypass,
		},
	}

	runtimeWires, err := CreateRuntimeWires(wires)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rw := runtimeWires[0]
	if rw.Name != "sensor.out_to_processor.in" {
		t.Errorf("expected Name %q, got %q", "sensor.out_to_processor.in", rw.Name)
	}
	if rw.Type != flow.WireSimple {
		t.Errorf("expected Type %q, got %q", flow.WireSimple, rw.Type)
	}
}

// TestCreateRuntimeWires_VirtualIgnored 는 flow.Wire.Virtual 플래그가 RuntimeWire
// 생성(채널 종류/버퍼/모드)에 어떠한 영향도 주지 않음을 검증한다.
// virtual=true 와이어와 virtual=false 와이어는 완전히 동일한 RuntimeWire 를
// 만들어야 한다 (SPEC-LINK-001 REQ-LINK-005/006, AC-B3: display-only).
func TestCreateRuntimeWires_VirtualIgnored(t *testing.T) {
	base := func(virtual bool) flow.Wire {
		return flow.Wire{
			ID:           "w1",
			Name:         "sensor",
			Type:         flow.WireSimple,
			SourceNodeID: "n1",
			SourcePort:   "out",
			TargetNodeID: "n2",
			TargetPort:   "in",
			Mode:         flow.WireBuffer,
			BufferSize:   8,
			Virtual:      virtual,
		}
	}

	plain, err := CreateRuntimeWires([]flow.Wire{base(false)})
	if err != nil {
		t.Fatalf("virtual=false 변환 실패: %v", err)
	}
	virt, err := CreateRuntimeWires([]flow.Wire{base(true)})
	if err != nil {
		t.Fatalf("virtual=true 변환 실패: %v", err)
	}

	pw, vw := plain[0], virt[0]

	// 라우팅에 영향을 주는 모든 필드가 동일해야 한다.
	if pw.Mode != vw.Mode {
		t.Errorf("Mode 불일치: virtual=false %q, virtual=true %q", pw.Mode, vw.Mode)
	}
	if pw.BufferSize != vw.BufferSize {
		t.Errorf("BufferSize 불일치: %d vs %d", pw.BufferSize, vw.BufferSize)
	}
	if cap(pw.Ch) != cap(vw.Ch) {
		t.Errorf("채널 용량 불일치: virtual 이 채널 종류에 영향을 줌 (%d vs %d)", cap(pw.Ch), cap(vw.Ch))
	}
	if pw.Type != vw.Type || pw.SourceNodeID != vw.SourceNodeID || pw.TargetNodeID != vw.TargetNodeID ||
		pw.SourcePort != vw.SourcePort || pw.TargetPort != vw.TargetPort {
		t.Error("토폴로지 필드가 virtual 에 따라 달라짐")
	}
}

// TestRuntimeWire_VirtualMessageDeliveryIdentical 는 virtual 여부와 무관하게
// 메시지 전달 동작(전달/순서)이 동일함을 직접 입증한다 (REQ-LINK-005, AC-B3).
// virtual=true 토폴로지와 virtual=false 토폴로지에서 동일 메시지를 흘려보내면
// 수신 메시지 ID 순서가 완전히 같아야 한다.
func TestRuntimeWire_VirtualMessageDeliveryIdentical(t *testing.T) {
	// deliver 는 와이어로 송신한 메시지 ID 와 수신한 메시지 ID 슬라이스를 반환한다.
	// 수신 순서가 송신 순서와 동일한지(순서 불변) 외부에서 비교할 수 있게 한다.
	deliver := func(virtual bool) (sent, received []string) {
		wires := []flow.Wire{
			{
				ID:           "w1",
				SourceNodeID: "n1",
				SourcePort:   "out",
				TargetNodeID: "n2",
				TargetPort:   "in",
				Mode:         flow.WireBuffer,
				BufferSize:   4,
				Virtual:      virtual,
			},
		}
		rws, err := CreateRuntimeWires(wires)
		if err != nil {
			t.Fatalf("CreateRuntimeWires 실패(virtual=%v): %v", virtual, err)
		}
		rw := rws[0]
		ctx := context.Background()

		msgs := []message.Message{message.New(), message.New(), message.New()}
		for _, m := range msgs {
			if err := rw.Send(ctx, m); err != nil {
				t.Fatalf("Send 실패(virtual=%v): %v", virtual, err)
			}
			sent = append(sent, m.ID())
		}

		for range msgs {
			received = append(received, (<-rw.Ch).ID())
		}
		return sent, received
	}

	plainSent, plainRecv := deliver(false)
	virtSent, virtRecv := deliver(true)

	// 1) 두 경우 모두 송신 순서와 수신 순서가 완전히 동일해야 한다(순서/전달 불변).
	for i := range plainSent {
		if plainSent[i] != plainRecv[i] {
			t.Errorf("virtual=false 순서 불변 위반: idx %d 송신 %q != 수신 %q", i, plainSent[i], plainRecv[i])
		}
	}
	for i := range virtSent {
		if virtSent[i] != virtRecv[i] {
			t.Errorf("virtual=true 순서 불변 위반: idx %d 송신 %q != 수신 %q", i, virtSent[i], virtRecv[i])
		}
	}

	// 2) 전달된 메시지 개수가 virtual 여부와 무관하게 동일해야 한다.
	if len(plainRecv) != len(virtRecv) {
		t.Fatalf("수신 개수가 virtual 에 따라 달라짐: %d vs %d", len(plainRecv), len(virtRecv))
	}
}
