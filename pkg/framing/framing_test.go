package framing

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"testing"
)

// --- New 팩토리 테스트 ---

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		opts    Options
		wantErr error
	}{
		{name: "raw 프레이머 생성", mode: ModeRaw, opts: Options{BufferSize: 1024}},
		{name: "raw 프레이머 기본 버퍼", mode: ModeRaw, opts: Options{}},
		{name: "newline 프레이머 생성", mode: ModeNewline, opts: Options{BufferSize: 1024}},
		{name: "newline 프레이머 기본값", mode: ModeNewline, opts: Options{}},
		{name: "length_prefix 프레이머 생성", mode: ModeLengthPrefix, opts: Options{MaxMessageSize: 1024}},
		{name: "fixed_size 프레이머 생성", mode: ModeFixedSize, opts: Options{FixedSize: 64}},
		{name: "stream 프레이머 생성", mode: ModeStream, opts: Options{BufferSize: 1024}},
		{name: "stream 프레이머 기본 버퍼", mode: ModeStream, opts: Options{}},
		{name: "frame 프레이머 생성", mode: ModeFrame, opts: Options{
			STX: []byte{0x02}, LengthOffset: 1, LengthSize: 1, Checksum: "none",
		}},
		{name: "잘못된 프레이밍 타입", mode: "unknown", wantErr: ErrInvalidFraming},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := New(tt.mode, tt.opts)
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
	f, err := New(ModeRaw, Options{BufferSize: 1024})
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
	f, err := New(ModeRaw, Options{BufferSize: bufSize})
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
	f, err := New(ModeRaw, Options{BufferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	_, readErr := f.Read(&buf)
	if readErr == nil {
		t.Fatal("빈 리더에서 오류를 기대했으나 nil 반환")
	}
}

// TestRawFramer_Read_CapLimited 는 rawFramer.Read 가 반환하는 슬라이스의
// cap 이 len 으로 제한되는지 검증한다 (2026-05-14 hotfix).
//
// 수정 전(버그): Read 가 buf[:n] 을 반환하므로 cap == bufferSize 이다.
// downstream 의 append 가 공유 backing 배열에 써넣어 다른 프레임을 변조할 수 있다.
// 수정 후: buf[:n:n] (three-index slice) 로 cap 을 n 으로 제한하여 append 가
// 반드시 새 배열을 할당하도록 강제한다.
func TestRawFramer_Read_CapLimited(t *testing.T) {
	bufSize := 1024
	f, err := New(ModeRaw, Options{BufferSize: bufSize})
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("hello raw framing")
	got, readErr := f.Read(bytes.NewReader(data))
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

// --- NewlineFramer 테스트 ---

func TestNewlineFramer_ReadWrite(t *testing.T) {
	f, err := New(ModeNewline, Options{BufferSize: 1024})
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
	f, err := New(ModeNewline, Options{BufferSize: 1024, Delimiter: '\t'})
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
	f, err := New(ModeNewline, Options{BufferSize: 1024})
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

	// ConfigureScanner 를 통해 scanner 를 재사용해야 다중 메시지 읽기가 정상 동작한다.
	// newlineFramer.Read() 는 매번 새 scanner 를 생성하므로 버퍼된 데이터를 잃는다.
	sc, ok := f.(ScannerConfigurer)
	if !ok {
		t.Fatal("newline framer 가 ScannerConfigurer 를 구현해야 함")
	}
	scanner := sc.ConfigureScanner(&buf)
	for _, want := range msgs {
		if !scanner.Scan() {
			t.Fatalf("scan 실패: %v", scanner.Err())
		}
		if string(scanner.Bytes()) != want {
			t.Fatalf("기대값: %q, 실제값: %q", want, string(scanner.Bytes()))
		}
	}
}

// --- LengthPrefixFramer 테스트 ---

func TestLengthPrefixFramer_ReadWrite(t *testing.T) {
	f, err := New(ModeLengthPrefix, Options{})
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
	f, err := New(ModeLengthPrefix, Options{MaxMessageSize: maxSize})
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
	f, err := New(ModeLengthPrefix, Options{MaxMessageSize: maxSize})
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
	f, err := New(ModeLengthPrefix, Options{})
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
	f, err := New(ModeLengthPrefix, Options{})
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
	f, err := New(ModeFixedSize, Options{FixedSize: fixedSize})
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
	f, err := New(ModeFixedSize, Options{FixedSize: fixedSize})
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
	f, err := New(ModeFixedSize, Options{FixedSize: fixedSize})
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
	f, err := New(ModeFixedSize, Options{FixedSize: fixedSize})
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

// --- newlineFramer 추가 엣지 케이스 ---

func TestNewlineFramer_ReadEOF(t *testing.T) {
	// newlineFramer.Read on empty reader returns io.EOF
	f, err := New(ModeNewline, Options{BufferSize: 1024})
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
	f, err := New(ModeNewline, Options{BufferSize: 8})
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
	f, err := New(ModeNewline, Options{BufferSize: 1024})
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
	f, err := New(ModeLengthPrefix, Options{})
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
	f, err := New(ModeLengthPrefix, Options{})
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
	f, err := New(ModeLengthPrefix, Options{})
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
	f, err := New(ModeLengthPrefix, Options{MaxMessageSize: 0})
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
	f, err := New(ModeStream, Options{BufferSize: 1024})
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

	f, _ := New(ModeStream, Options{BufferSize: 1024})
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

	f, _ := New(ModeStream, Options{BufferSize: 1024})
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

	f, _ := New(ModeStream, Options{BufferSize: 1024})
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
	f, _ := New(ModeStream, Options{BufferSize: 1024})
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
	f, err := New(ModeStream, Options{BufferSize: 0})
	if err != nil {
		t.Fatalf("streamFramer 생성 실패: %v", err)
	}
	sf := f.(*streamFramer)
	if sf.bufferSize != DefaultBufferSize {
		t.Fatalf("기본 버퍼 크기: %d, 실제: %d", DefaultBufferSize, sf.bufferSize)
	}
}

// --- frameFramer 테스트 ---

func TestFrameFramer_Read(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		input   []byte
		want    []byte
		wantErr error
	}{
		{
			name: "단일바이트 STX 프레임 수신",
			opts: Options{
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
			opts: Options{
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
			opts: Options{
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
			opts: Options{
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
			opts: Options{
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
			opts: Options{
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
			opts: Options{
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
			opts: Options{
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
			opts: Options{
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
			opts: Options{
				STX:                  []byte{0x02},
				LengthOffset:         1,
				LengthSize:           1,
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
			opts: Options{
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
			opts: Options{
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
			opts: Options{
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
			opts: Options{
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
			opts: Options{
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
			f, err := New(ModeFrame, tt.opts)
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

// TestFrameFramer_LGCPSamples 는 실제 LGCP 프로토콜 샘플로 frameFramer 를
// 검증한다. LGCP 프레임 구조:
//
//	56 [LEN] 04 [DA 4B] 04 [SA 4B] [CMD 2B] [SEQ0] [PLEN] [PAYLOAD] [SEQ1] [CRC16]
//
// LEN 필드는 전체 프레임 길이(STX 포함)이므로
// length_includes_header=true 와 length_adjustment=0 로 설정해야 한다.
// 참조: references/protocols/LGCP_Protocol_Analysis.md §3
func TestFrameFramer_LGCPSamples(t *testing.T) {
	lgcpOpts := Options{
		STX:                  []byte{0x56},
		LengthOffset:         1,
		LengthSize:           1,
		LengthEndian:         "big",
		LengthIncludesHeader: true,
		LengthAdjustment:     0,
		Checksum:             "none",
		MaxMessageSize:       256,
	}

	// lgcp-valid.jsonl 에서 추출한 실제 샘플
	samples := []struct {
		name string
		hex  string
		size int
	}{
		{
			name: "plen=26 요청 프레임 (cmd=0204 status)",
			hex:  "562d044455006504445500000204b11a110010c018001ac01300134013c016001841188829c01dc03954a06492",
			size: 45,
		},
		{
			name: "plen=41 응답 프레임 (cmd=0204 status)",
			hex:  "563c044455000004445500670204df2960c161d05e62104c624162d0c86f816fc07490198a8060816100b0406400645054648a64c065411fd857e37a",
			size: 60,
		},
		{
			name: "plen=1 Keep-alive (cmd=0604 keepalive)",
			hex:  "561404ffffffff04445500000604000102a1b7ed",
			size: 20,
		},
	}

	for _, s := range samples {
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			raw, err := hex.DecodeString(s.hex)
			if err != nil {
				t.Fatalf("hex 디코딩 실패: %v", err)
			}
			if len(raw) != s.size {
				t.Fatalf("샘플 크기 불일치: 기대 %d, 실제 %d", s.size, len(raw))
			}

			f, err := New(ModeFrame, lgcpOpts)
			if err != nil {
				t.Fatalf("framer 생성 실패: %v", err)
			}

			got, err := f.Read(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("Read 실패: %v", err)
			}
			if !bytes.Equal(got, raw) {
				t.Fatalf("프레임 불일치\n기대: %x\n실제: %x", raw, got)
			}
		})
	}

	// 연속 프레임 스트림 검증 — 세 프레임을 연결해서 하나씩 올바르게 떼어내는지
	t.Run("연속 프레임 스트림에서 개별 프레임 분리", func(t *testing.T) {
		t.Parallel()
		var stream bytes.Buffer
		expected := make([][]byte, 0, len(samples))
		for _, s := range samples {
			raw, _ := hex.DecodeString(s.hex)
			stream.Write(raw)
			expected = append(expected, raw)
		}

		f, err := New(ModeFrame, lgcpOpts)
		if err != nil {
			t.Fatalf("framer 생성 실패: %v", err)
		}

		reader := bytes.NewReader(stream.Bytes())
		for i, want := range expected {
			got, err := f.Read(reader)
			if err != nil {
				t.Fatalf("프레임 %d Read 실패: %v", i, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("프레임 %d 불일치\n기대: %x\n실제: %x", i, want, got)
			}
		}

		// 네 번째 Read 는 EOF 여야 한다
		if _, err := f.Read(reader); err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("스트림 끝에서 EOF 기대, 실제 오류: %v", err)
		}
	})

	// 현재 examples/agents/serial-lgcp-capture.yaml 의 이전 설정
	// (length_includes_header=false, length_adjustment=-1) 이 잘못되었음을
	// 회귀 방지 차원에서 명시적으로 검증한다. 이 설정으로 LEN=45 프레임을
	// 읽으면 total 46 바이트를 읽으려 하여 다음 프레임의 첫 바이트를
	// 소비하며 동기화가 깨진다.
	t.Run("잘못된 설정 회귀 방지: length_includes_header=false + adjustment=-1", func(t *testing.T) {
		t.Parallel()
		brokenOpts := Options{
			STX:                  []byte{0x56},
			LengthOffset:         1,
			LengthSize:           1,
			LengthEndian:         "big",
			LengthIncludesHeader: false,
			LengthAdjustment:     -1,
			Checksum:             "none",
			MaxMessageSize:       256,
		}

		// plen=26 (45B) + plen=1 keep-alive (20B) 연결
		raw1, _ := hex.DecodeString(samples[0].hex) // 45B
		raw3, _ := hex.DecodeString(samples[2].hex) // 20B
		var stream bytes.Buffer
		stream.Write(raw1)
		stream.Write(raw3)

		f, err := New(ModeFrame, brokenOpts)
		if err != nil {
			t.Fatalf("framer 생성 실패: %v", err)
		}

		got, err := f.Read(bytes.NewReader(stream.Bytes()))
		if err != nil {
			t.Fatalf("첫 Read 실패: %v", err)
		}
		// 잘못된 설정으로는 원본 프레임과 일치하지 않아야 한다
		// (1바이트 초과 읽기로 다음 프레임의 0x56 을 먹어버림)
		if bytes.Equal(got, raw1) {
			t.Fatalf("잘못된 설정인데 정상 프레임이 반환됨 — 테스트 가정 오류")
		}
		if len(got) != len(raw1)+1 {
			t.Fatalf("잘못된 설정으로 1바이트 초과 읽기 기대, 실제 길이: %d (기대: %d)", len(got), len(raw1)+1)
		}
	})
}

func TestFrameFramer_Write(t *testing.T) {
	// frameFramer.Write 는 데이터를 그대로 전달한다
	f, err := New(ModeFrame, Options{
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

// --- Drain API 테스트 ---
//
// Drain(buf []byte) ([][]byte, []byte, error) 는 인메모리 버퍼에서 완성된
// 프레임을 모두 추출하는 byte-slice 기반 API 이다. Read(io.Reader) 와 달리
// 부분 프레임을 호출자가 유지할 remainder 로 돌려준다. framer 노드의
// rolling-buffer 경로에서 사용된다.

func TestRawFramer_Drain_WholeBufferAsOneFrame(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeRaw, Options{BufferSize: 1024})
	frames, rem, err := f.Drain([]byte("hello"))
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 1 || string(frames[0]) != "hello" {
		t.Fatalf("기대 1개 프레임 'hello', 실제: %v", frames)
	}
	if len(rem) != 0 {
		t.Fatalf("remainder 가 비어있어야 함, 실제: %q", rem)
	}
}

func TestRawFramer_Drain_EmptyBuffer(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeRaw, Options{BufferSize: 1024})
	frames, rem, err := f.Drain([]byte{})
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("빈 버퍼에서 0개 프레임 기대, 실제: %d", len(frames))
	}
	if len(rem) != 0 {
		t.Fatalf("빈 remainder 기대, 실제: %d", len(rem))
	}
}

func TestNewlineFramer_Drain_SingleLine(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeNewline, Options{BufferSize: 1024})
	frames, rem, err := f.Drain([]byte("hello\n"))
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("기대 프레임 수 1, 실제: %d", len(frames))
	}
	// newlineFramer 의 splitFunc 은 delimiter 를 제외한 토큰을 반환한다.
	if string(frames[0]) != "hello" {
		t.Fatalf("기대 'hello', 실제: %q", frames[0])
	}
	if len(rem) != 0 {
		t.Fatalf("remainder 가 비어있어야 함, 실제: %q", rem)
	}
}

func TestNewlineFramer_Drain_MultipleLines(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeNewline, Options{BufferSize: 1024})
	frames, rem, err := f.Drain([]byte("a\nbb\nccc\n"))
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	want := []string{"a", "bb", "ccc"}
	if len(frames) != len(want) {
		t.Fatalf("기대 프레임 수 %d, 실제: %d", len(want), len(frames))
	}
	for i, w := range want {
		if string(frames[i]) != w {
			t.Fatalf("프레임 %d: 기대 %q, 실제 %q", i, w, frames[i])
		}
	}
	if len(rem) != 0 {
		t.Fatalf("remainder 가 비어있어야 함, 실제: %q", rem)
	}
}

func TestNewlineFramer_Drain_PartialLine(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeNewline, Options{BufferSize: 1024})
	frames, rem, err := f.Drain([]byte("a\nbb"))
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 1 || string(frames[0]) != "a" {
		t.Fatalf("기대 1개 프레임 'a', 실제: %v", frames)
	}
	if string(rem) != "bb" {
		t.Fatalf("remainder 기대 'bb', 실제: %q", rem)
	}
}

func TestNewlineFramer_Drain_CustomDelimiter(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeNewline, Options{BufferSize: 1024, Delimiter: '|'})
	frames, rem, err := f.Drain([]byte("a|bb|ccc"))
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("기대 2개 프레임, 실제: %d", len(frames))
	}
	if string(frames[0]) != "a" || string(frames[1]) != "bb" {
		t.Fatalf("프레임 불일치: %q, %q", frames[0], frames[1])
	}
	if string(rem) != "ccc" {
		t.Fatalf("remainder 기대 'ccc', 실제: %q", rem)
	}
}

func TestNewlineFramer_Drain_EmptyBuffer(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeNewline, Options{BufferSize: 1024})
	frames, rem, err := f.Drain([]byte{})
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("0개 프레임 기대, 실제: %d", len(frames))
	}
	if len(rem) != 0 {
		t.Fatalf("비어있는 remainder 기대, 실제: %d", len(rem))
	}
}

func TestLengthPrefixFramer_Drain_SingleFrame(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeLengthPrefix, Options{})
	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, 5)
	buf.Write(header)
	buf.Write([]byte("hello"))

	frames, rem, err := f.Drain(buf.Bytes())
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 1 || string(frames[0]) != "hello" {
		t.Fatalf("기대 1개 프레임 'hello', 실제: %v", frames)
	}
	if len(rem) != 0 {
		t.Fatalf("비어있는 remainder 기대, 실제: %q", rem)
	}
}

func TestLengthPrefixFramer_Drain_MultipleFrames(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeLengthPrefix, Options{})
	var buf bytes.Buffer
	// 3 프레임: "AA", "BBB", "CCCC"
	for _, payload := range []string{"AA", "BBB", "CCCC"} {
		header := make([]byte, 4)
		binary.BigEndian.PutUint32(header, uint32(len(payload)))
		buf.Write(header)
		buf.Write([]byte(payload))
	}

	frames, rem, err := f.Drain(buf.Bytes())
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("기대 3개 프레임, 실제: %d", len(frames))
	}
	want := []string{"AA", "BBB", "CCCC"}
	for i, w := range want {
		if string(frames[i]) != w {
			t.Fatalf("프레임 %d: 기대 %q, 실제 %q", i, w, frames[i])
		}
	}
	if len(rem) != 0 {
		t.Fatalf("비어있는 remainder 기대, 실제: %q", rem)
	}
}

func TestLengthPrefixFramer_Drain_PartialHeader(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeLengthPrefix, Options{})
	// 4 바이트 미만 → 헤더 불완전
	frames, rem, err := f.Drain([]byte{0x00, 0x00})
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("0개 프레임 기대, 실제: %d", len(frames))
	}
	if len(rem) != 2 {
		t.Fatalf("remainder 2 바이트 기대, 실제: %d", len(rem))
	}
}

func TestLengthPrefixFramer_Drain_PartialPayload(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeLengthPrefix, Options{})
	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, 10)
	buf.Write(header)
	buf.Write([]byte("abc")) // 10 바이트 중 3 바이트만

	frames, rem, err := f.Drain(buf.Bytes())
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("0개 프레임 기대, 실제: %d", len(frames))
	}
	if len(rem) != buf.Len() {
		t.Fatalf("remainder 가 전체 버퍼를 유지해야 함, 기대 %d, 실제 %d", buf.Len(), len(rem))
	}
}

func TestLengthPrefixFramer_Drain_MaxSizeExceeded(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeLengthPrefix, Options{MaxMessageSize: 100})
	// 길이 필드가 200 을 지시 → 최대 크기 초과
	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, 200)
	buf.Write(header)

	_, _, err := f.Drain(buf.Bytes())
	if err == nil {
		t.Fatal("오류를 기대했으나 nil 반환")
	}
	if !errors.Is(err, ErrMaxMessageSize) {
		t.Fatalf("기대한 오류: ErrMaxMessageSize, 실제: %v", err)
	}
}

func TestLengthPrefixFramer_Drain_FrameThenPartial(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeLengthPrefix, Options{})
	var buf bytes.Buffer
	// 첫 프레임: "AAA" (완전)
	h1 := make([]byte, 4)
	binary.BigEndian.PutUint32(h1, 3)
	buf.Write(h1)
	buf.Write([]byte("AAA"))
	// 두 번째 프레임: 길이 5 지시, 2 바이트만 제공 (불완전)
	h2 := make([]byte, 4)
	binary.BigEndian.PutUint32(h2, 5)
	buf.Write(h2)
	buf.Write([]byte("BB"))

	frames, rem, err := f.Drain(buf.Bytes())
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 1 || string(frames[0]) != "AAA" {
		t.Fatalf("기대 1개 프레임 'AAA', 실제: %v", frames)
	}
	// remainder 는 4 바이트 헤더 + 2 바이트 = 6 바이트 이어야 한다
	if len(rem) != 6 {
		t.Fatalf("remainder 6 바이트 기대, 실제: %d", len(rem))
	}
}

func TestFixedSizeFramer_Drain_MultipleFrames(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeFixedSize, Options{FixedSize: 3})
	frames, rem, err := f.Drain([]byte("abcdefghij"))
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("기대 3개 프레임, 실제: %d", len(frames))
	}
	want := []string{"abc", "def", "ghi"}
	for i, w := range want {
		if string(frames[i]) != w {
			t.Fatalf("프레임 %d: 기대 %q, 실제 %q", i, w, frames[i])
		}
	}
	if string(rem) != "j" {
		t.Fatalf("remainder 기대 'j', 실제: %q", rem)
	}
}

func TestFixedSizeFramer_Drain_PartialFrame(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeFixedSize, Options{FixedSize: 3})
	frames, rem, err := f.Drain([]byte("ab"))
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("0개 프레임 기대, 실제: %d", len(frames))
	}
	if string(rem) != "ab" {
		t.Fatalf("remainder 기대 'ab', 실제: %q", rem)
	}
}

func TestFixedSizeFramer_Drain_ExactMultiple(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeFixedSize, Options{FixedSize: 2})
	frames, rem, err := f.Drain([]byte("ababab"))
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("기대 3개 프레임, 실제: %d", len(frames))
	}
	if len(rem) != 0 {
		t.Fatalf("remainder 가 비어있어야 함, 실제: %q", rem)
	}
}

func TestStreamFramer_Drain_Passthrough(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeStream, Options{BufferSize: 1024})
	frames, rem, err := f.Drain([]byte("hello world"))
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("기대 1개 프레임, 실제: %d", len(frames))
	}
	if string(frames[0]) != "hello world" {
		t.Fatalf("프레임 불일치: %q", frames[0])
	}
	if len(rem) != 0 {
		t.Fatalf("비어있는 remainder 기대, 실제: %q", rem)
	}
}

func TestStreamFramer_Drain_EmptyBuffer(t *testing.T) {
	t.Parallel()
	f, _ := New(ModeStream, Options{BufferSize: 1024})
	frames, rem, err := f.Drain([]byte{})
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("0개 프레임 기대, 실제: %d", len(frames))
	}
	if len(rem) != 0 {
		t.Fatalf("비어있는 remainder 기대, 실제: %d", len(rem))
	}
}

func TestFrameFramer_Drain_LGCPSingle(t *testing.T) {
	t.Parallel()
	lgcpOpts := Options{
		STX:                  []byte{0x56},
		LengthOffset:         1,
		LengthSize:           1,
		LengthEndian:         "big",
		LengthIncludesHeader: true,
		LengthAdjustment:     0,
		Checksum:             "none",
		MaxMessageSize:       256,
	}
	raw, _ := hex.DecodeString("561404ffffffff04445500000604000102a1b7ed")

	f, _ := New(ModeFrame, lgcpOpts)
	frames, rem, err := f.Drain(raw)
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("기대 1개 프레임, 실제: %d", len(frames))
	}
	if !bytes.Equal(frames[0], raw) {
		t.Fatalf("프레임 불일치\n기대: %x\n실제: %x", raw, frames[0])
	}
	if len(rem) != 0 {
		t.Fatalf("비어있는 remainder 기대, 실제: %d", len(rem))
	}
}

func TestFrameFramer_Drain_LGCPMultipleBackToBack(t *testing.T) {
	t.Parallel()
	lgcpOpts := Options{
		STX:                  []byte{0x56},
		LengthOffset:         1,
		LengthSize:           1,
		LengthEndian:         "big",
		LengthIncludesHeader: true,
		LengthAdjustment:     0,
		Checksum:             "none",
		MaxMessageSize:       256,
	}
	hexes := []string{
		"562d044455006504445500000204b11a110010c018001ac01300134013c016001841188829c01dc03954a06492",
		"563c044455000004445500670204df2960c161d05e62104c624162d0c86f816fc07490198a8060816100b0406400645054648a64c065411fd857e37a",
		"561404ffffffff04445500000604000102a1b7ed",
	}
	var concatenated bytes.Buffer
	var expected [][]byte
	for _, h := range hexes {
		b, _ := hex.DecodeString(h)
		concatenated.Write(b)
		expected = append(expected, b)
	}

	f, _ := New(ModeFrame, lgcpOpts)
	frames, rem, err := f.Drain(concatenated.Bytes())
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != len(expected) {
		t.Fatalf("기대 프레임 수 %d, 실제: %d", len(expected), len(frames))
	}
	for i, want := range expected {
		if !bytes.Equal(frames[i], want) {
			t.Fatalf("프레임 %d 불일치\n기대: %x\n실제: %x", i, want, frames[i])
		}
	}
	if len(rem) != 0 {
		t.Fatalf("비어있는 remainder 기대, 실제: %d", len(rem))
	}
}

func TestFrameFramer_Drain_NoiseBeforeSTX(t *testing.T) {
	t.Parallel()
	opts := Options{
		STX:          []byte{0x02},
		LengthOffset: 1,
		LengthSize:   1,
		Checksum:     "none",
	}
	// 노이즈(0xFF, 0xFE, 0xFD) + 정상 프레임
	input := []byte{0xFF, 0xFE, 0xFD, 0x02, 0x02, 'h', 'i'}
	wantFrame := []byte{0x02, 0x02, 'h', 'i'}

	f, _ := New(ModeFrame, opts)
	frames, _, err := f.Drain(input)
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("기대 1개 프레임, 실제: %d", len(frames))
	}
	if !bytes.Equal(frames[0], wantFrame) {
		t.Fatalf("프레임 불일치\n기대: %x\n실제: %x", wantFrame, frames[0])
	}
}

func TestFrameFramer_Drain_PartialFrame(t *testing.T) {
	t.Parallel()
	// STX + 길이필드 + 페이로드 절반만 있는 경우 → 0개 프레임, remainder = STX 부터 유지
	opts := Options{
		STX:          []byte{0x02},
		LengthOffset: 1,
		LengthSize:   1,
		Checksum:     "none",
	}
	// length=5 지시, 페이로드 3 바이트만 제공
	input := []byte{0x02, 0x05, 'a', 'b', 'c'}

	f, _ := New(ModeFrame, opts)
	frames, rem, err := f.Drain(input)
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("0개 프레임 기대, 실제: %d", len(frames))
	}
	if len(rem) != len(input) {
		t.Fatalf("remainder 가 STX 부터 전체 유지되어야 함, 기대 %d, 실제 %d", len(input), len(rem))
	}
	if !bytes.Equal(rem, input) {
		t.Fatalf("remainder 내용 불일치")
	}
}

func TestFrameFramer_Drain_ETXMismatch(t *testing.T) {
	t.Parallel()
	opts := Options{
		STX:          []byte{0x02},
		ETX:          []byte{0x03},
		LengthOffset: 1,
		LengthSize:   1,
		Checksum:     "none",
	}
	// ETX 위치에 0xFF (잘못된 ETX)
	input := []byte{0x02, 0x02, 'a', 0xFF}

	f, _ := New(ModeFrame, opts)
	_, _, err := f.Drain(input)
	if err == nil {
		t.Fatal("오류를 기대했으나 nil 반환")
	}
	if !errors.Is(err, ErrETXMismatch) {
		t.Fatalf("기대한 오류: ErrETXMismatch, 실제: %v", err)
	}
}

// TestDrain_MatchesRead 는 raw/newline/length_prefix/fixed_size/frame 모드에
// 대해 Drain 과 Read 루프가 생성하는 프레임 시퀀스가 바이트 단위로 동일한지
// 교차 검증한다. stream 모드는 Read (유휴 타임아웃 의존) 와 Drain (즉시 반환)
// 의미가 달라 검증에서 제외한다.
func TestDrain_MatchesRead(t *testing.T) {
	t.Parallel()

	lgcpOpts := Options{
		STX:                  []byte{0x56},
		LengthOffset:         1,
		LengthSize:           1,
		LengthEndian:         "big",
		LengthIncludesHeader: true,
		LengthAdjustment:     0,
		Checksum:             "none",
		MaxMessageSize:       256,
	}
	lgcpRaw, _ := hex.DecodeString("561404ffffffff04445500000604000102a1b7ed")

	// length_prefix 테스트 데이터
	var lpBuf bytes.Buffer
	for _, payload := range []string{"alpha", "beta", "gamma"} {
		h := make([]byte, 4)
		binary.BigEndian.PutUint32(h, uint32(len(payload)))
		lpBuf.Write(h)
		lpBuf.Write([]byte(payload))
	}

	cases := []struct {
		name  string
		mode  string
		opts  Options
		input []byte
	}{
		{
			name:  "raw 단일 청크",
			mode:  ModeRaw,
			opts:  Options{BufferSize: 128},
			input: []byte("raw bytes"),
		},
		{
			name:  "newline 여러 라인",
			mode:  ModeNewline,
			opts:  Options{BufferSize: 1024},
			input: []byte("a\nbb\nccc\n"),
		},
		{
			name:  "length_prefix 3개 프레임",
			mode:  ModeLengthPrefix,
			opts:  Options{},
			input: lpBuf.Bytes(),
		},
		{
			name:  "fixed_size 3개 프레임",
			mode:  ModeFixedSize,
			opts:  Options{FixedSize: 4},
			input: []byte("abcdefghijkl"),
		},
		{
			name:  "frame LGCP keep-alive",
			mode:  ModeFrame,
			opts:  lgcpOpts,
			input: lgcpRaw,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Drain 경로
			fDrain, err := New(tc.mode, tc.opts)
			if err != nil {
				t.Fatalf("Drain framer 생성 실패: %v", err)
			}
			drainFrames, drainRem, drainErr := fDrain.Drain(tc.input)
			if drainErr != nil {
				t.Fatalf("Drain 오류: %v", drainErr)
			}

			// Read 루프 경로. newline 모드는 bufio.Scanner 를 재사용해야
			// 하므로 ScannerConfigurer 경로를 사용한다 (SerialConnReader 와 동일).
			fRead, err := New(tc.mode, tc.opts)
			if err != nil {
				t.Fatalf("Read framer 생성 실패: %v", err)
			}
			var readFrames [][]byte
			reader := bytes.NewReader(tc.input)
			if sc, ok := fRead.(ScannerConfigurer); ok && tc.mode == ModeNewline {
				scanner := sc.ConfigureScanner(reader)
				for scanner.Scan() {
					frame := make([]byte, len(scanner.Bytes()))
					copy(frame, scanner.Bytes())
					readFrames = append(readFrames, frame)
				}
			} else {
				for {
					frame, rerr := fRead.Read(reader)
					if errors.Is(rerr, io.EOF) || errors.Is(rerr, io.ErrUnexpectedEOF) {
						break
					}
					if rerr != nil {
						t.Fatalf("Read 오류: %v", rerr)
					}
					if frame == nil || len(frame) == 0 {
						break
					}
					readFrames = append(readFrames, frame)
					// raw 모드는 한 번 읽으면 전체를 소비한다. 추가 Read 는 EOF.
					if tc.mode == ModeRaw {
						break
					}
				}
			}

			// 비교 (stream 모드 제외). newline 모드의 경우 Read 는 EOF 에
			// 도달하면 잔여 데이터도 프레임으로 반환하므로 drain 결과와 동일해야 한다.
			if len(drainFrames) != len(readFrames) {
				t.Fatalf("프레임 수 불일치: Drain=%d, Read=%d\nDrain: %v\nRead: %v",
					len(drainFrames), len(readFrames), drainFrames, readFrames)
			}
			for i := range drainFrames {
				if !bytes.Equal(drainFrames[i], readFrames[i]) {
					t.Fatalf("프레임 %d 불일치\nDrain: %x\nRead:  %x", i, drainFrames[i], readFrames[i])
				}
			}
			// length_prefix/fixed_size/frame 등 정렬된 버퍼 입력에서는
			// remainder 가 비어있어야 한다. newline 의 경우 마지막 delimiter
			// 가 있는 입력이면 remainder 가 비어있다.
			if len(drainRem) != 0 {
				t.Fatalf("remainder 가 비어있어야 함, 실제: %d 바이트", len(drainRem))
			}
		})
	}
}
