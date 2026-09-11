// Variya: bounded decorative motion. No canvas, animation loop, or per-scroll React state.
import { useEffect, useRef, useState } from 'react';
import { Button } from 'antd';
const points = [[8,22],[24,58],[35,14],[47,76],[58,36],[67,64],[74,18],[83,48],[92,28],[15,83],[55,92],[91,84]];

export default function ReportParticles() {
  const root = useRef<HTMLDivElement>(null);
  const field = useRef<HTMLDivElement>(null);
  const [paused, setPaused] = useState(() => localStorage.getItem('jc_report_motion_paused') === 'true');
  const [hidden, setHidden] = useState(() => document.visibilityState === 'hidden');
  const [inView, setInView] = useState(true);
  const [reduced, setReduced] = useState(() => window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false);
  const [desktop, setDesktop] = useState(() => window.matchMedia?.('(min-width: 768px)').matches ?? false);
  const moving = !paused && !hidden && inView && !reduced;
  useEffect(() => { localStorage.setItem('jc_report_motion_paused', String(paused)); }, [paused]);
  useEffect(() => {
    const visibility = () => setHidden(document.visibilityState === 'hidden');
    const media = window.matchMedia?.('(prefers-reduced-motion: reduce)');
    const viewport = window.matchMedia?.('(min-width: 768px)');
    const preferences = () => { setReduced(media?.matches ?? false); setDesktop(viewport?.matches ?? false); };
    document.addEventListener('visibilitychange', visibility);
    media?.addEventListener?.('change', preferences);
    viewport?.addEventListener?.('change', preferences);
    const observer = typeof IntersectionObserver === 'undefined' ? null : new IntersectionObserver(entries => {
      setInView(entries.some(entry => entry.isIntersecting));
    }, { rootMargin: '40px', threshold: 0 });
    if (root.current) observer?.observe(root.current);
    return () => {
      document.removeEventListener('visibilitychange', visibility);
      media?.removeEventListener?.('change', preferences);
      viewport?.removeEventListener?.('change', preferences);
      observer?.disconnect();
    };
  }, []);
  useEffect(() => {
    const layer = field.current;
    if (!layer) return;
    layer.style.transform = 'translate3d(0, 0, 0)';
    if (!moving || !desktop) return;
    let frame: number | null = null;
    const update = () => {
      frame = null;
      const top = root.current?.getBoundingClientRect().top ?? 0;
      const y = Math.max(-10, Math.min(10, (window.innerHeight * 0.25 - top) * 0.035));
      layer.style.transform = `translate3d(0, ${y.toFixed(2)}px, 0)`;
    };
    const onScroll = () => { if (frame === null) frame = requestAnimationFrame(update); };
    window.addEventListener('scroll', onScroll, { passive: true });
    onScroll();
    return () => {
      window.removeEventListener('scroll', onScroll);
      if (frame !== null) cancelAnimationFrame(frame);
      layer.style.transform = 'translate3d(0, 0, 0)';
    };
  }, [moving, desktop]);
  return <div ref={root} className="jc-particle-region" data-motion={moving ? 'running' : 'paused'} data-parallax={moving && desktop ? 'active' : 'off'}>
    <div ref={field} className="jc-particle-field" aria-hidden="true">
      {points.map(([x,y], index) => <span key={index} className="jc-report-particle" style={{ left: `${x}%`, top: `${y}%`, animationDelay: `${-index * 1.7}s` }} />)}
    </div>
    {!reduced && <Button className="jc-particle-toggle" type="text" size="small" aria-pressed={paused} onClick={() => setPaused(!paused)}>{paused ? '开启背景动态' : '暂停背景动态'}</Button>}
  </div>;
}
