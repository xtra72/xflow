package century

import (
	"fmt"
	"io"
	"time"

	"go.bug.st/serial"
)

// ---------------------------------------------------------------------------
// transport_serial.go — Century 시리얼 트랜스포트 (REQ-CENTURY-002)
//
// v0.1.x 까지 inline 으로 존재하던 (실제로는 production 에서 wiring 되지 않았던) 시리얼
// dialing 을 M6 (v0.2.0) 에서 별도 파일로 분리한다. 동작은 LG / Samsung 의 기존
// defaultSerialOpener 와 동일하며, 본 에이전트는 결과 *serial.Port 를 RX-only 로 사용한다.
// transport.Write() 는 어떠한 경우에도 호출되지 않는다 (AC-B9 invariant).
// ---------------------------------------------------------------------------

// CenturySerialOpener 는 시리얼 포트 opener 의 단위 테스트 hook 이다.
// 운영 코드는 nil 인 경우 defaultCenturySerialOpener 를 사용한다.
var CenturySerialOpener func(port string, baudRate, dataBits, stopBits int, parity string, readTimeout time.Duration) (io.ReadWriteCloser, error)

// openSerialTransport 는 Hvacr01Config 의 시리얼 파라미터로 RS-485 포트를 연다 (REQ-CENTURY-002).
//
// 본 트랜스포트는 RX-only 로 사용된다 — captureLoop 가 Read 만 수행하고
// transport.Write() 는 절대 호출하지 않는다 (AC-B9).
func openSerialTransport(cfg Hvacr01Config) (io.ReadWriteCloser, error) {
	opener := CenturySerialOpener
	if opener == nil {
		opener = defaultCenturySerialOpener
	}
	readTimeout := 200 * time.Millisecond
	return opener(cfg.SerialPort, cfg.BaudRate, cfg.DataBits, cfg.StopBits, cfg.Parity, readTimeout)
}

// defaultCenturySerialOpener 는 go.bug.st/serial 을 사용해 시리얼 포트를 연다.
// LG / Samsung 의 defaultSerialOpener 와 구조적으로 동일하다.
func defaultCenturySerialOpener(port string, baudRate, dataBits, stopBits int, parity string, readTimeout time.Duration) (io.ReadWriteCloser, error) {
	mode := &serial.Mode{
		BaudRate: baudRate,
		DataBits: dataBits,
	}
	switch stopBits {
	case 1:
		mode.StopBits = serial.OneStopBit
	case 2:
		mode.StopBits = serial.TwoStopBits
	default:
		mode.StopBits = serial.OneStopBit
	}
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
		return nil, fmt.Errorf("century serial: unsupported parity %q", parity)
	}

	conn, err := serial.Open(port, mode)
	if err != nil {
		return nil, fmt.Errorf("century serial: open %s: %w", port, err)
	}
	if readTimeout <= 0 {
		readTimeout = 200 * time.Millisecond
	}
	if err := conn.SetReadTimeout(readTimeout); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("century serial: set read timeout %s: %w", port, err)
	}
	return conn, nil
}
