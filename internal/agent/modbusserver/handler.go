package modbusserver

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"

	modbus "github.com/xtra/xflow/internal/agent/modbus"
)

// ---------------------------------------------------------------------------
// ConnectionHandler interface
// ---------------------------------------------------------------------------

// ConnectionHandler is the interface for handling individual MODBUS/TCP connections.
type ConnectionHandler interface {
	HandleConnection(ctx context.Context, conn net.Conn)
}

// ---------------------------------------------------------------------------
// ModbusHandler
// ---------------------------------------------------------------------------

// ModbusHandler handles a single MODBUS/TCP connection. It reads MBAP frames,
// dispatches PDUs to the appropriate device's RequestHandler via DeviceManager,
// and sends responses back.
type ModbusHandler struct {
	deviceManager *DeviceManager
	msgCh         chan<- map[string]any
	logger        *slog.Logger
}

// NewModbusHandler creates a new ModbusHandler with a DeviceManager for multi-device routing.
func NewModbusHandler(deviceManager *DeviceManager, msgCh chan<- map[string]any, logger *slog.Logger) *ModbusHandler {
	return &ModbusHandler{
		deviceManager: deviceManager,
		msgCh:         msgCh,
		logger:        logger,
	}
}

// HandleConnection processes MODBUS/TCP frames on a single connection.
// It runs a loop reading MBAP frames, routing to the appropriate device's
// RequestHandler based on UnitID, and writing responses until the context
// is cancelled or the connection errors.
func (mh *ModbusHandler) HandleConnection(ctx context.Context, conn net.Conn) {
	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Step 1: Read MBAP header (7 bytes)
		header := make([]byte, modbus.MBAPHeaderSize)
		_, err := io.ReadFull(conn, header)
		if err != nil {
			if err != io.EOF && ctx.Err() == nil {
				mh.logWarn("MBAP header read failed", "error", err)
			}
			return
		}

		// Step 2: Parse MBAP header
		txID := binary.BigEndian.Uint16(header[0:2])
		protocolID := binary.BigEndian.Uint16(header[2:4])
		length := binary.BigEndian.Uint16(header[4:6])
		unitID := header[6]

		// Step 3: Read PDU (Length - 1 bytes, since UnitID is included in Length)
		if length < 2 {
			mh.logWarn("MBAP length too short", "length", length)
			continue
		}
		pduLen := int(length) - 1
		pdu := make([]byte, pduLen)
		_, err = io.ReadFull(conn, pdu)
		if err != nil {
			if err != io.EOF && ctx.Err() == nil {
				mh.logWarn("PDU read failed", "error", err)
			}
			return
		}

		// Step 4: Validate ProtocolID
		if protocolID != modbus.MBAPProtocolID {
			mh.logWarn("invalid protocol ID", "protocolID", protocolID)
			continue
		}

		// Step 5: Route by UnitID using DeviceManager
		var respPDU []byte
		if len(pdu) > 0 {
			fc := pdu[0]

			if unitID == 0 {
				// Broadcast: UnitID=0
				respPDU = mh.handleBroadcast(pdu, fc, conn.RemoteAddr().String())
			} else {
				// Normal: lookup device by UnitID
				dev := mh.deviceManager.GetDevice(unitID)
				if dev == nil {
					mh.logWarn("unit ID not found",
						"unitID", unitID)
					continue
				}
				respPDU = mh.handleDeviceRequest(dev, pdu, fc, conn.RemoteAddr().String())
			}
		}

		if respPDU == nil {
			continue
		}

		// Step 6: Build MBAP response frame
		respLen := uint16(1 + len(respPDU)) // UnitID(1) + PDU
		respFrame := make([]byte, modbus.MBAPHeaderSize+len(respPDU))
		binary.BigEndian.PutUint16(respFrame[0:2], txID)
		binary.BigEndian.PutUint16(respFrame[2:4], modbus.MBAPProtocolID)
		binary.BigEndian.PutUint16(respFrame[4:6], respLen)
		respFrame[6] = unitID
		copy(respFrame[7:], respPDU)

		// Step 7: Write response
		_, err = conn.Write(respFrame)
		if err != nil {
			if ctx.Err() == nil {
				mh.logWarn("response write failed", "error", err)
			}
			return
		}
	}
}

// handleDeviceRequest dispatches a request to a specific device.
func (mh *ModbusHandler) handleDeviceRequest(dev *Device, pdu []byte, fc byte, remoteAddr string) []byte {
	if isWriteFC(fc) {
		dev.Stats.RecordWrite()
		respPDU, cs := dev.ReqHandler.HandleWriteRequest(pdu, remoteAddr)
		if cs != nil {
			mh.sendChangeNotification(cs, remoteAddr, dev.UnitID)
		}
		return respPDU
	}
	dev.Stats.RecordRead()
	return dev.ReqHandler.HandleRequest(pdu)
}

// handleBroadcast handles broadcast requests (UnitID=0).
// For write operations: fan-out to ALL devices.
// For read operations: use the first device.
func (mh *ModbusHandler) handleBroadcast(pdu []byte, fc byte, remoteAddr string) []byte {
	if isWriteFC(fc) {
		// Fan-out write to all devices
		var lastResp []byte
		for _, dev := range mh.deviceManager.GetAllDevices() {
			dev.Stats.RecordWrite()
			respPDU, cs := dev.ReqHandler.HandleWriteRequest(pdu, remoteAddr)
			if cs != nil {
				mh.sendChangeNotification(cs, remoteAddr, dev.UnitID)
			}
			lastResp = respPDU
		}
		return lastResp
	}
	// Read from first device
	first := mh.deviceManager.FirstDevice()
	if first == nil {
		return nil
	}
	first.Stats.RecordRead()
	return first.ReqHandler.HandleRequest(pdu)
}

// isWriteFC returns true if the function code is a write operation.
func isWriteFC(fc byte) bool {
	switch fc {
	case modbus.FC05WriteSingleCoil,
		modbus.FC06WriteSingleRegister,
		modbus.FC15WriteMultipleCoils,
		modbus.FC16WriteMultipleRegisters:
		return true
	default:
		return false
	}
}

// sendChangeNotification sends a register change notification to msgCh (non-blocking).
func (mh *ModbusHandler) sendChangeNotification(cs *ChangeSet, remoteAddr string, unitID byte) {
	notification := map[string]any{
		"type":       "register_change",
		"area":       cs.Area,
		"address":    cs.Address,
		"quantity":   cs.Quantity,
		"old_values": cs.OldValues,
		"new_values": cs.NewValues,
		"source":     remoteAddr,
		"unit_id":    unitID,
	}

	select {
	case mh.msgCh <- notification:
	default:
		mh.logWarn("change notification channel full, dropping notification")
	}
}

// logWarn logs a warning message if a logger is available.
func (mh *ModbusHandler) logWarn(msg string, args ...any) {
	if mh.logger != nil {
		mh.logger.Warn(fmt.Sprintf("modbus-handler: %s", msg), args...)
	}
}
