package xsfm

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// ---------------------------------------------------------------------------
// B4 — 셀렉터 fan-out (그룹/역사/호선 일괄 제어, REQ-XSFM-001-04-01..09)
// ---------------------------------------------------------------------------
//
// 제어 명령(set_power / set_fan_speed / set_multiple)의 대상은 단일 device_id 또는
// 셀렉터(station / line / group_id)로 지정된다. 셀렉터는 오직 "대상 집합 선정"만 담당하며,
// 멤버별 제어 실행은 개별 제어(Module 3)의 controlDevice 경로를 그대로 재사용한다(중복 구현
// 금지, REQ-04-08). fan-out 은 멤버별로 controlDevice 를 호출하고 각 멤버가 자체 응답 대기를
// 수행하며(B3), best-effort 로 한 멤버의 실패가 나머지를 중단시키지 않는다(REQ-04-03).
//
// 동시성 규약: 대상 도출(DevicesByStation/DevicesByLine/GroupMembers)은 각자 락을 스냅샷
// 후 해제하며, 그다음 controlDevice 를 멤버마다 호출한다(멤버별 로스터 RLock + pending 락).
// 로스터 락을 fan-out 전체에 걸쳐 잡지 않으며, 레지스트리 락을 로스터 락에 중첩하지 않는다.
// 멤버는 고루틴으로 동시 실행되며, 각 controlDevice 가 control_response_timeout 만큼 블록할
// 수 있으므로 순차 fan-out(N×timeout) 대신 동시 fan-out 으로 지연을 상수화한다. B3 pending
// 레지스트리가 동시성 안전이라 동시 fan-out 이 안전하다.

// selectorRef 는 fan-out 대상 셀렉터의 종류와 값이다 (집계 응답 표기용).
type selectorRef struct {
	Type  string `json:"type"` // "group_id" | "station" | "line"
	Value string `json:"value"`
}

// groupResult 는 fan-out 집계 응답의 멤버별 결과이다 (REQ-XSFM-001-04-07).
type groupResult struct {
	DeviceID string `json:"device_id"`
	Status   string `json:"status"`          // "ok" | "error" | "timeout"
	Error    string `json:"error,omitempty"` // status != "ok" 일 때만 채워진다.
}

// fanOutResponse 는 셀렉터 fan-out 집계 응답이다 (REQ-XSFM-001-04-07).
//
// 최상위 status: 모든 멤버 ok → "ok"; 하나 이상 error/timeout 이며 ok 가 1개 이상 → "partial";
// 전부 실패 → "error". Excluded 는 line 셀렉터에서 미등록 station 을 참조해 대상에서 제외된
// device_id 목록이다 (REQ-XSFM-001-02-13/04-09).
type fanOutResponse struct {
	Selector selectorRef   `json:"selector"`
	Results  []groupResult `json:"results"`
	Excluded []string      `json:"excluded,omitempty"`
	Status   string        `json:"status"`
}

// controlStep 은 한 멤버에 순차 적용할 단일 제어 단위이다 (command kind + 페이로드).
// set_multiple(power=on + fan_speed) 은 전원 ON 을 먼저, 풍량을 그다음으로 방출하기 위해
// 2단계 플랜이 된다 (REQ-03-04 순서 보장). 그 외 명령은 1단계 플랜이다.
type controlStep struct {
	command string
	cmd     commandPayload
}

// buildControlPlan 은 명령·params 를 검증하여 멤버별로 실행할 제어 플랜을 구성한다.
//
// 값 검증(풍량 1/2/3, 필수 params 존재)은 개별 제어(Module 3)와 동일한 헬퍼(boolParam/
// intParam)를 재사용하며 셀렉터와 무관하므로 fan-out 전에 한 번만 수행한다 (REQ-04-08). 잘못된
// params 는 여기서 에러가 되어 어떠한 방출도 일어나지 않는다 (개별 제어와 동일 의미론).
//
// NOTE(power-off 게이트): 개별 set_fan_speed 의 전원 OFF 게이트(fan_speed_power_off_policy)
// 는 멤버별 현재 전원 상태에 의존하는 개별 제어 편의 정책이므로 셀렉터 fan-out 에는 적용하지
// 않는다. fan-out 은 미션 지시대로 멤버마다 controlDevice 를 반복하며, 값 검증·2-축·응답
// 대기·인코딩 경로만 재사용한다 (REQ-04-08). 대량 일괄 제어는 원 명령을 그대로 방출한다.
func buildControlPlan(command string, params map[string]any) ([]controlStep, error) {
	switch command {
	case "set_power":
		power, err := boolParam(params, "power")
		if err != nil {
			return nil, err
		}
		return []controlStep{{command: "set_power", cmd: commandPayload{Power: &power}}}, nil

	case "set_fan_speed":
		fs, err := intParam(params, "fan_speed")
		if err != nil {
			return nil, err
		}
		if fs < 1 || fs > 3 {
			return nil, fmt.Errorf("%w: got %d", ErrInvalidFanSpeed, fs)
		}
		return []controlStep{{command: "set_fan_speed", cmd: commandPayload{FanSpeed: &fs}}}, nil

	case "set_multiple":
		powerPtr, fanPtr, err := parseSetMultipleParams(params)
		if err != nil {
			return nil, err
		}
		if powerPtr != nil && *powerPtr && fanPtr != nil {
			// REQ-03-04: 전원 ON 을 먼저, 그다음 fan_speed (개별 제어와 동일 순서).
			return []controlStep{
				{command: "set_power", cmd: commandPayload{Power: powerPtr}},
				{command: "set_fan_speed", cmd: commandPayload{FanSpeed: fanPtr}},
			}, nil
		}
		return []controlStep{{command: "set_multiple", cmd: commandPayload{Power: powerPtr, FanSpeed: fanPtr}}}, nil

	default:
		return nil, fmt.Errorf("%w: %q is not a fan-out control command", ErrInvalidCommand, command)
	}
}

// dispatchControl 은 제어 명령(set_power/set_fan_speed/set_multiple)을 대상 지정 방식으로
// 라우팅한다 (REQ-XSFM-001-04-04 우선순위).
//
// device_id 가 지정되면 최우선으로 단일 디바이스 경로(개별 제어 Module 3)로 처리하며 셀렉터가
// 함께 지정돼도 fan-out 하지 않는다(device_id > station > line > group_id). device_id 가 없으면
// handleSelectorControl 이 station → line → group_id 순으로 fan-out 한다.
func (a *XSFMAgent) dispatchControl(req processRequest) ([]byte, error) {
	if req.DeviceID != "" {
		switch req.Command {
		case "set_power":
			return a.handleSetPower(req)
		case "set_fan_speed":
			return a.handleSetFanSpeed(req)
		case "set_multiple":
			return a.handleSetMultiple(req)
		default:
			return nil, fmt.Errorf("%w: %q is not a control command", ErrInvalidCommand, req.Command)
		}
	}
	return a.handleSelectorControl(req)
}

// handleSelectorControl 은 device_id 가 없는 제어 명령을 셀렉터로 라우팅해 fan-out 한다.
//
// 셀렉터 우선순위(REQ-XSFM-001-04-04): device_id > station > line > group_id. device_id 는
// 상위(dispatchControl)에서 이미 단일 경로로 처리되므로, 여기서는 station → line → group_id
// 순으로 첫 번째로 지정된 셀렉터를 결정론적으로 해석한다. 셀렉터가 하나도 없으면 대상 미지정
// 이므로 ErrInvalidCommand 를 반환한다.
func (a *XSFMAgent) handleSelectorControl(req processRequest) ([]byte, error) {
	var sel selectorRef
	var targets, excluded []string

	switch {
	case req.Station != "":
		sel = selectorRef{Type: "station", Value: req.Station}
		targets = a.DevicesByStation(req.Station)
	case req.Line != "":
		sel = selectorRef{Type: "line", Value: req.Line}
		targets, excluded = a.DevicesByLine(req.Line)
	case req.GroupID != "":
		sel = selectorRef{Type: "group_id", Value: req.GroupID}
		// 미등록 커스텀 그룹은 ErrGroupNotFound 로 거부한다(REQ-05-05). station:/line:/레거시
		// group_id 는 파생/무회귀 경로로 수렴하므로 존재 검사 없이 GroupMembers 로 대상을 도출한다
		// (RD-4 셀렉터 병존 — group_id=station:<code> 는 station 셀렉터와 동일 결과). GroupMembers
		// 는 접두사 분기 + 로스터 대조 필터를 적용하며(RD-2/RD-3), 빈 대상은 fanOutControl 이
		// ErrEmptyGroup 으로 처리한다(REQ-05-04).
		if err := a.ensureGroupExists(req.GroupID); err != nil {
			return nil, err
		}
		targets = a.GroupMembers(req.GroupID)
	default:
		return nil, fmt.Errorf("%w: control requires device_id or a selector (station/line/group_id)", ErrInvalidCommand)
	}

	// 값 검증 + 제어 플랜 구성 (fan-out 전 1회, 개별 제어와 동일 검증 재사용). 잘못된 params
	// 는 여기서 거부되어 어떠한 멤버로도 방출되지 않는다.
	plan, err := buildControlPlan(req.Command, req.Params)
	if err != nil {
		return nil, err
	}

	return a.fanOutControl(sel, targets, excluded, plan)
}

// fanOutControl 은 대상 멤버 집합에 제어 플랜을 동시 fan-out 하고 집계 응답을 반환한다.
//
// 빈 대상 집합은 ErrEmptyGroup 을 반환하며 어떠한 방출도 하지 않는다(REQ-04-02, group/station/
// line 공통). 각 멤버는 고루틴에서 controlDevice 를 통해 자체 응답 대기를 수행하고, best-effort
// 로 한 멤버의 실패가 나머지를 중단시키지 않는다. 결과 슬라이스는 멤버별 고유 인덱스에만 기록
// 되므로 동시 쓰기 race 가 없다.
func (a *XSFMAgent) fanOutControl(sel selectorRef, targets, excluded []string, plan []controlStep) ([]byte, error) {
	if len(targets) == 0 {
		return nil, fmt.Errorf("%w: selector %s=%q", ErrEmptyGroup, sel.Type, sel.Value)
	}

	results := make([]groupResult, len(targets))
	var wg sync.WaitGroup
	for i, id := range targets {
		wg.Add(1)
		go func(i int, deviceID string) {
			defer wg.Done()
			results[i] = a.executeMemberPlan(deviceID, plan)
		}(i, id)
	}
	wg.Wait()

	resp := fanOutResponse{
		Selector: sel,
		Results:  results,
		Excluded: excluded,
		Status:   aggregateStatus(results),
	}

	// 그룹/셀렉터 감사 시임 (REQ-04-01 step 5): 셀렉터 종류·값 + 멤버별 결과를 B7 이 소비한다.
	a.recordGroupAudit(sel, resp.Results)

	return json.Marshal(resp)
}

// executeMemberPlan 은 한 멤버에 제어 플랜을 순차 실행하고 멤버 결과를 반환한다.
//
// 플랜의 각 단계는 controlDevice 를 재사용한다(인코딩·방출·응답 대기 공유, REQ-04-08). 한
// 단계가 실패하면 그 멤버의 남은 단계는 중단하되(예: 전원 ON 실패 시 풍량 미방출) 다른 멤버는
// 영향받지 않는다(best-effort 는 멤버 간 격리이며 고루틴 분리로 보장된다).
func (a *XSFMAgent) executeMemberPlan(deviceID string, plan []controlStep) groupResult {
	for _, step := range plan {
		if _, err := a.controlDevice(deviceID, step.command, step.cmd); err != nil {
			gr := groupResult{DeviceID: deviceID, Error: err.Error()}
			if errors.Is(err, ErrControlTimeout) {
				gr.Status = "timeout"
				gr.Error = "ErrControlTimeout" // 집계 응답 표기(REQ-04-07 예시와 일치).
			} else {
				gr.Status = "error"
			}
			return gr
		}
	}
	return groupResult{DeviceID: deviceID, Status: "ok"}
}

// aggregateStatus 는 멤버별 결과로 최상위 집계 상태를 도출한다.
// 전부 ok → "ok"; 전부 실패 → "error"; 그 외(일부 성공+일부 실패) → "partial".
func aggregateStatus(results []groupResult) string {
	ok := 0
	for _, r := range results {
		if r.Status == "ok" {
			ok++
		}
	}
	switch {
	case ok == len(results):
		return "ok"
	case ok == 0:
		return "error"
	default:
		return "partial"
	}
}

// recordGroupAudit 는 그룹/셀렉터 제어 감사 레코드 기록 표면이다 (REQ-04-01 step 5,
// REQ-XSFM-001-06-03 그룹 절). 구현은 audit.go 에 있다 — 셀렉터 종류·값 + fan-out 멤버
// 목록 + 집계 결과를 요약 레코드로 남긴다(멤버별 device_id 감사는 controlDevice 가 담당, B7).
