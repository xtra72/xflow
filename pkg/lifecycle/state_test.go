package lifecycle

import "testing"

// TestState_String 은 각 State 상수의 String() 반환값을 검증한다 (AC-LIFE-001-01).
func TestState_String(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		expected string
	}{
		{name: "Created 상태", state: StateCreated, expected: "created"},
		{name: "Initializing 상태", state: StateInitializing, expected: "initializing"},
		{name: "Running 상태", state: StateRunning, expected: "running"},
		{name: "Paused 상태", state: StatePaused, expected: "paused"},
		{name: "Stopping 상태", state: StateStopping, expected: "stopping"},
		{name: "Stopped 상태", state: StateStopped, expected: "stopped"},
		{name: "Error 상태", state: StateError, expected: "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.expected {
				t.Errorf("State(%q).String() = %q, 기대값 %q", string(tt.state), got, tt.expected)
			}
		})
	}
}

// TestState_IsValid 는 유효한 상태에 대해 true를, 유효하지 않은 상태에 대해 false를 반환하는지 검증한다 (AC-LIFE-001-02).
func TestState_IsValid(t *testing.T) {
	validStates := []struct {
		name  string
		state State
	}{
		{name: "Created", state: StateCreated},
		{name: "Initializing", state: StateInitializing},
		{name: "Running", state: StateRunning},
		{name: "Paused", state: StatePaused},
		{name: "Stopping", state: StateStopping},
		{name: "Stopped", state: StateStopped},
		{name: "Error", state: StateError},
	}

	for _, tt := range validStates {
		t.Run(tt.name+"_유효함", func(t *testing.T) {
			if !tt.state.IsValid() {
				t.Errorf("State(%q).IsValid() = false, 유효한 상태이므로 true를 기대했다", tt.state)
			}
		})
	}

	invalidStates := []struct {
		name  string
		state State
	}{
		{name: "빈 문자열", state: State("")},
		{name: "알 수 없는 상태", state: State("unknown")},
		{name: "대문자 변형", state: State("Running")},
		{name: "공백 포함", state: State(" created")},
	}

	for _, tt := range invalidStates {
		t.Run(tt.name+"_유효하지_않음", func(t *testing.T) {
			if tt.state.IsValid() {
				t.Errorf("State(%q).IsValid() = true, 유효하지 않은 상태이므로 false를 기대했다", tt.state)
			}
		})
	}
}

// TestParseState_Valid 는 유효한 문자열을 올바른 State로 파싱하는지 검증한다 (AC-LIFE-001-03).
func TestParseState_Valid(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected State
	}{
		{name: "created 파싱", input: "created", expected: StateCreated},
		{name: "initializing 파싱", input: "initializing", expected: StateInitializing},
		{name: "running 파싱", input: "running", expected: StateRunning},
		{name: "paused 파싱", input: "paused", expected: StatePaused},
		{name: "stopping 파싱", input: "stopping", expected: StateStopping},
		{name: "stopped 파싱", input: "stopped", expected: StateStopped},
		{name: "error 파싱", input: "error", expected: StateError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseState(tt.input)
			if err != nil {
				t.Fatalf("ParseState(%q) 에서 예상치 못한 에러: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("ParseState(%q) = %q, 기대값 %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestParseState_Invalid 는 유효하지 않은 문자열에 대해 ErrInvalidState를 반환하는지 검증한다 (AC-LIFE-001-03).
func TestParseState_Invalid(t *testing.T) {
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
			_, err := ParseState(tt.input)
			if err == nil {
				t.Errorf("ParseState(%q) 에서 에러를 기대했으나 nil을 반환했다", tt.input)
			}
			if err != ErrInvalidState {
				t.Errorf("ParseState(%q) 에러 = %v, 기대값 ErrInvalidState", tt.input, err)
			}
		})
	}
}

// TestIsValidTransition_Valid 는 유효한 상태 전이가 true를 반환하는지 검증한다 (AC-LIFE-001-04).
func TestIsValidTransition_Valid(t *testing.T) {
	tests := []struct {
		name string
		from State
		to   State
	}{
		// Created -> Initializing
		{name: "Created에서 Initializing으로", from: StateCreated, to: StateInitializing},

		// Initializing -> Running, Error
		{name: "Initializing에서 Running으로", from: StateInitializing, to: StateRunning},
		{name: "Initializing에서 Error로", from: StateInitializing, to: StateError},

		// Running -> Paused, Stopping, Error
		{name: "Running에서 Paused로", from: StateRunning, to: StatePaused},
		{name: "Running에서 Stopping으로", from: StateRunning, to: StateStopping},
		{name: "Running에서 Error로", from: StateRunning, to: StateError},

		// Paused -> Running, Stopping, Error
		{name: "Paused에서 Running으로", from: StatePaused, to: StateRunning},
		{name: "Paused에서 Stopping으로", from: StatePaused, to: StateStopping},
		{name: "Paused에서 Error로", from: StatePaused, to: StateError},

		// Stopping -> Stopped, Error
		{name: "Stopping에서 Stopped로", from: StateStopping, to: StateStopped},
		{name: "Stopping에서 Error로", from: StateStopping, to: StateError},

		// Stopped -> Created
		{name: "Stopped에서 Created로", from: StateStopped, to: StateCreated},

		// Error -> Stopping, Stopped, Created
		{name: "Error에서 Stopping으로", from: StateError, to: StateStopping},
		{name: "Error에서 Stopped로", from: StateError, to: StateStopped},
		{name: "Error에서 Created로", from: StateError, to: StateCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !IsValidTransition(tt.from, tt.to) {
				t.Errorf("IsValidTransition(%q, %q) = false, 유효한 전이이므로 true를 기대했다", tt.from, tt.to)
			}
		})
	}
}

// TestIsValidTransition_Invalid 는 유효하지 않은 상태 전이가 false를 반환하는지 검증한다 (AC-LIFE-001-05).
func TestIsValidTransition_Invalid(t *testing.T) {
	tests := []struct {
		name string
		from State
		to   State
	}{
		// Created에서 Running으로 직접 전이 불가
		{name: "Created에서 Running으로 불가", from: StateCreated, to: StateRunning},
		// Created에서 자기 자신으로 전이 불가
		{name: "Created에서 Created로 불가", from: StateCreated, to: StateCreated},
		// Running에서 Created로 직접 전이 불가
		{name: "Running에서 Created로 불가", from: StateRunning, to: StateCreated},
		// Stopped에서 Running으로 직접 전이 불가
		{name: "Stopped에서 Running으로 불가", from: StateStopped, to: StateRunning},
		// Initializing에서 Paused로 불가
		{name: "Initializing에서 Paused로 불가", from: StateInitializing, to: StatePaused},
		// Stopping에서 Running으로 불가
		{name: "Stopping에서 Running으로 불가", from: StateStopping, to: StateRunning},
		// Paused에서 Created로 불가
		{name: "Paused에서 Created로 불가", from: StatePaused, to: StateCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsValidTransition(tt.from, tt.to) {
				t.Errorf("IsValidTransition(%q, %q) = true, 유효하지 않은 전이이므로 false를 기대했다", tt.from, tt.to)
			}
		})
	}
}

// TestValidTransitions_Completeness 는 ValidTransitions 맵이 모든 상태에 대한 전이 규칙을 포함하는지 검증한다 (AC-LIFE-001-06).
func TestValidTransitions_Completeness(t *testing.T) {
	allStates := []State{
		StateCreated,
		StateInitializing,
		StateRunning,
		StatePaused,
		StateStopping,
		StateStopped,
		StateError,
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

// TestValidTransitions_ExpectedCounts 는 각 상태의 유효 전이 개수가 올바른지 검증한다 (AC-LIFE-001-07).
func TestValidTransitions_ExpectedCounts(t *testing.T) {
	tests := []struct {
		name          string
		state         State
		expectedCount int
	}{
		{name: "Created의 전이 수", state: StateCreated, expectedCount: 1},
		{name: "Initializing의 전이 수", state: StateInitializing, expectedCount: 2},
		{name: "Running의 전이 수", state: StateRunning, expectedCount: 3},
		{name: "Paused의 전이 수", state: StatePaused, expectedCount: 3},
		{name: "Stopping의 전이 수", state: StateStopping, expectedCount: 2},
		{name: "Stopped의 전이 수", state: StateStopped, expectedCount: 1},
		{name: "Error의 전이 수", state: StateError, expectedCount: 3},
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
	unknownState := State("unknown")

	if IsValidTransition(unknownState, StateRunning) {
		t.Error("알 수 없는 from 상태에서 IsValidTransition이 true를 반환했다")
	}
	if IsValidTransition(StateCreated, unknownState) {
		t.Error("알 수 없는 to 상태에서 IsValidTransition이 true를 반환했다")
	}
}
