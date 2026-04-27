package samsung

import (
	"testing"
)

// TestNASAAddressString 은 NASAAddress.String() 메서드가
// "XX.XX.XX" 형식의 점 구분 16진수를 반환하는지 검증한다.
func TestNASAAddressString(t *testing.T) {
	tests := []struct {
		name string
		addr NASAAddress
		want string
	}{
		{name: "controller address", addr: NASAAddress{0x6A, 0xEE, 0xFF}, want: "6A.EE.FF"},
		{name: "broadcast all", addr: NASAAddress{0xB0, 0xFF, 0xFF}, want: "B0.FF.FF"},
		{name: "broadcast indoor", addr: NASAAddress{0xB2, 0xFF, 0x20}, want: "B2.FF.20"},
		{name: "outdoor unit 0", addr: NASAAddress{0x10, 0x00, 0x00}, want: "10.00.00"},
		{name: "indoor unit 0-1", addr: NASAAddress{0x20, 0x00, 0x01}, want: "20.00.01"},
		{name: "zero address", addr: NASAAddress{0x00, 0x00, 0x00}, want: "00.00.00"},
		{name: "max address", addr: NASAAddress{0xFF, 0xFF, 0xFF}, want: "FF.FF.FF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.addr.String()
			if got != tt.want {
				t.Errorf("NASAAddress(%v).String() = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

// TestNASAAddressHex 는 NASAAddress.Hex() 메서드가
// "XXXXXX" 형식의 컴팩트 16진수를 반환하는지 검증한다.
func TestNASAAddressHex(t *testing.T) {
	tests := []struct {
		name string
		addr NASAAddress
		want string
	}{
		{name: "controller address", addr: NASAAddress{0x6A, 0xEE, 0xFF}, want: "6AEEFF"},
		{name: "broadcast all", addr: NASAAddress{0xB0, 0xFF, 0xFF}, want: "B0FFFF"},
		{name: "indoor unit", addr: NASAAddress{0x20, 0x00, 0x01}, want: "200001"},
		{name: "zero address", addr: NASAAddress{0x00, 0x00, 0x00}, want: "000000"},
		{name: "max address", addr: NASAAddress{0xFF, 0xFF, 0xFF}, want: "FFFFFF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.addr.Hex()
			if got != tt.want {
				t.Errorf("NASAAddress(%v).Hex() = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

// TestNASAAddressIsOutdoor 는 첫 번째 바이트가 0x10인 주소를 실외기로 판별하는지 검증한다.
func TestNASAAddressIsOutdoor(t *testing.T) {
	tests := []struct {
		name string
		addr NASAAddress
		want bool
	}{
		{name: "outdoor unit 0", addr: NASAAddress{0x10, 0x00, 0x00}, want: true},
		{name: "outdoor unit 5", addr: NASAAddress{0x10, 0x05, 0x00}, want: true},
		{name: "indoor unit", addr: NASAAddress{0x20, 0x00, 0x01}, want: false},
		{name: "controller", addr: NASAAddress{0x6A, 0xEE, 0xFF}, want: false},
		{name: "broadcast", addr: NASAAddress{0xB0, 0xFF, 0xFF}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.addr.IsOutdoor()
			if got != tt.want {
				t.Errorf("NASAAddress(%v).IsOutdoor() = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

// TestNASAAddressIsIndoor 는 첫 번째 바이트가 0x20인 주소를 실내기로 판별하는지 검증한다.
func TestNASAAddressIsIndoor(t *testing.T) {
	tests := []struct {
		name string
		addr NASAAddress
		want bool
	}{
		{name: "indoor unit 0-0", addr: NASAAddress{0x20, 0x00, 0x00}, want: true},
		{name: "indoor unit 0-1", addr: NASAAddress{0x20, 0x00, 0x01}, want: true},
		{name: "indoor unit 3-7", addr: NASAAddress{0x20, 0x03, 0x07}, want: true},
		{name: "outdoor unit", addr: NASAAddress{0x10, 0x00, 0x00}, want: false},
		{name: "controller", addr: NASAAddress{0x6A, 0xEE, 0xFF}, want: false},
		{name: "broadcast", addr: NASAAddress{0xB0, 0xFF, 0xFF}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.addr.IsIndoor()
			if got != tt.want {
				t.Errorf("NASAAddress(%v).IsIndoor() = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

// TestNASAAddressIsController 는 주소가 {0x6A, 0xEE, 0xFF}인지 검증한다.
func TestNASAAddressIsController(t *testing.T) {
	tests := []struct {
		name string
		addr NASAAddress
		want bool
	}{
		{name: "exact controller address", addr: NASAAddress{0x6A, 0xEE, 0xFF}, want: true},
		{name: "outdoor unit", addr: NASAAddress{0x10, 0x00, 0x00}, want: false},
		{name: "indoor unit", addr: NASAAddress{0x20, 0x00, 0x01}, want: false},
		{name: "similar but not controller", addr: NASAAddress{0x6A, 0xEE, 0xFE}, want: false},
		{name: "broadcast", addr: NASAAddress{0xB0, 0xFF, 0xFF}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.addr.IsController()
			if got != tt.want {
				t.Errorf("NASAAddress(%v).IsController() = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

// TestNASAAddressIsBroadcast 는 첫 번째 바이트가 0xB0, 0xB2, 0xB3인 주소를
// 브로드캐스트로 판별하는지 검증한다.
func TestNASAAddressIsBroadcast(t *testing.T) {
	tests := []struct {
		name string
		addr NASAAddress
		want bool
	}{
		{name: "broadcast all (B0)", addr: NASAAddress{0xB0, 0xFF, 0xFF}, want: true},
		{name: "broadcast indoor (B2)", addr: NASAAddress{0xB2, 0xFF, 0x20}, want: true},
		{name: "broadcast indoor specific (B3)", addr: NASAAddress{0xB3, 0x00, 0x01}, want: true},
		{name: "outdoor B0 variant", addr: NASAAddress{0xB0, 0x01, 0xFF}, want: true},
		{name: "outdoor unit", addr: NASAAddress{0x10, 0x00, 0x00}, want: false},
		{name: "indoor unit", addr: NASAAddress{0x20, 0x00, 0x01}, want: false},
		{name: "controller", addr: NASAAddress{0x6A, 0xEE, 0xFF}, want: false},
		{name: "B1 is not broadcast", addr: NASAAddress{0xB1, 0xFF, 0xFF}, want: false},
		{name: "B4 is not broadcast", addr: NASAAddress{0xB4, 0xFF, 0xFF}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.addr.IsBroadcast()
			if got != tt.want {
				t.Errorf("NASAAddress(%v).IsBroadcast() = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

// TestNASAAddressOutdoorIndex 는 실외기 주소의 두 번째 바이트(물리 주소)를 반환하는지 검증한다.
func TestNASAAddressOutdoorIndex(t *testing.T) {
	tests := []struct {
		name string
		addr NASAAddress
		want byte
	}{
		{name: "outdoor index 0", addr: NASAAddress{0x10, 0x00, 0x00}, want: 0x00},
		{name: "outdoor index 5", addr: NASAAddress{0x10, 0x05, 0x00}, want: 0x05},
		{name: "outdoor index 15", addr: NASAAddress{0x10, 0x0F, 0x00}, want: 0x0F},
		{name: "any address returns second byte", addr: NASAAddress{0x20, 0x03, 0x07}, want: 0x03},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.addr.OutdoorIndex()
			if got != tt.want {
				t.Errorf("NASAAddress(%v).OutdoorIndex() = 0x%02X, want 0x%02X", tt.addr, got, tt.want)
			}
		})
	}
}

// TestNASAAddressIndoorIndex 는 실내기 주소에서 (outdoor, indoor) 바이트 쌍을 반환하는지 검증한다.
func TestNASAAddressIndoorIndex(t *testing.T) {
	tests := []struct {
		name        string
		addr        NASAAddress
		wantOutdoor byte
		wantIndoor  byte
	}{
		{name: "indoor 0-0", addr: NASAAddress{0x20, 0x00, 0x00}, wantOutdoor: 0x00, wantIndoor: 0x00},
		{name: "indoor 0-1", addr: NASAAddress{0x20, 0x00, 0x01}, wantOutdoor: 0x00, wantIndoor: 0x01},
		{name: "indoor 3-7", addr: NASAAddress{0x20, 0x03, 0x07}, wantOutdoor: 0x03, wantIndoor: 0x07},
		{name: "any address returns second and third bytes", addr: NASAAddress{0x10, 0x05, 0x0A}, wantOutdoor: 0x05, wantIndoor: 0x0A},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOutdoor, gotIndoor := tt.addr.IndoorIndex()
			if gotOutdoor != tt.wantOutdoor || gotIndoor != tt.wantIndoor {
				t.Errorf("NASAAddress(%v).IndoorIndex() = (0x%02X, 0x%02X), want (0x%02X, 0x%02X)",
					tt.addr, gotOutdoor, gotIndoor, tt.wantOutdoor, tt.wantIndoor)
			}
		})
	}
}

// TestNASAAddressConstants 는 사전 정의된 주소 상수가 올바른 값을 갖는지 검증한다.
func TestNASAAddressConstants(t *testing.T) {
	tests := []struct {
		name string
		addr NASAAddress
		want NASAAddress
	}{
		{name: "AddrController", addr: AddrController, want: NASAAddress{0x6A, 0xEE, 0xFF}},
		{name: "AddrBroadcastAll", addr: AddrBroadcastAll, want: NASAAddress{0xB0, 0xFF, 0xFF}},
		{name: "AddrBroadcastIndoor", addr: AddrBroadcastIndoor, want: NASAAddress{0xB2, 0xFF, 0x20}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.addr != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.addr, tt.want)
			}
		})
	}
}

// TestNewOutdoorAddr 는 실외기 주소 생성 함수가 {0x10, index, 0x00}을 반환하는지 검증한다.
func TestNewOutdoorAddr(t *testing.T) {
	tests := []struct {
		name  string
		index byte
		want  NASAAddress
	}{
		{name: "index 0", index: 0x00, want: NASAAddress{0x10, 0x00, 0x00}},
		{name: "index 1", index: 0x01, want: NASAAddress{0x10, 0x01, 0x00}},
		{name: "index 15", index: 0x0F, want: NASAAddress{0x10, 0x0F, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewOutdoorAddr(tt.index)
			if got != tt.want {
				t.Errorf("NewOutdoorAddr(0x%02X) = %v, want %v", tt.index, got, tt.want)
			}
		})
	}
}

// TestNewIndoorAddr 는 실내기 주소 생성 함수가 {0x20, outdoor, indoor}를 반환하는지 검증한다.
func TestNewIndoorAddr(t *testing.T) {
	tests := []struct {
		name    string
		outdoor byte
		indoor  byte
		want    NASAAddress
	}{
		{name: "indoor 0-0", outdoor: 0x00, indoor: 0x00, want: NASAAddress{0x20, 0x00, 0x00}},
		{name: "indoor 0-1", outdoor: 0x00, indoor: 0x01, want: NASAAddress{0x20, 0x00, 0x01}},
		{name: "indoor 3-7", outdoor: 0x03, indoor: 0x07, want: NASAAddress{0x20, 0x03, 0x07}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewIndoorAddr(tt.outdoor, tt.indoor)
			if got != tt.want {
				t.Errorf("NewIndoorAddr(0x%02X, 0x%02X) = %v, want %v", tt.outdoor, tt.indoor, got, tt.want)
			}
		})
	}
}

// TestNewOutdoorBroadcast 는 실외기 브로드캐스트 주소 생성 함수가 {0xB0, index, 0xFF}를 반환하는지 검증한다.
func TestNewOutdoorBroadcast(t *testing.T) {
	tests := []struct {
		name  string
		index byte
		want  NASAAddress
	}{
		{name: "outdoor broadcast 0", index: 0x00, want: NASAAddress{0xB0, 0x00, 0xFF}},
		{name: "outdoor broadcast 1", index: 0x01, want: NASAAddress{0xB0, 0x01, 0xFF}},
		{name: "outdoor broadcast 15", index: 0x0F, want: NASAAddress{0xB0, 0x0F, 0xFF}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewOutdoorBroadcast(tt.index)
			if got != tt.want {
				t.Errorf("NewOutdoorBroadcast(0x%02X) = %v, want %v", tt.index, got, tt.want)
			}
		})
	}
}

// TestNewIndoorBroadcast 는 실내기 브로드캐스트 주소 생성 함수가 {0xB3, outdoor, indoor}를 반환하는지 검증한다.
func TestNewIndoorBroadcast(t *testing.T) {
	tests := []struct {
		name    string
		outdoor byte
		indoor  byte
		want    NASAAddress
	}{
		{name: "indoor broadcast 0-0", outdoor: 0x00, indoor: 0x00, want: NASAAddress{0xB3, 0x00, 0x00}},
		{name: "indoor broadcast 0-1", outdoor: 0x00, indoor: 0x01, want: NASAAddress{0xB3, 0x00, 0x01}},
		{name: "indoor broadcast 3-7", outdoor: 0x03, indoor: 0x07, want: NASAAddress{0xB3, 0x03, 0x07}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewIndoorBroadcast(tt.outdoor, tt.indoor)
			if got != tt.want {
				t.Errorf("NewIndoorBroadcast(0x%02X, 0x%02X) = %v, want %v", tt.outdoor, tt.indoor, got, tt.want)
			}
		})
	}
}

// TestParseNASAAddress 는 문자열 파싱 함수가 다양한 형식의 입력을 올바르게 처리하는지 검증한다.
func TestParseNASAAddress(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    NASAAddress
		wantErr error
	}{
		// 스페이스 구분 16진수 형식
		{name: "spaced hex indoor", input: "20 00 01", want: NASAAddress{0x20, 0x00, 0x01}},
		{name: "spaced hex outdoor", input: "10 00 00", want: NASAAddress{0x10, 0x00, 0x00}},
		{name: "spaced hex controller", input: "6A EE FF", want: NASAAddress{0x6A, 0xEE, 0xFF}},
		{name: "spaced hex broadcast", input: "B0 FF FF", want: NASAAddress{0xB0, 0xFF, 0xFF}},

		// 점 구분 16진수 형식
		{name: "dotted hex indoor", input: "20.00.01", want: NASAAddress{0x20, 0x00, 0x01}},
		{name: "dotted hex outdoor", input: "10.00.00", want: NASAAddress{0x10, 0x00, 0x00}},
		{name: "dotted hex controller", input: "6A.EE.FF", want: NASAAddress{0x6A, 0xEE, 0xFF}},
		{name: "dotted hex broadcast", input: "B0.FF.FF", want: NASAAddress{0xB0, 0xFF, 0xFF}},
		{name: "dotted lowercase", input: "2a.00.ff", want: NASAAddress{0x2A, 0x00, 0xFF}},

		// 컴팩트 16진수 형식
		{name: "compact hex indoor", input: "200001", want: NASAAddress{0x20, 0x00, 0x01}},
		{name: "compact hex outdoor", input: "100000", want: NASAAddress{0x10, 0x00, 0x00}},
		{name: "compact hex controller", input: "6AEEFF", want: NASAAddress{0x6A, 0xEE, 0xFF}},
		{name: "compact hex broadcast", input: "B0FFFF", want: NASAAddress{0xB0, 0xFF, 0xFF}},

		// 대소문자 혼합
		{name: "lowercase compact", input: "2a00ff", want: NASAAddress{0x2A, 0x00, 0xFF}},
		{name: "uppercase compact", input: "2A00FF", want: NASAAddress{0x2A, 0x00, 0xFF}},
		{name: "mixed case compact", input: "2a00FF", want: NASAAddress{0x2A, 0x00, 0xFF}},
		{name: "lowercase spaced", input: "2a 00 ff", want: NASAAddress{0x2A, 0x00, 0xFF}},
		{name: "mixed case spaced", input: "2A 00 ff", want: NASAAddress{0x2A, 0x00, 0xFF}},

		// 에러 케이스
		{name: "empty string", input: "", wantErr: ErrInvalidAddress},
		{name: "too short", input: "20", wantErr: ErrInvalidAddress},
		{name: "too short compact", input: "2000", wantErr: ErrInvalidAddress},
		{name: "too long compact", input: "20000100", wantErr: ErrInvalidAddress},
		{name: "invalid hex chars", input: "ZZZZZZ", wantErr: ErrInvalidAddress},
		{name: "invalid spaced hex", input: "ZZ ZZ ZZ", wantErr: ErrInvalidAddress},
		{name: "partial invalid", input: "20 GG 01", wantErr: ErrInvalidAddress},
		{name: "wrong separator", input: "20-00-01", wantErr: ErrInvalidAddress},
		{name: "spaces only", input: "   ", wantErr: ErrInvalidAddress},
		{name: "spaced but wrong length", input: "20 00", wantErr: ErrInvalidAddress},
		{name: "four bytes spaced", input: "20 00 01 02", wantErr: ErrInvalidAddress},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNASAAddress(tt.input)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("ParseNASAAddress(%q) = %v, want error %v", tt.input, got, tt.wantErr)
					return
				}
				if err != tt.wantErr {
					t.Errorf("ParseNASAAddress(%q) error = %v, want %v", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseNASAAddress(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("ParseNASAAddress(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseNASAAddressRoundTrip 은 String()과 Hex()로 출력한 값을 다시 파싱하면
// 원래 주소와 동일한지 검증한다.
func TestParseNASAAddressRoundTrip(t *testing.T) {
	addresses := []NASAAddress{
		{0x10, 0x00, 0x00},
		{0x20, 0x00, 0x01},
		{0x6A, 0xEE, 0xFF},
		{0xB0, 0xFF, 0xFF},
		{0xB2, 0xFF, 0x20},
		{0xB3, 0x03, 0x07},
		{0x00, 0x00, 0x00},
		{0xFF, 0xFF, 0xFF},
	}
	for _, addr := range addresses {
		t.Run("String/"+addr.String(), func(t *testing.T) {
			parsed, err := ParseNASAAddress(addr.String())
			if err != nil {
				t.Fatalf("ParseNASAAddress(%q) error: %v", addr.String(), err)
			}
			if parsed != addr {
				t.Errorf("round-trip String failed: %v -> %q -> %v", addr, addr.String(), parsed)
			}
		})
		t.Run("Hex/"+addr.Hex(), func(t *testing.T) {
			parsed, err := ParseNASAAddress(addr.Hex())
			if err != nil {
				t.Fatalf("ParseNASAAddress(%q) error: %v", addr.Hex(), err)
			}
			if parsed != addr {
				t.Errorf("round-trip Hex failed: %v -> %q -> %v", addr, addr.Hex(), parsed)
			}
		})
	}
}

// TestConstantsClassification 은 상수 주소들이 올바른 분류 메서드 결과를 반환하는지 검증한다.
func TestConstantsClassification(t *testing.T) {
	t.Run("AddrController is controller", func(t *testing.T) {
		if !AddrController.IsController() {
			t.Error("AddrController.IsController() = false, want true")
		}
		if AddrController.IsOutdoor() {
			t.Error("AddrController.IsOutdoor() = true, want false")
		}
		if AddrController.IsIndoor() {
			t.Error("AddrController.IsIndoor() = true, want false")
		}
		if AddrController.IsBroadcast() {
			t.Error("AddrController.IsBroadcast() = true, want false")
		}
	})

	t.Run("AddrBroadcastAll is broadcast", func(t *testing.T) {
		if !AddrBroadcastAll.IsBroadcast() {
			t.Error("AddrBroadcastAll.IsBroadcast() = false, want true")
		}
		if AddrBroadcastAll.IsOutdoor() {
			t.Error("AddrBroadcastAll.IsOutdoor() = true, want false")
		}
		if AddrBroadcastAll.IsIndoor() {
			t.Error("AddrBroadcastAll.IsIndoor() = true, want false")
		}
		if AddrBroadcastAll.IsController() {
			t.Error("AddrBroadcastAll.IsController() = true, want false")
		}
	})

	t.Run("AddrBroadcastIndoor is broadcast", func(t *testing.T) {
		if !AddrBroadcastIndoor.IsBroadcast() {
			t.Error("AddrBroadcastIndoor.IsBroadcast() = false, want true")
		}
		if AddrBroadcastIndoor.IsOutdoor() {
			t.Error("AddrBroadcastIndoor.IsOutdoor() = true, want false")
		}
		if AddrBroadcastIndoor.IsIndoor() {
			t.Error("AddrBroadcastIndoor.IsIndoor() = true, want false")
		}
	})
}
