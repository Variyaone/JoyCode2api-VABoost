/// <reference types="node" />
// Variya light workspace identity regression tests, 2026-09-11.
import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import ProjectCredits from './ProjectCredits';
import ModelLogo from './ModelLogo';
import { brand } from '../brand';
import { colors, lightTheme } from '../theme';

afterEach(cleanup);

describe('independent light identity', () => {
  it('keeps maintainer and upstream provenance distinct', () => {
    render(<ProjectCredits />);
    expect(screen.queryByRole('link', { name: '由 Variya 维护' })).toBeNull();
    expect(brand.repository).toBe('https://github.com/variyaone/JoyCode2api-VABoost');
    expect(screen.getByRole('link', { name: /vibe-coding-labs/ }).getAttribute('href')).toBe(brand.upstream);
    expect(screen.getByRole('link', { name: /176ca9d/ }).getAttribute('href')).toBe(brand.baseline);
    expect(screen.getByRole('link', { name: 'CC11001100' }).getAttribute('href')).toBe(brand.upstreamMaintainer);
    expect(screen.getByRole('complementary', { name: '使用与信息安全声明' })).toBeTruthy();
    expect(document.body.textContent).toContain('Apache-2.0');
  });
  it('uses explicit light tokens independent of system color scheme', () => {
    expect(lightTheme.token?.colorBgContainer).toBe('#FFFFFF');
    expect(lightTheme.token?.colorBgLayout).toBe('#F5F6F8');
    expect(lightTheme.token?.colorText).toBe('#252A34');
    expect(colors.accent).toBe('#5966A6');
  });
  it.each([['GLM-5.3', 'glm'], ['Kimi-K3', 'kimi'], ['GPT-6 Astra', 'openai'], ['Doubao-Seed', 'bytedance']])('renders a legible monochrome logo for %s', (model, name) => {
    const { container } = render(<ModelLogo model={model} />);
    expect(container.querySelector('img')?.getAttribute('src')).toBe(`/logo-${name}.svg`);
    const svg = readFileSync(resolve('public', `logo-${name}.svg`), 'utf8');
    expect(svg).toContain('fill="#252A34"');
    expect(svg).not.toContain('fill="#F8FAFC"');
  });
  it('retains Z.ai geometry and the original asset license', () => {
    const svg = readFileSync(resolve('public/logo-glm.svg'), 'utf8');
    expect(svg).toContain('<title>Z.ai</title>');
    expect(svg).toContain('Copyright (c) 2023 LobeHub');
    expect(svg).toContain('Permission is hereby granted');
  });
  it('does not pretend an unknown model has a known brand', () => {
    const { container } = render(<ModelLogo model="Unknown-model" />);
    expect(container.querySelector('img')).toBeNull();
  });
});
