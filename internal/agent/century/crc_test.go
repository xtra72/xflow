package century

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// TestCRC16ARC_KnownVectors verifies the canonical CRC-16/ARC algorithm
// against well-known reference vectors. The protocol spec §9 provides the
// Python reference implementation:
//
//	def crc16_arc(data: bytes) -> int:
//	    crc = 0x0000
//	    for b in data:
//	        crc ^= b
//	        for _ in range(8):
//	            crc = (crc >> 1) ^ 0xA001 if (crc & 1) else (crc >> 1)
//	    return crc
//
// (REQ-CENTURY-004)
func TestCRC16ARC_KnownVectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
		want uint16
	}{
		{
			name: "empty data returns 0x0000 (init)",
			data: []byte{},
			want: 0x0000,
		},
		{
			name: "single zero byte",
			data: []byte{0x00},
			want: 0x0000,
		},
		{
			name: "single 0x01 byte",
			data: []byte{0x01},
			want: 0xC0C1,
		},
		{
			// Classic CRC-16/ARC check vector for ASCII "123456789".
			// Source: CRC RevEng catalogue (CRC-16/ARC).
			name: "ASCII 123456789",
			data: []byte("123456789"),
			want: 0xBB3D,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := CRC16ARC(tc.data)
			if got != tc.want {
				t.Fatalf("CRC16ARC(%x) = 0x%04X, want 0x%04X", tc.data, got, tc.want)
			}
		})
	}
}

// TestCRC16ARC_CAP3Fixtures validates that every CAP-3 raw frame from spec
// 부록 A produces the stored CRC trailer. This anchors the implementation
// against ground truth captures.
//
// (REQ-CENTURY-004, AC-A7 part 1)
func TestCRC16ARC_CAP3Fixtures(t *testing.T) {
	t.Parallel()

	fixtures := []string{
		"cap3_read_req_reg02.bin",
		"cap3_reg02_response.bin",
		"cap3_read_req_reg03.bin",
		"cap3_reg03_response.bin",
		"cap3_read_req_reg04.bin",
		"cap3_reg04_response.bin",
		"cap3_write_reg04.bin",
		"cap3_ack.bin",
	}

	for _, name := range fixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw := mustReadFixture(t, name)
			if len(raw) < 4 {
				t.Fatalf("fixture %s too short: %d bytes", name, len(raw))
			}
			body := raw[:len(raw)-2]
			stored := binary.LittleEndian.Uint16(raw[len(raw)-2:])
			got := CRC16ARC(body)
			if got != stored {
				t.Fatalf("fixture %s: CRC16ARC(body) = 0x%04X, want 0x%04X (stored LE)", name, got, stored)
			}
		})
	}
}

// TestCRC16ARC_RejectsModbusInit ensures that switching to Modbus RTU init
// (0xFFFF) breaks validation of every CAP-3 frame. This is the regression
// guard required by REQ-CENTURY-004 and AC-A7 part 2: an implementation
// that accidentally uses init 0xFFFF must visibly fail.
func TestCRC16ARC_RejectsModbusInit(t *testing.T) {
	t.Parallel()

	fixtures := []string{
		"cap3_read_req_reg02.bin",
		"cap3_reg02_response.bin",
		"cap3_read_req_reg03.bin",
		"cap3_reg03_response.bin",
		"cap3_read_req_reg04.bin",
		"cap3_reg04_response.bin",
		"cap3_write_reg04.bin",
		"cap3_ack.bin",
	}

	for _, name := range fixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw := mustReadFixture(t, name)
			body := raw[:len(raw)-2]
			stored := binary.LittleEndian.Uint16(raw[len(raw)-2:])

			arc := CRC16ARC(body)
			modbus := crc16WithInit(body, 0xFFFF)

			if arc != stored {
				t.Fatalf("sanity: CRC16ARC(body) = 0x%04X, want 0x%04X", arc, stored)
			}
			if modbus == stored {
				t.Fatalf("regression: Modbus init (0xFFFF) should NOT match stored CRC (0x%04X) for fixture %s; got 0x%04X",
					stored, name, modbus)
			}
		})
	}
}

// crc16WithInit is a parametrized variant of CRC-16/ARC used only by the
// regression test (TestCRC16ARC_RejectsModbusInit) to demonstrate that an
// implementation with the wrong init value cannot pass validation. It is
// not exported.
func crc16WithInit(data []byte, init uint16) uint16 {
	crc := init
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

// mustReadFixture loads a raw frame binary from testdata/.
func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}
