// 로그 사건 → i18n 키 매핑 (@SPEC:SPEC-REMOTE-LOG-001).
//
// 컴포넌트 파일에서 분리했다. 컴포넌트 파일이 컴포넌트만 내보내야 Fast Refresh 가
// 동작한다(react-refresh/only-export-components) — managerViewTab.ts 와 같은 이유다.

import type { RemoteLogEntry } from '@/types/remote';

/**
 * 필터 목록에 세우는 사건 종류.
 *
 * 서버가 사건 목록을 주지 않으므로 여기 적은 것이 곧 고를 수 있는 값이다. 서버에
 * 새 액션이 생기면 이 목록에도 더해야 한다 — 빠뜨리면 그 사건은 목록에 나타나되
 * 필터로는 고를 수 없다.
 */
export const REMOTE_LOG_ACTIONS = [
  'register',
  'approve',
  'reject',
  'revoke',
  'delete',
  'connect',
  'disconnect',
  'access',
  'command',
  'version_update',
] as const;

/**
 * 사건을 사람이 읽는 말로 옮긴다.
 *
 * 이미지 업데이트 지시는 별도 액션이 아니라 `command` + `system`/`update` 로 남는다
 * (같은 사건에 행을 둘 만들지 않기 위해서다 — storage 주석 참조). 그 조합만 따로
 * 집어 "이미지 업데이트" 로 읽어 준다. 그러지 않으면 목록에서 가장 중요한 사건이
 * 수많은 명령 줄에 섞여 보이지 않는다.
 */
export function logActionKey(entry: RemoteLogEntry): string {
  if (entry.action === 'command' && entry.domain === 'system' && entry.command_action === 'update') {
    return 'remote.log.action.imageUpdate';
  }
  return `remote.log.action.${entry.action}`;
}

