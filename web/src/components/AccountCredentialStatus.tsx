import { CheckCircleOutlined, CloseCircleOutlined, QuestionCircleOutlined } from '@ant-design/icons';
import { Tag, Tooltip, Typography } from 'antd';
import type { Account } from '../api';

export function credentialState(value: unknown): 'passed' | 'failed' | 'unknown' {
  if (value === 1) return 'passed';
  if (value === 0) return 'failed';
  return 'unknown';
}

export interface AccountCredentialStatusProps {
  account: Pick<Account, 'credential_valid' | 'credential_checked_at' | 'provider'>;
  compact?: boolean;
}

const states = {
  passed: { label: '上次校验通过', color: 'success', icon: <CheckCircleOutlined aria-hidden /> },
  failed: { label: '上次校验失败', color: 'error', icon: <CloseCircleOutlined aria-hidden /> },
  unknown: { label: '尚未验证', color: 'default', icon: <QuestionCircleOutlined aria-hidden /> },
};

function recordTime(value: unknown): string {
  // Preserve known backend timestamp formats verbatim, without guessing a timezone
  // or relying on browser-specific parsing of SQLite's local datetime strings.
  if (typeof value !== 'string') return '未知';
  const timestamp = value.trim();
  const date = '\\d{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\\d|3[01])';
  const time = '(?:[01]\\d|2[0-3]):[0-5]\\d:[0-5]\\d';
  const sqlite = new RegExp(`^${date} ${time}$`);
  const rfc3339 = new RegExp(`^${date}T${time}(?:\\.\\d+)?(?:Z|[+-](?:[01]\\d|2[0-3]):[0-5]\\d)$`);
  return sqlite.test(timestamp) || rfc3339.test(timestamp) ? timestamp : '未知';
}

export default function AccountCredentialStatus({ account, compact = false }: AccountCredentialStatusProps) {
  const state = credentialState(account.credential_valid);
  const { label, color, icon } = states[state];
  const time = `记录时间：${recordTime(account.credential_checked_at)}`;
  const isOpenClaw = account.provider === 'openclaw';
  const explanation = isOpenClaw
    ? 'OpenClaw 无账号级凭证：校验走本机京ME桌面端（HiOffice）实时换取 me_token，通过代表后端当前可用。'
    : '后端保存的历史校验记录，不是实时上游探测，不能代表当前上游连通性。';
  const failureExplanation = state === 'failed'
    ? (isOpenClaw
      ? '失败通常意味着京ME桌面端未运行或未登录，启动并登录后重试即可。'
      : '失败可能由网络或认证问题导致，不表示已确认过期。')
    : '';

  return (
    <Tooltip title={`${explanation}${failureExplanation}${time}`} trigger={['hover', 'focus']}>
      <span tabIndex={0} style={{ display: 'inline-flex', flexDirection: 'column', alignItems: 'flex-start', gap: 4 }}>
        <Tag color={color} icon={icon} style={{ marginInlineEnd: 0 }}>{label}</Tag>
        {!compact && <Typography.Text type="secondary" style={{ fontSize: 12 }}>{time}</Typography.Text>}
      </span>
    </Tooltip>
  );
}
