// 전역(앱 전체 공유) key-value 설정 타입.
//
// 백엔드 internal/api/handler/settings.go 의 SettingsResponse 와 매핑된다.
// value 는 서버가 스키마를 강제하지 않는 불투명 JSON 으로, 키별로 프론트엔드가
// 형태를 정한다(예: device-list-columns → DeviceColumnsSetting).

/**
 * 전역 설정 조회/저장 응답.
 * GET/PUT /settings/{key} 가 반환하는 { key, value } 형태.
 * value 는 저장한 JSON 그대로(불투명).
 */
export interface SettingResponse<T = unknown> {
  key: string;
  value: T;
}

/**
 * 디바이스 목록 컬럼 구성 설정값.
 * 키 `device-list-columns` 의 value 형태.
 * columns 는 표시할 컬럼 키 배열(순서는 무시, 렌더는 정의 순서 사용).
 */
export interface DeviceColumnsSetting {
  columns: string[];
}
