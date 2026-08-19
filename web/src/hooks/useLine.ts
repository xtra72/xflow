// React Query hooks for xsfm line(라인) management (SPEC-XSFM-LINE-001 Module 6, M6).
//
// 라인(line)은 1급 엔티티로, 백엔드 라인 레지스트리가 SSOT 이다. `Line{Code, Name, Order}`
// 이며 group id 의 `line:<code>` 로 사용된다. 코드는 통일 포맷 `^[a-z0-9][a-z0-9_-]*$` 를
// 만족해야 한다(RD-6). 쓰기 명령(add/remove_line)은 표준 exec 계약
// `execAgent(id, { command, params })` 로, 읽기 전용인 list_lines 는
// `queryAgent(id, { command })` 로 전송한다(useStation.ts / useGroups.ts 패턴 미러).
// 인자는 params 아래에 중첩한다.
//
// 명령 API(백엔드 확정, M1~M5 완료):
//   - add_line    { code, name, order? } → { status, ... }  (upsert, code 포맷 검증)
//   - remove_line { code }               → { status } | ErrLineInUse(참조 역사 존재 시 거부, RD-5)
//   - list_lines  {}                     → { status, lines: [{ code, name, order }] } (Order 오름차순)

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import * as agentService from '@/services/api/agentService';

// ---- 타입 ----

/** 라인(line) 항목. code 는 고유 식별자, name 은 표시명, order 는 UI 정렬 키(오름차순). */
export interface Line {
  code: string;
  name: string;
  order: number;
}

// ---- 코드 포맷 검증(클라이언트 사이드 힌트) ----

/**
 * 통일 코드 포맷(RD-6). 소문자 영숫자로 시작, 이후 소문자 영숫자·언더스코어·하이픈 허용.
 * 백엔드가 최종 강제하지만, 폼에서 즉시 힌트를 주기 위해 프런트도 동일 정규식으로 사전 검증한다.
 */
export const LINE_CODE_PATTERN = /^[a-z0-9][a-z0-9_-]*$/;

/** 코드가 통일 포맷을 만족하는지 판별한다(빈 문자열은 false). */
export function isValidLineCode(code: string): boolean {
  return LINE_CODE_PATTERN.test(code);
}

/**
 * 백엔드 `ErrLineInUse`(참조 역사 존재로 remove_line 거부, RD-5) 여부를 판별한다.
 * 백엔드 에러 메시지: "xsfm: line is in use (referenced by one or more stations)".
 * 메시지 원문 매칭이 취약하지 않도록 안정적 부분 문자열("in use")로 판별한다.
 */
export function isLineInUseError(err: unknown): boolean {
  return err instanceof Error && err.message.includes('in use');
}

// ---- 쿼리 키 ----

const linesKey = (agentId: string) => ['xsfm-lines', agentId] as const;

// ---- 라인(line) 쿼리 ----

/**
 * 라인 목록 조회 (list_lines, Order 오름차순). 백엔드가 정렬해 반환하지만 방어적으로 정렬한다.
 *
 * @param refetchInterval - 지정 시 주기 폴링(ms). 미지정 시 폴링 없음(기존 동작).
 */
export function useLines(agentId: string, refetchInterval?: number) {
  return useQuery({
    queryKey: linesKey(agentId),
    queryFn: async () => {
      const res = await agentService.queryAgent(agentId, { command: 'list_lines' });
      const lines = (res as unknown as { lines?: Line[] }).lines ?? [];
      // Order 오름차순(동률은 code tiebreak)으로 방어적 정렬.
      return [...lines].sort((a, b) => a.order - b.order || a.code.localeCompare(b.code));
    },
    enabled: !!agentId,
    refetchInterval,
  });
}

// ---- 라인(line) 뮤테이션 ----

export interface AddLineVariables {
  code: string;
  name: string;
  /** UI 정렬 키. 생략 시 백엔드가 생성 순번을 부여한다. */
  order?: number;
}

/** 라인 추가/갱신 (add_line upsert). 코드 포맷은 백엔드가 최종 검증한다. */
export function useAddLine(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (v: AddLineVariables) =>
      agentService.execAgent(agentId, { command: 'add_line', params: { ...v } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: linesKey(agentId) }),
  });
}

/**
 * 라인 삭제 (remove_line). 참조 역사가 존재하면 백엔드가 `ErrLineInUse` 로 거부한다(RD-5).
 * 호출부에서 isLineInUseError 로 판별해 친화적 메시지를 노출한다.
 */
export function useRemoveLine(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (code: string) =>
      agentService.execAgent(agentId, { command: 'remove_line', params: { code } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: linesKey(agentId) }),
  });
}
