// Variya 2026-09-12: dual-platform star badges. Counts come from the proxy cache;
// "star" opens the repo page where the logged-in user confirms with one click.
import { StarFilled, StarOutlined } from '@ant-design/icons';
import { Tooltip } from 'antd';
import { useEffect, useState } from 'react';
import { api } from '../api';

function GithubGlyph() {
  return <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" style={{ fill: 'currentColor' }}>
    <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z" />
  </svg>;
}

function GiteeGlyph() {
  return <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" style={{ fill: 'currentColor' }}>
    <path d="M8 0a8 8 0 1 0 0 16A8 8 0 0 0 8 0Zm3.7 8.9H7.3a.9.9 0 0 1 0-1.8h3a.9.9 0 0 1 .4 1.8Zm-.8-3.5H6.6a.9.9 0 0 1 0-1.8h4.3a.9.9 0 0 1 0 1.8ZM5.2 11.5a.9.9 0 0 1-.9-.9V5.4a.9.9 0 0 1 1.8 0v5.2a.9.9 0 0 1-.9.9Z" />
  </svg>;
}

interface StarBadgeProps {
  platform: 'github' | 'gitee';
  count: number | null;
  repoUrl: string;
}

function StarBadge({ platform, count, repoUrl }: StarBadgeProps) {
  const [starred, setStarred] = useState(() => localStorage.getItem(`jc_starred_${platform}`) === 'true');
  const label = platform === 'github' ? 'GitHub' : 'Gitee';
  const note = platform === 'github'
    ? '在 GitHub 上为 joyCode2api-VABoost 加星，查看源码与更新'
    : '在 Gitee 上为 joyCode2api-VABoost 加星，国内访问更快';
  return <Tooltip title={<>{note}<br />点击进入仓库页面，登录后一键 Star</>}>
    <a className={`jc-star-badge${starred ? ' jc-starred' : ''}`} href={repoUrl} target="_blank" rel="noopener noreferrer"
      aria-label={`${label} 星标，当前 ${count ?? '…'} 颗星`}
      onClick={() => { setStarred(true); localStorage.setItem(`jc_starred_${platform}`, 'true'); }}>
      {platform === 'github' ? <GithubGlyph /> : <GiteeGlyph />}
      <span className="jc-star-count">{count === null ? '…' : count.toLocaleString()}</span>
      {starred ? <StarFilled className="jc-star-icon" /> : <StarOutlined className="jc-star-icon" />}
    </a>
  </Tooltip>;
}

export default function RepoStarBadges() {
  const [state, setState] = useState<{ github: number | null; gitee: number | null; repos: { github: string; gitee: string } }>({
    github: null, gitee: null,
    repos: { github: 'https://github.com/variyaone/JoyCode2api-VABoost', gitee: 'https://gitee.com/variyaone/JoyCode2api-VABoost' },
  });
  useEffect(() => {
    const controller = new AbortController();
    api.getRepoStars(controller.signal)
      .then(data => setState({ github: data.github, gitee: data.gitee, repos: data.repos }))
      .catch(() => { /* badges stay with placeholder counts */ });
    return () => controller.abort();
  }, []);
  return <div className="jc-star-badges" role="group" aria-label="仓库星标">
    <StarBadge platform="github" count={state.github} repoUrl={state.repos.github} />
    <StarBadge platform="gitee" count={state.gitee} repoUrl={state.repos.gitee} />
  </div>;
}
