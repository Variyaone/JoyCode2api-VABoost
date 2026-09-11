// Modified by Variya, 2026-09-12: repo attribution only; maintainer line lives in the header badges.
import { brand } from '../brand';
import UsageNotice from './UsageNotice';

export default function ProjectCredits() {
  return <footer className="jc-project-footer">
    <p>基于 <a href={brand.upstream} target="_blank" rel="noopener noreferrer">vibe-coding-labs / JoyCode2Api</a> 独立改造（joyCode2api-VABoost）。
      感谢上游维护者 <a href={brand.upstreamMaintainer} target="_blank" rel="noopener noreferrer">CC11001100</a> 及贡献者。</p>
    <p>上游源码版本 0.6.1，基线 <a href={brand.baseline} target="_blank" rel="noopener noreferrer">176ca9d（2026-07-15）</a>。
      保留 Apache-2.0 许可及原有版权声明。</p>
    <UsageNotice />
  </footer>;
}
