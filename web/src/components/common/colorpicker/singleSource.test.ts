// 색을 고르는 자리가 정말로 하나인지 못으로 박는다.
//
// 통합을 끝낸 다음 날 누군가 `<input type="color">` 를 한 줄 더 놓아도 타입 검사도
// 린트도 잡지 못한다. 화면은 멀쩡히 뜨고, 그 자리만 브라우저 기본 대화상자로 열릴
// 뿐이다 — 갈라짐은 언제나 그렇게 조용히 돌아온다. 여섯 벌이던 프리셋 사본도 같은
// 방식으로 늘어났다.
//
// 그래서 사람이 손으로 돌리던 grep 을 시험으로 옮긴다. 아래 네 가지가 못이다.
//
//   1. 네이티브 색 입력이 없다
//   2. 철거한 옛 부품의 이름이 어디에도 없다
//   3. 프리셋 배열 사본이 되살아나지 않았다
//   4. 색 뒤에 알파 두 자리를 손으로 이어붙이는 자리가 없다
//
// @spec SPEC-COLOR-001 §결정 1 · §결정 3 · §결정 5 (불변식 I3 · I4 · I6)

import { describe, expect, it } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

/** `web/src`. 이 파일에서 네 단계 위다. */
const SRC = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..');

/** 시험 파일을 뺀 모든 소스. 시험은 옛 이름을 이야기할 수 있어야 한다. */
function sources(): readonly { path: string; text: string }[] {
  const out: { path: string; text: string }[] = [];
  const walk = (dir: string): void => {
    for (const entry of readdirSync(dir)) {
      const full = join(dir, entry);
      if (statSync(full).isDirectory()) {
        if (entry !== 'node_modules' && entry !== '__mocks__') walk(full);
        continue;
      }
      if (!/\.tsx?$/.test(entry) || entry.includes('.test.')) continue;
      out.push({ path: relative(SRC, full), text: readFileSync(full, 'utf8') });
    }
  };
  walk(SRC);
  return out;
}

/** 주석 줄을 뺀 일치 — 옛 이름을 설명하는 주석까지 잡으면 문서를 못 쓴다. */
function hits(re: RegExp): readonly string[] {
  const found: string[] = [];
  for (const { path, text } of sources()) {
    text.split('\n').forEach((line, i) => {
      if (/^\s*(\/\/|\*|\/\*)/.test(line)) return;
      if (re.test(line)) found.push(`${path}:${i + 1}: ${line.trim()}`);
    });
  }
  return found;
}

describe('색 고르개는 하나뿐이다 (I4)', () => {
  it('네이티브 색 입력이 남아 있지 않다', () => {
    expect(hits(/type="color"/)).toEqual([]);
  });

  it('철거한 옛 부품의 이름이 어디에도 없다', () => {
    expect(hits(/ColorSwatchButton|PanelColorFreeInput|PanelSettingsDropdown/)).toEqual([]);
  });

  it('색을 고르는 자리는 전부 공용 부품을 쓴다', () => {
    // 옮긴 자리는 39다(옛 네이티브 22 + 옛 스와치 17). 한 컴포넌트가 여러 자리를
    // 대신하는 곳이 있어 사용 건수는 그보다 적을 수 있으나, 서른은 넘어야 한다 —
    // 이 수가 뚝 떨어졌다면 어딘가가 공용 부품 밖으로 빠져나간 것이다.
    expect(hits(/<ColorPicker/).length).toBeGreaterThanOrEqual(30);
  });
});

describe('프리셋은 한 벌뿐이다 (I6)', () => {
  it('갈라졌던 프리셋 배열 사본이 되살아나지 않았다', () => {
    // `HEATMAP_COLOR_PRESETS` 는 스와치 목록이 아니라 gradient 정지점 묶음이라
    // 통합 대상이 아니다(spec.md §범위).
    const copies = hits(
      /\b(PANEL_COLORS|COLOR_PRESETS|SUB_COLOR_PRESETS|LOG_COLOR_PRESETS|COLOR_PALETTE)\b/,
    ).filter((line) => !/HEATMAP_COLOR_PRESETS/.test(line));
    expect(copies).toEqual([]);
  });

  it('자리마다 프리셋을 따로 넘기지 않는다', () => {
    // `presets` 속성은 부품에 남아 있지만 쓰는 자리는 0 이어야 한다 — 하나라도
    // 쓰이는 순간 팔레트가 다시 갈라지기 시작한다.
    expect(hits(/\bpresets=/)).toEqual([]);
  });
});

describe('알파 두 자리를 손으로 잇지 않는다 (I3)', () => {
  // 한 벌로는 놓친다 — `${색}${TILE_TINT_ALPHA}` 꼴이 첫 정규식에 걸리지 않는다.
  // 실측으로 확인한 사실이라 두 벌을 나란히 둔다.
  it('`${색}NN` 꼴이 없다', () => {
    expect(hits(/\$\{[^}]*\}[0-9a-fA-F]{2}`/)).toEqual([]);
  });

  it('`${색}${…ALPHA}` 꼴이 없다', () => {
    expect(hits(/\$\{[^}]*\}\$\{[A-Z_]*(ALPHA|TINT|OPACITY)[A-Z_]*\}/)).toEqual([]);
  });
});
