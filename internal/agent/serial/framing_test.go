package serial

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

// --- NewSerialFramer 팩토리 테스트 ---

func TestNewSerialFramer(t *testing.T) {
	tests := []struct {
		name        string
		framingType string
		opts        FramerOptions
		wantErr     error
	}{
		{name: "raw 프레이머 생성", framingType: FramingRaw, opts: FramerOptions{BufferSize: 1024}},
		{name: "raw 프레이머 기본 버퍼", framingType: FramingRaw, opts: FramerOptions{}},
		{name: "newline 프레이머 생성", framingType: FramingNewline, opts: FramerOptions{BufferSize: 1024}},
		{name: "newline 프레이머 기본값", framingType: FramingNewline, opts: FramerOptions{}},
		{name: "length_prefix 프레이머 생성", framingType: FramingLengthPrefix, opts: FramerOptions{MaxMessageSize: 1024}},
		{name: "fixed_size 프레이머 생성", framingType: FramingFixedSize, opts: FramerOptions{FixedSize: 64}},
		{name: "잘못된 프레이밍 타입", framingType: "unknown", wantErr: ErrInvalidFraming},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewSerialFramer(tt.framingType, tt.opts)
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
	f, err := NewSerialFramer(FramingRaw, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("hello raw framing")
	var buf bytes.Buffer

	if writeErr := f.Write(&buf, data); writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}

	got, readErr := f.Read(&buf)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("기대값: %q, 실제값: %q", data, got)
	}
}

func TestRawFramer_LargeData(t *testing.T) {
	bufSize := 64
	f, err := NewSerialFramer(FramingRaw, FramerOptions{BufferSize: bufSize})
	if err != nil {
		t.Fatal(err)
	}

	// 버퍼보다 큰 데이터 전송 — raw 는 버퍼 크기만큼만 읽음
	data := bytes.Repeat([]byte("A"), bufSize*3)
	r := bytes.NewReader(data)

	got, readErr := f.Read(r)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	// raw 프레이머는 최대 BufferSize 만큼 읽는다
	if len(got) > bufSize {
		t.Fatalf("버퍼 크기(%d)를 초과하여 읽음: %d", bufSize, len(got))
	}
}

func TestRawFramer_EmptyRead(t *testing.T) {
	f, err := NewSerialFramer(FramingRaw, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	_, readErr := f.Read(&buf)
	if readErr == nil {
		t.Fatal("빈 리더에서 오류를 기대했으나 nil 반환")
	}
}

// --- NewlineFramer 테스트 ---

func TestNewlineFramer_ReadWrite(t *testing.T) {
	f, err := NewSerialFramer(FramingNewline, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("hello newline framing")
	var buf bytes.Buffer

	if writeErr := f.Write(&buf, msg); writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}

	got, readErr := f.Read(&buf)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("기대값: %q, 실제값: %q", msg, got)
	}
}

func TestNewlineFramer_CustomDelimiter(t *testing.T) {
	f, err := NewSerialFramer(FramingNewline, FramerOptions{BufferSize: 1024, Delimiter: '\t'})
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("tab delimited message")
	var buf bytes.Buffer

	if writeErr := f.Write(&buf, msg); writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}

	// 쓰기가 탭 구분자를 추가했는지 확인
	written := buf.Bytes()
	if written[len(written)-1] != '\t' {
		t.Fatalf("구분자가 탭이어야 하는데 %d", written[len(written)-1])
	}

	got, readErr := f.Read(&buf)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("기대값: %q, 실제값: %q", msg, got)
	}
}

func TestNewlineFramer_MultipleMessages(t *testing.T) {
	f, err := NewSerialFramer(FramingNewline, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	msgs := []string{"first", "second", "third"}
	var buf bytes.Buffer

	// 모든 메시지를 한 번에 쓰기
	for _, m := range msgs {
		if writeErr := f.Write(&buf, []byte(m)); writeErr != nil {
			t.Fatalf("쓰기 오류: %v", writeErr)
		}
	}

	// SerialConnReader 를 사용하여 scanner 를 재사용해야 다중 메시지 읽기가 정상 동작한다.
	// newlineFramer.Read() 는 매번 새 scanner 를 생성하므로 버퍼된 데이터를 잃는다.
	cr := NewSerialConnReader(f, &buf)
	for _, want := range msgs {
		got, readErr := cr.Read()
		if readErr != nil {
			t.Fatalf("읽기 오류: %v", readErr)
		}
		if string(got) != want {
			t.Fatalf("기대값: %q, 실제값: %q", want, string(got))
		}
	}
}

// --- LengthPrefixFramer 테스트 ---

func TestLengthPrefixFramer_ReadWrite(t *testing.T) {
	f, err := NewSerialFramer(FramingLengthPrefix, FramerOptions{})
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("length prefixed message")
	var buf bytes.Buffer

	if writeErr := f.Write(&buf, msg); writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}

	got, readErr := f.Read(&buf)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("기대값: %q, 실제값: %q", msg, got)
	}
}

func TestLengthPrefixFramer_MaxMessageSize_Read(t *testing.T) {
	maxSize := 100
	f, err := NewSerialFramer(FramingLengthPrefix, FramerOptions{MaxMessageSize: maxSize})
	if err != nil {
		t.Fatal(err)
	}

	// 최대 크기 초과하는 길이 헤더를 직접 작성
	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(maxSize+1))
	buf.Write(header)

	_, readErr := f.Read(&buf)
	if readErr == nil {
		t.Fatal("오류를 기대했으나 nil 반환")
	}
	if !errors.Is(readErr, ErrMaxMessageSize) {
		t.Fatalf("기대한 오류: %v, 실제: %v", ErrMaxMessageSize, readErr)
	}
}

func TestLengthPrefixFramer_MaxMessageSize_Write(t *testing.T) {
	maxSize := 50
	f, err := NewSerialFramer(FramingLengthPrefix, FramerOptions{MaxMessageSize: maxSize})
	if err != nil {
		t.Fatal(err)
	}

	// 최대 크기 초과 메시지 쓰기
	bigMsg := bytes.Repeat([]byte("X"), maxSize+1)
	var buf bytes.Buffer
	writeErr := f.Write(&buf, bigMsg)
	if writeErr == nil {
		t.Fatal("오류를 기대했으나 nil 반환")
	}
	if !errors.Is(writeErr, ErrMaxMessageSize) {
		t.Fatalf("기대한 오류: %v, 실제: %v", ErrMaxMessageSize, writeErr)
	}
}

func TestLengthPrefixFramer_MultipleMessages(t *testing.T) {
	f, err := NewSerialFramer(FramingLengthPrefix, FramerOptions{})
	if err != nil {
		t.Fatal(err)
	}

	msgs := []string{"alpha", "beta", "gamma"}
	var buf bytes.Buffer

	for _, m := range msgs {
		if writeErr := f.Write(&buf, []byte(m)); writeErr != nil {
			t.Fatalf("쓰기 오류: %v", writeErr)
		}
	}

	for _, want := range msgs {
		got, readErr := f.Read(&buf)
		if readErr != nil {
			t.Fatalf("읽기 오류: %v", readErr)
		}
		if string(got) != want {
			t.Fatalf("기대값: %q, 실제값: %q", want, string(got))
		}
	}
}

func TestLengthPrefixFramer_EmptyMessage(t *testing.T) {
	f, err := NewSerialFramer(FramingLengthPrefix, FramerOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	// 빈 메시지 쓰기 (길이 0)
	if writeErr := f.Write(&buf, []byte{}); writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}

	got, readErr := f.Read(&buf)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if len(got) != 0 {
		t.Fatalf("빈 메시지를 기대했으나 %d 바이트 수신", len(got))
	}
}

// --- FixedSizeFramer 테스트 ---

func TestFixedSizeFramer_ReadWrite(t *testing.T) {
	fixedSize := 16
	f, err := NewSerialFramer(FramingFixedSize, FramerOptions{FixedSize: fixedSize})
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("exact16bytesXXX!")
	if len(msg) != fixedSize {
		t.Fatalf("테스트 데이터 길이가 %d 여야 하지만 %d 입니다", fixedSize, len(msg))
	}

	var buf bytes.Buffer
	if writeErr := f.Write(&buf, msg); writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}

	got, readErr := f.Read(&buf)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("기대값: %q, 실제값: %q", msg, got)
	}
}

func TestFixedSizeFramer_ShortData(t *testing.T) {
	// 짧은 데이터 전송 시 제로 패딩
	fixedSize := 16
	f, err := NewSerialFramer(FramingFixedSize, FramerOptions{FixedSize: fixedSize})
	if err != nil {
		t.Fatal(err)
	}

	shortMsg := []byte("short")
	var buf bytes.Buffer
	if writeErr := f.Write(&buf, shortMsg); writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}

	got, readErr := f.Read(&buf)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if len(got) != fixedSize {
		t.Fatalf("기대 길이: %d, 실제 길이: %d", fixedSize, len(got))
	}
	// 앞부분은 원본 데이터와 일치
	if !bytes.Equal(got[:len(shortMsg)], shortMsg) {
		t.Fatalf("데이터 접두사 불일치: %q", got[:len(shortMsg)])
	}
	// 나머지는 제로 패딩
	for i := len(shortMsg); i < fixedSize; i++ {
		if got[i] != 0 {
			t.Fatalf("위치 %d 에 제로 패딩이 아닌 %d 발견", i, got[i])
		}
	}
}

func TestFixedSizeFramer_LongDataTruncated(t *testing.T) {
	// 긴 데이터는 고정 크기로 잘림
	fixedSize := 8
	f, err := NewSerialFramer(FramingFixedSize, FramerOptions{FixedSize: fixedSize})
	if err != nil {
		t.Fatal(err)
	}

	longMsg := []byte("this is longer than 8 bytes")
	var buf bytes.Buffer
	if writeErr := f.Write(&buf, longMsg); writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}

	got, readErr := f.Read(&buf)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if len(got) != fixedSize {
		t.Fatalf("기대 길이: %d, 실제 길이: %d", fixedSize, len(got))
	}
	if !bytes.Equal(got, longMsg[:fixedSize]) {
		t.Fatalf("기대값: %q, 실제값: %q", longMsg[:fixedSize], got)
	}
}

func TestFixedSizeFramer_ShortRead(t *testing.T) {
	// 고정 크기를 채우기 전에 데이터가 끝나면 io.ErrUnexpectedEOF
	fixedSize := 16
	f, err := NewSerialFramer(FramingFixedSize, FramerOptions{FixedSize: fixedSize})
	if err != nil {
		t.Fatal(err)
	}

	// 고정 크기보다 적은 데이터
	shortData := bytes.NewReader([]byte("short"))
	_, readErr := f.Read(shortData)
	if readErr == nil {
		t.Fatal("오류를 기대했으나 nil 반환")
	}
	if !errors.Is(readErr, io.ErrUnexpectedEOF) {
		t.Fatalf("기대한 오류: %v, 실제: %v", io.ErrUnexpectedEOF, readErr)
	}
}

// --- SerialConnReader 테스트 ---

func TestSerialConnReader_NewlineFramer(t *testing.T) {
	// SerialConnReader 는 newlineFramer 에서 scanner 를 재사용한다
	f, err := NewSerialFramer(FramingNewline, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	// 여러 메시지를 한 번에 버퍼에 쓰기
	var buf bytes.Buffer
	msgs := []string{"first", "second", "third"}
	for _, m := range msgs {
		buf.WriteString(m)
		buf.WriteByte('\n')
	}

	cr := NewSerialConnReader(f, &buf)

	// SerialConnReader 는 내부 scanner 를 재사용하여 연속 읽기 가능
	for _, want := range msgs {
		got, readErr := cr.Read()
		if readErr != nil {
			t.Fatalf("읽기 오류: %v", readErr)
		}
		if string(got) != want {
			t.Fatalf("기대값: %q, 실제값: %q", want, string(got))
		}
	}

	// 모든 데이터 소진 후 EOF
	_, readErr := cr.Read()
	if readErr != io.EOF {
		t.Fatalf("EOF 를 기대했으나: %v", readErr)
	}
}

func TestSerialConnReader_RawFramer(t *testing.T) {
	// raw 프레이머에서는 scanner 없이 직접 framer.Read 호출
	f, err := NewSerialFramer(FramingRaw, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("raw data here")
	buf := bytes.NewBuffer(data)
	cr := NewSerialConnReader(f, buf)

	got, readErr := cr.Read()
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("기대값: %q, 실제값: %q", data, got)
	}
}

func TestSerialConnReader_LengthPrefixFramer(t *testing.T) {
	f, err := NewSerialFramer(FramingLengthPrefix, FramerOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	msg := []byte("prefixed message")
	// 수동으로 length prefix 프레임 작성
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(msg)))
	buf.Write(header)
	buf.Write(msg)

	cr := NewSerialConnReader(f, &buf)
	got, readErr := cr.Read()
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("기대값: %q, 실제값: %q", msg, got)
	}
}

func TestSerialConnReader_FixedSizeFramer(t *testing.T) {
	fixedSize := 8
	f, err := NewSerialFramer(FramingFixedSize, FramerOptions{FixedSize: fixedSize})
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("12345678") // 정확히 8바이트
	buf := bytes.NewBuffer(data)
	cr := NewSerialConnReader(f, buf)

	got, readErr := cr.Read()
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("기대값: %q, 실제값: %q", data, got)
	}
}

// --- newlineFramer 추가 엣지 케이스 ---

func TestNewlineFramer_ReadEOF(t *testing.T) {
	// newlineFramer.Read on empty reader returns io.EOF
	f, err := NewSerialFramer(FramingNewline, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	var emptyBuf bytes.Buffer
	_, readErr := f.Read(&emptyBuf)
	if readErr != io.EOF {
		t.Fatalf("기대한 오류: io.EOF, 실제: %v", readErr)
	}
}

func TestNewlineFramer_ReadScannerError(t *testing.T) {
	// newlineFramer.Read should propagate scanner errors
	f, err := NewSerialFramer(FramingNewline, FramerOptions{BufferSize: 8})
	if err != nil {
		t.Fatal(err)
	}

	// Create data that exceeds the tiny buffer size without a delimiter
	// This will cause scanner to report a "token too long" error
	longLine := bytes.Repeat([]byte("A"), 100)
	buf := bytes.NewBuffer(longLine)

	_, readErr := f.Read(buf)
	if readErr == nil {
		t.Fatal("오류를 기대했으나 nil 반환")
	}
	// bufio.Scanner returns "token too long" when buffer is exceeded
}

func TestNewlineFramer_ReadDataWithoutDelimiterAtEOF(t *testing.T) {
	// When data has no trailing delimiter and reader reaches EOF,
	// the splitFunc returns remaining data
	f, err := NewSerialFramer(FramingNewline, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	// Data without trailing newline
	buf := bytes.NewBufferString("no trailing newline")
	got, readErr := f.Read(buf)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if string(got) != "no trailing newline" {
		t.Fatalf("기대값: %q, 실제값: %q", "no trailing newline", string(got))
	}
}

// --- lengthPrefixFramer 추가 엣지 케이스 ---

func TestLengthPrefixFramer_ReadHeaderEOF(t *testing.T) {
	// Reading from empty reader should return error (can't read header)
	f, err := NewSerialFramer(FramingLengthPrefix, FramerOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var emptyBuf bytes.Buffer
	_, readErr := f.Read(&emptyBuf)
	if readErr == nil {
		t.Fatal("오류를 기대했으나 nil 반환")
	}
}

func TestLengthPrefixFramer_ReadPartialHeader(t *testing.T) {
	// Partial header (less than 4 bytes) should return error
	f, err := NewSerialFramer(FramingLengthPrefix, FramerOptions{})
	if err != nil {
		t.Fatal(err)
	}

	buf := bytes.NewBuffer([]byte{0x00, 0x00}) // only 2 bytes
	_, readErr := f.Read(buf)
	if readErr == nil {
		t.Fatal("오류를 기대했으나 nil 반환")
	}
}

func TestLengthPrefixFramer_ReadTruncatedPayload(t *testing.T) {
	// Header says 10 bytes but only 3 available
	f, err := NewSerialFramer(FramingLengthPrefix, FramerOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, 10)
	buf.Write(header)
	buf.Write([]byte("abc")) // only 3 bytes, header says 10

	_, readErr := f.Read(&buf)
	if readErr == nil {
		t.Fatal("오류를 기대했으나 nil 반환")
	}
}

func TestLengthPrefixFramer_WriteNoMaxMessageSize(t *testing.T) {
	// With maxMessageSize=0 (unlimited), large messages should succeed
	f, err := NewSerialFramer(FramingLengthPrefix, FramerOptions{MaxMessageSize: 0})
	if err != nil {
		t.Fatal(err)
	}

	bigMsg := bytes.Repeat([]byte("X"), 10000)
	var buf bytes.Buffer
	writeErr := f.Write(&buf, bigMsg)
	if writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}

	// Verify it can be read back
	got, readErr := f.Read(&buf)
	if readErr != nil {
		t.Fatalf("읽기 오류: %v", readErr)
	}
	if !bytes.Equal(got, bigMsg) {
		t.Fatal("읽기 데이터 불일치")
	}
}

// --- SerialConnReader scanner error case ---

func TestSerialConnReader_NewlineFramer_ScannerError(t *testing.T) {
	// Test SerialConnReader with newlineFramer when scanner encounters error
	f, err := NewSerialFramer(FramingNewline, FramerOptions{BufferSize: 8})
	if err != nil {
		t.Fatal(err)
	}

	// Data exceeding buffer without delimiter triggers scanner error
	longLine := bytes.Repeat([]byte("B"), 100)
	buf := bytes.NewBuffer(longLine)
	cr := NewSerialConnReader(f, buf)

	_, readErr := cr.Read()
	if readErr == nil {
		t.Fatal("오류를 기대했으나 nil 반환")
	}
}
