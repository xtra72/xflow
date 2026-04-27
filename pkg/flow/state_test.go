package flow

import "testing"

// TestFlowState_String 은 각 FlowState 상수의 String() 반환값을 검증한다.
func TestFlowState_String(t *testing.T) {
	tests := []struct {
		name     string
		state    FlowState
		expected string
	}{
		{name: "Stored 상태", state: FlowStored, expected: "stored"},
		{name: "Loaded 상태", state: FlowLoaded, expected: "loaded"},
		{name: "Initializing 상태", state: FlowInitializing, expected: "initializing"},
		{name: "Running 상태", state: FlowRunning, expected: "running"},
		{name: "Paused 상태", state: FlowPaused, expected: "paused"},
		{name: "Stopping 상태", state: FlowStopping, expected: "stopping"},
		{name: "Stopped 상태", state: FlowStopped, expected: "stopped"},
		{name: "Error 상태", state: FlowError, expected: "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.expected {
				t.Errorf("FlowState(%q).String() = %q, 기대값 %q", string(tt.state), got, tt.expected)
			}
		})
	}
}

// TestParseFlowState_Valid 는 유효한 문자열을 올바른 FlowState로 파싱하는지 검증한다.
func TestParseFlowState_Valid(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected FlowState
	}{
		{name: "stored 파싱", input: "stored", expected: FlowStored},
		{name: "loaded 파싱", input: "loaded", expected: FlowLoaded},
		{name: "initializing 파싱", input: "initializing", expected: FlowInitializing},
		{name: "running 파싱", input: "running", expected: FlowRunning},
		{name: "paused 파싱", input: "paused", expected: FlowPaused},
		{name: "stopping 파싱", input: "stopping", expected: FlowStopping},
		{name: "stopped 파싱", input: "stopped", expected: FlowStopped},
		{name: "error 파싱", input: "error", expected: FlowError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFlowState(tt.input)
			if err != nil {
				t.Fatalf("ParseFlowState(%q) 에서 예상치 못한 에러: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("ParseFlowState(%q) = %q, 기대값 %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestParseFlowState_Invalid 는 유효하지 않은 문자열에 대해 ErrInvalidFlowState를 반환하는지 검증한다.
func TestParseFlowState_Invalid(t *testing.T) {
	invalidInputs := []struct {
		name  string
		input string
	}{
		{name: "빈 문자열", input: ""},
		{name: "알 수 없는 상태", input: "unknown"},
		{name: "대문자 변형", input: "Running"},
		{name: "공백 포함", input: " running"},
		{name: "숫자 입력", input: "123"},
		{name: "특수문자 입력", input: "run-ning"},
	}

	for _, tt := range invalidInputs {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFlowState(tt.input)
			if err == nil {
				t.Errorf("ParseFlowState(%q) 에서 에러를 기대했으나 nil을 반환했다", tt.input)
			}
			if err != ErrInvalidFlowState {
				t.Errorf("ParseFlowState(%q) 에러 = %v, 기대값 ErrInvalidFlowState", tt.input, err)
			}
		})
	}
}

// TestIsValidTransition_Valid 는 유효한 상태 전이가 true를 반환하는지 검증한다 (AC-20, AC-22).
func TestIsValidTransition_Valid(t *testing.T) {
	tests := []struct {
		name string
		from FlowState
		to   FlowState
	}{
		// Stored -> Loaded
		{name: "Stored에서 Loaded로", from: FlowStored, to: FlowLoaded},

		// Loaded -> Initializing, Stored
		{name: "Loaded에서 Initializing으로", from: FlowLoaded, to: FlowInitializing},
		{name: "Loaded에서 Stored로", from: FlowLoaded, to: FlowStored},

		// Initializing -> Running, Error, Stopped
		{name: "Initializing에서 Running으로", from: FlowInitializing, to: FlowRunning},
		{name: "Initializing에서 Error로", from: FlowInitializing, to: FlowError},
		{name: "Initializing에서 Stopped로", from: FlowInitializing, to: FlowStopped},

		// Running -> Paused, Stopping, Error
		{name: "Running에서 Paused로", from: FlowRunning, to: FlowPaused},
		{name: "Running에서 Stopping으로", from: FlowRunning, to: FlowStopping},
		{name: "Running에서 Error로", from: FlowRunning, to: FlowError},

		// Paused -> Running, Stopping, Error
		{name: "Paused에서 Running으로", from: FlowPaused, to: FlowRunning},
		{name: "Paused에서 Stopping으로", from: FlowPaused, to: FlowStopping},
		{name: "Paused에서 Error로", from: FlowPaused, to: FlowError},

		// Stopping -> Stopped, Error
		{name: "Stopping에서 Stopped로", from: FlowStopping, to: FlowStopped},
		{name: "Stopping에서 Error로", from: FlowStopping, to: FlowError},

		// Stopped -> Loaded, Stored
		{name: "Stopped에서 Loaded로", from: FlowStopped, to: FlowLoaded},
		{name: "Stopped에서 Stored로", from: FlowStopped, to: FlowStored},

		// Error -> Stopped, Initializing
		{name: "Error에서 Stopped로", from: FlowError, to: FlowStopped},
		{name: "Error에서 Initializing으로", from: FlowError, to: FlowInitializing},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !IsValidTransition(tt.from, tt.to) {
				t.Errorf("IsValidTransition(%q, %q) = false, 유효한 전이이므로 true를 기대했다", tt.from, tt.to)
			}
		})
	}
}

// TestIsValidTransition_Invalid 는 유효하지 않은 상태 전이가 false를 반환하는지 검증한다 (AC-21, AC-22).
func TestIsValidTransition_Invalid(t *testing.T) {
	tests := []struct {
		name string
		from FlowState
		to   FlowState
	}{
		// Stored에서 직접 Running으로는 불가
		{name: "Stored에서 Running으로 불가", from: FlowStored, to: FlowRunning},
		// Running에서 직접 Stored로는 불가
		{name: "Running에서 Stored로 불가", from: FlowRunning, to: FlowStored},
		// Stored에서 자기 자신으로 전이 불가
		{name: "Stored에서 Stored로 불가", from: FlowStored, to: FlowStored},
		// Running에서 직접 Loaded로 불가
		{name: "Running에서 Loaded로 불가", from: FlowRunning, to: FlowLoaded},
		// Paused에서 Stored로 불가
		{name: "Paused에서 Stored로 불가", from: FlowPaused, to: FlowStored},
		// Stopping에서 Running으로 불가
		{name: "Stopping에서 Running으로 불가", from: FlowStopping, to: FlowRunning},
		// Error에서 Running으로 불가
		{name: "Error에서 Running으로 불가", from: FlowError, to: FlowRunning},
		// Initializing에서 Stored로 불가
		{name: "Initializing에서 Stored로 불가", from: FlowInitializing, to: FlowStored},
		// Initializing에서 Loaded로 불가
		{name: "Initializing에서 Loaded로 불가", from: FlowInitializing, to: FlowLoaded},
		// Stored에서 Error로 불가
		{name: "Stored에서 Error로 불가", from: FlowStored, to: FlowError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsValidTransition(tt.from, tt.to) {
				t.Errorf("IsValidTransition(%q, %q) = true, 유효하지 않은 전이이므로 false를 기대했다", tt.from, tt.to)
			}
		})
	}
}

// TestValidTransitions_Completeness 는 ValidTransitions 맵이 모든 상태에 대한 전이 규칙을 포함하는지 검증한다 (AC-24).
func TestValidTransitions_Completeness(t *testing.T) {
	allStates := []FlowState{
		FlowStored,
		FlowLoaded,
		FlowInitializing,
		FlowRunning,
		FlowPaused,
		FlowStopping,
		FlowStopped,
		FlowError,
	}

	for _, state := range allStates {
		t.Run("전이 맵에 "+string(state)+" 존재", func(t *testing.T) {
			targets, exists := ValidTransitions[state]
			if !exists {
				t.Errorf("ValidTransitions 맵에 %q 상태가 정의되어 있지 않다", state)
			}
			if len(targets) == 0 {
				t.Errorf("ValidTransitions[%q] 의 전이 대상이 비어있다", state)
			}
		})
	}
}

// TestValidTransitions_ExpectedCounts 는 각 상태의 유효 전이 개수가 올바른지 검증한다 (AC-24).
func TestValidTransitions_ExpectedCounts(t *testing.T) {
	tests := []struct {
		name          string
		state         FlowState
		expectedCount int
	}{
		{name: "Stored의 전이 수", state: FlowStored, expectedCount: 1},
		{name: "Loaded의 전이 수", state: FlowLoaded, expectedCount: 2},
		{name: "Initializing의 전이 수", state: FlowInitializing, expectedCount: 3},
		{name: "Running의 전이 수", state: FlowRunning, expectedCount: 3},
		{name: "Paused의 전이 수", state: FlowPaused, expectedCount: 3},
		{name: "Stopping의 전이 수", state: FlowStopping, expectedCount: 2},
		{name: "Stopped의 전이 수", state: FlowStopped, expectedCount: 2},
		{name: "Error의 전이 수", state: FlowError, expectedCount: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets := ValidTransitions[tt.state]
			if len(targets) != tt.expectedCount {
				t.Errorf("ValidTransitions[%q] 전이 수 = %d, 기대값 %d", tt.state, len(targets), tt.expectedCount)
			}
		})
	}
}

// TestIsValidTransition_UnknownState 는 알 수 없는 상태에 대해 false를 반환하는지 검증한다.
func TestIsValidTransition_UnknownState(t *testing.T) {
	unknownState := FlowState("unknown")

	if IsValidTransition(unknownState, FlowRunning) {
		t.Error("알 수 없는 from 상태에서 IsValidTransition이 true를 반환했다")
	}
	if IsValidTransition(FlowStored, unknownState) {
		t.Error("알 수 없는 to 상태에서 IsValidTransition이 true를 반환했다")
	}
}
