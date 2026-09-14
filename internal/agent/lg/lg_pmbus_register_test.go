package lg

import (
	"errors"
	"testing"

	"github.com/xtra/xflow/internal/agent/hvac"
)

// ---------------------------------------------------------------------------
// 주소 계산 (AC-001 ~ AC-003)
// ---------------------------------------------------------------------------

// TestPmbusAddressLookupTable 은 문서 §4.6 실내기별 주소 조견표의 전 16행을 검증한다.
// 기대값은 문서 표에서 그대로 옮긴 것이다.
func TestPmbusAddressLookupTable(t *testing.T) {
	type row struct {
		n                                uint16
		coilStart, discreteStart         uint16
		holdingStart, mode, fan, setTemp uint16
		inputStart, errCode, roomTemp    uint16
	}
	table := []row{
		{0, 0, 0, 0, 0, 1, 2, 0, 0, 1},
		{1, 16, 16, 20, 20, 21, 22, 20, 20, 21},
		{2, 32, 32, 40, 40, 41, 42, 40, 40, 41},
		{3, 48, 48, 60, 60, 61, 62, 60, 60, 61},
		{4, 64, 64, 80, 80, 81, 82, 80, 80, 81},
		{5, 80, 80, 100, 100, 101, 102, 100, 100, 101},
		{6, 96, 96, 120, 120, 121, 122, 120, 120, 121},
		{7, 112, 112, 140, 140, 141, 142, 140, 140, 141},
		{8, 128, 128, 160, 160, 161, 162, 160, 160, 161},
		{9, 144, 144, 180, 180, 181, 182, 180, 180, 181},
		{10, 160, 160, 200, 200, 201, 202, 200, 200, 201},
		{11, 176, 176, 220, 220, 221, 222, 220, 220, 221},
		{12, 192, 192, 240, 240, 241, 242, 240, 240, 241},
		{13, 208, 208, 260, 260, 261, 262, 260, 260, 261},
		{14, 224, 224, 280, 280, 281, 282, 280, 280, 281},
		{15, 240, 240, 300, 300, 301, 302, 300, 300, 301},
	}

	for _, r := range table {
		checks := []struct {
			label string
			got   uint16
			want  uint16
		}{
			{"coil start", pmbusCoilAddr(r.n, 0), r.coilStart},
			{"power coil", pmbusCoilAddr(r.n, pmbusCoilPower), r.coilStart},
			{"discrete start", pmbusDiscreteAddr(r.n, 0), r.discreteStart},
			{"holding start", pmbusHoldingAddr(r.n, 0), r.holdingStart},
			{"mode", pmbusHoldingAddr(r.n, pmbusHoldingMode), r.mode},
			{"fan", pmbusHoldingAddr(r.n, pmbusHoldingFanSpeed), r.fan},
			{"set temp", pmbusHoldingAddr(r.n, pmbusHoldingSetTemp), r.setTemp},
			{"input start", pmbusInputAddr(r.n, 0), r.inputStart},
			{"error code", pmbusInputAddr(r.n, pmbusInputErrorCode), r.errCode},
			{"room temp", pmbusInputAddr(r.n, pmbusInputRoomTemp), r.roomTemp},
		}
		for _, c := range checks {
			if c.got != c.want {
				t.Errorf("N=%d %s = %d, want %d", r.n, c.label, c.got, c.want)
			}
		}
	}
}

// TestPmbusMaxDefinedAddresses 는 문서 §4.6 각주가 명시한 최대 정의 주소를 확인한다.
func TestPmbusMaxDefinedAddresses(t *testing.T) {
	if got := pmbusCoilAddr(15, pmbusCoilERVEco); got != 249 {
		t.Errorf("max coil = %d, want 249", got)
	}
	if got := pmbusDiscreteAddr(15, pmbusDiscreteErrorKind); got != 244 {
		t.Errorf("max discrete = %d, want 244", got)
	}
	if got := pmbusHoldingAddr(15, pmbusHoldingERVMode); got != 305 {
		t.Errorf("max holding = %d, want 305", got)
	}
	if got := pmbusInputAddr(15, pmbusInputSolar); got != 305 {
		t.Errorf("max input = %d, want 305", got)
	}
}

func TestPmbusScanBitIndex(t *testing.T) {
	cases := []struct {
		n, item uint16
		want    int
	}{
		{0, pmbusDiscreteConnected, 0},
		{0, pmbusDiscreteAlarm, 1},
		{1, pmbusDiscreteConnected, 16},
		{3, pmbusDiscreteFilterAlarm, 50},
		{15, pmbusDiscreteErrorKind, 244},
	}
	for _, c := range cases {
		if got := pmbusScanBitIndex(c.n, c.item); got != c.want {
			t.Errorf("scanBitIndex(%d, %d) = %d, want %d", c.n, c.item, got, c.want)
		}
	}
	// 스캔 범위를 넘지 않아야 한다.
	if pmbusScanBitIndex(15, pmbusCoilBlockSize-1) >= pmbusScanBitCount {
		t.Error("scan bit index overflows scan range")
	}
}

// ---------------------------------------------------------------------------
// 온도 인코딩 / 디코딩 (AC-005 ~ AC-008)
// ---------------------------------------------------------------------------

func TestPmbusDecodeTemp(t *testing.T) {
	cases := []struct {
		name  string
		raw   uint16
		scale int
		want  float64
	}{
		{"positive 24.5", 0x00F5, 10, 24.5},
		{"positive 21.0", 0x00D2, 10, 21.0},
		{"positive 23.2", 0x00E8, 10, 23.2},
		{"zero", 0x0000, 10, 0.0},
		// 문서 §4.5: 0xFFC4 = −60 = −6.0 °C. signed 해석이 아니면 6553.2 가 된다.
		{"negative -6.0", 0xFFC4, 10, -6.0},
		{"negative -0.1", 0xFFFF, 10, -0.1},
		{"scale 1", 24, 1, 24.0},
		{"scale 1 negative", 0xFFFA, 1, -6.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pmbusDecodeTemp(c.raw, c.scale)
			// 10 으로 나눈 값의 부동소수 비교 — 0.001 허용.
			if diff := got - c.want; diff > 0.001 || diff < -0.001 {
				t.Errorf("decodeTemp(0x%04X, %d) = %v, want %v", c.raw, c.scale, got, c.want)
			}
		})
	}
}

// TestPmbusDecodeTemp_SignedNotUnsigned 는 signed 해석을 명시적으로 고정한다.
// uint16 으로 해석하면 6553.2 가 나오므로, 이 테스트가 회귀를 잡는다.
func TestPmbusDecodeTemp_SignedNotUnsigned(t *testing.T) {
	got := pmbusDecodeTemp(0xFFC4, 10)
	if got > 0 {
		t.Fatalf("0xFFC4 decoded as %v (positive) — unsigned interpretation leaked in", got)
	}
}

func TestPmbusEncodeTemp(t *testing.T) {
	cases := []struct {
		tempC float64
		scale int
		want  uint16
	}{
		{24.0, 10, 240},
		{26.0, 10, 260},
		{16.0, 10, 160},
		{30.0, 10, 300},
		{24.5, 10, 245},
		{24.0, 1, 24},
		{-6.0, 10, 0xFFC4},
	}
	for _, c := range cases {
		if got := pmbusEncodeTemp(c.tempC, c.scale); got != c.want {
			t.Errorf("encodeTemp(%v, %d) = 0x%04X (%d), want 0x%04X (%d)",
				c.tempC, c.scale, got, got, c.want, c.want)
		}
	}
}

// TestPmbusTempRoundTrip 은 인코딩 → 디코딩 왕복이 값을 보존하는지 확인한다.
func TestPmbusTempRoundTrip(t *testing.T) {
	for v := 160; v <= 300; v++ {
		tempC := float64(v) / 10
		enc := pmbusEncodeTemp(tempC, 10)
		dec := pmbusDecodeTemp(enc, 10)
		if diff := dec - tempC; diff > 0.001 || diff < -0.001 {
			t.Fatalf("round trip failed at %.1f: encoded 0x%04X, decoded %v", tempC, enc, dec)
		}
	}
}

// ---------------------------------------------------------------------------
// 모드 / 풍량 매핑 (AC-009 ~ AC-011)
// ---------------------------------------------------------------------------

func TestPmbusModeToUnifiedID(t *testing.T) {
	cases := []struct {
		code uint16
		want int
	}{
		{pmbusModeCool, hvac.ModeCool},
		{pmbusModeDry, hvac.ModeDry},
		{pmbusModeFan, hvac.ModeFan},
		{pmbusModeAuto, hvac.ModeOffOrAuto},
		{pmbusModeHeat, hvac.ModeHeat},
		{99, hvac.ModeOffOrAuto}, // 알 수 없는 코드 폴백
	}
	for _, c := range cases {
		if got := pmbusModeToUnifiedID(c.code); got != c.want {
			t.Errorf("modeToUnifiedID(%d) = %d, want %d", c.code, got, c.want)
		}
	}
}

func TestPmbusModeFromName(t *testing.T) {
	cases := map[string]uint16{
		"cool": pmbusModeCool, "cooling": pmbusModeCool,
		"dry": pmbusModeDry, "dehumidify": pmbusModeDry,
		"fan":  pmbusModeFan,
		"auto": pmbusModeAuto,
		"heat": pmbusModeHeat, "heating": pmbusModeHeat,
	}
	for name, want := range cases {
		got, err := pmbusModeFromName(name)
		if err != nil {
			t.Errorf("modeFromName(%q): %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("modeFromName(%q) = %d, want %d", name, got, want)
		}
	}
	if _, err := pmbusModeFromName("turbo-cool"); !errors.Is(err, ErrHvacr03InvalidMode) {
		t.Errorf("unknown mode should be rejected, got %v", err)
	}
}

// TestPmbusFanToUnifiedID_DefaultAutoCode 는 문서 기본값(4=자동)의 매핑을 검증한다.
func TestPmbusFanToUnifiedID_DefaultAutoCode(t *testing.T) {
	const autoCode = 4
	cases := []struct {
		code uint16
		want int
	}{
		{pmbusFanLow, hvac.FanLow},
		{pmbusFanMedium, hvac.FanMedium},
		{pmbusFanHigh, hvac.FanHigh},
		{4, hvac.FanAuto},
		{0, hvac.FanOff},
	}
	for _, c := range cases {
		if got := pmbusFanToUnifiedID(c.code, autoCode); got != c.want {
			t.Errorf("fanToUnifiedID(%d, auto=%d) = %d, want %d", c.code, autoCode, got, c.want)
		}
	}
}

// TestPmbusFanToUnifiedID_CorrectedAutoCode 는 실측 교정(5=자동) 경로를 검증한다.
// LGCP 는 4=초강, 5=자동이므로, 현장에서 값 4 가 초강으로 동작하면 이 설정을 쓴다.
func TestPmbusFanToUnifiedID_CorrectedAutoCode(t *testing.T) {
	const autoCode = 5
	if got := pmbusFanToUnifiedID(4, autoCode); got != hvac.FanTurbo {
		t.Errorf("fan code 4 with auto=5 should be turbo, got %d", got)
	}
	if got := pmbusFanToUnifiedID(5, autoCode); got != hvac.FanAuto {
		t.Errorf("fan code 5 with auto=5 should be auto, got %d", got)
	}
}

func TestPmbusFanFromName(t *testing.T) {
	const autoCode = 4
	cases := map[string]uint16{
		"low": pmbusFanLow, "slow": pmbusFanLow,
		"medium": pmbusFanMedium, "mid": pmbusFanMedium,
		"high": pmbusFanHigh,
		"auto": 4,
	}
	for name, want := range cases {
		got, err := pmbusFanFromName(name, autoCode)
		if err != nil {
			t.Errorf("fanFromName(%q): %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("fanFromName(%q) = %d, want %d", name, got, want)
		}
	}
	// autoCode 가 4 일 때 turbo 는 표현할 수 없다 — 문서의 4값 체계에는 turbo 가 없다.
	if _, err := pmbusFanFromName("turbo", 4); !errors.Is(err, ErrHvacr03InvalidFanSpeed) {
		t.Errorf("turbo with auto=4 should be rejected, got %v", err)
	}
	// autoCode 가 5 로 교정되면 4 가 turbo 로 성립한다.
	if got, err := pmbusFanFromName("turbo", 5); err != nil || got != 4 {
		t.Errorf("turbo with auto=5 should map to 4, got %d (%v)", got, err)
	}
	if _, err := pmbusFanFromName("hurricane", autoCode); !errors.Is(err, ErrHvacr03InvalidFanSpeed) {
		t.Errorf("unknown fan speed should be rejected, got %v", err)
	}
}

func TestPmbusERVModeFromName(t *testing.T) {
	cases := map[string]uint16{
		"heat_exchange": pmbusERVModeHeatExchange,
		"heat-exchange": pmbusERVModeHeatExchange,
		"auto":          pmbusERVModeAuto,
		"normal":        pmbusERVModeNormal,
	}
	for name, want := range cases {
		got, err := pmbusERVModeFromName(name)
		if err != nil {
			t.Errorf("ervModeFromName(%q): %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("ervModeFromName(%q) = %d, want %d", name, got, want)
		}
	}
	if _, err := pmbusERVModeFromName("bypass"); err == nil {
		t.Error("unknown erv mode should be rejected")
	}
}

func TestPmbusLockCoilItem(t *testing.T) {
	cases := map[string]uint16{
		"remote":  pmbusCoilLockRemote,
		"mode":    pmbusCoilLockMode,
		"fan":     pmbusCoilLockFan,
		"temp":    pmbusCoilLockTemp,
		"address": pmbusCoilLockAddress,
	}
	for target, want := range cases {
		got, err := pmbusLockCoilItem(target)
		if err != nil {
			t.Errorf("lockCoilItem(%q): %v", target, err)
			continue
		}
		if got != want {
			t.Errorf("lockCoilItem(%q) = %d, want %d", target, got, want)
		}
	}
	if _, err := pmbusLockCoilItem("everything"); err == nil {
		t.Error("unknown lock target should be rejected")
	}
}

// ---------------------------------------------------------------------------
// 주소 파싱 (AC-012, AC-027)
// ---------------------------------------------------------------------------

func TestPmbusParseUnitAddr_Base0(t *testing.T) {
	cases := map[string]uint16{"0": 0, "3": 3, "15": 15}
	for s, want := range cases {
		got, err := pmbusParseUnitAddr(s, 0)
		if err != nil {
			t.Errorf("parseUnitAddr(%q, 0): %v", s, err)
			continue
		}
		if got != want {
			t.Errorf("parseUnitAddr(%q, 0) = %d, want %d", s, got, want)
		}
	}
}

// TestPmbusParseUnitAddr_Base1 은 실측이 1-base 로 밝혀졌을 때의 교정 경로를 검증한다.
func TestPmbusParseUnitAddr_Base1(t *testing.T) {
	got, err := pmbusParseUnitAddr("1", 1)
	if err != nil {
		t.Fatalf("parseUnitAddr(\"1\", 1): %v", err)
	}
	if got != 0 {
		t.Errorf("parseUnitAddr(\"1\", base=1) = %d, want 0", got)
	}
	if _, err := pmbusParseUnitAddr("0", 1); !errors.Is(err, ErrHvacr03InvalidAddress) {
		t.Errorf("address 0 with base=1 should be out of range, got %v", err)
	}
	if got, err := pmbusParseUnitAddr("16", 1); err != nil || got != 15 {
		t.Errorf("parseUnitAddr(\"16\", base=1) = %d (%v), want 15", got, err)
	}
}

func TestPmbusParseUnitAddr_Rejects(t *testing.T) {
	bad := []string{
		"",         // 누락
		"16",       // base 0 에서 범위 초과
		"-1",       // 음수
		"0x10",     // hex 표기
		"44550065", // LGCP 물리 주소 — Modbus N 과 별개의 값이다
		" 3",       // 공백
		"3.0",      // 소수
		"abc",      // 비숫자
	}
	for _, s := range bad {
		if _, err := pmbusParseUnitAddr(s, 0); err == nil {
			t.Errorf("parseUnitAddr(%q, 0) should fail", s)
		}
	}
}

func TestPmbusFormatUnitAddr(t *testing.T) {
	if got := pmbusFormatUnitAddr(3, 0); got != "3" {
		t.Errorf("formatUnitAddr(3, 0) = %q, want \"3\"", got)
	}
	if got := pmbusFormatUnitAddr(0, 1); got != "1" {
		t.Errorf("formatUnitAddr(0, 1) = %q, want \"1\"", got)
	}
	// 왕복 일관성.
	for base := 0; base <= 1; base++ {
		for n := uint16(0); n <= 15; n++ {
			s := pmbusFormatUnitAddr(n, base)
			back, err := pmbusParseUnitAddr(s, base)
			if err != nil || back != n {
				t.Errorf("round trip failed: n=%d base=%d → %q → %d (%v)", n, base, s, back, err)
			}
		}
	}
}
