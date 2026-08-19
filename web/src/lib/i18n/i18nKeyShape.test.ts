import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

import en from './en.json';
import ko from './ko.json';

/**
 * i18n 키 형상 가드.
 *
 * `t()` 에 네임스페이스 객체(하위 키를 가진 노드)를 넘기면 번역이 잡히지 않고
 * 키 문자열이 그대로 화면에 노출된다. 타입 검사도 테스트도 이를 잡지 못한다 —
 * 호출은 유효하고 렌더도 성공하며, 다만 사용자에게 `dashboard.addPanel` 같은
 * 원문 키가 보일 뿐이다. 실제로 그 형태의 결함이 배포된 적이 있어 가드를 둔다.
 */

const SRC_ROOT = path.resolve(__dirname, '../..');

/** 소스 트리의 .ts / .tsx 파일을 모은다. */
function collectSourceFiles(dir: string, out: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      collectSourceFiles(full, out);
    } else if (/\.tsx?$/.test(entry.name)) {
      out.push(full);
    }
  }
  return out;
}

/** 점 표기법 키를 번역 트리에서 조회한다. */
function resolveKey(tree: unknown, key: string): unknown {
  return key
    .split('.')
    .reduce<unknown>(
      (node, seg) =>
        node && typeof node === 'object' ? (node as Record<string, unknown>)[seg] : undefined,
      tree,
    );
}

// `t('...')` 호출만 잡는다. 앞자리 경계가 없으면 `new Event('error')` 의 꼬리가
// 함께 매치되어 오검출이 난다.
const T_CALL = /(?<![A-Za-z0-9_$])t\('([A-Za-z0-9_.]+)'\)/g;

describe('i18n 키 형상', () => {
  it('t() 호출이 네임스페이스 객체를 가리키지 않는다', () => {
    const offenders: string[] = [];

    for (const file of collectSourceFiles(SRC_ROOT)) {
      const source = fs.readFileSync(file, 'utf8');
      for (const match of source.matchAll(T_CALL)) {
        const key = match[1];
        if (key === undefined) continue;
        const value = resolveKey(ko, key);
        if (value !== undefined && typeof value === 'object') {
          offenders.push(`${path.relative(SRC_ROOT, file)}: t('${key}')`);
        }
      }
    }

    expect(offenders).toEqual([]);
  });

  it('ko 와 en 의 키 집합이 일치한다', () => {
    const flatten = (tree: Record<string, unknown>, prefix = ''): string[] =>
      Object.entries(tree).flatMap(([k, v]) =>
        v && typeof v === 'object'
          ? flatten(v as Record<string, unknown>, `${prefix}${k}.`)
          : [`${prefix}${k}`],
      );

    const koKeys = flatten(ko as Record<string, unknown>);
    const enKeys = flatten(en as Record<string, unknown>);

    expect(koKeys.filter((k) => !enKeys.includes(k))).toEqual([]);
    expect(enKeys.filter((k) => !koKeys.includes(k))).toEqual([]);
  });
});
