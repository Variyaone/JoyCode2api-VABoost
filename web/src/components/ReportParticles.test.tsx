import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import ReportParticles from './ReportParticles';

afterEach(() => { cleanup(); localStorage.clear(); vi.unstubAllGlobals(); Reflect.deleteProperty(document, 'visibilityState'); });
function environment(reduced = false) {
  Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' });
  const callbacks: Array<() => void> = [];
  const media = { matches: reduced, addEventListener: vi.fn((_event, fn) => callbacks.push(fn)), removeEventListener: vi.fn() };
  vi.stubGlobal('matchMedia', () => media);
  return { media, callbacks };
}
describe('bounded report particles', () => {
  it('renders twelve decorative particles and supports a persistent pause in this view', () => {
    environment(); const { container } = render(<ReportParticles />);
    expect(container.querySelectorAll('.jc-report-particle')).toHaveLength(12);
    expect(container.querySelector('.jc-particle-field')?.getAttribute('aria-hidden')).toBe('true');
    fireEvent.click(screen.getByRole('button', { name: '暂停背景动态' }));
    expect(container.firstElementChild?.getAttribute('data-motion')).toBe('paused');
    expect(screen.getByRole('button', { name: '开启背景动态' }).getAttribute('aria-pressed')).toBe('true');
    fireEvent.click(screen.getByRole('button', { name: '开启背景动态' }));
    expect(container.firstElementChild?.getAttribute('data-motion')).toBe('running');
  });
  it('restores a saved pause without restarting animation on remount', () => {
    environment(); localStorage.setItem('jc_report_motion_paused', 'true');
    const { container } = render(<ReportParticles />);
    expect(container.firstElementChild?.getAttribute('data-motion')).toBe('paused');
    expect(screen.getByRole('button', { name: '开启背景动态' })).toBeTruthy();
  });
  it('respects reduced motion initially and on preference change', () => {
    const { media, callbacks } = environment(true); const { container, unmount } = render(<ReportParticles />);
    expect(container.firstElementChild?.getAttribute('data-motion')).toBe('paused');
    expect(screen.queryByRole('button')).toBeNull();
    act(() => { media.matches = false; callbacks.forEach(fn => fn()); });
    expect(container.firstElementChild?.getAttribute('data-motion')).toBe('running');
    unmount(); expect(media.removeEventListener).toHaveBeenCalled();
  });
  it('schedules parallax only for scrolling and stops offscreen without an idle frame loop', () => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' });
    vi.stubGlobal('matchMedia', (query: string) => ({ matches: query.includes('min-width'), addEventListener() {}, removeEventListener() {} }));
    let visibility!: (entries: { isIntersecting: boolean }[]) => void;
    const disconnect = vi.fn();
    vi.stubGlobal('IntersectionObserver', class {
      constructor(callback: typeof visibility) { visibility = callback; }
      observe() {} disconnect = disconnect;
    });
    const frames = new Map<number, FrameRequestCallback>();
    let id = 0;
    const raf = vi.fn((callback: FrameRequestCallback) => { frames.set(++id, callback); return id; });
    const cancel = vi.fn((key: number) => { frames.delete(key); });
    vi.stubGlobal('requestAnimationFrame', raf);
    vi.stubGlobal('cancelAnimationFrame', cancel);
    const { container, unmount } = render(<ReportParticles />);
    expect(raf).toHaveBeenCalledTimes(1);
    act(() => { const callback = frames.get(1)!; frames.delete(1); callback(0); });
    expect(frames.size).toBe(0);
    expect(raf).toHaveBeenCalledTimes(1);
    act(() => { window.dispatchEvent(new Event('scroll')); window.dispatchEvent(new Event('scroll')); window.dispatchEvent(new Event('scroll')); });
    expect(raf).toHaveBeenCalledTimes(2);
    act(() => { visibility([{ isIntersecting: false }]); });
    expect(frames.size).toBe(0);
    expect(container.firstElementChild?.getAttribute('data-motion')).toBe('paused');
    act(() => { window.dispatchEvent(new Event('scroll')); });
    expect(raf).toHaveBeenCalledTimes(2);
    expect(container.querySelector<HTMLElement>('.jc-particle-field')?.style.transform).toBe('translate3d(0, 0, 0)');
    unmount(); expect(disconnect).toHaveBeenCalledTimes(1);
  });
  it('pauses when document becomes hidden without making a request', () => {
    environment(); const fetch = vi.fn(); vi.stubGlobal('fetch', fetch);
    const { container } = render(<ReportParticles />);
    act(() => { Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' }); document.dispatchEvent(new Event('visibilitychange')); });
    expect(container.firstElementChild?.getAttribute('data-motion')).toBe('paused');
    expect(fetch).not.toHaveBeenCalled();
  });
});
