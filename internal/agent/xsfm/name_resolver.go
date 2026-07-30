package xsfm

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// 이름 기반 제어 셀렉터 리졸버 (SPEC-XSFM-NAMESEL-001 Module 1)
// ---------------------------------------------------------------------------
//
// 이름 셀렉터(device_name/group_name)는 기존 id/code 셀렉터 위에 가산되는 얇은 해소 레이어이다.
// 리졸버는 이름을 정확히 하나의 대상(device_id 또는 그룹 id)으로 해소만 하고 제어를 실행하지
// 않는다 — 디스패치(handleSelectorControl)가 해소 결과를 기존 개별/fan-out 경로에 위임한다
// (단일 책임, RD-1/A-3).
//
// 매칭 규칙(RD-6): 입력과 비교 대상 Name 양쪽을 strings.TrimSpace 로 trim 한 뒤 정확히 일치할
// 때만 매치이며, 대소문자 구분(case folding 없음)이다. 따라서 대소문자만 다른 이름은 매치되지
// 않는다.
//
// 모호성 안전(RD-2, fail-closed): 이름은 유일하지 않을 수 있으므로 0/1/≥2 매치 세 경우를 모두
// 다룬다 — 0 매치는 not-found, ≥2 매치는 ErrAmbiguousName(무방출), 정확히 1개만 진행.
//
// 락 규율(REQ-01-05/NF-02): 리졸버는 로스터/그룹 레지스트리 락을 취득해 스냅샷을 뜬 뒤 해제하고
// 락 미보유 상태로 반환한다. 두 락을 절대 중첩하지 않으며(프로젝트 RWMutex 재진입 deadlock 트랩
// 회피), 반환 후 호출부(디스패치)가 락 미보유 상태에서 기존 경로를 호출한다.

// DeviceByName 은 Name 이 trim 후 정확 일치(대소문자 구분, RD-6)하는 유일한 디바이스의
// device_id 를 반환한다 (REQ-XSFM-NAMESEL-001-01-01/03/04/06).
//
// 0 매치 → ErrDeviceNotFound, ≥2 매치 → ErrAmbiguousName(무방출), 정확히 1개 → device_id.
// 로스터 RLock 하에 매치를 수집한 뒤 락을 해제하고 반환한다(스냅샷-안전, 락 미보유 반환).
//
// @MX:WARN: 로스터 RLock 하에서는 매치 수집만 하고 스냅샷을 뜬 뒤 락을 해제할 것 — 다른 락
// (그룹/station 레지스트리·pending)을 이 임계구역 안에서 취득하지 말 것.
// @MX:REASON: 두 락을 중첩하면 fan-out 동시 실행 시 프로젝트 RWMutex 재진입 deadlock 트랩에
// 걸린다(HVAC 에이전트 교훈, REQ-01-05/NF-02). 호출부는 락 미보유로 기존 경로를 호출한다.
func (a *XSFMAgent) DeviceByName(name string) (string, error) {
	target := strings.TrimSpace(name)

	a.mu.RLock()
	var matches []string
	for id, dev := range a.devices {
		if strings.TrimSpace(dev.Name) == target {
			matches = append(matches, id)
		}
	}
	a.mu.RUnlock()

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%w: name %q", ErrDeviceNotFound, name)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("%w: name %q matches %d devices", ErrAmbiguousName, name, len(matches))
	}
}

// GroupByName 은 전 타입 그룹(custom + 파생 station/line, RD-5)에서 표시명 Name 이 trim 후 정확
// 일치(대소문자 구분, RD-6)하는 유일한 그룹의 id 를 반환한다 (REQ-XSFM-NAMESEL-001-01-02/03/04/06).
//
// 0 매치 → ErrGroupNotFound, ≥2 매치 → ErrAmbiguousName(무방출), 정확히 1개 → 그룹 id. 파생
// 그룹 표시명과 커스텀 그룹명이 충돌하면 다중 매치가 되어 ErrAmbiguousName(RD-2)으로 안전
// 거부된다(타입 우선순위 없음). allGroups 는 각 레지스트리 락을 내부에서 스냅샷 후 해제하므로
// 이 함수는 어떤 락도 보유하지 않은 상태에서 매칭한다(락 중첩 없음).
func (a *XSFMAgent) GroupByName(name string) (string, error) {
	target := strings.TrimSpace(name)

	var matches []string
	for _, g := range a.allGroups() {
		if strings.TrimSpace(g.Name) == target {
			matches = append(matches, g.ID)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%w: name %q", ErrGroupNotFound, name)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("%w: name %q matches %d groups", ErrAmbiguousName, name, len(matches))
	}
}
