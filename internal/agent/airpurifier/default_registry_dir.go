package airpurifier

import "sync"

// ---------------------------------------------------------------------------
// 기본 영속 경로 디렉터리 (registry_path/station_registry_path 자동 기본값)
// ---------------------------------------------------------------------------
//
// 배경: 과거에는 registry_path/station_registry_path 가 빈 값이면 영속화가 완전히
// 비활성(인메모리 전용)이라, 설정을 비워둔 사용자는 재시작 시 역사/위치/기기 등록이
// 모두 사라졌다("저장 안됨"). 이를 해결하기 위해 서버 데이터 디렉터리를 기준으로 한
// 기본 경로를 도입한다: 설정이 비어 있어도 <dataDir>/airpurifier/<agentID>/ 아래에
// station_registry.json / device_registry.json 이 기본 영속된다.
//
// 배선은 audit.go 의 SetAuditRepository / getAuditRepository 싱글턴 패턴을 그대로
// 미러한다: main.go 가 startup 시 SetDefaultRegistryDir 로 단 한 번 주입하며, 미설정
// (빈 문자열) 상태에서는 종전 동작(빈 설정 → 인메모리)을 그대로 유지한다. 단위 테스트는
// 이 setter 를 호출하지 않으므로 파일을 쓰지 않는다(하위호환).
var (
	defaultRegistryDirMu sync.RWMutex
	defaultRegistryDir   string
)

// SetDefaultRegistryDir 는 패키지-레벨 기본 영속 디렉터리를 설정한다(audit 저장소 패턴).
// 일반적으로 main.go 의 startup 코드에서 단 한 번 호출한다. 빈 문자열이면 기본 경로
// 비활성(종전 동작: 빈 설정 → 인메모리).
func SetDefaultRegistryDir(dir string) {
	defaultRegistryDirMu.Lock()
	defer defaultRegistryDirMu.Unlock()
	defaultRegistryDir = dir
}

// getDefaultRegistryDir 는 현재 설정된 기본 영속 디렉터리를 반환한다(없으면 "").
// 호출자는 "" 인 경우 기본 경로를 유도하지 않고 종전 동작(설정 경로 or 인메모리)을 따른다.
func getDefaultRegistryDir() string {
	defaultRegistryDirMu.RLock()
	defer defaultRegistryDirMu.RUnlock()
	return defaultRegistryDir
}
