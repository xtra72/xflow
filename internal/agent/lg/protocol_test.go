package lg

import (
	"errors"
	"testing"
)

func TestBuildStatusQuery(t *testing.T) {
	p := NewLGAPProtocol()

	tests := []struct {
		name string
		zone byte
	}{
		{name: "zone 0x10", zone: 0x10},
		{name: "zone 0x00", zone: 0x00},
		{name: "zone 0xFF", zone: 0xFF},
		{name: "zone 0x23", zone: 0x23},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkt := p.BuildStatusQuery(tt.zone)

			// Must be exactly RequestSize (8) bytes
			if len(pkt) != RequestSize {
				t.Fatalf("BuildStatusQuery(0x%02X) len = %d, want %d", tt.zone, len(pkt), RequestSize)
			}

			// TX0: header
			if pkt[0] != HeaderByte {
				t.Errorf("pkt[0] = 0x%02X, want HeaderByte 0x%02X", pkt[0], HeaderByte)
			}

			// TX1: command byte
			if pkt[1] != CommandByte {
				t.Errorf("pkt[1] = 0x%02X, want CommandByte 0x%02X", pkt[1], CommandByte)
			}

			// TX2: command ID
			if pkt[2] != CommandID {
				t.Errorf("pkt[2] = 0x%02X, want CommandID 0x%02X", pkt[2], CommandID)
			}

			// TX3: zone
			if pkt[3] != tt.zone {
				t.Errorf("pkt[3] = 0x%02X, want zone 0x%02X", pkt[3], tt.zone)
			}

			// TX4: flags = 0 (read-only)
			if pkt[4] != 0x00 {
				t.Errorf("pkt[4] = 0x%02X, want 0x00 (read-only, no flags)", pkt[4])
			}

			// TX5: mode combo = 0
			if pkt[5] != 0x00 {
				t.Errorf("pkt[5] = 0x%02X, want 0x00", pkt[5])
			}

			// TX6: temperature = 0
			if pkt[6] != 0x00 {
				t.Errorf("pkt[6] = 0x%02X, want 0x00", pkt[6])
			}

			// TX7: valid checksum
			if !VerifyChecksum(pkt) {
				t.Errorf("BuildStatusQuery(0x%02X) has invalid checksum: pkt=%#v", tt.zone, pkt)
			}
		})
	}
}

func TestBuildControlCommand(t *testing.T) {
	p := NewLGAPProtocol()

	tests := []struct {
		name      string
		zone      byte
		flags     byte
		modeCombo byte
		temp      byte
	}{
		{
			name:      "power on, cool/low, 25 degrees",
			zone:      0x10,
			flags:     FlagPower,
			modeCombo: EncodeModeCombo(ModeCool, FanLow, false),
			temp:      EncodeTargetTemp(25),
		},
		{
			name:      "no flags, heat/high/swing, 30 degrees",
			zone:      0x23,
			flags:     0x00,
			modeCombo: EncodeModeCombo(ModeHeat, FanHigh, true),
			temp:      EncodeTargetTemp(30),
		},
		{
			name:      "power+lock+plasma flags",
			zone:      0x11,
			flags:     FlagPower | FlagLock | FlagPlasma,
			modeCombo: EncodeModeCombo(ModeAuto, FanAuto, false),
			temp:      EncodeTargetTemp(22),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkt := p.BuildControlCommand(tt.zone, tt.flags, tt.modeCombo, tt.temp)

			// Must be exactly RequestSize (8) bytes
			if len(pkt) != RequestSize {
				t.Fatalf("BuildControlCommand() len = %d, want %d", len(pkt), RequestSize)
			}

			// TX0: header
			if pkt[0] != HeaderByte {
				t.Errorf("pkt[0] = 0x%02X, want HeaderByte 0x%02X", pkt[0], HeaderByte)
			}

			// TX1: command byte
			if pkt[1] != CommandByte {
				t.Errorf("pkt[1] = 0x%02X, want CommandByte 0x%02X", pkt[1], CommandByte)
			}

			// TX2: command ID
			if pkt[2] != CommandID {
				t.Errorf("pkt[2] = 0x%02X, want CommandID 0x%02X", pkt[2], CommandID)
			}

			// TX3: zone
			if pkt[3] != tt.zone {
				t.Errorf("pkt[3] = 0x%02X, want zone 0x%02X", pkt[3], tt.zone)
			}

			// TX4: flags must always have FlagExecute set
			if pkt[4]&FlagExecute == 0 {
				t.Errorf("pkt[4] = 0x%02X, FlagExecute bit not set", pkt[4])
			}
			expectedFlags := tt.flags | FlagExecute
			if pkt[4] != expectedFlags {
				t.Errorf("pkt[4] = 0x%02X, want 0x%02X", pkt[4], expectedFlags)
			}

			// TX5: mode combo
			if pkt[5] != tt.modeCombo {
				t.Errorf("pkt[5] = 0x%02X, want modeCombo 0x%02X", pkt[5], tt.modeCombo)
			}

			// TX6: temperature
			if pkt[6] != tt.temp {
				t.Errorf("pkt[6] = 0x%02X, want temp 0x%02X", pkt[6], tt.temp)
			}

			// TX7: valid checksum
			if !VerifyChecksum(pkt) {
				t.Errorf("BuildControlCommand() has invalid checksum: pkt=%#v", pkt)
			}
		})
	}
}

func TestBuildControlCommand_FlagExecuteAlwaysSet(t *testing.T) {
	p := NewLGAPProtocol()

	// Even when flags=0x00, FlagExecute should be set
	pkt := p.BuildControlCommand(0x10, 0x00, 0x00, 0x00)
	if pkt[4]&FlagExecute == 0 {
		t.Errorf("FlagExecute not set when flags=0x00: pkt[4]=0x%02X", pkt[4])
	}
	if pkt[4] != FlagExecute {
		t.Errorf("pkt[4] = 0x%02X, want only FlagExecute 0x%02X", pkt[4], FlagExecute)
	}
}

// buildValidResponse creates a valid 16-byte LGAP response with correct checksum.
func buildValidResponse(t *testing.T, fields map[int]byte) []byte {
	t.Helper()
	data := make([]byte, ResponseSize)
	data[0] = HeaderByte // RX0: header

	for idx, val := range fields {
		if idx >= 0 && idx < ResponseSize {
			data[idx] = val
		}
	}
	// Ensure header is always set
	data[0] = HeaderByte

	// Set checksum as last byte
	data[15] = CalcLGAPChecksum(data[:15])
	return data
}

func TestDecodeResponse_Valid(t *testing.T) {
	p := NewLGAPProtocol()

	data := buildValidResponse(t, map[int]byte{
		1:  0x00, // Status: normal
		2:  FlagPower | FlagPlasma, // FlagsEcho
		4:  0x10, // Zone
		5:  0x00, // Error: none
		6:  EncodeModeCombo(ModeCool, FanHigh, true), // ModeCombo
		7:  EncodeTargetTemp(25),                       // TargetTemp
		8:  132,  // RoomTemp -> 20.0
		9:  147,  // PipeInTemp -> 15.0
		10: 162,  // PipeOutTemp -> 10.0
		11: 204,  // ZoneLoad: idle
		12: 0x00, // ZonePower: running
		13: 100,  // DesignLoad
		14: 80,   // ODULoad
	})

	resp, err := p.DecodeResponse(data)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}

	if resp.Status != 0x00 {
		t.Errorf("Status = 0x%02X, want 0x00", resp.Status)
	}
	if resp.FlagsEcho != (FlagPower | FlagPlasma) {
		t.Errorf("FlagsEcho = 0x%02X, want 0x%02X", resp.FlagsEcho, FlagPower|FlagPlasma)
	}
	if resp.Zone != 0x10 {
		t.Errorf("Zone = 0x%02X, want 0x10", resp.Zone)
	}
	if resp.Error != 0x00 {
		t.Errorf("Error = 0x%02X, want 0x00", resp.Error)
	}
	if resp.TargetTemp != EncodeTargetTemp(25) {
		t.Errorf("TargetTemp = 0x%02X, want 0x%02X", resp.TargetTemp, EncodeTargetTemp(25))
	}
	if resp.RoomTemp != 132 {
		t.Errorf("RoomTemp = %d, want 132", resp.RoomTemp)
	}
	if resp.ZoneLoad != 204 {
		t.Errorf("ZoneLoad = %d, want 204", resp.ZoneLoad)
	}
	if resp.ZonePower != 0x00 {
		t.Errorf("ZonePower = 0x%02X, want 0x00", resp.ZonePower)
	}
	if resp.DesignLoad != 100 {
		t.Errorf("DesignLoad = %d, want 100", resp.DesignLoad)
	}
	if resp.ODULoad != 80 {
		t.Errorf("ODULoad = %d, want 80", resp.ODULoad)
	}

	// Verify Raw is copied
	for i := 0; i < ResponseSize; i++ {
		if resp.Raw[i] != data[i] {
			t.Errorf("Raw[%d] = 0x%02X, want 0x%02X", i, resp.Raw[i], data[i])
		}
	}
}

func TestDecodeResponse_WrongSize(t *testing.T) {
	p := NewLGAPProtocol()

	sizes := []int{0, 1, 7, 8, 15, 17, 32}
	for _, size := range sizes {
		data := make([]byte, size)
		if size > 0 {
			data[0] = HeaderByte
		}
		_, err := p.DecodeResponse(data)
		if err == nil {
			t.Errorf("DecodeResponse(len=%d) expected error, got nil", size)
		}
	}
}

func TestDecodeResponse_WrongHeader(t *testing.T) {
	p := NewLGAPProtocol()

	data := make([]byte, ResponseSize)
	data[0] = 0xFF // wrong header
	data[15] = CalcLGAPChecksum(data[:15])

	_, err := p.DecodeResponse(data)
	if err == nil {
		t.Fatal("DecodeResponse() expected error for wrong header, got nil")
	}
}

func TestDecodeResponse_WrongChecksum(t *testing.T) {
	p := NewLGAPProtocol()

	data := buildValidResponse(t, map[int]byte{
		1: 0x00,
		4: 0x10,
	})
	// Corrupt the checksum
	data[15] ^= 0xFF

	_, err := p.DecodeResponse(data)
	if err == nil {
		t.Fatal("DecodeResponse() expected error for wrong checksum, got nil")
	}
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("DecodeResponse() error = %v, want ErrChecksumMismatch", err)
	}
}

func TestBuildStatusQuery_PacketStructureRoundTrip(t *testing.T) {
	p := NewLGAPProtocol()

	zone := byte(0x10)
	pkt := p.BuildStatusQuery(zone)

	// Verify the packet structure by manual reconstruction
	expected := make([]byte, RequestSize)
	expected[0] = HeaderByte
	expected[1] = CommandByte
	expected[2] = CommandID
	expected[3] = zone
	expected[4] = 0x00
	expected[5] = 0x00
	expected[6] = 0x00
	expected[7] = CalcLGAPChecksum(expected[:7])

	for i := range expected {
		if pkt[i] != expected[i] {
			t.Errorf("pkt[%d] = 0x%02X, want 0x%02X", i, pkt[i], expected[i])
		}
	}
}

func TestCalculateChecksum(t *testing.T) {
	p := NewLGAPProtocol()

	// CalculateChecksum should delegate to CalcLGAPChecksum
	data := []byte{0x10, 0x00, 0xA0, 0x10}
	got := p.CalculateChecksum(data)
	want := CalcLGAPChecksum(data)
	if got != want {
		t.Errorf("CalculateChecksum() = 0x%02X, want 0x%02X", got, want)
	}
}
