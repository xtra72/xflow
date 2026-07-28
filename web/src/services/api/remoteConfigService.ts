// 원격 관리 "클라이언트" 설정 API 서비스 (SPEC-REMOTE-001 원격 관리 클라이언트 설정 UI).
//
// 이 인스턴스(피관리 노드)가 관리 서버에 등록될 때 쓰는 client 모드 설정을 조회/편집한다.
// 값은 서버의 config 오버라이드 레이어에 영속화되며(원본 config 파일 보존), mutable 키는
// 즉시, 비-mutable 키는 재시작 후 적용된다. admin 전용 엔드포인트다.
//
//   GET /api/v1/system/remote-config
//   PUT /api/v1/system/remote-config

import { get, put } from './client';

/** 서버에 노출할 자원 범위 ("all" | "none" | 명시 목록). */
export interface RemoteClientExposure {
  flows: string;
  agents: string;
  devices: string;
}

/** GET 응답. 시크릿은 값 대신 *_set(bool) 로만 노출된다. */
export interface RemoteClientConfig {
  mode: string; // disabled | client | server
  server_url: string;
  instance_id: string;
  auto_register: boolean;
  heartbeat_interval: string; // Go duration 문자열 (예: "30s")
  enrollment_token_set: boolean;
  bootstrap_secret_set: boolean;
  exposure: RemoteClientExposure;
  require_secure: boolean;
  insecure_skip_verify: boolean;
  display_width: number;
  display_height: number;
  /** 편집 시 재시작이 필요한(비-mutable) config 키 목록. */
  restart_required_fields: string[];
}

/** PUT 요청. 모든 필드는 선택(부분 업데이트) — 미전송 필드는 변경되지 않는다.
 * 시크릿(enrollment_token/bootstrap_secret): 미전송=유지, ""=해제, 값=갱신. */
export interface RemoteClientConfigUpdate {
  mode?: string;
  server_url?: string;
  instance_id?: string;
  auto_register?: boolean;
  heartbeat_interval?: string;
  enrollment_token?: string;
  bootstrap_secret?: string;
  exposure_flows?: string;
  exposure_agents?: string;
  exposure_devices?: string;
  require_secure?: boolean;
  insecure_skip_verify?: boolean;
  display_width?: number;
  display_height?: number;
}

/** PUT 응답: 적용된 키와 재시작이 필요한 키. */
export interface RemoteClientConfigUpdateResult {
  applied: string[];
  needs_restart: string[];
}

/** 현재 원격 클라이언트 설정을 조회한다(admin 전용). */
export async function getRemoteClientConfig(): Promise<RemoteClientConfig> {
  return get<RemoteClientConfig>('/system/remote-config');
}

/** 원격 클라이언트 설정을 부분 업데이트한다(admin 전용). */
export async function updateRemoteClientConfig(
  req: RemoteClientConfigUpdate,
): Promise<RemoteClientConfigUpdateResult> {
  return put<RemoteClientConfigUpdateResult>('/system/remote-config', req);
}
