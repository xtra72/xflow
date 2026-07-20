package dto

// 원격 관리 "클라이언트" 설정 DTO (SPEC-REMOTE-001 원격 관리 클라이언트 설정 UI).
//
// 이 인스턴스(피관리 노드)가 관리 서버에 등록될 때 쓰는 client 모드 설정을 UI 에서
// 조회/편집하기 위한 응답/요청 페이로드이다. 값은 config 오버라이드 레이어에 영속화되며
// (원본 config 파일 보존), mutable 키는 즉시, 비-mutable 키는 재시작 후 적용된다.
//
// 시크릿(enrollment_token / bootstrap_secret)은 GET 응답에서 값을 노출하지 않고
// "설정됨" 여부(bool)만 반환한다. PUT 에서는 nil(미전송)이면 기존값 유지, 빈 문자열이면
// 해제, 값이 있으면 갱신한다.

// RemoteClientExposureDTO 는 서버에 노출할 자원 범위이다("all" | "none" | 명시 목록).
type RemoteClientExposureDTO struct {
	Flows   string `json:"flows"`
	Agents  string `json:"agents"`
	Devices string `json:"devices"`
}

// RemoteClientConfigResponse 는 GET /system/remote-config 응답 페이로드이다.
// 시크릿은 값 대신 *Set(bool) 로만 노출한다.
type RemoteClientConfigResponse struct {
	Mode               string                  `json:"mode"` // disabled | client | server
	ServerURL          string                  `json:"server_url"`
	InstanceID         string                  `json:"instance_id"`
	AutoRegister       bool                    `json:"auto_register"`
	HeartbeatInterval  string                  `json:"heartbeat_interval"` // Go duration 문자열 (예: "30s")
	EnrollmentTokenSet bool                    `json:"enrollment_token_set"`
	BootstrapSecretSet bool                    `json:"bootstrap_secret_set"`
	Exposure           RemoteClientExposureDTO `json:"exposure"`
	RequireSecure      bool                    `json:"require_secure"`
	InsecureSkipVerify bool                    `json:"insecure_skip_verify"`
	DisplayWidth       int                     `json:"display_width"`
	DisplayHeight      int                     `json:"display_height"`

	// RestartRequiredFields 는 편집 시 재시작이 필요한(비-mutable) 필드 목록으로,
	// UI 가 "재시작 후 적용" 배지를 표시하는 데 쓴다.
	RestartRequiredFields []string `json:"restart_required_fields"`
}

// RemoteClientConfigUpdate 는 PUT /system/remote-config 요청 본문이다.
// 모든 필드는 포인터로, nil(미전송)이면 해당 키를 변경하지 않는다(부분 업데이트).
type RemoteClientConfigUpdate struct {
	Mode               *string `json:"mode"`
	ServerURL          *string `json:"server_url"`
	InstanceID         *string `json:"instance_id"`
	AutoRegister       *bool   `json:"auto_register"`
	HeartbeatInterval  *string `json:"heartbeat_interval"`
	EnrollmentToken    *string `json:"enrollment_token"` // nil=유지, ""=해제, 그 외=갱신
	BootstrapSecret    *string `json:"bootstrap_secret"`
	ExposureFlows      *string `json:"exposure_flows"`
	ExposureAgents     *string `json:"exposure_agents"`
	ExposureDevices    *string `json:"exposure_devices"`
	RequireSecure      *bool   `json:"require_secure"`
	InsecureSkipVerify *bool   `json:"insecure_skip_verify"`
	DisplayWidth       *int    `json:"display_width"`
	DisplayHeight      *int    `json:"display_height"`
}

// RemoteClientConfigUpdateResult 는 PUT 응답으로, 적용된 키와 재시작이 필요한 키를 알린다.
type RemoteClientConfigUpdateResult struct {
	Applied      []string `json:"applied"`       // 오버라이드에 영속화된 config 키 목록
	NeedsRestart []string `json:"needs_restart"` // 완전 적용에 재시작이 필요한 키(비-mutable)
}
