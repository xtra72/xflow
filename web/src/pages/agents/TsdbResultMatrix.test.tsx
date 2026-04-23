// TsdbResultMatrix 단위 테스트 — 매트릭스 렌더링, 누락 셀 em-dash, 타임스탬프 포맷.
//
// @spec SPEC-WEB-005

import { describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/react';

import TsdbResultMatrix from './TsdbResultMatrix';
import type { TsdbQueryResponse } from '@/services/api/tsdb';

function makeResponse(): TsdbQueryResponse {
  const t0 = new Date(2026, 3, 23, 0, 0, 0).getTime();
  const t1 = new Date(2026, 3, 23, 1, 0, 0).getTime();
  return {
    results: [
      {
        key: 'temp,room=1',
        points: [
          { timestampMs: t0, value: 21.5 },
          { timestampMs: t1, value: 22.0 },
        ],
      },
      {
        key: 'temp,room=2',
        points: [
          { timestampMs: t0, value: 18.3 },
          // t1 버킷 누락 — em-dash 가 렌더되어야 함
        ],
      },
    ],
  };
}

describe('TsdbResultMatrix', () => {
  it('선택된 시리즈 키를 컬럼 헤더로 표시', () => {
    render(<TsdbResultMatrix keys={['temp,room=1', 'temp,room=2']} response={makeResponse()} />);
    expect(screen.getByRole('columnheader', { name: 'temp,room=1' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'temp,room=2' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: /타임스탬프/ })).toBeInTheDocument();
  });

  it('시간 버킷이 오름차순으로 정렬되어 렌더링', () => {
    render(<TsdbResultMatrix keys={['temp,room=1', 'temp,room=2']} response={makeResponse()} />);
    expect(screen.getByText('2026-04-23 00:00:00')).toBeInTheDocument();
    expect(screen.getByText('2026-04-23 01:00:00')).toBeInTheDocument();
  });

  it('누락된 버킷×시리즈 교차점은 em-dash(—) 로 표시', () => {
    render(<TsdbResultMatrix keys={['temp,room=1', 'temp,room=2']} response={makeResponse()} />);
    // t1 행에서 temp,room=2 셀은 — 여야 함.
    const row = screen.getByText('2026-04-23 01:00:00').closest('tr')!;
    expect(within(row).getByText('—')).toBeInTheDocument();
  });

  it('값이 존재하는 셀은 포맷된 숫자 렌더링', () => {
    render(<TsdbResultMatrix keys={['temp,room=1']} response={makeResponse()} />);
    expect(screen.getByText('21.5')).toBeInTheDocument();
    // 22.0 은 Integer 로 인식되어 "22" 로 출력.
    expect(screen.getByText('22')).toBeInTheDocument();
  });

  it('단일 시리즈도 정상 렌더링 (1개 데이터 컬럼)', () => {
    const resp: TsdbQueryResponse = {
      results: [
        {
          key: 'only',
          points: [{ timestampMs: new Date(2026, 3, 23).getTime(), value: 1 }],
        },
      ],
    };
    render(<TsdbResultMatrix keys={['only']} response={resp} />);
    expect(screen.getByRole('columnheader', { name: 'only' })).toBeInTheDocument();
  });

  it('emptyCellPlaceholder 커스텀 문자를 사용', () => {
    const resp: TsdbQueryResponse = {
      results: [
        {
          key: 'a',
          points: [{ timestampMs: new Date(2026, 3, 23).getTime(), value: 1 }],
        },
      ],
    };
    render(
      <TsdbResultMatrix
        keys={['a', 'b']}
        response={resp}
        emptyCellPlaceholder="N/A"
      />,
    );
    expect(screen.getByText('N/A')).toBeInTheDocument();
  });

  it('키가 없으면 안내 메시지 렌더링', () => {
    render(<TsdbResultMatrix keys={[]} response={{ results: [] }} />);
    expect(screen.getByText(/선택된 시리즈가 없습니다/)).toBeInTheDocument();
  });

  it('결과가 없으면 안내 메시지 렌더링', () => {
    render(
      <TsdbResultMatrix
        keys={['a']}
        response={{ results: [{ key: 'a', points: [] }] }}
      />,
    );
    expect(screen.getByText(/쿼리 결과가 비어 있습니다/)).toBeInTheDocument();
  });
});
