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
		ID:  "w1",
		Ch:  make(chan message.Message, 1),
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

func TestRuntimeWire_Send_ContextCancelled(t *testing.T) {
	// 컨텍스트가 취소되면 전송이 실패해야 한다.
	rw := &RuntimeWire{
		ID:  "w1",
		Ch:  make(chan message.Message), // unbuffered - 수신자가 없으면 블록
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
		ID:  "w1",
		Ch:  make(chan message.Message, 1),
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
