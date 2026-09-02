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
	notifyOnWrite bool       // 외부 통신(와이어 쓰기) register_change 알림 발행 여부 (opt-in, 기본 false)
	obs           *serverObs // 관측성 배선(클라이언트 레지스트리 + 프레임 로그 토글); nil 가능
	logger        *slog.Logger
}

// NewModbusHandler creates a new ModbusHandler with a DeviceManager for multi-device routing.
// notifyOnWrite 가 true 일 때만 원격 마스터의 와이어 쓰기에 대해 register_change 알림을 발행한다.
// obs 는 클라이언트 레지스트리 기록과 프레임 로그를 담당하며 nil 이어도 안전하다.
func NewModbusHandler(deviceManager *DeviceManager, msgCh chan<- map[string]any, notifyOnWrite bool, obs *serverObs, logger *slog.Logger) *ModbusHandler {
	return &ModbusHandler{
		deviceManager: deviceManager,
		msgCh:         msgCh,
		notifyOnWrite: notifyOnWrite,
		obs:           obs,
		logger:        logger,
	}
}

// HandleConnection processes MODBUS/TCP frames on a single connection.
// It runs a loop reading MBAP frames, routing to the appropriate device's
// RequestHandler based on UnitID, and writing responses until the context
// is cancelled or the connection errors.
func (mh *ModbusHandler) HandleConnection(ctx context.Context, conn net.Conn) {
	// 연결당 상수인 원격 주소를 한 번만 구한다(레지스트리 기록/프레임 로그에 재사용).
	remoteAddr := conn.RemoteAddr().String()

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

		// RX 프레임 로그 (log_frames): 전체 인바운드 ADU = MBAP 헤더(7) + PDU.
		if mh.obs.framesOn() {
			inADU := make([]byte, 0, len(header)+len(pdu))
			inADU = append(inADU, header...)
			inADU = append(inADU, pdu...)
			logFrame(mh.logger, true, mh.obs.rawOn(), "RX", remoteAddr, unitID, pdu[0], inADU)
		}

		// 클라이언트 레지스트리에 접근 unit_id 기록 (TCP 전용; obs/registry nil 이면 no-op).
		mh.obs.record(remoteAddr, unitID)

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
				respPDU = mh.handleBroadcast(pdu, fc, remoteAddr)
			} else {
				// Normal: lookup device by UnitID
				dev := mh.deviceManager.GetDevice(unitID)
				if dev == nil {
					mh.logWarn("unit ID not found",
						"unitID", unitID)
					continue
				}
				respPDU = mh.handleDeviceRequest(dev, pdu, fc, remoteAddr)
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

		// TX 프레임 로그 (log_frames): 전체 아웃바운드 ADU = MBAP 응답 프레임.
		if mh.obs.framesOn() {
			logFrame(mh.logger, true, mh.obs.rawOn(), "TX", remoteAddr, unitID, respPDU[0], respFrame)
		}

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
	var respPDU []byte
	if isWriteFC(fc) {
		dev.Stats.RecordWrite()
		var cs *ChangeSet
		respPDU, cs = dev.ReqHandler.HandleWriteRequest(pdu, remoteAddr)
		if cs != nil {
			mh.sendChangeNotification(cs, remoteAddr, dev.UnitID)
		}
	} else {
		dev.Stats.RecordRead()
		respPDU = dev.ReqHandler.HandleRequest(pdu)
	}

	// 디바이스 상태 로그 (에이전트 로그 레벨이 유일한 제어): 요청별 활동은 DEBUG,
	// 예외 응답(디바이스 에러)은 WARN 으로 남긴다. 별도 config 토글은 없다.
	mh.logDeviceActivity(dev, fc, pdu, respPDU)
	return respPDU
}

// logDeviceActivity 는 디바이스 요청 결과를 graduated slog 레벨로 로그한다.
//   - WARN: 응답이 MODBUS 예외 PDU 이면 디바이스 에러로 간주하고 ErrorCount 를 올린다.
//   - DEBUG: 정상 요청별 활동(unit_id + fc + area/address). 포매팅 비용을 피하려고
//     로거 레벨이 DEBUG 일 때만 계산한다.
func (mh *ModbusHandler) logDeviceActivity(dev *Device, fc byte, pdu, respPDU []byte) {
	if mh.logger == nil {
		return
	}

	// 예외 응답 감지: 예외 PDU 는 [fc|0x80, exCode] 형식(최상위 비트 set).
	if len(respPDU) >= 2 && respPDU[0]&0x80 != 0 {
		dev.Stats.RecordError()
		mh.logger.Warn("modbus-gateway: device error",
			"unit_id", dev.UnitID, "fc", fmt.Sprintf("0x%02X", fc), "exception", respPDU[1])
		return
	}

	if mh.logger.Enabled(context.Background(), slog.LevelDebug) {
		var addr uint16
		if len(pdu) >= 3 {
			addr = binary.BigEndian.Uint16(pdu[1:3])
		}
		mh.logger.Debug("modbus-gateway: device request",
			"unit_id", dev.UnitID, "fc", fmt.Sprintf("0x%02X", fc),
			"area", areaForFC(fc), "address", addr)
	}
}

// areaForFC 는 function code 로부터 레지스터 영역 이름을 반환한다(로그용).
func areaForFC(fc byte) string {
	switch fc {
	case modbus.FC01ReadCoils, modbus.FC05WriteSingleCoil, modbus.FC15WriteMultipleCoils:
		return "coils"
	case modbus.FC02ReadDiscreteInputs:
		return "discrete_inputs"
	case modbus.FC03ReadHoldingRegisters, modbus.FC06WriteSingleRegister, modbus.FC16WriteMultipleRegisters:
		return "holding_registers"
	case modbus.FC04ReadInputRegisters:
		return "input_registers"
	default:
		return "unknown"
	}
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
// notify_on_write 가 false(기본)이면 와이어 쓰기 알림을 발행하지 않는다(opt-in).
// 이 게이트는 와이어 경로(외부 마스터 쓰기)에만 적용되며, 플로우 입력 경로의
// sendChangeEvent(set_*/bulk_write)에는 영향을 주지 않는다.
func (mh *ModbusHandler) sendChangeNotification(cs *ChangeSet, remoteAddr string, unitID byte) {
	if !mh.notifyOnWrite {
		return
	}

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
