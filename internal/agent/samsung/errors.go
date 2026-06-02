package samsung

import "errors"

var (
	// ErrInvalidTransportType 는 알 수 없는 transport_type 이 지정된 경우 반환된다.
	// 유효 값: "serial", "tcp-client", "tcp-server".
	ErrInvalidTransportType = errors.New("samsung_hvacr01: invalid transport_type (must be serial, tcp-client, or tcp-server)")
	// ErrDeprecatedTCPTransport 는 2026-05-29 breaking 으로 제거된 "tcp" 값이 지정된 경우 반환된다.
	// 사용자에게 "tcp-client" 또는 "tcp-server" 로의 명시적 마이그레이션을 안내한다.
	ErrDeprecatedTCPTransport  = errors.New("samsung_hvacr01: transport_type 'tcp' is removed; use 'tcp-client' (외부 서버 접속) or 'tcp-server' (수신 대기)")
	ErrSerialPortRequired      = errors.New("samsung_hvacr01: serial port path required")
	ErrTCPHostRequired         = errors.New("samsung_hvacr01: TCP host required")
	ErrTCPPortRequired         = errors.New("samsung_hvacr01: TCP port required")
	ErrTCPListenFailed         = errors.New("samsung_hvacr01: tcp-server listen failed")
	ErrDeviceNotFound          = errors.New("samsung_hvacr01: device not found")
	ErrInvalidMode             = errors.New("samsung_hvacr01: invalid operation mode")
	ErrInvalidFanSpeed         = errors.New("samsung_hvacr01: invalid fan speed")
	ErrTemperatureOutOfRange   = errors.New("samsung_hvacr01: temperature out of range (16.0-30.0)")
	ErrChecksumMismatch        = errors.New("samsung_hvacr01: CRC16-CCITT checksum mismatch")
	ErrProtocolParseFailed     = errors.New("samsung_hvacr01: protocol parse failed")
	ErrInvalidFrameSTX         = errors.New("samsung_hvacr01: invalid STX byte (expected 0x32)")
	ErrInvalidFrameETX         = errors.New("samsung_hvacr01: invalid ETX byte (expected 0x34)")
	ErrInvalidFrameLength      = errors.New("samsung_hvacr01: frame length mismatch")
	ErrInvalidAddress          = errors.New("samsung_hvacr01: invalid NASA address format")
	ErrInvalidMessageSetIndex  = errors.New("samsung_hvacr01: invalid message set index nibble")
	ErrDeviceOffline           = errors.New("samsung_hvacr01: device is offline")
	ErrDeviceNotReady          = errors.New("samsung_hvacr01: device not ready")
	ErrTransportNotConnected   = errors.New("samsung_hvacr01: transport not connected")
	ErrInvalidCommand          = errors.New("samsung_hvacr01: invalid command")
	ErrDeviceAlreadyRegistered = errors.New("samsung_hvacr01: device already registered")
	ErrConfigDeviceProtected   = errors.New("samsung_hvacr01: config-based device cannot be removed")
	ErrSequenceNumOverflow     = errors.New("samsung_hvacr01: sequence number overflow")
	ErrDeviceIDNotFound        = errors.New("samsung_hvacr01: device ID not found")
	ErrDuplicateDeviceID       = errors.New("samsung_hvacr01: duplicate device ID")
	ErrDevicePoweredOff        = errors.New("samsung_hvacr01: device is powered off, only power control is allowed")
)
