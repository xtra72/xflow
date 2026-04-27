package serial

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/xtra/xflow/pkg/framing"
)

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

// --- SerialConnReader 테스트 ---
//
// framer 엔진 자체의 단위 테스트는 pkg/framing/framing_test.go 로 이동되었다.
// 본 파일은 시리얼 에이전트 전용 래퍼인 SerialConnReader 의 동작만 검증한다.

func TestSerialConnReader_NewlineFramer(t *testing.T) {
	// SerialConnReader 는 newlineFramer 에서 scanner 를 재사용한다
	f, err := framing.New(framing.ModeNewline, framing.Options{BufferSize: 1024})
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
	f, err := framing.New(framing.ModeRaw, framing.Options{BufferSize: 1024})
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
	f, err := framing.New(framing.ModeLengthPrefix, framing.Options{})
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
	f, err := framing.New(framing.ModeFixedSize, framing.Options{FixedSize: fixedSize})
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

func TestSerialConnReader_StreamFramer(t *testing.T) {
	// SerialConnReader 를 통해 streamFramer 사용
	r := &idleReader{
		chunks: [][]byte{
			[]byte("chunk1"),
			[]byte("chunk2"),
		},
	}
	f, _ := framing.New(framing.ModeStream, framing.Options{BufferSize: 1024})
	cr := NewSerialConnReader(f, r)

	got, err := cr.Read()
	if err != nil {
		t.Fatalf("읽기 오류: %v", err)
	}
	if string(got) != "chunk1chunk2" {
		t.Fatalf("기대값: %q, 실제값: %q", "chunk1chunk2", got)
	}
}

func TestSerialConnReader_NewlineFramer_ScannerError(t *testing.T) {
	// Test SerialConnReader with newlineFramer when scanner encounters error
	f, err := framing.New(framing.ModeNewline, framing.Options{BufferSize: 8})
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
