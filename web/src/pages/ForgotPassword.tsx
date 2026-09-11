// Modified by Variya 2026-09-11: restrained light UI and readable commands.
import React, { useEffect, useState } from 'react';
import { Button, Typography, Alert } from 'antd';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { Link } from 'react-router-dom';
import { authApi } from '../api';
import BrandMark from '../components/BrandMark';
import ProjectCredits from '../components/ProjectCredits';

const { Title, Text, Paragraph } = Typography;

type TokenType = 'prompt' | 'path' | 'subcommand' | 'flag' | 'value';

const TOKEN_COLORS: Record<TokenType, string> = {
  prompt: '#646B78',
  path: '#252A34',
  subcommand: '#5966A6',
  flag: '#5966A6',
  value: '#252A34',
};

interface Token {
  text: string;
  type: TokenType;
}

function tokenizeCommand(cmd: string): Token[] {
  const tokens: Token[] = [];
  const parts = cmd.split(/\s+/);

  for (let i = 0; i < parts.length; i++) {
    const part = parts[i];
    if (i === 0) {
      tokens.push({ text: part, type: 'path' });
    } else if (i === 1) {
      tokens.push({ text: part, type: 'subcommand' });
    } else if (part.startsWith('-')) {
      tokens.push({ text: part, type: 'flag' });
      if (i + 1 < parts.length) {
        tokens.push({ text: parts.slice(i + 1).join(' '), type: 'value' });
        break;
      }
    } else {
      tokens.push({ text: parts.slice(i).join(' '), type: 'value' });
      break;
    }
  }

  return tokens;
}

const codeBlockStyle: React.CSSProperties = {
  background: '#F5F6F8',
  color: '#252A34',
  border: '1px solid #E1E4EA',
  padding: '14px 18px',
  borderRadius: 8,
  marginTop: 8,
  marginBottom: 20,
  fontSize: 14,
  fontFamily: 'var(--jc-font-code)',
  lineHeight: 1.6,
  overflow: 'auto',
  whiteSpace: 'pre-wrap' as const,
  wordBreak: 'break-all' as const,
};

function BashCode({ children }: { children: string }) {
  const tokens = tokenizeCommand(children);
  return (
    <pre style={codeBlockStyle}>
      <span style={{ color: TOKEN_COLORS.prompt, userSelect: 'none' }}>$ </span>
      {tokens.map((t, i) => (
        <React.Fragment key={i}>
          {i > 0 && ' '}
          <span style={{ color: TOKEN_COLORS[t.type] }}>{t.text}</span>
        </React.Fragment>
      ))}
    </pre>
  );
}

const ForgotPasswordPage: React.FC = () => {
  const [exePath, setExePath] = useState('./JoyCode2Api');

  useEffect(() => {
    authApi.status().then((res) => {
      if (res.exe_path) {
        setExePath(res.exe_path);
      }
    }).catch(() => {});
  }, []);

  return (
    <div className="jc-auth-bg">
      <div className="jc-auth-card" style={{ width: 660 }}>
        <div className="jc-auth-logo"><BrandMark size={40} /></div>
        <Title level={3} style={{ marginBottom: 8 }}>忘记密码</Title>
        <Paragraph type="secondary" style={{ marginBottom: 24 }}>
          Dashboard 的 root 密码需要通过服务器命令行重置。
        </Paragraph>

        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 20 }}
          message="在服务器终端执行以下命令"
        />

        <Text strong>交互式重置（会提示你输入新密码）：</Text>
        <BashCode>{`${exePath} reset-password`}</BashCode>

        <Text strong>直接指定新密码：</Text>
        <BashCode>{`${exePath} reset-password -p 你的新密码`}</BashCode>

        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 24 }}
          message={
            <span>
              密码至少 <Text strong>6 位</Text>，以 bcrypt 哈希加密存储在 SQLite 数据库中。
              重置后所有已登录的会话需要重新登录。
            </span>
          }
        />

        <Link to="/login">
          <Button icon={<ArrowLeftOutlined />} type="primary" ghost>
            返回登录
          </Button>
        </Link>
        <ProjectCredits />
      </div>
    </div>
  );
};

export default ForgotPasswordPage;
