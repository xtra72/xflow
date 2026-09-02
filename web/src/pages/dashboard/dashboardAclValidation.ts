// 대시보드 ACL 항목의 클라이언트측 유효성 검증 (SPEC-DASHBOARD-004 M7 7.2).
//
// 서버(internal/api/handler/dashboard_acl.go `validateACLEntries`)가 판정의
// 최종 권위다. 여기서 같은 규칙을 다시 두는 이유는 **거부 사유를 사용자 언어로
// 구분해 보여주기 위해서**다 — 서버 400 은 한국어 고정 메시지 하나로 오고,
// adminErrors.ts 가 명시하듯 메시지 문자열로 분기하는 것은 계약이 아니다.
//
// 따라서 역할 분담은 다음과 같다.
//   - 이 모듈: 저장 전에 거부 사유를 사유별로 구분해 안내한다(로케일 대응).
//   - 서버:    최종 거부. 이 모듈이 알 수 없는 사실(목록을 읽을 권한이 없어
//              사용자·역할 실재를 확인하지 못하는 경우)은 서버가 400 으로 막고,
//              호출부가 서버 메시지를 그대로 노출한다.
//
// 서버와 동일하게 **첫 위반에서 멈춘다** — ACL 은 전량 치환이므로 하나라도
// 유효하지 않으면 전체가 거부되고, 부분 저장은 존재하지 않는다(spec.md §2.9, §4.5).
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.9 E3, §2.13 UB1 #5, acceptance.md AC-17)

import type { DashboardAclEntry } from '@/types/dashboard';

/** subject 접두사 — 서버 `dashboardacl.SubjectPrefix{User,Role}` 과 동일하다. */
export const SUBJECT_PREFIX_USER = 'user:';
export const SUBJECT_PREFIX_ROLE = 'role:';

/** 허용되는 권한 레벨. `edit` > `view` (spec.md §2.2). */
const VALID_LEVELS: ReadonlySet<string> = new Set(['view', 'edit']);

/** 검증 실패 1건. 서버와 같이 첫 위반만 보고한다. */
export interface AclValidationIssue {
  /** 위반한 행의 인덱스. 행에 귀속되지 않으면 -1. */
  index: number;
  /** 안내 문구의 i18n 키 (`dashboard.acl.error.*`). 사유마다 서로 다르다. */
  messageKey: string;
  /** 문구의 `{value}` 자리에 들어갈 값. 값이 없으면 빈 문자열. */
  value: string;
}

/** 검증에 필요한 문맥. */
export interface AclValidationContext {
  /** 대시보드 소유자 username. 빈 문자열이면 소유자 검사를 건너뛴다(인증 비활성). */
  owner: string;
  /**
   * 실재하는 사용자 이름. `undefined` 면 목록을 읽지 못한 것이므로 실재 검사를
   * 건너뛴다 — 확인할 수 없는 사실을 근거로 사용자의 저장을 막지 않는다.
   */
  knownUsers?: readonly string[];
  /** 실재하는 역할 이름. `undefined` 의 의미는 `knownUsers` 와 같다. */
  knownRoles?: readonly string[];
}

function issue(index: number, messageKey: string, value = ''): AclValidationIssue {
  return { index, messageKey, value };
}

/**
 * ACL 항목 목록을 검증한다. 문제가 없으면 `null`.
 *
 * 사유 구분은 acceptance.md AC-17 의 9행과 1:1 이다.
 *   접두사 없음 / 미지원 접두사 / 빈 이름 / 미지원 레벨 / 중복 / 소유자 /
 *   존재하지 않는 사용자 / 존재하지 않는 역할
 */
export function validateAclEntries(
  entries: readonly DashboardAclEntry[],
  ctx: AclValidationContext,
): AclValidationIssue | null {
  const seen = new Set<string>();

  for (let i = 0; i < entries.length; i += 1) {
    const { subject, level } = entries[i]!;

    if (!VALID_LEVELS.has(level)) {
      return issue(i, 'dashboard.acl.error.levelInvalid', level);
    }
    if (seen.has(subject)) {
      return issue(i, 'dashboard.acl.error.duplicate', subject);
    }
    seen.add(subject);

    if (subject.startsWith(SUBJECT_PREFIX_USER)) {
      const username = subject.slice(SUBJECT_PREFIX_USER.length);
      if (!username) return issue(i, 'dashboard.acl.error.nameEmpty');
      // 소유권은 ACL 보다 항상 우선하므로 소유자 등재는 무의미하고,
      // "나를 뺐다" 는 오해를 만든다(spec.md §2.13 UB1 #5).
      if (ctx.owner && username === ctx.owner) {
        return issue(i, 'dashboard.acl.error.owner', username);
      }
      if (ctx.knownUsers && !ctx.knownUsers.includes(username)) {
        return issue(i, 'dashboard.acl.error.userNotFound', username);
      }
      continue;
    }

    if (subject.startsWith(SUBJECT_PREFIX_ROLE)) {
      const role = subject.slice(SUBJECT_PREFIX_ROLE.length);
      if (!role) return issue(i, 'dashboard.acl.error.nameEmpty');
      if (ctx.knownRoles && !ctx.knownRoles.includes(role)) {
        return issue(i, 'dashboard.acl.error.roleNotFound', role);
      }
      continue;
    }

    // 접두사가 아예 없는 경우와 지원하지 않는 접두사를 구분해 안내한다.
    // 전자는 형식을 모르는 것이고 후자는 대상 축을 오해한 것이라, 다음 행동이 다르다.
    return subject.includes(':')
      ? issue(i, 'dashboard.acl.error.prefixUnsupported', subject)
      : issue(i, 'dashboard.acl.error.prefixMissing', subject);
  }

  return null;
}
