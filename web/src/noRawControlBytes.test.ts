// 소스에 날 제어문자를 두지 않는다.
//
// `EditorPage.tsx` 가 노드 ID 를 이어 붙이는 구분자로 `'\x00'` 을 **소스 이스케이프가
// 아니라 날바이트로** 담고 있었다. 런타임은 멀쩡했다 — 같은 문자이므로. 조용히 깨진
// 것은 **도구** 쪽이다: grep 은 NUL 을 만나면 그 파일을 이진으로 판정하고 통째로
// 건너뛴다. 그래서 이 저장소의 grep 기반 조사·가드가 그 파일 하나를 못 본 채
// "없다"를 답해 왔고, 그 침묵 위에서 SPEC 의 재고 조사가 틀렸다.
//
// 타입 검사도 린트도 이것을 잡지 않는다. 눈으로도 안 보인다. 그래서 여기서 막는다.
//
// @spec SPEC-THEME-001 (M2 착수 중 발견)

import { describe, expect, it } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const SRC = dirname(fileURLToPath(import.meta.url));

/**
 * 소스에 날로 있어서는 안 되는 제어문자.
 *
 * 탭(0x09) · 개행(0x0A·0x0D)은 뺀다 — 그것들은 정상적인 소스 문자다. 남는 것은
 * 어느 것도 소스에 날로 들어갈 이유가 없고, 들어가면 도구가 파일을 이진으로 본다.
 */
// 제어문자를 찾는 것이 이 정규식의 일이므로 `no-control-regex` 는 여기서만 끈다.
// eslint-disable-next-line no-control-regex
const RAW_CONTROL = /[\u0000-\u0008\u000B\u000C\u000E-\u001F]/;

function sourceFiles(): readonly string[] {
  const out: string[] = [];
  const walk = (dir: string): void => {
    for (const entry of readdirSync(dir)) {
      const full = join(dir, entry);
      if (statSync(full).isDirectory()) {
        if (entry !== 'node_modules' && entry !== '__snapshots__') walk(full);
        continue;
      }
      if (/\.(tsx?|css|json)$/.test(entry)) out.push(full);
    }
  };
  walk(SRC);
  return out;
}

describe('소스에 날 제어문자가 없다', () => {
  it('grep 이 이진으로 판정할 파일이 하나도 없다', () => {
    const offenders: string[] = [];
    for (const path of sourceFiles()) {
      const text = readFileSync(path, 'utf8');
      const m = RAW_CONTROL.exec(text);
      if (m === null) continue;
      const line = text.slice(0, m.index).split('\n').length;
      const code = m[0]!.charCodeAt(0).toString(16).padStart(4, '0');
      offenders.push(`${relative(SRC, path)}:${line} U+${code.toUpperCase()}`);
    }
    // 구분자로 제어문자가 필요하면 소스 이스케이프(`'\0'`)로 적는다 — 런타임 문자는
    // 같고, 도구는 파일을 계속 읽을 수 있다.
    expect(offenders).toEqual([]);
  });
});
