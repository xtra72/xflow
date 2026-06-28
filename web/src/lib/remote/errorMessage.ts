// 원격 작업 실패 시 사용자에게 보여줄 상세 메시지 추출 유틸.
//
// 백엔드는 노드-측 에러(예: "바이너리 다운로드: ...", "공개키 로드: ...")를
// command 응답 → API 에러 메시지로 보존해 전달한다. client.ts 응답 인터셉터는 이를
// APIError(.message) 로 변환한다. 일반 토스트가 이 메시지를 가리지 않도록 덧붙인다.

/**
 * 에러에서 사용자에게 보여줄 상세 문자열을 추출한다.
 * - APIError / Error: `.message`
 * - axios 평문 응답: `.response.data` (문자열)
 * 추출 실패 시 빈 문자열을 반환한다(호출부에서 일반 메시지에 안전하게 덧붙일 수 있도록).
 */
export function errorDetail(err: unknown): string {
  if (err && typeof err === 'object') {
    const e = err as { message?: string; response?: { data?: unknown } };
    const data = e.response?.data;
    if (typeof data === 'string' && data.trim() !== '') return `: ${data.trim()}`;
    if (typeof e.message === 'string' && e.message !== '') return `: ${e.message}`;
  }
  return '';
}
