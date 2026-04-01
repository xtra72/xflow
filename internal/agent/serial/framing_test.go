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
		{name: "stream 프레이머 생성", framingType: FramingStream, opts: FramerOptions{BufferSize: 1024}},
		{name: "stream 프레이머 기본 버퍼", framingType: FramingStream, opts: FramerOptions{}},
		{name: "frame 프레이머 생성", framingType: FramingFrame, opts: FramerOptions{
			STX: []byte{0x02}, LengthOffset: 1, LengthSize: 1, Checksum: "none",
		}},
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

// --- streamFramer 테스트 ---

// idleReader 는 시리얼 포트의 유휴 타임아웃 동작을 시뮬레이션한다.
// chunks 의 데이터를 순서대로 반환하고, 모든 데이터를 반환한 뒤에는
// n=0, err=nil (타임아웃 시뮬레이션)을 반환한다.
type idleReader struct {
	chunks [][]byte
	idx    int
	idles  int // 타임아웃 횟수 (테스트 무한 루프 방지)
}

func (r *idleReader) Read(p []byte) (int, error) {
	if r.idx < len(r.chunks) {
		n := copy(p, r.chunks[r.idx])
		r.idx++
		return n, nil
	}
	r.idles++
	return 0, nil // 시리얼 포트 타임아웃 시뮬레이션
}

func TestStreamFramer_Creation(t *testing.T) {
	f, err := NewSerialFramer(FramingStream, FramerOptions{BufferSize: 1024})
	if err != nil {
		t.Fatalf("streamFramer 생성 실패: %v", err)
	}
	if f == nil {
		t.Fatal("streamFramer 가 nil")
	}
}

func TestStreamFramer_AccumulatesUntilIdle(t *testing.T) {
	// 3개의 청크를 순서대로 수신 → 타임아웃 → 하나의 프레임으로 반환
	r := &idleReader{
		chunks: [][]byte{
			[]byte("hel"),
			[]byte("lo "),
			[]byte("world"),
		},
	}

	f, _ := NewSerialFramer(FramingStream, FramerOptions{BufferSize: 1024})
	got, err := f.Read(r)
	if err != nil {
		t.Fatalf("읽기 오류: %v", err)
	}
	want := []byte("hello world")
	if !bytes.Equal(got, want) {
		t.Fatalf("기대값: %q, 실제값: %q", want, got)
	}
}

func TestStreamFramer_SingleChunk(t *testing.T) {
	r := &idleReader{
		chunks: [][]byte{[]byte("single packet")},
	}

	f, _ := NewSerialFramer(FramingStream, FramerOptions{BufferSize: 1024})
	got, err := f.Read(r)
	if err != nil {
		t.Fatalf("읽기 오류: %v", err)
	}
	if string(got) != "single packet" {
		t.Fatalf("기대값: %q, 실제값: %q", "single packet", got)
	}
}

func TestStreamFramer_ErrorWithNoData(t *testing.T) {
	// 데이터 없이 에러 발생 시 에러 전파
	errReader := &errorAfterIdleReader{idleCount: 2, err: io.EOF}

	f, _ := NewSerialFramer(FramingStream, FramerOptions{BufferSize: 1024})
	_, err := f.Read(errReader)
	if err != io.EOF {
		t.Fatalf("기대한 오류: io.EOF, 실제: %v", err)
	}
}

// errorAfterIdleReader 는 일정 횟수 타임아웃 후 에러를 반환한다.
type errorAfterIdleReader struct {
	idleCount int
	count     int
	err       error
}

func (r *errorAfterIdleReader) Read(p []byte) (int, error) {
	r.count++
	if r.count > r.idleCount {
		return 0, r.err
	}
	return 0, nil
}

func TestStreamFramer_Write(t *testing.T) {
	f, _ := NewSerialFramer(FramingStream, FramerOptions{BufferSize: 1024})
	var buf bytes.Buffer
	data := []byte("write test data")
	if err := f.Write(&buf, data); err != nil {
		t.Fatalf("쓰기 오류: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("기대값: %q, 실제값: %q", data, buf.Bytes())
	}
}

func TestStreamFramer_DefaultBufferSize(t *testing.T) {
	// BufferSize 를 0 으로 지정하면 기본값(4096) 사용
	f, err := NewSerialFramer(FramingStream, FramerOptions{BufferSize: 0})
	if err != nil {
		t.Fatalf("streamFramer 생성 실패: %v", err)
	}
	sf := f.(*streamFramer)
	if sf.bufferSize != DefaultBufferSize {
		t.Fatalf("기본 버퍼 크기: %d, 실제: %d", DefaultBufferSize, sf.bufferSize)
	}
}

func TestSerialConnReader_StreamFramer(t *testing.T) {
	// SerialConnReader 를 통해 streamFramer 사용
	r := &idleReader{
		chunks: [][]byte{
			[]byte("chunk1"),
			[]byte("chunk2"),
		},
	}
	f, _ := NewSerialFramer(FramingStream, FramerOptions{BufferSize: 1024})
	cr := NewSerialConnReader(f, r)

	got, err := cr.Read()
	if err != nil {
		t.Fatalf("읽기 오류: %v", err)
	}
	if string(got) != "chunk1chunk2" {
		t.Fatalf("기대값: %q, 실제값: %q", "chunk1chunk2", got)
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

// --- frameFramer 테스트 ---

func TestFrameFramer_Read(t *testing.T) {
	tests := []struct {
		name    string
		opts    FramerOptions
		input   []byte
		want    []byte
		wantErr error
	}{
		{
			name: "단일바이트 STX 프레임 수신",
			opts: FramerOptions{
				STX:          []byte{0x02},
				LengthOffset: 1,
				LengthSize:   1,
				Checksum:     "none",
			},
			// STX(0x02) + length(0x03) + payload('a','b','c') = 5 바이트
			input: []byte{0x02, 0x03, 'a', 'b', 'c'},
			want:  []byte{0x02, 0x03, 'a', 'b', 'c'},
		},
		{
			name: "멀티바이트 STX 프레임 수신",
			opts: FramerOptions{
				STX:          []byte{0xAA, 0x55},
				LengthOffset: 2,
				LengthSize:   1,
				Checksum:     "none",
			},
			// STX(0xAA,0x55) + length(0x02) + payload('h','i') = 5 바이트
			input: []byte{0xAA, 0x55, 0x02, 'h', 'i'},
			want:  []byte{0xAA, 0x55, 0x02, 'h', 'i'},
		},
		{
			name: "ETX 포함 프레임 수신",
			opts: FramerOptions{
				STX:          []byte{0x02},
				ETX:          []byte{0x03},
				LengthOffset: 1,
				LengthSize:   1,
				Checksum:     "none",
			},
			// STX(0x02) + length(0x02) + payload('a') + ETX(0x03) = 4 바이트
			// length=2 는 페이로드(1) + ETX(1) 포함
			input: []byte{0x02, 0x02, 'a', 0x03},
			want:  []byte{0x02, 0x02, 'a', 0x03},
		},
		{
			name: "ETX 불일치 오류",
			opts: FramerOptions{
				STX:          []byte{0x02},
				ETX:          []byte{0x03},
				LengthOffset: 1,
				LengthSize:   1,
				Checksum:     "none",
			},
			// ETX 위치에 0xFF (잘못된 ETX)
			input:   []byte{0x02, 0x02, 'a', 0xFF},
			wantErr: ErrETXMismatch,
		},
		{
			name: "sum8 체크섬 정상 수신",
			opts: FramerOptions{
				STX:          []byte{0x02},
				LengthOffset: 1,
				LengthSize:   1,
				Checksum:     "sum8",
			},
			// STX(0x02) + length(0x02) + payload('A') + checksum
			// frame 데이터: [0x02, 0x02, 'A', cs]
			// length=2 는 payload(1) + checksum(1)
			// cs = sum8([0x02, 0x02, 0x41]) = 0x02+0x02+0x41 = 0x45
			input: []byte{0x02, 0x02, 'A', 0x45},
			want:  []byte{0x02, 0x02, 'A', 0x45},
		},
		{
			name: "XOR 체크섬 정상 수신",
			opts: FramerOptions{
				STX:          []byte{0x02},
				LengthOffset: 1,
				LengthSize:   1,
				Checksum:     "xor",
			},
			// frame 데이터: [0x02, 0x02, 'A', cs]
			// cs = xor([0x02, 0x02, 0x41]) = 0x02^0x02^0x41 = 0x41
			input: []byte{0x02, 0x02, 'A', 0x41},
			want:  []byte{0x02, 0x02, 'A', 0x41},
		},
		{
			name: "체크섬 불일치 오류",
			opts: FramerOptions{
				STX:          []byte{0x02},
				LengthOffset: 1,
				LengthSize:   1,
				Checksum:     "sum8",
			},
			// 잘못된 체크섬 0xFF
			input:   []byte{0x02, 0x02, 'A', 0xFF},
			wantErr: ErrChecksumMismatch,
		},
		{
			name: "2바이트 길이 빅엔디안",
			opts: FramerOptions{
				STX:          []byte{0x02},
				LengthOffset: 1,
				LengthSize:   2,
				LengthEndian: "big",
				Checksum:     "none",
			},
			// STX(0x02) + length(0x00,0x03) + payload('x','y','z') = 6 바이트
			input: []byte{0x02, 0x00, 0x03, 'x', 'y', 'z'},
			want:  []byte{0x02, 0x00, 0x03, 'x', 'y', 'z'},
		},
		{
			name: "2바이트 길이 리틀엔디안",
			opts: FramerOptions{
				STX:          []byte{0x02},
				LengthOffset: 1,
				LengthSize:   2,
				LengthEndian: "little",
				Checksum:     "none",
			},
			// STX(0x02) + length(0x03,0x00) little-endian = 3 + payload('x','y','z') = 6 바이트
			input: []byte{0x02, 0x03, 0x00, 'x', 'y', 'z'},
			want:  []byte{0x02, 0x03, 0x00, 'x', 'y', 'z'},
		},
		{
			name: "길이에 헤더 포함",
			opts: FramerOptions{
				STX:                  []byte{0x02},
				LengthOffset:        1,
				LengthSize:          1,
				LengthIncludesHeader: true,
				Checksum:             "none",
			},
			// STX(0x02) + length(0x05) + payload('a','b','c')
			// length=5 는 헤더(STX+length=2바이트) + 나머지 페이로드(3바이트) 포함
			input: []byte{0x02, 0x05, 'a', 'b', 'c'},
			want:  []byte{0x02, 0x05, 'a', 'b', 'c'},
		},
		{
			name: "STX 앞 노이즈 무시",
			opts: FramerOptions{
				STX:          []byte{0x02},
				LengthOffset: 1,
				LengthSize:   1,
				Checksum:     "none",
			},
			// 노이즈(0xFF, 0xFE, 0xFD) + 정상 프레임
			input: []byte{0xFF, 0xFE, 0xFD, 0x02, 0x02, 'h', 'i'},
			want:  []byte{0x02, 0x02, 'h', 'i'},
		},
		{
			name: "프레임 크기 초과 오류",
			opts: FramerOptions{
				STX:            []byte{0x02},
				LengthOffset:   1,
				LengthSize:     1,
				MaxMessageSize: 10,
				Checksum:       "none",
			},
			// length=20 → 총 22바이트로 maxMessageSize(10) 초과
			input:   append([]byte{0x02, 20}, bytes.Repeat([]byte{'X'}, 20)...),
			wantErr: ErrFrameTooLarge,
		},
		{
			name: "length_adjustment 양수 보정",
			opts: FramerOptions{
				STX:              []byte{0x02},
				LengthOffset:     1,
				LengthSize:       1,
				Checksum:         "none",
				LengthAdjustment: 2,
			},
			// STX(0x02) + length(0x01) + payload 3바이트 (1+2 보정)
			input: []byte{0x02, 0x01, 'a', 'b', 'c'},
			want:  []byte{0x02, 0x01, 'a', 'b', 'c'},
		},
		{
			name: "length_adjustment 음수 보정",
			opts: FramerOptions{
				STX:              []byte{0x02},
				LengthOffset:     1,
				LengthSize:       1,
				Checksum:         "none",
				LengthAdjustment: -1,
			},
			// STX(0x02) + length(0x03) + payload 2바이트 (3-1 보정)
			input: []byte{0x02, 0x03, 'a', 'b'},
			want:  []byte{0x02, 0x03, 'a', 'b'},
		},
		{
			name: "NASA 프로토콜 length_adjustment -1 + ETX",
			opts: FramerOptions{
				STX:              []byte{0x32},
				ETX:              []byte{0x34},
				LengthOffset:     1,
				LengthSize:       2,
				LengthEndian:     "big",
				LengthAdjustment: -1,
				Checksum:         "none",
			},
			// NASA: STX(0x32) + LEN(0x00,0x06) + body(2B) + CRC(2B) + ETX(0x34)
			// LEN=6 은 LEN(2)+body(2)+CRC(2) 포함, STX/ETX 미포함
			// header = STX+LEN = 3B, payloadLen = 6 + (-1) = 5
			// payload 5B = body(2) + CRC(2) + ETX(1)
			input: []byte{0x32, 0x00, 0x06, 0x41, 0x42, 0xDE, 0xAD, 0x34},
			want:  []byte{0x32, 0x00, 0x06, 0x41, 0x42, 0xDE, 0xAD, 0x34},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewSerialFramer(FramingFrame, tt.opts)
			if err != nil {
				t.Fatalf("프레이머 생성 오류: %v", err)
			}

			r := bytes.NewReader(tt.input)
			got, readErr := f.Read(r)

			if tt.wantErr != nil {
				if readErr == nil {
					t.Fatal("오류를 기대했으나 nil 반환")
				}
				if !errors.Is(readErr, tt.wantErr) {
					t.Fatalf("기대한 오류: %v, 실제: %v", tt.wantErr, readErr)
				}
				return
			}
			if readErr != nil {
				t.Fatalf("예상치 못한 오류: %v", readErr)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("기대값: %x, 실제값: %x", tt.want, got)
			}
		})
	}
}

func TestFrameFramer_Write(t *testing.T) {
	// frameFramer.Write 는 데이터를 그대로 전달한다
	f, err := NewSerialFramer(FramingFrame, FramerOptions{
		STX:          []byte{0x02},
		LengthOffset: 1,
		LengthSize:   1,
		Checksum:     "none",
	})
	if err != nil {
		t.Fatalf("프레이머 생성 오류: %v", err)
	}

	data := []byte{0x02, 0x03, 'a', 'b', 'c'}
	var buf bytes.Buffer
	if writeErr := f.Write(&buf, data); writeErr != nil {
		t.Fatalf("쓰기 오류: %v", writeErr)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("기대값: %x, 실제값: %x", data, buf.Bytes())
	}
}

// --- 체크섬 헬퍼 함수 테스트 ---

func TestChecksumHelpers(t *testing.T) {
	t.Run("checksumSum8 계산", func(t *testing.T) {
		// sum8([0x01, 0x02, 0x03]) = 0x06
		data := []byte{0x01, 0x02, 0x03}
		got := checksumSum8(data)
		if got != 0x06 {
			t.Fatalf("기대값: 0x06, 실제값: 0x%02X", got)
		}

		// sum8 오버플로우: [0xFF, 0x01] = 0x00 (wrap around)
		data2 := []byte{0xFF, 0x01}
		got2 := checksumSum8(data2)
		if got2 != 0x00 {
			t.Fatalf("기대값: 0x00, 실제값: 0x%02X", got2)
		}

		// 빈 데이터
		got3 := checksumSum8([]byte{})
		if got3 != 0x00 {
			t.Fatalf("기대값: 0x00, 실제값: 0x%02X", got3)
		}
	})

	t.Run("checksumXOR 계산", func(t *testing.T) {
		// xor([0x01, 0x02, 0x03]) = 0x01^0x02^0x03 = 0x00
		data := []byte{0x01, 0x02, 0x03}
		got := checksumXOR(data)
		if got != 0x00 {
			t.Fatalf("기대값: 0x00, 실제값: 0x%02X", got)
		}

		// xor([0xFF, 0x0F]) = 0xF0
		data2 := []byte{0xFF, 0x0F}
		got2 := checksumXOR(data2)
		if got2 != 0xF0 {
			t.Fatalf("기대값: 0xF0, 실제값: 0x%02X", got2)
		}

		// 빈 데이터
		got3 := checksumXOR([]byte{})
		if got3 != 0x00 {
			t.Fatalf("기대값: 0x00, 실제값: 0x%02X", got3)
		}
	})
}
