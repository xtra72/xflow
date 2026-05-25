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

// --- 버퍼 aliasing 회귀 테스트 (2026-05-14 hotfix) ---

// TestNewlineFramer_Read_ReturnsCopy 는 newlineFramer.Read 가 bufio.Scanner 의
// 내부 버퍼를 그대로 반환하지 않고 독립된 복사본을 반환하는지 검증한다.
//
// bufio.Scanner.Bytes() 는 다음 Scan() 호출에 의해 무효화되는 슬라이스를
// 반환하며, 그 backing 배열은 scanner.Buffer 로 설정한 버퍼이다.
//
// 수정 전(버그): Read 가 scanner.Bytes() 를 그대로 반환하므로 결과 슬라이스가
// scanner 내부 버퍼를 aliasing 한다 (cap 이 프레임 길이보다 크다).
// 수정 후: 복사본을 반환하므로 cap == len 이고, scanner 버퍼와 독립적이다.
func TestNewlineFramer_Read_ReturnsCopy(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	f, err := NewFramer(FramingNewline, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	frameA := []byte("frame-A")
	frameB := []byte("frame-B-longer-content")

	// 두 프레임을 한 번에 전송하여 scanner 내부 버퍼에 frameA 이후 데이터가
	// 함께 존재하도록 만든다.
	go func() {
		_, _ = client.Write(append(append([]byte{}, frameA...), '\n'))
		_, _ = client.Write(append(append([]byte{}, frameB...), '\n'))
	}()

	got, readErr := f.Read(server)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if !bytes.Equal(got, frameA) {
		t.Fatalf("기대값: %q, 실제값: %q", frameA, got)
	}

	// 복사본은 cap 이 len 과 같아야 한다. scanner.Bytes() 를 그대로 반환하면
	// cap 이 scanner 버퍼 크기까지 커진다.
	if cap(got) != len(got) {
		t.Fatalf("Read 결과가 scanner 내부 버퍼를 aliasing 한다: cap=%d, len=%d", cap(got), len(got))
	}
}

// TestRawFramer_Read_CapLimited 는 rawFramer.Read 가 반환하는 슬라이스의
// cap 이 len 으로 제한되는지 검증한다.
//
// 수정 전(버그): Read 가 buf[:n] 을 반환하므로 cap == bufferSize 이다.
// downstream 의 append 가 공유 backing 배열에 써넣어 다른 프레임을 변조할 수 있다.
// 수정 후: buf[:n:n] (three-index slice) 로 cap 을 n 으로 제한하여 append 가
// 반드시 새 배열을 할당하도록 강제한다.
func TestRawFramer_Read_CapLimited(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	bufSize := 1024
	f, err := NewFramer(FramingRaw, FramerOptions{BufferSize: bufSize})
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("hello raw framing")
	go func() {
		_, _ = client.Write(data)
	}()

	got, readErr := f.Read(server)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("기대값: %q, 실제값: %q", data, got)
	}
	if cap(got) != len(got) {
		t.Fatalf("rawFramer.Read 결과의 cap 이 제한되지 않음: cap=%d, len=%d (bufferSize=%d)", cap(got), len(got), bufSize)
	}
}
