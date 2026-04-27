package socket

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
)

// --- NewFramer 팩토리 테스트 ---

func TestNewFramer(t *testing.T) {
	tests := []struct {
		name        string
		framingType string
		opts        FramerOptions
		wantErr     error
	}{
		{name: "raw 프레이머 생성", framingType: FramingRaw, opts: FramerOptions{BufferSize: 1024}},
		{name: "newline 프레이머 생성", framingType: FramingNewline, opts: FramerOptions{BufferSize: 1024}},
		{name: "length_prefix 프레이머 생성", framingType: FramingLengthPrefix, opts: FramerOptions{BufferSize: 1024}},
		{name: "fixed_size 프레이머 생성", framingType: FramingFixedSize, opts: FramerOptions{FixedSize: 64}},
		{name: "잘못된 프레이밍 타입", framingType: "unknown", wantErr: ErrInvalidFraming},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewFramer(tt.framingType, tt.opts)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("오류를 기대했으나 nil 반환")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("기대한 오류: %v, 실제: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("예상치 못한 오류: %v", err)
			}
			if f == nil {
				t.Fatal("프레이머가 nil")
			}
		})
	}
}

// --- RawFramer 테스트 ---

func TestRawFramer_ReadWrite(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	f, err := NewFramer(FramingRaw, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("hello raw framing")
	errCh := make(chan error, 1)

	go func() {
		errCh <- f.Write(client, data)
	}()

	got, readErr := f.Read(server)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if writeErr := <-errCh; writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("기대값: %q, 실제값: %q", data, got)
	}
}

func TestRawFramer_LargeData(t *testing.T) {
	server, client := net.Pipe()

	bufSize := 64
	f, err := NewFramer(FramingRaw, FramerOptions{BufferSize: bufSize})
	if err != nil {
		t.Fatal(err)
	}

	// 버퍼보다 큰 데이터 전송 — raw 는 버퍼 크기만큼만 읽음
	data := bytes.Repeat([]byte("A"), bufSize*3)

	go func() {
		// 쓰기가 블록될 수 있음 — 읽기 측에서 일부만 읽으면 나머지 대기
		// 연결을 닫아서 남은 쓰기가 해제되도록 함
		_, _ = client.Write(data)
	}()

	got, readErr := f.Read(server)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	// raw 프레이머는 최대 BufferSize 만큼 읽는다
	if len(got) > bufSize {
		t.Fatalf("버퍼 크기(%d)를 초과하여 읽음: %d", bufSize, len(got))
	}

	// 고루틴 정리: 연결 닫기
	server.Close()
	client.Close()
}

// --- NewlineFramer 테스트 ---

func TestNewlineFramer_ReadWrite(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	f, err := NewFramer(FramingNewline, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("hello newline framing")
	errCh := make(chan error, 1)

	go func() {
		errCh <- f.Write(client, msg)
	}()

	got, readErr := f.Read(server)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if writeErr := <-errCh; writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("기대값: %q, 실제값: %q", msg, got)
	}
}

func TestNewlineFramer_CustomDelimiter(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	f, err := NewFramer(FramingNewline, FramerOptions{BufferSize: 1024, Delimiter: '\t'})
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("tab delimited message")
	errCh := make(chan error, 1)

	go func() {
		errCh <- f.Write(client, msg)
	}()

	got, readErr := f.Read(server)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if writeErr := <-errCh; writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("기대값: %q, 실제값: %q", msg, got)
	}
}

func TestNewlineFramer_MultipleMessages(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	f, err := NewFramer(FramingNewline, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	msgs := []string{"first", "second", "third"}
	errCh := make(chan error, 1)

	go func() {
		for _, m := range msgs {
			if writeErr := f.Write(client, []byte(m)); writeErr != nil {
				errCh <- writeErr
				return
			}
		}
		errCh <- nil
	}()

	for _, want := range msgs {
		got, readErr := f.Read(server)
		if readErr != nil {
			t.Fatalf("읽기 오류: %v", readErr)
		}
		if string(got) != want {
			t.Fatalf("기대값: %q, 실제값: %q", want, string(got))
		}
	}

	if writeErr := <-errCh; writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}
}

// --- LengthPrefixFramer 테스트 ---

func TestLengthPrefixFramer_ReadWrite(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	f, err := NewFramer(FramingLengthPrefix, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("length prefixed message")
	errCh := make(chan error, 1)

	go func() {
		errCh <- f.Write(client, msg)
	}()

	got, readErr := f.Read(server)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if writeErr := <-errCh; writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("기대값: %q, 실제값: %q", msg, got)
	}
}

func TestLengthPrefixFramer_MaxMessageSize_Read(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	maxSize := 100
	f, err := NewFramer(FramingLengthPrefix, FramerOptions{
		BufferSize:     1024,
		MaxMessageSize: maxSize,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 최대 크기 초과하는 길이 헤더를 직접 전송
	done := make(chan struct{})
	go func() {
		defer close(done)
		header := make([]byte, 4)
		binary.BigEndian.PutUint32(header, uint32(maxSize+1))
		_, _ = client.Write(header)
	}()

	_, readErr := f.Read(server)
	<-done
	if readErr == nil {
		t.Fatal("오류를 기대했으나 nil 반환")
	}
	if !errors.Is(readErr, ErrMaxMessageSize) {
		t.Fatalf("기대한 오류: %v, 실제: %v", ErrMaxMessageSize, readErr)
	}
}

func TestLengthPrefixFramer_MaxMessageSize_Write(t *testing.T) {
	_, client := net.Pipe()
	defer client.Close()

	maxSize := 50
	f, err := NewFramer(FramingLengthPrefix, FramerOptions{
		BufferSize:     1024,
		MaxMessageSize: maxSize,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 최대 크기 초과 메시지 쓰기
	bigMsg := bytes.Repeat([]byte("X"), maxSize+1)
	writeErr := f.Write(client, bigMsg)
	if writeErr == nil {
		t.Fatal("오류를 기대했으나 nil 반환")
	}
	if !errors.Is(writeErr, ErrMaxMessageSize) {
		t.Fatalf("기대한 오류: %v, 실제: %v", ErrMaxMessageSize, writeErr)
	}
}

func TestLengthPrefixFramer_MultipleMessages(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	f, err := NewFramer(FramingLengthPrefix, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	msgs := []string{"alpha", "beta", "gamma"}
	errCh := make(chan error, 1)

	go func() {
		for _, m := range msgs {
			if writeErr := f.Write(client, []byte(m)); writeErr != nil {
				errCh <- writeErr
				return
			}
		}
		errCh <- nil
	}()

	for _, want := range msgs {
		got, readErr := f.Read(server)
		if readErr != nil {
			t.Fatalf("읽기 오류: %v", readErr)
		}
		if string(got) != want {
			t.Fatalf("기대값: %q, 실제값: %q", want, string(got))
		}
	}

	if writeErr := <-errCh; writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}
}

// --- FixedSizeFramer 테스트 ---

func TestFixedSizeFramer_ReadWrite(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	fixedSize := 16
	f, err := NewFramer(FramingFixedSize, FramerOptions{FixedSize: fixedSize})
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("exact16bytesXXX!")
	if len(msg) != fixedSize {
		t.Fatalf("테스트 데이터 길이가 %d 여야 하지만 %d 입니다", fixedSize, len(msg))
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Write(client, msg)
	}()

	got, readErr := f.Read(server)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if writeErr := <-errCh; writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("기대값: %q, 실제값: %q", msg, got)
	}
}

func TestFixedSizeFramer_ShortData(t *testing.T) {
	// 짧은 데이터 전송 시 제로 패딩
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	fixedSize := 16
	f, err := NewFramer(FramingFixedSize, FramerOptions{FixedSize: fixedSize})
	if err != nil {
		t.Fatal(err)
	}

	shortMsg := []byte("short")
	errCh := make(chan error, 1)

	go func() {
		errCh <- f.Write(client, shortMsg)
	}()

	got, readErr := f.Read(server)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if writeErr := <-errCh; writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}
	if len(got) != fixedSize {
		t.Fatalf("기대 길이: %d, 실제 길이: %d", fixedSize, len(got))
	}
	// 앞부분은 원본 데이터와 일치
	if !bytes.Equal(got[:len(shortMsg)], shortMsg) {
		t.Fatalf("데이터 접두사 불일치: %q", got[:len(shortMsg)])
	}
}

func TestFixedSizeFramer_LongDataTruncated(t *testing.T) {
	// 긴 데이터는 고정 크기로 잘림
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	fixedSize := 8
	f, err := NewFramer(FramingFixedSize, FramerOptions{FixedSize: fixedSize})
	if err != nil {
		t.Fatal(err)
	}

	longMsg := []byte("this is longer than 8 bytes")
	errCh := make(chan error, 1)

	go func() {
		errCh <- f.Write(client, longMsg)
	}()

	got, readErr := f.Read(server)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if writeErr := <-errCh; writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}
	if len(got) != fixedSize {
		t.Fatalf("기대 길이: %d, 실제 길이: %d", fixedSize, len(got))
	}
	if !bytes.Equal(got, longMsg[:fixedSize]) {
		t.Fatalf("기대값: %q, 실제값: %q", longMsg[:fixedSize], got)
	}
}

func TestFixedSizeFramer_ConnectionClose(t *testing.T) {
	// 고정 크기를 채우기 전에 연결이 닫히면 EOF
	server, client := net.Pipe()
	defer server.Close()

	fixedSize := 16
	f, err := NewFramer(FramingFixedSize, FramerOptions{FixedSize: fixedSize})
	if err != nil {
		t.Fatal(err)
	}

	// 고정 크기보다 적은 데이터를 쓰고 연결 닫기
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = client.Write([]byte("short"))
		client.Close()
	}()

	_, readErr := f.Read(server)
	<-done
	if readErr == nil {
		t.Fatal("오류를 기대했으나 nil 반환")
	}
	if !errors.Is(readErr, io.ErrUnexpectedEOF) {
		t.Fatalf("기대한 오류: %v, 실제: %v", io.ErrUnexpectedEOF, readErr)
	}
}
