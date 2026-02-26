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
// dispatches PDUs to a RequestHandler, and sends responses back.
type ModbusHandler struct {
	unitID     byte
	reqHandler *RequestHandler
	msgCh      chan<- map[string]any
	logger     *slog.Logger
}

// NewModbusHandler creates a new ModbusHandler.
func NewModbusHandler(unitID byte, reqHandler *RequestHandler, msgCh chan<- map[string]any, logger *slog.Logger) *ModbusHandler {
	return &ModbusHandler{
		unitID:     unitID,
		reqHandler: reqHandler,
		msgCh:      msgCh,
		logger:     logger,
	}
}

// HandleConnection processes MODBUS/TCP frames on a single connection.
// It runs a loop reading MBAP frames, dispatching to the RequestHandler,
// and writing responses until the context is cancelled or the connection errors.
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

		// Step 5: Check UnitID (0 is broadcast, accept all)
		if unitID != 0 && unitID != mh.unitID {
			mh.logWarn("unit ID mismatch",
				"expected", mh.unitID,
				"received", unitID)
			continue
		}

		// Step 6: Dispatch based on function code
		var respPDU []byte
		if len(pdu) > 0 {
			fc := pdu[0]
			if isWriteFC(fc) {
				remoteAddr := conn.RemoteAddr().String()
				var cs *ChangeSet
				respPDU, cs = mh.reqHandler.HandleWriteRequest(pdu, remoteAddr)

				// Send change notification if applicable
				if cs != nil {
					mh.sendChangeNotification(cs, remoteAddr)
				}
			} else {
				respPDU = mh.reqHandler.HandleRequest(pdu)
			}
		}

		if respPDU == nil {
			continue
		}

		// Step 7: Build MBAP response frame
		respLen := uint16(1 + len(respPDU)) // UnitID(1) + PDU
		respFrame := make([]byte, modbus.MBAPHeaderSize+len(respPDU))
		binary.BigEndian.PutUint16(respFrame[0:2], txID)
		binary.BigEndian.PutUint16(respFrame[2:4], modbus.MBAPProtocolID)
		binary.BigEndian.PutUint16(respFrame[4:6], respLen)
		respFrame[6] = unitID
		copy(respFrame[7:], respPDU)

		// Step 8: Write response
		_, err = conn.Write(respFrame)
		if err != nil {
			if ctx.Err() == nil {
				mh.logWarn("response write failed", "error", err)
			}
			return
		}
	}
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
func (mh *ModbusHandler) sendChangeNotification(cs *ChangeSet, remoteAddr string) {
	notification := map[string]any{
		"type":       "register_change",
		"area":       cs.Area,
		"address":    cs.Address,
		"quantity":   cs.Quantity,
		"old_values": cs.OldValues,
		"new_values": cs.NewValues,
		"source":     remoteAddr,
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
