// RegisterRemapEditor(modbus-remap) 테스트 — SPEC-MODBUS-007 (multi-target fan-out).
//
// 검증:
//   - rule 방출: source_unit_id? + targets[] (≥1), target_area 있음/없음
//   - source_unit_id 없으면 생략
//   - 다중 타깃(2개) 방출
//   - legacy 최상위 단일 target_* → 1-원소 targets 로 정규화 후 방출
//   - template 방출: source_unit_id?, target_area?, 음수 offset
//   - readOnly 버튼 숨김
//
// i18n: ko.json 을 점 표기로 해석하는 mock.

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { RemapRulesEditor, RemapTemplatesEditor } from './RegisterRemapEditor';

vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const resolve = (key: string): string => {
    const v = key.split('.').reduce<unknown>(
      (o, p) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined),
      ko,
    );
    return typeof v === 'string' ? v : key;
  };
  return {
    useTranslation: () => ({ t: resolve, locale: 'ko' as const, setLocale: () => {} }),
  };
});

function lastEmit(onChange: ReturnType<typeof vi.fn>): unknown {
  return onChange.mock.calls.at(-1)?.[0];
}

describe('RemapRulesEditor', () => {
  it('규칙 추가 → source_unit_id 없이 targets 배열(1개, target_area 생략) 방출', () => {
    const onChange = vi.fn();
    render(<RemapRulesEditor value={[]} onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: '규칙 추가' }));
    fireEvent.change(screen.getByRole('spinbutton', { name: '소스 주소' }), {
      target: { value: '0' },
    });
    fireEvent.change(screen.getByRole('spinbutton', { name: '개수' }), {
      target: { value: '10' },
    });
    fireEvent.change(screen.getByRole('spinbutton', { name: '유닛 ID' }), {
      target: { value: '1' },
    });
    fireEvent.change(screen.getByRole('spinbutton', { name: '대상 주소' }), {
      target: { value: '100' },
    });

    expect(lastEmit(onChange)).toEqual([
      {
        source_area: 'holding_registers',
        source_address: 0,
        count: 10,
        targets: [{ target_unit_id: 1, target_address: 100 }],
      },
    ]);
  });

  it('source_unit_id 입력 + target_area 선택 시 방출에 포함', () => {
    const onChange = vi.fn();
    render(<RemapRulesEditor value={[]} onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: '규칙 추가' }));
    fireEvent.change(screen.getByRole('spinbutton', { name: '소스 유닛 ID' }), {
      target: { value: '1' },
    });
    fireEvent.change(screen.getByRole('combobox', { name: '소스 영역' }), {
      target: { value: 'input_registers' },
    });
    fireEvent.change(screen.getByRole('spinbutton', { name: '개수' }), {
      target: { value: '4' },
    });
    fireEvent.change(screen.getByRole('spinbutton', { name: '유닛 ID' }), {
      target: { value: '2' },
    });
    fireEvent.change(screen.getByRole('combobox', { name: '대상 영역' }), {
      target: { value: 'holding_registers' },
    });
    fireEvent.change(screen.getByRole('spinbutton', { name: '대상 주소' }), {
      target: { value: '200' },
    });

    expect(lastEmit(onChange)).toEqual([
      {
        source_unit_id: 1,
        source_area: 'input_registers',
        source_address: 0,
        count: 4,
        targets: [{ target_unit_id: 2, target_area: 'holding_registers', target_address: 200 }],
      },
    ]);
  });

  it('다중 타깃(2개) 팬아웃을 방출한다 (하나는 area 유지, 하나는 area 변경)', () => {
    const onChange = vi.fn();
    render(
      <RemapRulesEditor
        value={[
          {
            source_unit_id: 1,
            source_area: 'holding_registers',
            source_address: 0,
            count: 10,
            targets: [
              { target_unit_id: 2, target_address: 100 },
              { target_unit_id: 3, target_area: 'input_registers', target_address: 200 },
            ],
          },
        ]}
        onChange={onChange}
      />,
    );

    // 방출 트리거: source_address 변경.
    fireEvent.change(screen.getByRole('spinbutton', { name: '소스 주소' }), {
      target: { value: '5' },
    });

    expect(lastEmit(onChange)).toEqual([
      {
        source_unit_id: 1,
        source_area: 'holding_registers',
        source_address: 5,
        count: 10,
        targets: [
          { target_unit_id: 2, target_address: 100 },
          { target_unit_id: 3, target_area: 'input_registers', target_address: 200 },
        ],
      },
    ]);
  });

  it('타깃 추가 버튼으로 타깃을 늘린다 (targets 2개)', () => {
    const onChange = vi.fn();
    render(<RemapRulesEditor value={[]} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: '규칙 추가' }));
    fireEvent.click(screen.getByRole('button', { name: '타깃 추가' }));

    const emitted = lastEmit(onChange) as Array<{ targets: unknown[] }>;
    expect(emitted[0]!.targets).toHaveLength(2);
  });

  it('legacy 최상위 단일 target_* config 을 1-원소 targets 로 정규화해 방출한다', () => {
    const onChange = vi.fn();
    render(
      <RemapRulesEditor
        value={[
          {
            source_area: 'coils',
            source_address: 5,
            count: 2,
            target_unit_id: 7,
            target_area: 'discrete_inputs',
            target_address: 50,
          },
        ]}
        onChange={onChange}
      />,
    );

    // 방출 트리거: 개수 변경.
    fireEvent.change(screen.getByRole('spinbutton', { name: '개수' }), {
      target: { value: '3' },
    });

    expect(lastEmit(onChange)).toEqual([
      {
        source_area: 'coils',
        source_address: 5,
        count: 3,
        targets: [{ target_unit_id: 7, target_area: 'discrete_inputs', target_address: 50 }],
      },
    ]);
  });

  it('readOnly 는 규칙 추가 / 타깃 추가 버튼을 숨긴다', () => {
    render(
      <RemapRulesEditor
        value={[
          {
            source_area: 'coils',
            source_address: 0,
            count: 1,
            targets: [{ target_unit_id: 1, target_address: 0 }],
          },
        ]}
        onChange={vi.fn()}
        readOnly
      />,
    );
    expect(screen.queryByRole('button', { name: '규칙 추가' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '타깃 추가' })).not.toBeInTheDocument();
  });
});

describe('RemapTemplatesEditor', () => {
  it('템플릿 추가 → {area, offset, device_id, start, count} 방출 (source_unit_id/target_area 생략)', () => {
    const onChange = vi.fn();
    render(<RemapTemplatesEditor value={[]} onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: '템플릿 추가' }));
    fireEvent.change(screen.getByRole('spinbutton', { name: '오프셋' }), {
      target: { value: '1000' },
    });
    fireEvent.change(screen.getByRole('spinbutton', { name: '디바이스 ID' }), {
      target: { value: '3' },
    });
    fireEvent.change(screen.getByRole('spinbutton', { name: '시작' }), {
      target: { value: '0' },
    });
    fireEvent.change(screen.getByRole('spinbutton', { name: '개수' }), {
      target: { value: '8' },
    });

    expect(lastEmit(onChange)).toEqual([
      { area: 'holding_registers', offset: 1000, device_id: 3, start: 0, count: 8 },
    ]);
  });

  it('source_unit_id + 음수 offset + target_area 를 방출에 포함한다', () => {
    const onChange = vi.fn();
    render(<RemapTemplatesEditor value={[]} onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: '템플릿 추가' }));
    fireEvent.change(screen.getByRole('spinbutton', { name: '소스 유닛 ID' }), {
      target: { value: '1' },
    });
    fireEvent.change(screen.getByRole('spinbutton', { name: '오프셋' }), {
      target: { value: '-5' },
    });
    fireEvent.change(screen.getByRole('combobox', { name: '대상 영역' }), {
      target: { value: 'coils' },
    });

    expect(lastEmit(onChange)).toEqual([
      {
        source_unit_id: 1,
        area: 'holding_registers',
        offset: -5,
        device_id: 1,
        start: 0,
        count: 1,
        target_area: 'coils',
      },
    ]);
  });

  it('readOnly 는 추가 버튼을 숨긴다', () => {
    render(
      <RemapTemplatesEditor
        value={[{ area: 'coils', offset: 0, device_id: 1, start: 0, count: 1 }]}
        onChange={vi.fn()}
        readOnly
      />,
    );
    expect(screen.queryByRole('button', { name: '템플릿 추가' })).not.toBeInTheDocument();
  });
});
