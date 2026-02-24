package samsung

import "errors"

var (
	ErrInvalidTransportType    = errors.New("samsung-nasa: invalid transport type")
	ErrSerialPortRequired      = errors.New("samsung-nasa: serial port path required")
	ErrTCPAddressRequired      = errors.New("samsung-nasa: TCP address required")
	ErrDeviceNotFound          = errors.New("samsung-nasa: device not found")
	ErrInvalidMode             = errors.New("samsung-nasa: invalid operation mode")
	ErrInvalidFanSpeed         = errors.New("samsung-nasa: invalid fan speed")
	ErrTemperatureOutOfRange   = errors.New("samsung-nasa: temperature out of range (16.0-30.0)")
	ErrChecksumMismatch        = errors.New("samsung-nasa: CRC16-CCITT checksum mismatch")
	ErrProtocolParseFailed     = errors.New("samsung-nasa: protocol parse failed")
	ErrInvalidFrameSTX         = errors.New("samsung-nasa: invalid STX byte (expected 0x32)")
	ErrInvalidFrameETX         = errors.New("samsung-nasa: invalid ETX byte (expected 0x34)")
	ErrInvalidFrameLength      = errors.New("samsung-nasa: frame length mismatch")
	ErrInvalidAddress          = errors.New("samsung-nasa: invalid NASA address format")
	ErrInvalidMessageSetIndex  = errors.New("samsung-nasa: invalid message set index nibble")
	ErrDeviceOffline           = errors.New("samsung-nasa: device is offline")
	ErrDeviceNotReady          = errors.New("samsung-nasa: device not ready")
	ErrTransportNotConnected   = errors.New("samsung-nasa: transport not connected")
	ErrInvalidCommand          = errors.New("samsung-nasa: invalid command")
	ErrDeviceAlreadyRegistered = errors.New("samsung-nasa: device already registered")
	ErrConfigDeviceProtected   = errors.New("samsung-nasa: config-based device cannot be removed")
	ErrSequenceNumOverflow     = errors.New("samsung-nasa: sequence number overflow")
	ErrDeviceIDNotFound        = errors.New("samsung-nasa: device ID not found")
	ErrDuplicateDeviceID       = errors.New("samsung-nasa: duplicate device ID")
)
