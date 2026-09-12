// 그룹 안 문구 렌더링 시험 (SPEC-CANVAS-004, defect B).
//
// 그룹 안에 있는 텍스트 요소는 복합 키(그룹id/부품id)로 찾아야 하는데, walkDrawables
// 가 복합 키를 낸다. 만약 texts 맵이 평평한 요소 id 로 만들어진다면 lookup 이 실패해
// 텍스트가 렌더되지 않는다 (drawElement.ts:420 에서 texts[key] ?? element.text).

import { describe, expect, it } from 'vitest';

import type { TextElement } from './canvasConfig';
import type { GroupElement } from './group/groupTypes';
import { frameKey, walkDrawables } from './group/frameKey';

describe('그룹 안 텍스트 키 매칭 (defect B)', () => {
  it('walkDrawables 는 최상위 요소에 평평한 키를 낸다', () => {
    // 최상위 텍스트 요소
    const txt: TextElement = {
      id: 'txt-1',
      kind: 'text',
      geometry: { x: 0, y: 0 },
      text: 'Hello',
      style: { fill: '#000' },
    };

    const drawables = Array.from(walkDrawables([txt]));
    expect(drawables).toHaveLength(1);
    expect(drawables[0]?.key).toBe('txt-1');
    expect(drawables[0]?.element.id).toBe('txt-1');
  });

  it('walkDrawables 는 그룹 부품에 복합 키를 낸다', () => {
    // 그룹 안에 텍스트 부품
    const grp: GroupElement = {
      id: 'grp-1',
      kind: 'group',
      geometry: { x: 0, y: 0, w: 200, h: 200 },
      parts: [
        {
          id: 'label',
          kind: 'text',
          geometry: { x: 10, y: 10 },
          text: 'Group Label',
          style: { fill: '#000' },
        },
      ],
    };

    const drawables = Array.from(walkDrawables([grp]));
    expect(drawables).toHaveLength(1);
    // walkDrawables 는 복합 키를 낸다
    expect(drawables[0]?.key).toBe(frameKey('grp-1', 'label'));
    expect(drawables[0]?.element.kind).toBe('text');
    expect(drawables[0]?.group?.id).toBe('grp-1');
  });

  it('texts 맵에서 복합 키로 조회할 수 있다', () => {
    const grp: GroupElement = {
      id: 'grp-1',
      kind: 'group',
      geometry: { x: 0, y: 0, w: 200, h: 200 },
      parts: [
        {
          id: 'label',
          kind: 'text',
          geometry: { x: 10, y: 10 },
          text: 'Default Label',
          style: { fill: '#000' },
        },
      ],
    };

    const compositeKey = frameKey('grp-1', 'label');
    const texts = { [compositeKey]: 'Modified Label' };

    // walkDrawables 가 낸 키로 조회하면 찾을 수 있다
    const drawables = Array.from(walkDrawables([grp]));
    const drawable = drawables[0]!;
    expect(texts[drawable.key]).toBe('Modified Label');
  });

  it('평평한 키로 조회하면 찾을 수 없다 (현재 결함의 원인)', () => {
    const grp: GroupElement = {
      id: 'grp-1',
      kind: 'group',
      geometry: { x: 0, y: 0, w: 200, h: 200 },
      parts: [
        {
          id: 'label',
          kind: 'text',
          geometry: { x: 10, y: 10 },
          text: 'Default Label',
          style: { fill: '#000' },
        },
      ],
    };

    // 잘못된 키 - 평평한 'label' 만 있고 복합 키는 없다
    const texts: Record<string, string> = { 'label': 'This Will Not Show' };

    // walkDrawables 는 'grp-1/label' 을 낸다
    const drawables = Array.from(walkDrawables([grp]));
    const drawable = drawables[0]!;
    expect(drawable.key).toBe('grp-1/label');

    // texts['grp-1/label'] 은 undefined 이므로 요소의 기본 text 가 쓰인다
    expect(texts[drawable.key]).toBeUndefined();
    // 이 어긋남이 "그룹 안 텍스트가 안 보인다" 를 만든다.
  });
});
