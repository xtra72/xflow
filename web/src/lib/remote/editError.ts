// 원격 자원 편집 실패 → 사용자 메시지 매핑 (SPEC-REMOTE-001 M7, REQ-I11).
//
// 백엔드 상태 코드 의미(internal/api/handler/remote_editing.go + remote_admin.go
// mapRemoteCommandError, requireExposed):
//   503 → 대상 노드 오프라인/미관리 (적용 불가)
//   504 → 명령 타임아웃 (미적용)
//   502 → 노드 어댑터 적용 실패 (검증/충돌 — 노드 오류 사유 전달)
//   404 → 노출 범위 밖 또는 미존재 자원
//
// 어떤 실패에서도 서버 미러 캐시는 갱신되지 않는다(REQ-E08 — 성공 시에만 갱신).

/** APIError 등에서 HTTP status 를 안전하게 추출한다. */
export function extractStatus(err: unknown): number | undefined {
  if (err && typeof err === 'object' && 'status' in err) {
    const s = (err as { status: unknown }).status;
    if (typeof s === 'number') return s;
  }
  return undefined;
}

/** APIError 등에서 서버 메시지(노드 오류 사유 등)를 안전하게 추출한다. */
export function extractMessage(err: unknown): string | undefined {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message: unknown }).message;
    if (typeof m === 'string' && m.trim() !== '') return m;
  }
  return undefined;
}

/**
 * 원격 편집 에러를 i18n 키 기반 사용자 메시지로 변환한다(REQ-I11).
 *
 * 502(노드 적용 실패)는 노드가 전달한 오류 사유를 함께 보여줄 수 있도록,
 * 서버 메시지를 괄호로 덧붙인다(REQ-I10 — 오류 사유 표시).
 */
export function remoteEditErrorMessage(
  err: unknown,
  t: (k: string) => string,
): string {
  const status = extractStatus(err);
  switch (status) {
    case 503:
      return t('remote.edit.errorUnavailable');
    case 504:
      return t('remote.edit.errorTimeout');
    case 502: {
      const base = t('remote.edit.errorFailed');
      const reason = extractMessage(err);
      return reason ? `${base} (${reason})` : base;
    }
    case 404:
      return t('remote.edit.errorNotExposed');
    default:
      return t('remote.edit.errorGeneric');
  }
}
