// 통신이 필요한 항목인지 가리는 규칙.

import { describe, it, expect } from 'vitest';

import { needsReception } from './deviceLabels';

describe('needsReception', () => {
  it('메타데이터는 통신과 무관하다 — 사용자가 적어 둔 값이다', () => {
    expect(needsReception('meta.name')).toBe(false);
    expect(needsReception('meta.location')).toBe(false);
    expect(needsReception('meta.label.dev_eui')).toBe(false);
  });

  it('게이트웨이 수신 정보는 파생이지만 업링크가 있어야 생긴다', () => {
    expect(needsReception('gw.rssi')).toBe(true);
    expect(needsReception('gw.gateway_id')).toBe(true);
  });

  it('디바이스가 보고하는 속성은 통신이 필요하다', () => {
    expect(needsReception('temperature')).toBe(true);
    expect(needsReception('battery')).toBe(true);
  });
});
