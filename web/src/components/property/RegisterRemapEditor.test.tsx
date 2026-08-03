// RegisterRemapEditor(modbus-remap) 테스트 — SPEC-MODBUS-007
// (컴팩트 목록 + 팝업 + C/D/H/I 약어 + 일괄등록 + 이름지정 템플릿 패턴 + 적용 materialize).
//
// i18n: ko.json 을 점 표기로 해석하는 mock.

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';

import {
  RegisterRemapEditor,
  materializeTemplate,
  parseBulkRules,
} from './RegisterRemapEditor';

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

function lastEmit(onChange: ReturnType<typeof vi.fn>): { rules: unknown; templates: unknown } {
  return onChange.mock.calls.at(-1)?.[0] as { rules: unknown; templates: unknown };
}
function dialog(): HTMLElement {
  return screen.getByRole('dialog');
}

// ───────────────────────── 순수 함수: parseBulkRules ─────────────────────────

describe('parseBulkRules', () => {
  it('약어(H) + target_area 생략(keep) 6열 → 규칙 1개(1 타깃)', () => {
    const r = parseBulkRules('H,0,10,2,,100');
    expect(r.errors).toEqual([]);
    expect(r.rules).toEqual([
      {
        source_area: 'holding_registers',
        source_address: 0,
        count: 10,
        targets: [{ target_unit_id: 2, target_address: 100 }],
      },
    ]);
  });

  it('전체 이름 area + target_area(I) + source_unit_id(7열) → 포함', () => {
    const r = parseBulkRules('coils,0,8,3,input_registers,200,1');
    expect(r.errors).toEqual([]);
    expect(r.rules).toEqual([
      {
        source_unit_id: 1,
        source_area: 'coils',
        source_address: 0,
        count: 8,
        targets: [{ target_unit_id: 3, target_area: 'input_registers', target_address: 200 }],
      },
    ]);
  });

  it('탭 구분 + 약어 대소문자 무관', () => {
    const r = parseBulkRules('d\t0\t4\t2\tc\t50');
    expect(r.errors).toEqual([]);
    expect(r.rules[0]).toMatchObject({
      source_area: 'discrete_inputs',
      targets: [{ target_unit_id: 2, target_area: 'coils', target_address: 50 }],
    });
  });

  it('헤더 줄(첫 셀이 영역 아님) 자동 무시', () => {
    const r = parseBulkRules('src,addr,cnt,unit,tarea,taddr\nH,0,4,1,,10');
    expect(r.errors).toEqual([]);
    expect(r.rules).toHaveLength(1);
  });

  it('잘못된 열 개수(5열) → wrongColumnCount', () => {
    const r = parseBulkRules('H,0,10,2,100');
    expect(r.rules).toEqual([]);
    expect(r.errors).toEqual([{ line: 1, code: 'wrongColumnCount' }]);
  });

  it('잘못된 대상 영역 → invalidTargetArea', () => {
    const r = parseBulkRules('H,0,10,2,ZZ,100');
    expect(r.errors).toEqual([{ line: 1, code: 'invalidTargetArea' }]);
  });
});

// ───────────────────────── 순수 함수: materializeTemplate ─────────────────────────

describe('materializeTemplate', () => {
  it('2 패턴 규칙을 start/device_id 로 구체 rules 로 확장한다', () => {
    const tpl = {
      name: 'sensor-block',
      rules: [
        {
          source_area: 'holding_registers',
          source_offset: 0,
          count: 8,
          targets: [{ target_offset: 100, target_unit_offset: 0 }],
        },
        {
          source_area: 'input_registers',
          source_offset: 8,
          count: 4,
          targets: [{ target_area: 'holding_registers', target_offset: 200, target_unit_offset: 1 }],
        },
      ],
    };
    expect(materializeTemplate(tpl, 1000, 5)).toEqual([
      {
        source_area: 'holding_registers',
        source_address: 1000,
        count: 8,
        targets: [{ target_unit_id: 5, target_address: 1100 }],
      },
      {
        source_area: 'input_registers',
        source_address: 1008,
        count: 4,
        targets: [{ target_unit_id: 6, target_area: 'holding_registers', target_address: 1200 }],
      },
    ]);
  });

  it('source_unit_id as-is, target_unit_offset 생략 → device_id 그대로', () => {
    const tpl = {
      name: 't',
      rules: [
        {
          source_unit_id: 9,
          source_area: 'coils',
          source_offset: 5,
          count: 1,
          targets: [{ target_offset: 0 }],
        },
      ],
    };
    expect(materializeTemplate(tpl, 100, 3)).toEqual([
      {
        source_unit_id: 9,
        source_area: 'coils',
        source_address: 105,
        count: 1,
        targets: [{ target_unit_id: 3, target_address: 100 }],
      },
    ]);
  });
});

// ───────────────────────── UI: rules 방출 ─────────────────────────

describe('RegisterRemapEditor rules', () => {
  it('규칙 추가(팝업) → 단일 타깃 규칙을 { rules } 로 방출한다', () => {
    const onChange = vi.fn();
    render(
      <RegisterRemapEditor rulesValue={[]} templatesValue={[]} onChange={onChange} />,
    );

    fireEvent.click(screen.getByRole('button', { name: '규칙 추가' }));
    fireEvent.change(within(dialog()).getByRole('spinbutton', { name: '소스 주소' }), {
      target: { value: '0' },
    });
    fireEvent.change(within(dialog()).getByRole('spinbutton', { name: '개수' }), {
      target: { value: '10' },
    });
    fireEvent.change(within(dialog()).getByRole('spinbutton', { name: '유닛 ID' }), {
      target: { value: '2' },
    });
    fireEvent.change(within(dialog()).getByRole('spinbutton', { name: '대상 주소' }), {
      target: { value: '100' },
    });
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    expect(lastEmit(onChange).rules).toEqual([
      {
        source_area: 'holding_registers',
        source_address: 0,
        count: 10,
        targets: [{ target_unit_id: 2, target_address: 100 }],
      },
    ]);
  });

  it('팝업에서 타깃 추가 → 다중 타깃(2개) 팬아웃 방출', () => {
    const onChange = vi.fn();
    render(
      <RegisterRemapEditor rulesValue={[]} templatesValue={[]} onChange={onChange} />,
    );
    fireEvent.click(screen.getByRole('button', { name: '규칙 추가' }));
    fireEvent.click(within(dialog()).getByRole('button', { name: '타깃 추가' }));
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    const rules = lastEmit(onChange).rules as Array<{ targets: unknown[] }>;
    expect(rules[0]!.targets).toHaveLength(2);
  });

  it('C/D/H/I 약어를 목록 요약에 표시한다', () => {
    render(
      <RegisterRemapEditor
        rulesValue={[
          {
            source_area: 'coils',
            source_address: 5,
            count: 2,
            targets: [{ target_unit_id: 3, target_area: 'input_registers', target_address: 50 }],
          },
        ]}
        templatesValue={[]}
        onChange={vi.fn()}
      />,
    );
    // "C5 ×2 → u3·I50"
    expect(screen.getByText(/C5/)).toBeInTheDocument();
    expect(screen.getByText(/u3·I50/)).toBeInTheDocument();
  });

  it('일괄등록: 약어/전체 이름 혼합 + optional source_unit_id → rules append', () => {
    const onChange = vi.fn();
    render(
      <RegisterRemapEditor rulesValue={[]} templatesValue={[]} onChange={onChange} />,
    );
    fireEvent.click(screen.getByRole('button', { name: '일괄등록' }));
    fireEvent.change(screen.getByRole('textbox', { name: '일괄등록' }), {
      target: { value: 'H,0,10,2,,100\ncoils,0,8,3,I,200,1' },
    });
    fireEvent.click(screen.getByRole('button', { name: '적용' }));

    expect(lastEmit(onChange).rules).toEqual([
      {
        source_area: 'holding_registers',
        source_address: 0,
        count: 10,
        targets: [{ target_unit_id: 2, target_address: 100 }],
      },
      {
        source_unit_id: 1,
        source_area: 'coils',
        source_address: 0,
        count: 8,
        targets: [{ target_unit_id: 3, target_area: 'input_registers', target_address: 200 }],
      },
    ]);
  });

  it('legacy 최상위 단일 target_* 를 targets 로 정규화해 round-trip 방출한다', () => {
    const onChange = vi.fn();
    render(
      <RegisterRemapEditor
        rulesValue={[
          {
            source_area: 'coils',
            source_address: 5,
            count: 2,
            target_unit_id: 7,
            target_area: 'discrete_inputs',
            target_address: 50,
          },
        ]}
        templatesValue={[]}
        onChange={onChange}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: '편집' }));
    fireEvent.click(within(dialog()).getByRole('button', { name: '저장' }));

    expect(lastEmit(onChange).rules).toEqual([
      {
        source_area: 'coils',
        source_address: 5,
        count: 2,
        targets: [{ target_unit_id: 7, target_area: 'discrete_inputs', target_address: 50 }],
      },
    ]);
  });
});

// ───────────────────────── UI: templates round-trip + apply ─────────────────────────

describe('RegisterRemapEditor templates', () => {
  const tplValue = [
    {
      name: 'sensor-block',
      rules: [
        {
          source_area: 'holding_registers',
          source_offset: 0,
          count: 8,
          targets: [{ target_offset: 100, target_unit_offset: 0 }],
        },
        {
          source_area: 'input_registers',
          source_offset: 8,
          count: 4,
          targets: [{ target_area: 'holding_registers', target_offset: 200, target_unit_offset: 1 }],
        },
      ],
    },
  ];

  it('템플릿 패턴을 round-trip 방출한다 (이름 편집 트리거, 패턴 보존)', () => {
    const onChange = vi.fn();
    render(
      <RegisterRemapEditor rulesValue={[]} templatesValue={tplValue} onChange={onChange} />,
    );
    // 이름 편집으로 emit 트리거 — 패턴 규칙 배열은 그대로 보존되어야 한다.
    fireEvent.change(screen.getByRole('textbox', { name: '템플릿 이름' }), {
      target: { value: 'renamed' },
    });
    expect(lastEmit(onChange).templates).toEqual([
      { ...tplValue[0], name: 'renamed' },
    ]);
  });

  it('적용(start=1000, device_id=5) → 구체 rules 를 append 한다', () => {
    const onChange = vi.fn();
    render(
      <RegisterRemapEditor rulesValue={[]} templatesValue={tplValue} onChange={onChange} />,
    );
    fireEvent.click(screen.getByRole('button', { name: '적용' }));
    fireEvent.change(within(dialog()).getByRole('spinbutton', { name: '시작 주소' }), {
      target: { value: '1000' },
    });
    fireEvent.change(within(dialog()).getByRole('spinbutton', { name: 'device_id' }), {
      target: { value: '5' },
    });
    fireEvent.click(within(dialog()).getByRole('button', { name: '적용' }));

    expect(lastEmit(onChange).rules).toEqual([
      {
        source_area: 'holding_registers',
        source_address: 1000,
        count: 8,
        targets: [{ target_unit_id: 5, target_address: 1100 }],
      },
      {
        source_area: 'input_registers',
        source_address: 1008,
        count: 4,
        targets: [{ target_unit_id: 6, target_area: 'holding_registers', target_address: 1200 }],
      },
    ]);
    // 템플릿은 그대로 보존.
    expect(lastEmit(onChange).templates).toEqual(tplValue);
  });

  it('readOnly 는 규칙/템플릿 추가 및 일괄등록 버튼을 숨긴다', () => {
    render(
      <RegisterRemapEditor
        rulesValue={[
          {
            source_area: 'coils',
            source_address: 0,
            count: 1,
            targets: [{ target_unit_id: 1, target_address: 0 }],
          },
        ]}
        templatesValue={tplValue}
        onChange={vi.fn()}
        readOnly
      />,
    );
    expect(screen.queryByRole('button', { name: '규칙 추가' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '일괄등록' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '템플릿 추가' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '적용' })).not.toBeInTheDocument();
  });
});
