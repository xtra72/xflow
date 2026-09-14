package lg

// ---------------------------------------------------------------------------
// PMBUSB00A 레지스터 맵 — 주소 계산 및 항목 상수
//
// SPEC-LG-HVACR-003 § M1. 기준 문서: references/protocols/PMBUSB00A_Modbus_Protocol_Analysis.md
//
// 주소 체계 주의: 매뉴얼의 00001 / 10001 / 40001 / 30001 표기는 Modbus 레지스터
// "번호"(1-base)이고, 실제 전송 프레임에 들어가는 프로토콜 주소는 0-base 이다.
// 본 패키지는 0-base 주소만 다루며, 1-base 번호 표기를 상수로 정의하지 않는다
// (두 체계가 코드에 공존하면 1 차이 버그의 온상이 된다).
// ---------------------------------------------------------------------------

// 블록 크기. Coil/Discrete 는 실내기당 16, Holding/Input 은 실내기당 20 을 차지한다.
const (
	pmbusCoilBlockSize    = 16
	pmbusHoldingBlockSize = 20
)

// 실내기 주소 N 의 유효 범위. 모듈 1개가 담당하는 범위가 16대이다.
const (
	pmbusMinUnitAddr = 0
	pmbusMaxUnitAddr = 15
	pmbusMaxUnits    = pmbusMaxUnitAddr + 1
)

// Coil 항목 번호 (0-base). 문서 §4.2 의 원문자 ①~⑩ 에 대응한다.
// FC 01(읽기) / 05, 15(쓰기).
const (
	pmbusCoilPower       = 0 // ① 운전 On/Off — 0=정지, 1=운전
	pmbusCoilSwing       = 1 // ② 자동 풍향 스윙 (에어컨)
	pmbusCoilFilterReset = 2 // ③ 필터 알람 해제
	pmbusCoilLockRemote  = 3 // ④ 리모컨 전체 잠금
	pmbusCoilLockMode    = 4 // ⑤ 운전 모드 잠금
	pmbusCoilLockFan     = 5 // ⑥ 풍량 잠금
	pmbusCoilLockTemp    = 6 // ⑦ 설정 온도 잠금
	pmbusCoilLockAddress = 7 // ⑧ 실내기 주소 잠금
	pmbusCoilERVRapid    = 8 // ⑨ 급속 환기 (ERV)
	pmbusCoilERVEco      = 9 // ⑩ 에너지 절약 (ERV)

	pmbusCoilCount = 10 // 정의된 Coil 항목 수 (FC01 읽기 개수)
)

// Discrete Input 항목 번호 (0-base). 문서 §4.3 의 ①~⑤ 에 대응한다. FC 02(읽기 전용).
const (
	pmbusDiscreteConnected   = 0 // ① 실내기 연결 상태 — 0=미연결, 1=연결
	pmbusDiscreteAlarm       = 1 // ② 알람(에러) 발생
	pmbusDiscreteFilterAlarm = 2 // ③ 필터 알람
	pmbusDiscreteTempBasis   = 3 // ④ 목표 온도 기준 — 0=공기, 1=물 (하이드로킷)
	pmbusDiscreteErrorKind   = 4 // ⑤ 에러 구분 — 0=CH 타입, 1=BC 타입 (하이드로킷)

	pmbusDiscreteCount = 5 // 정의된 Discrete 항목 수 (단일 실내기 FC02 읽기 개수)
)

// Holding Register 항목 번호 (0-base). 문서 §4.4 의 ①~⑥ 에 대응한다.
// FC 03(읽기) / 06, 16(쓰기).
const (
	pmbusHoldingMode          = 0 // ① 운전 모드 — 0=냉방, 1=제습, 2=송풍, 3=자동, 4=난방
	pmbusHoldingFanSpeed      = 1 // ② 풍량 (또는 급탕 온도)
	pmbusHoldingSetTemp       = 2 // ③ 설정 온도 (×temp_scale)
	pmbusHoldingTempLimitHigh = 3 // ④ 설정 온도 상한 제한
	pmbusHoldingTempLimitLow  = 4 // ⑤ 설정 온도 하한 제한
	pmbusHoldingERVMode       = 5 // ⑥ 환기 운전 모드 — 0=열교환, 1=자동, 2=보통 (ERV)

	pmbusHoldingCount = 6 // 정의된 Holding 항목 수 (FC03 읽기 개수)
)

// Input Register 항목 번호 (0-base). 문서 §4.5 의 ①~⑥ 에 대응한다. FC 04(읽기 전용).
const (
	pmbusInputErrorCode = 0 // ① 에러 코드 — 0=정상
	pmbusInputRoomTemp  = 1 // ② 실내 온도 (signed, ×temp_scale)
	pmbusInputPipeIn    = 2 // ③ 배관 입구 / 입수 온도
	pmbusInputPipeOut   = 3 // ④ 배관 출구 / 출수 온도
	pmbusInputWaterTank = 4 // ⑤ 급탕 탱크 온도 (하이드로킷)
	pmbusInputSolar     = 5 // ⑥ 태양열 온도 (AWHP)

	pmbusInputCount = 6 // 정의된 Input 항목 수 (FC04 읽기 개수)
)

// 전체 스캔 읽기 개수. Coil/Discrete 블록 할당 범위는 0~255 이므로, 한 트랜잭션으로
// 16대 × 16bit 를 모두 읽는다.
const pmbusScanBitCount = pmbusMaxUnits * pmbusCoilBlockSize // 256

// Modbus function code.
const (
	pmbusFCReadCoils          byte = 0x01
	pmbusFCReadDiscreteInputs byte = 0x02
	pmbusFCReadHolding        byte = 0x03
	pmbusFCReadInput          byte = 0x04
	pmbusFCWriteSingleCoil    byte = 0x05
	pmbusFCWriteSingleReg     byte = 0x06
	pmbusFCWriteMultipleRegs  byte = 0x10
)

// 운전 모드 프로토콜 코드 (Holding ①). 문서 §4.4.
// LGCP 의 [64 50 XY] 하위 니블과 동일한 코드 체계이다 — 역공학 결과의 교차 검증점.
const (
	pmbusModeCool = 0
	pmbusModeDry  = 1
	pmbusModeFan  = 2
	pmbusModeAuto = 3
	pmbusModeHeat = 4
)

// 풍량 프로토콜 코드 (Holding ②). 문서 §4.4.
// 자동에 해당하는 코드는 설정(fan_auto_code)으로 결정되므로 여기에 상수를 두지 않는다.
const (
	pmbusFanLow    = 1
	pmbusFanMedium = 2
	pmbusFanHigh   = 3
)

// 환기 운전 모드 코드 (Holding ⑥, ERV).
const (
	pmbusERVModeHeatExchange = 0
	pmbusERVModeAuto         = 1
	pmbusERVModeNormal       = 2
)

// 설정 온도의 절대 허용 범위 (°C). 문서 §4.4 ③.
const (
	pmbusMinSetTempC = 16.0
	pmbusMaxSetTempC = 30.0
)

// ---------------------------------------------------------------------------
// 주소 계산
//
// 전부 0-base 프로토콜 주소를 반환한다. item 인자도 0-base 항목 번호이다.
// ---------------------------------------------------------------------------

// pmbusCoilAddr 은 실내기 N 의 Coil 항목 주소를 반환한다 (N×16 + item).
func pmbusCoilAddr(n, item uint16) uint16 {
	return n*pmbusCoilBlockSize + item
}

// pmbusDiscreteAddr 은 실내기 N 의 Discrete Input 항목 주소를 반환한다 (N×16 + item).
// Coil 과 동일한 블록 크기를 쓰지만 별개의 주소 공간이다.
func pmbusDiscreteAddr(n, item uint16) uint16 {
	return n*pmbusCoilBlockSize + item
}

// pmbusHoldingAddr 은 실내기 N 의 Holding Register 항목 주소를 반환한다 (N×20 + item).
func pmbusHoldingAddr(n, item uint16) uint16 {
	return n*pmbusHoldingBlockSize + item
}

// pmbusInputAddr 은 실내기 N 의 Input Register 항목 주소를 반환한다 (N×20 + item).
// Holding 과 동일한 블록 크기를 쓰지만 별개의 주소 공간이다.
func pmbusInputAddr(n, item uint16) uint16 {
	return n*pmbusHoldingBlockSize + item
}

// pmbusScanBitIndex 는 전체 스캔(FC02 0~255) 결과에서 실내기 N 의 item 비트가
// 위치한 인덱스를 반환한다.
func pmbusScanBitIndex(n, item uint16) int {
	return int(n*pmbusCoilBlockSize + item)
}
