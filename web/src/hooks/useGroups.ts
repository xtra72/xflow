// React Query hooks for xsfm group management (SPEC-XSFM-GROUP-001 Module 6, M6).
//
// 그룹(group)은 1급 엔티티로, 백엔드 그룹 레지스트리가 SSOT 이다. id 는 타입 접두사 인코딩
// (`custom:<name|uuid>` / `station:<code>` / `line:<code>`)을 사용하며, type 접두사로 편집 가능
// 여부(custom 만 편집)를 판별한다. 모든 명령은 표준 exec 계약 `execAgent(id, { command, params })`
// 로 전송하며(useStation.ts 의 add_station 패턴 미러), 인자는 params 아래에 중첩한다.
//
// 명령 API(백엔드 확정):
//   - add_group    { name, members? }            → { status, group_id }  (type=custom 생성)
//   - remove_group { group_id }                  → { status, group_id }
//   - set_group    { group_id, name?, members? } → { status, group_id }  (부분 갱신: 생략 필드 보존)
//   - list_groups  {}                            → { status, groups:[{ id, name, type, member_count, members }] }
//
// 기본 그룹(type=station|line)은 파생·읽기 전용이며 add/set/remove 로 편집할 수 없다(백엔드가 거부).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import * as agentService from '@/services/api/agentService';

// ---- 타입 ----

/** 그룹 타입. 접두사 인코딩된 id 에서 파생 판별 가능하다. custom 만 편집 가능. */
export type GroupType = 'custom' | 'station' | 'line';

/** 그룹(group) 항목 (list_groups 응답). members 는 device_id 목록, member_count 는 그 길이. */
export interface Group {
  id: string;
  name: string;
  type: GroupType;
  member_count: number;
  members: string[];
}

/** 그룹이 커스텀(편집 가능)인지 판별한다. 기본 그룹(station/line)은 읽기 전용. */
export function isCustomGroup(group: Pick<Group, 'type'>): boolean {
  return group.type === 'custom';
}

// ---- 쿼리 키 ----

const groupsKey = (agentId: string) => ['xsfm-groups', agentId] as const;

// ---- 그룹(group) 쿼리 ----

/**
 * 그룹 목록 조회 (list_groups). 기본(station/line) + 커스텀 그룹을 결정적 순서로 반환한다.
 *
 * @param refetchInterval - 지정 시 주기 폴링(ms). 대시보드 패널이 로스터를 주기 갱신할 때 사용.
 *   미지정 시 폴링 없음(기존 동작).
 */
export function useGroups(agentId: string, refetchInterval?: number) {
  return useQuery({
    queryKey: groupsKey(agentId),
    queryFn: async () => {
      const res = await agentService.execAgent(agentId, { command: 'list_groups' });
      const groups = (res as unknown as { groups?: Group[] }).groups ?? [];
      // members 누락 방어(빈 그룹) + member_count 정합(백엔드 미제공 시 members 길이로 폴백).
      return groups.map((g) => ({
        ...g,
        members: g.members ?? [],
        member_count: g.member_count ?? (g.members?.length ?? 0),
      }));
    },
    enabled: !!agentId,
    refetchInterval,
  });
}

// ---- 그룹(group) 뮤테이션 ----

/** add_group 응답 형태(execAgent 는 엔벨로프를 벗겨 Process 결과를 그대로 반환). */
export interface AddGroupResult {
  status: string;
  group_id: string;
}

export interface AddGroupVariables {
  name: string;
  members?: string[];
}

/** 커스텀 그룹 생성 (add_group, type=custom). 생성된 group_id 를 성공 토스트에 노출하려 결과 반환. */
export function useAddGroup(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (v: AddGroupVariables): Promise<AddGroupResult> => {
      const res = await agentService.execAgent(agentId, { command: 'add_group', params: { ...v } });
      return res as unknown as AddGroupResult;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: groupsKey(agentId) }),
  });
}

export interface SetGroupVariables {
  group_id: string;
  /** 생략 시 이름 보존(부분 갱신). */
  name?: string;
  /** 생략 시 멤버 보존(부분 갱신). 빈 배열을 보내면 멤버 전체 비움. */
  members?: string[];
}

/**
 * 커스텀 그룹 부분 갱신 (set_group). group_id 로 대상을 지정하고, 제공된 필드만 갱신된다
 * (생략 필드는 백엔드가 보존). 기본 그룹(station/line)에 대해서는 백엔드가 ErrGroupNotCustom 을 반환한다.
 */
export function useSetGroup(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (v: SetGroupVariables) =>
      agentService.execAgent(agentId, { command: 'set_group', params: { ...v } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: groupsKey(agentId) }),
  });
}

/** 커스텀 그룹 삭제 (remove_group). 기본 그룹은 백엔드가 ErrGroupNotCustom 으로 거부한다. */
export function useRemoveGroup(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (groupId: string) =>
      agentService.execAgent(agentId, { command: 'remove_group', params: { group_id: groupId } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: groupsKey(agentId) }),
  });
}
