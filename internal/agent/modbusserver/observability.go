package modbusserver

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// serverObs — 서버 관측성(observability) 배선 묶음
// ---------------------------------------------------------------------------
//
// TCP 핸들러(ModbusHandler)와 RTU 리스너(RTUListener)가 공유하는 관측성 상태를
// 하나로 묶는다. 프레임 로그 토글은 atomic 미러로 Configure 에서 재시작 없이 갱신되며
// (socket tcp_server.go 의 log_messages atomic.Bool 패턴), registry 는 TCP 전용
// 클라이언트 목록이다(RTU 는 nil). 모든 접근자는 obs 자체가 nil 이어도 안전하다
// (핸들러/리스너를 직접 생성하는 테스트가 obs 없이 동작하도록).
type serverObs struct {
	registry     *ClientRegistry // TCP 클라이언트 레지스트리 (RTU 는 nil)
	logFrames    *atomic.Bool    // log_frames 미러 (프레임 요약 로그 활성 여부)
	logRawFrames *atomic.Bool    // log_raw_frames 미러 (전체 ADU hex 포함 여부)
}

// framesOn 은 프레임 요약 로그(log_frames)가 켜져 있는지 반환한다.
func (o *serverObs) framesOn() bool {
	return o != nil && o.logFrames != nil && o.logFrames.Load()
}

// rawOn 은 전체 ADU hex 로그(log_raw_frames)가 켜져 있는지 반환한다.
// raw 는 frames 가 켜져 있을 때만 의미가 있다(logFrame 이 frames off 면 no-op).
func (o *serverObs) rawOn() bool {
	return o != nil && o.logRawFrames != nil && o.logRawFrames.Load()
}

// record 는 registry 가 있으면 클라이언트의 접근 unit_id 를 기록한다(TCP 전용).
func (o *serverObs) record(remoteAddr string, unitID byte) {
	if o != nil && o.registry != nil {
		o.registry.RecordAccess(remoteAddr, unitID)
	}
}

// add 는 registry 가 있으면 새 클라이언트 연결을 등록한다.
func (o *serverObs) add(remoteAddr string) {
	if o != nil && o.registry != nil {
		o.registry.Add(remoteAddr)
	}
}

// remove 는 registry 가 있으면 클라이언트 연결을 제거한다.
func (o *serverObs) remove(remoteAddr string) {
	if o != nil && o.registry != nil {
		o.registry.Remove(remoteAddr)
	}
}

// clients 는 registry 스냅샷을 반환한다(registry 없으면 nil — RTU).
func (o *serverObs) clients() []ClientInfo {
	if o != nil && o.registry != nil {
		return o.registry.List()
	}
	return nil
}

// ---------------------------------------------------------------------------
// logFrame — TX/RX 프레임 로그 헬퍼 (socket common.go logPacket 준용)
// ---------------------------------------------------------------------------

// logFrame 은 frames 가 켜져 있을 때 TX/RX 프레임 요약(방향/주소/unit_id/FC/바이트 길이)을
// INFO 로 로그한다. raw 가 true(log_raw_frames)면 전체 ADU hex 를 함께 남긴다.
// frames 가 false 이거나 logger 가 nil 이면 no-op 이다(opt-in 진단용). raw 는 frames 가
// 꺼져 있으면 아무 의미가 없다(그 경우 이 함수 자체가 no-op).
// dir 은 "RX"(수신) 또는 "TX"(송신), addr 은 상대 주소(빈 문자열이면 생략)이다.
func logFrame(logger *slog.Logger, frames, raw bool, dir, addr string, unitID, fc byte, adu []byte) {
	if !frames || logger == nil {
		return
	}
	attrs := make([]any, 0, 12)
	attrs = append(attrs, "dir", dir)
	if addr != "" {
		attrs = append(attrs, "addr", addr)
	}
	attrs = append(attrs, "unit_id", unitID, "fc", fmt.Sprintf("0x%02X", fc), "len", len(adu))
	if raw {
		attrs = append(attrs, "hex", hex.EncodeToString(adu))
	}
	logger.Info("modbus-gateway: frame", attrs...)
}
