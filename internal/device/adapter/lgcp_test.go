package adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLGCPAdapter_CommandSpec_target_temperature_param_name 는 lgcp 어댑터가
// target_temperature 명령의 파라미터 이름으로 'target_temp' 를 사용하는지 검증한다.
//
// 배경 (regression):
//   - 이전 spec 은 'temperature' 였으나 LGCPAgent.buildThermostatPayloadForCommand
//     가 params["target_temperature"] 를 요구하여 이름 불일치로
//     ErrLGCPMissingParam: target_temp 발생.
//   - NASA/LGAP/LGCP 공통 컨벤션 'target_temp' 로 통일.
//
// 참조: internal/agent/lg/lgcp_agent.go:703 (target_temperature 핸들러)
func TestLGCPAdapter_CommandSpec_target_temperature_param_name(t *testing.T) {
	specs := lgcpIndoorCommandSpecs()

	var setTempSpec *struct {
		Found bool
		Param string
	} = &struct {
		Found bool
		Param string
	}{}

	for _, s := range specs {
		if s.Name == "target_temperature" {
			setTempSpec.Found = true
			require.Len(t, s.Params, 1, "target_temperature 는 단일 파라미터여야 한다")
			setTempSpec.Param = s.Params[0].Name
			break
		}
	}

	require.True(t, setTempSpec.Found, "target_temperature spec 이 존재해야 한다")
	assert.Equal(t, "target_temperature", setTempSpec.Param,
		"파라미터 이름은 'target_temp' (이전 'temperature' 였으나 LGCP agent 와 불일치)")
}

// TestLGCPAdapter_CommandSpec_set_multiple_temp_param_name 는 set_multiple 의
// 온도 파라미터도 'target_temp' 를 사용하는지 검증한다.
//
// 참조: internal/agent/lg/lgcp_control.go:245 (buildControlPayload)
func TestLGCPAdapter_CommandSpec_set_multiple_temp_param_name(t *testing.T) {
	specs := lgcpIndoorCommandSpecs()

	var found bool
	var hasTargetTemp bool
	var hasOldTemperature bool

	for _, s := range specs {
		if s.Name == "set_multiple" {
			found = true
			for _, p := range s.Params {
				if p.Name == "target_temperature" {
					hasTargetTemp = true
				}
				if p.Name == "temperature" {
					hasOldTemperature = true
				}
			}
			break
		}
	}

	require.True(t, found, "set_multiple spec 이 존재해야 한다")
	assert.True(t, hasTargetTemp, "set_multiple 에 target_temp 파라미터가 있어야 한다")
	assert.False(t, hasOldTemperature, "set_multiple 에 옛 'temperature' 파라미터가 있으면 안 된다")
}
