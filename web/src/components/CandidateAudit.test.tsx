// Frozen v1 candidate evidence regressions. No network or model calls.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import CandidateAudit, { candidateAuditSummary, enhancedPassed } from './CandidateAudit';
import snapshot from '../data/candidateAudit.json';

beforeEach(() => {
  const computed = window.getComputedStyle.bind(window);
  vi.spyOn(window, 'getComputedStyle').mockImplementation(element => computed(element));
  vi.stubGlobal('matchMedia', vi.fn().mockImplementation(query => ({ matches: false, media: query, addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {} })));
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe('frozen candidate provenance', () => {
  it('keeps release status separate from six reviewed fixes', () => {
    expect(snapshot.schemaVersion).toBe(2);
    expect(snapshot.snapshotId).toBe('model-audit-20260911-reviewed-v1');
    expect(snapshot.productionDeployed).toBe(false);
    expect(snapshot.releaseReady).toBe(false);
    expect(snapshot.codeReview.items).toHaveLength(6);
    expect(snapshot.codeReview.items.every(item => item.verified)).toBe(true);
  });
  it('recomputes historical observations without upgrading them from enhanced checks', () => {
    expect(candidateAuditSummary()).toEqual({ modelCount: 13, totalCells: 65, chatModels: 12, chatCells: 60, toolExecuted: 60, toolResultReturned: 60, toolRoundtrips: 60, strictOutput: 56, ultraPassed: 11 });
    expect(snapshot.models.flatMap(m => m.cells)).toHaveLength(65);
    expect(snapshot.evidence).toHaveLength(80);
  });
  it.each(snapshot.models.map(m => [m.id, m] as const))('keeps five historical effort cells and unrecorded build identities for %s', (_id, model) => {
    expect(model.cells.map(c => c.effort)).toEqual(['low','medium','high','xhigh','max']);
    expect(model.cells.every(c => c.buildIdentity === 'not-recorded')).toBe(true);
    expect(model.ultracode.buildIdentity).toBe('not-recorded');
    expect(model.cells.every(c => snapshot.evidence.some(e => e.id === c.evidenceId))).toBe(true);
  });
  it('does not mark MiniMax strict failures or JoyAI parent bypass as success', () => {
    const minimax = snapshot.models.find(m => m.id === 'MiniMax-M3')!;
    expect(minimax.cells.filter(c => c.strictOutput && c.sdkResultExact)).toHaveLength(1);
    expect(snapshot.models.find(m => m.id === 'JoyAI-Code-1.5')!.ultracode.parentBypass).toBe(true);
    expect(snapshot.models.find(m => m.id === 'JoyCode-Base-V3')!.kind).toBe('code-completion');
  });
  it('binds only the two enhanced runs to the exact fixture, never the packaged executable', () => {
    const enhanced = snapshot.models.filter(enhancedPassed);
    expect(enhanced.map(m => m.id).sort()).toEqual(['GLM-5.3', 'GPT-6 Astra']);
    enhanced.forEach(m => {
      expect(m.enhancedVerification!.testFixtureSHA256).toBe(snapshot.candidate.testFixtureSHA256);
      expect(m.enhancedVerification!.testFixtureSHA256).not.toBe(snapshot.candidate.packagedExecutableSHA256);
    });
    expect(enhanced.find(m => m.id === 'GLM-5.3')!.enhancedVerification!.test).toBe('Bash');
    expect(enhanced.find(m => m.id === 'GPT-6 Astra')!.enhancedVerification!.test).toBe('Ultracode');
  });
  it('does not expose raw endpoints, payloads, filesystem paths, session IDs or credentials', () => {
    const text = JSON.stringify(snapshot);
    expect(text).not.toMatch(/https?:\/\/|[A-Z]:\\|pt_key|Bearer |eyJ[A-Za-z0-9_-]{20,}/);
    const prohibited = new Set(['path','sourcePath','request','messages','input','sessionId','agentId','runId','executionId','endpoint','api_token']);
    const visit = (value: unknown) => {
      if (!value || typeof value !== 'object') return;
      Object.entries(value).forEach(([key, child]) => { expect(prohibited.has(key), key).toBe(false); visit(child); });
    };
    visit(snapshot);
  });
});

describe('frozen evidence presentation', { timeout: 20000 }, () => {
  it('shows review scope, deployment boundary and first timeout plus retry', () => {
    const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); render(<CandidateAudit />);
    expect(screen.getByText(/NOT DEPLOYED/)).toBeTruthy();
    expect(screen.getByText(/NOT RELEASE READY/)).toBeTruthy();
    expect(screen.getByText(/六项代码补修已独立复核/)).toBeTruthy();
    expect(screen.getByRole('region', { name: '独立加强验证' })).toBeTruthy();
    expect(screen.getByText(/父运行首次触及 180s/)).toBeTruthy();
    expect(document.body.textContent).toContain('约 181s 通过');
    expect(document.body.textContent).toContain('本次是 Bash，不是新的 Ultracode 复验');
    expect(document.body.textContent).toContain('不能回填到旧矩阵');
    expect(fetch).not.toHaveBeenCalled();
  });
  it.each(['MiniMax-M3','JoyCode-Base-V3','GPT-6 Astra'])('expands all five original cells for %s', model => {
    const { container } = render(<CandidateAudit />);
    const row = container.querySelector(`tr[data-row-key="${model}"]`)!;
    fireEvent.click(row.querySelector('button')!);
    const detail = screen.getByRole('region', { name: `${model} 档位明细` });
    expect(detail.querySelectorAll('tbody tr[data-row-key]')).toHaveLength(5);
    if (model === 'MiniMax-M3') expect(within(detail).getAllByText('未通过').length).toBe(8);
    if (model === 'JoyCode-Base-V3') expect(within(detail).getAllByText('不适用').length).toBeGreaterThan(0);
    expect(detail.textContent).toContain('并非全部模型已完成加强后的逐请求路由');
  });
});
