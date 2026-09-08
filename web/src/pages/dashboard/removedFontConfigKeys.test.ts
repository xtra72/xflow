// 걷어낸 글자 모양 config 키가 되살아나지 않게 못을 박는다.
//
// 타이틀·요약 배지·표 머리글·표 본문의 글자 모양을 정하던 설정 넷을 지웠다. 설정 UI 만
// 지우고 **읽는 쪽**을 남기면 반쪽짜리가 된다: 저장된 대시보드는 화면에서만 옛 모양을
// 유지하는데 그것을 되돌릴 자리가 없다. 반대로 읽는 쪽만 지우고 쓰는 쪽이 남으면 아무
// 일도 하지 않는 설정이 화면에 남는다.
//
// 타입 검사도 린트도 이것을 잡지 못한다 — 패널 config 는 `Record<string, unknown>`
// (불투명 JSON)이라 없는 키를 읽어도 컴파일이 통과한다. 그래서 "소스에 이 키 이름이
// 아예 없다"를 원문 검사로 확인한다. 삭제를 확인하는 정직한 방법은 원문 검사뿐이다.

import { describe, expect, it } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

/** 이 파일은 `src/pages/dashboard/` 에 있으므로 두 단계 올라가면 `src` 다. */
const SRC = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

/** 지운 config 키들. 이름 그대로 소스 어디에도 남아 있으면 안 된다. */
const REMOVED_KEYS = ['title_font', 'badge_font', 'table_header_font', 'table_cell_font'] as const;

/**
 * 프로덕션 소스에서 키 이름을 찾는다.
 *
 * 테스트 파일은 뺀다 — 이 파일 자신이 키 이름을 문자열로 들고 있고, 회귀 테스트도
 * "저장된 옛 값을 읽지 않는다" 를 확인하려면 그 이름을 적어야 하기 때문이다.
 */
function offendingFiles(key: string): string[] {
  const hits: string[] = [];

  const walk = (dir: string): void => {
    for (const entry of readdirSync(dir)) {
      const full = join(dir, entry);
      if (statSync(full).isDirectory()) {
        if (entry !== 'node_modules' && entry !== '__mocks__') walk(full);
        continue;
      }
      if (!/\.tsx?$/.test(entry) || entry.includes('.test.')) continue;
      if (readFileSync(full, 'utf8').includes(key)) hits.push(relative(SRC, full));
    }
  };

  walk(SRC);
  return hits.sort();
}

describe('걷어낸 글자 모양 config 키', () => {
  it.each(REMOVED_KEYS)('%s 을(를) 읽거나 쓰는 코드가 없다', (key) => {
    // 실패 메시지에 파일을 담는다 — 키 이름만으로는 찾아 헤매게 된다.
    expect(offendingFiles(key)).toEqual([]);
  });

  it('검사 자체가 살아 있다 — 실제로 쓰이는 키는 잡아낸다', () => {
    // 위 검사가 언제나 빈 배열이면 통과 방식이 망가져도 알 수 없다. 남아 있는 형제 키
    // (`value_font`: 속성 그리드 카드에서 지금도 쓴다)로 탐지기가 도는지 확인한다.
    expect(offendingFiles('value_font').length).toBeGreaterThan(0);
  });
});
