package modbus

import (
	"errors"
	"testing"
	"time"

	modbus "github.com/xtra/xflow/internal/modbus"
)

// TestParseRegisterGroupConfig_PollInterval 는 그룹별 poll_interval 파싱을 검증한다(M5).
func TestParseRegisterGroupConfig_PollInterval(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		wantErr  bool
		wantDur  time.Duration
		hasValue bool
	}{
		{name: "미지정 → 0(폴백)", value: nil, wantDur: 0},
		{name: "유효 지속시간", value: "250ms", wantDur: 250 * time.Millisecond, hasValue: true},
		{name: "빈 문자열 → 0(폴백)", value: "", wantDur: 0, hasValue: true},
		{name: "잘못된 문자열 → 오류", value: "not-a-duration", wantErr: true, hasValue: true},
		{name: "음수 → 오류", value: "-5s", wantErr: true, hasValue: true},
		{name: "0 → 오류", value: "0s", wantErr: true, hasValue: true},
		{name: "비문자열 → 오류", value: 123, wantErr: true, hasValue: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := map[string]any{
				"name":          "g",
				"function_code": 3,
				"start_address": 0,
				"quantity":      10,
			}
			if tt.hasValue {
				m["poll_interval"] = tt.value
			}
			rg, err := parseRegisterGroupConfig(m, 0, 0)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("기대: 오류, 실제: nil (value=%v)", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("기대: 성공, 실제 오류: %v", err)
			}
			if rg.PollInterval != tt.wantDur {
				t.Errorf("PollInterval = %v, want %v", rg.PollInterval, tt.wantDur)
			}
		})
	}
}

// TestParseClientTypeMap_ByteOrderValidation 는 type_map byte_order 검증을 확인한다(M6, REQ-03).
// 별칭·4순열은 허용, 알 수 없는 값은 ErrUnsupportedByteOrder 로 거부한다.
func TestParseClientTypeMap_ByteOrderValidation(t *testing.T) {
	valid := []string{
		modbus.ByteOrderBigEndian, modbus.ByteOrderLittleEndian,
		modbus.ByteOrderABCD, modbus.ByteOrderBADC, modbus.ByteOrderCDAB, modbus.ByteOrderDCBA,
	}
	for _, bo := range valid {
		entries := []any{
			map[string]any{"address": 0, "data_type": "int32", "byte_order": bo},
		}
		got, err := parseClientTypeMap(entries, "test")
		if err != nil {
			t.Errorf("byte_order %q: 기대 성공, 실제 오류: %v", bo, err)
			continue
		}
		if len(got) != 1 || got[0].ByteOrder != bo {
			t.Errorf("byte_order %q: 파싱 결과 불일치: %+v", bo, got)
		}
	}

	// 알 수 없는 byte_order → ErrUnsupportedByteOrder.
	entries := []any{
		map[string]any{"address": 0, "data_type": "int32", "byte_order": "middle_endian"},
	}
	_, err := parseClientTypeMap(entries, "test")
	if !errors.Is(err, ErrUnsupportedByteOrder) {
		t.Errorf("잘못된 byte_order: 기대 ErrUnsupportedByteOrder, 실제: %v", err)
	}
}

// TestParseModbusConfig_GroupPollInterval_EndToEnd 는 전체 설정 파싱에서
// 그룹별 poll_interval 이 DeviceConfig 까지 반영되는지 검증한다(M5).
func TestParseModbusConfig_GroupPollInterval_EndToEnd(t *testing.T) {
	opts := map[string]any{
		"devices": []any{
			map[string]any{
				"id":   "d1",
				"host": "127.0.0.1",
				"register_groups": []any{
					map[string]any{
						"name": "fast", "function_code": 3, "start_address": 0,
						"quantity": 4, "poll_interval": "50ms",
					},
					map[string]any{
						"name": "default", "function_code": 3, "start_address": 100,
						"quantity": 4,
					},
				},
			},
		},
	}
	cfg, err := parseModbusConfig(opts)
	if err != nil {
		t.Fatalf("parseModbusConfig 오류: %v", err)
	}
	groups := cfg.Devices[0].RegisterGroups
	if groups[0].PollInterval != 50*time.Millisecond {
		t.Errorf("fast 그룹 PollInterval = %v, want 50ms", groups[0].PollInterval)
	}
	if groups[1].PollInterval != 0 {
		t.Errorf("default 그룹 PollInterval = %v, want 0(폴백)", groups[1].PollInterval)
	}
}
