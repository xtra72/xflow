// FormField boolean 기본값 렌더링 회귀 테스트.
//
// 버그: field.default 가 true 인데 value 가 undefined 일 때
// 체크박스가 unchecked 로 렌더링되어 SPEC-STORE-003 allow_dynamic_keys
// (default true) 같은 필드가 잘못 비활성화로 보인다.
//
// 수정 후: value 가 undefined 이면 field.default 를 fallback 으로 사용.
//
// @spec SPEC-STORE-003

import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import { FormField } from './FormField';
import type { ConfigField } from '@/types/node';

const boolField = (defaultValue?: unknown): ConfigField => ({
  name: 'allow_dynamic_keys',
  type: 'boolean',
  label: 'Allow dynamic keys',
  default: defaultValue,
});

describe('FormField boolean default fallback', () => {
  it('default=true, value=undefined 이면 체크박스가 checked 로 렌더된다', () => {
    render(
      <FormField
        field={boolField(true)}
        value={undefined}
        onChange={vi.fn()}
      />,
    );
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement;
    expect(checkbox.checked).toBe(true);
  });

  it('default=false, value=undefined 이면 체크박스가 unchecked 로 렌더된다', () => {
    render(
      <FormField
        field={boolField(false)}
        value={undefined}
        onChange={vi.fn()}
      />,
    );
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement;
    expect(checkbox.checked).toBe(false);
  });

  it('default=true, value=false 이면 명시값이 default 를 덮어써 unchecked 가 된다', () => {
    render(
      <FormField
        field={boolField(true)}
        value={false}
        onChange={vi.fn()}
      />,
    );
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement;
    expect(checkbox.checked).toBe(false);
  });

  it('default=true, value=true 이면 체크박스가 checked 로 렌더된다', () => {
    render(
      <FormField
        field={boolField(true)}
        value={true}
        onChange={vi.fn()}
      />,
    );
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement;
    expect(checkbox.checked).toBe(true);
  });

  it('default 미지정, value=undefined 이면 체크박스가 unchecked 로 렌더된다', () => {
    render(
      <FormField
        field={boolField(undefined)}
        value={undefined}
        onChange={vi.fn()}
      />,
    );
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement;
    expect(checkbox.checked).toBe(false);
  });

  it('readOnly=true + default=true + value=undefined 이면 disabled 상태에서도 checked 로 표시된다', () => {
    render(
      <FormField
        field={boolField(true)}
        value={undefined}
        onChange={vi.fn()}
        readOnly
      />,
    );
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement;
    expect(checkbox.checked).toBe(true);
    expect(checkbox.disabled).toBe(true);
  });
});
