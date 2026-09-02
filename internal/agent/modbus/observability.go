package modbus

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// clientObs — 클라이언트 관측성(observability) 배선 묶음 (F4)
// ---------------------------------------------------------------------------
//
// 게이트웨이(modbus-gateway) observability.go 의 serverObs/logFrame 규약을 클라이언트로
// 포팅한 것이다. TCP·RTU 트랜스포트가 공유하는 프레임 로그 토글을 하나로 묶으며, atomic
// 미러로 Configure 경로에서 재시작 없이 갱신된다(게이트웨이 log_frames atomic.Bool 패턴).
// 클라이언트에는 서버-측 클라이언트 레지스트리 개념이 없으므로 registry 필드는 포팅하지
// 않는다(프레임 토글만 미러링). 모든 접근자는 obs 자체가 nil 이어도 안전하다
// (트랜스포트를 직접 생성하는 테스트가 obs 없이 동작하도록 — 기본 no-op).
type clientObs struct {
	logFrames    *atomic.Bool // log_frames 미러 (프레임 요약 로그 활성 여부)
	logRawFrames *atomic.Bool // log_raw_frames 미러 (전체 ADU hex 포함 여부)
}

// newClientObs 는 초기 토글값으로 clientObs 를 생성한다(기본값 false → 완전 no-op).
func newClientObs(logFrames, logRawFrames bool) *clientObs {
	o := &clientObs{
		logFrames:    &atomic.Bool{},
		logRawFrames: &atomic.Bool{},
	}
	o.logFrames.Store(logFrames)
	o.logRawFrames.Store(logRawFrames)
	return o
}

// framesOn 은 프레임 요약 로그(log_frames)가 켜져 있는지 반환한다.
func (o *clientObs) framesOn() bool {
	return o != nil && o.logFrames != nil && o.logFrames.Load()
}

// rawOn 은 전체 ADU hex 로그(log_raw_frames)가 켜져 있는지 반환한다.
// raw 는 frames 가 켜져 있을 때만 의미가 있다(logFrame 이 frames off 면 no-op).
func (o *clientObs) rawOn() bool {
	return o != nil && o.logRawFrames != nil && o.logRawFrames.Load()
}

// ---------------------------------------------------------------------------
// logFrame — TX/RX 프레임 로그 헬퍼 (게이트웨이 logFrame 준용, 명명만 클라이언트로 적응)
// ---------------------------------------------------------------------------

// logFrame 은 frames 가 켜져 있을 때 TX/RX 프레임 요약(방향/주소/unit_id/FC/바이트 길이)을
// INFO 로 로그한다. raw 가 true(log_raw_frames)면 전체 ADU hex 를 함께 남긴다.
// frames 가 false 이거나 logger 가 nil 이면 no-op 이다(opt-in 진단용). raw 는 frames 가
// 꺼져 있으면 아무 의미가 없다(그 경우 이 함수 자체가 no-op).
// dir 은 "RX"(수신) 또는 "TX"(송신), addr 은 대상 엔드포인트(빈 문자열이면 생략)이다.
// 이 함수는 읽기 전용 관측이며, 전달된 adu 바이트를 변형하지 않는다(A-9).
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
	logger.Info("modbus-client: frame", attrs...)
}
