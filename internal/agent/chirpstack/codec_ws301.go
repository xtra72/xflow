package chirpstack

import "fmt"

// ws301ProfileName 은 WS301 코덱이 등록되는 ChirpStack deviceProfileName 이다.
// 업링크 deviceInfo.deviceProfileName 값과 일치해야 한다(실측 픽스처: "WS301").
const ws301ProfileName = "WS301"

// ---------------------------------------------------------------------------
// WS301 다운링크 바이트 테이블 (단일 수정 지점)
// ---------------------------------------------------------------------------
//
// 출처: Milesight WS301 User Guide V1.4 (2026-04-22) Ch.6 Downlink Commands p.31-32,
// milesight.com WS301 downlink 문서, 그리고 공식 Milesight-IoT/SensorDecoders
// ws301-encoder.js.
//
// 공통 규칙:
//   - TLV 형식: 0xFF <command-id> <value...>.
//   - 다운링크 fPort 기본값 85 (User Guide p.31: "The downlink port (application port)
//     is 85 by default", ToolBox 로 변경 가능).
//   - 멀티바이트 파라미터는 리틀엔디언이다 (User Guide p.29: Parameter/Data 필드는
//     little-endian 바이트 순서를 따른다).
//
// 향후 바이트 정정이 필요할 때 단일 편집 지점을 유지하기 위해 모든 바이트 값을 이
// var 블록 하나에 격리한다.
var ws301Downlink = struct {
	// FPort 는 다운링크 application port 이다 (User Guide V1.4 p.31 기본값 85).
	FPort uint8

	// Reboot 는 디바이스 재부팅 명령이다 (User Guide V1.4 p.31: ff 10 ff).
	// 후행 0xff 는 피연산자가 아니라 고정 페이로드 바이트이다.
	Reboot []byte

	// ReportIntervalPrefix 는 보고 주기 설정 TLV 프리픽스이다
	// (User Guide V1.4 p.31: ff 03 <lo> <hi>, UINT16 리틀엔디언, 단위 초).
	// 예: ff03b004 → 0x04b0 = 1200s = 20분.
	ReportIntervalPrefix []byte

	// QueryDeviceStatus 는 디바이스 상태 조회 명령이다.
	//
	// @MX:DEBT: ff 28 ff 는 공식 Milesight-IoT ws301-encoder.js 에만 존재한다.
	// @MX:CEILING: User Guide V1.4 및 milesight.com WS301 downlink 문서에는 미등재 —
	//   유저 가이드 대조로 확인된 reboot / set_report_interval 과 확신 수준이 다르다.
	// @MX:UPGRADE: 실기 확인 또는 Milesight 기술지원 확인이 완료되면 본 마커를 제거한다.
	QueryDeviceStatus []byte
}{
	FPort:                85,
	Reboot:               []byte{0xff, 0x10, 0xff},
	ReportIntervalPrefix: []byte{0xff, 0x03},
	QueryDeviceStatus:    []byte{0xff, 0x28, 0xff},
}

// WS301 보고 주기 유효 범위(초). 공식 encoder 와 동일하게 범위를 검증한다
// (User Guide V1.4 p.31: 60 ~ 64800 초).
const (
	ws301ReportIntervalMinSec = 60
	ws301ReportIntervalMaxSec = 64800
)

// WS301 이 해석하는 command 이름.
//
// WS301 은 자석식 도어 컨택 센서로 부저/알람 하드웨어가 없으므로 부저 계열 명령은
// 제공하지 않는다.
const (
	ws301CmdReboot              = "reboot"
	ws301CmdSetReportInterval   = "set_report_interval"
	ws301CmdQueryDeviceStatus   = "query_device_status"
	ws301ParamReportIntervalKey = "interval"
)

// ws301Codec 은 Milesight WS301 다운링크 코덱이다 (REQ-M2-02, v1 seed).
type ws301Codec struct{}

var _ DownlinkCodec = ws301Codec{}

func init() {
	RegisterDownlinkCodec(ws301ProfileName, ws301Codec{})
}

// Encode 는 typed command 를 WS301 TLV 다운링크 바이트로 인코딩한다.
//
// 미지 command 또는 범위 밖 파라미터는 에러로 거부한다 — 호출자는 발행하지 않는다
// (REQ-M2-04).
func (ws301Codec) Encode(cmd DownlinkCommand) (uint8, []byte, bool, error) {
	switch cmd.Name {
	case ws301CmdReboot:
		return ws301Downlink.FPort, cloneBytes(ws301Downlink.Reboot), cmd.Confirmed, nil

	case ws301CmdQueryDeviceStatus:
		return ws301Downlink.FPort, cloneBytes(ws301Downlink.QueryDeviceStatus), cmd.Confirmed, nil

	case ws301CmdSetReportInterval:
		sec, ok := intParam(cmd.Params, ws301ParamReportIntervalKey)
		if !ok {
			return 0, nil, false, fmt.Errorf(
				"chirpstack-codec(WS301): %s 명령에는 숫자 params.%s(초)가 필요합니다",
				ws301CmdSetReportInterval, ws301ParamReportIntervalKey)
		}
		if sec < ws301ReportIntervalMinSec || sec > ws301ReportIntervalMaxSec {
			return 0, nil, false, fmt.Errorf(
				"chirpstack-codec(WS301): 보고 주기 %d초는 유효 범위(%d~%d초)를 벗어납니다",
				sec, ws301ReportIntervalMinSec, ws301ReportIntervalMaxSec)
		}
		// UINT16 리틀엔디언 (User Guide V1.4 p.29 바이트 순서 규칙).
		data := append(cloneBytes(ws301Downlink.ReportIntervalPrefix),
			byte(sec&0xff), byte((sec>>8)&0xff))
		return ws301Downlink.FPort, data, cmd.Confirmed, nil

	default:
		return 0, nil, false, fmt.Errorf(
			"chirpstack-codec(WS301): 알 수 없는 command %q (지원: %s, %s, %s)",
			cmd.Name, ws301CmdReboot, ws301CmdSetReportInterval, ws301CmdQueryDeviceStatus)
	}
}

// cloneBytes 는 바이트 테이블의 상수 슬라이스가 호출자에 의해 변조되지 않도록 복사본을
// 반환한다.
func cloneBytes(b []byte) []byte {
	return append([]byte(nil), b...)
}

// intParam 은 params 에서 정수 파라미터를 꺼낸다. JSON 유래 숫자(float64)와 Go 정수
// 타입을 모두 허용하며, 키 부재/비숫자 타입/비정수 실수는 ok=false 이다.
func intParam(params map[string]any, key string) (int, bool) {
	v, ok := params[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		if n != float64(int(n)) {
			return 0, false
		}
		return int(n), true
	case float32:
		if n != float32(int(n)) {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}
