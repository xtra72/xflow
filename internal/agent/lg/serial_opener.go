package lg

import (
	"fmt"
	"io"
	"time"

	"go.bug.st/serial"
)

func init() {
	if LGAPSerialOpener == nil {
		LGAPSerialOpener = defaultSerialOpener
	}
}

// defaultSerialOpener 는 go.bug.st/serial을 사용하는 기본 시리얼 포트 opener이다.
func defaultSerialOpener(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error) {
	mode := &serial.Mode{
		BaudRate: baudRate,
		DataBits: dataBits,
	}

	// stopBits 변환
	switch stopBits {
	case 1:
		mode.StopBits = serial.OneStopBit
	case 2:
		mode.StopBits = serial.TwoStopBits
	default:
		mode.StopBits = serial.OneStopBit
	}

	// parity 변환
	switch parity {
	case "none", "":
		mode.Parity = serial.NoParity
	case "even":
		mode.Parity = serial.EvenParity
	case "odd":
		mode.Parity = serial.OddParity
	case "mark":
		mode.Parity = serial.MarkParity
	case "space":
		mode.Parity = serial.SpaceParity
	default:
		return nil, fmt.Errorf("unsupported parity: %s", parity)
	}

	conn, err := serial.Open(port, mode)
	if err != nil {
		return nil, fmt.Errorf("serial open %s: %w", port, err)
	}

	// Read 가 무한 블로킹되지 않도록 타임아웃 설정.
	// LGAP 기본 read_timeout: 500ms (config.go 참조)
	if err := conn.SetReadTimeout(500 * time.Millisecond); err != nil {
		conn.Close()
		return nil, fmt.Errorf("serial set read timeout %s: %w", port, err)
	}

	return conn, nil
}
