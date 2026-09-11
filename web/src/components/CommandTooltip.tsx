// Modified by Variya 2026-09-11: restrained light UI and readable command syntax.
import React from 'react';
import { Tooltip } from 'antd';

interface CommandTooltipProps {
  command: string;
  label: string;
  children: React.ReactElement;
}

const highlightShell = (cmd: string): React.ReactNode[] => {
  return cmd.split('\n').map((line, lineIdx, lines) => {
    const envMatch = line.match(/^([A-Z_]+)(=)(.*?)(\s*\\)?$/);
    const cmdMatch = line.match(/^(claude|codex)(\s+.*)?$/);
    let nodes: React.ReactNode[];

    if (envMatch) {
      const [, key, eq, value, trailing] = envMatch;
      nodes = [
        <span key="k" style={{ color: '#5966A6' }}>{key}</span>,
        <span key="e" style={{ color: '#646B78' }}>{eq}</span>,
        <span key="v" style={{ color: '#252A34' }}>{value}</span>,
      ];
      if (trailing) nodes.push(<span key="t" style={{ color: '#646B78' }}>{trailing}</span>);
    } else if (cmdMatch) {
      const [, cmd, rest] = cmdMatch;
      nodes = [
        <span key="c" style={{ color: '#5966A6', fontWeight: 600 }}>{cmd}</span>,
        <span key="r" style={{ color: '#252A34' }}>{rest || ''}</span>,
      ];
    } else {
      nodes = [<span key="t" style={{ color: '#252A34' }}>{line}</span>];
    }

    const isLast = lineIdx === lines.length - 1;
    return (
      <React.Fragment key={lineIdx}>
        {nodes}
        {!isLast && '\n'}
      </React.Fragment>
    );
  });
};

const CommandTooltip: React.FC<CommandTooltipProps> = ({ command, label, children }) => (
  <Tooltip
    title={
      <div style={{ padding: '4px 0' }}>
        <div style={{ fontSize: 12, color: '#646B78', marginBottom: 4 }}>
          {label} 命令（点击复制）
        </div>
        <pre style={{
          margin: 0, fontFamily: 'var(--jc-font-code)',
          fontSize: 12, lineHeight: 1.6, whiteSpace: 'pre-wrap', color: '#252A34',
          background: '#F5F6F8', border: '1px solid #E1E4EA', borderRadius: 4,
          padding: '6px 8px', overflowWrap: 'anywhere',
        }}>
          {highlightShell(command)}
        </pre>
      </div>
    }
    color="#FFFFFF"
    styles={{ root: { maxWidth: 'min(520px, calc(100vw - 24px))' }, container: { color: '#252A34', border: '1px solid #E1E4EA' } }}
    placement="left"
  >
    {children}
  </Tooltip>
);

export default CommandTooltip;
