export interface Account {
  user_id: string;
  nickname: string;
  remark: string;
  api_token: string;
  is_default: boolean;
  default_model: string;
  created_at?: string;
  display_order: number;
  active_sessions: number;
  total_requests: number;
  today_requests: number;
  total_tokens: number;
  today_tokens: number;
  credential_valid: number; // -1=unknown, 0=last check failed (not necessarily expired), 1=last check passed
  credential_checked_at?: string;
  credential_refreshed_at?: string;
  credential_error?: string;
}

export function accountDisplayName(a: { nickname?: string; remark?: string; user_id: string }): string {
  if (a.remark) return a.remark;
  if (a.nickname) return a.nickname;
  return a.user_id;
}

export interface ModelInfo {
  id: string;
  name: string;
}

export interface Stats {
  total_requests: number;
  total_input_tokens: number;
  total_output_tokens: number;
  accounts_count: number;
  avg_latency_ms: number;
  error_count: number;
  stream_count: number;
  success_count: number;
  by_model: { model: string; count: number }[];
  by_account: { user_id: string; nickname: string; remark: string; count: number }[];
  all_time: {
    total_requests: number;
    total_input_tokens: number;
    total_output_tokens: number;
    error_count: number;
  };
  hourly: {
    hour: string;
    count: number;
    input_tokens: number;
    output_tokens: number;
    errors: number;
  }[];
}

export interface ModelCapability {
  id: string;
  chat_api_model: string;
  api: 'chat' | 'responses' | 'anthropic';
  vision: boolean;
  reasoning: boolean;
  web_search: boolean;
  image_gen: boolean;
  max_output_tokens: number;
  advertised_ctx: number;
  measured_ctx: number;
  notes?: string;
}

export interface BenchmarkResult {
  source_id: string;
  public_model: string;
  variant: string;
  score: number | null;
  url: string;
  published_at: string | null;
  footnote: string;
}

export interface BenchmarkModel {
  id: string;
  mapping: 'name_match' | 'deployment_reference' | 'version_ambiguous' | 'unverified';
  note: string;
  results: BenchmarkResult[];
}

export interface BenchmarkSnapshot {
  schema_version: number;
  collected_at: string;
  notice: string;
  sources: {
    id: string;
    name: string;
    metric: string;
    version: string;
    unit: string;
    higher_is_better: boolean;
    url: string;
    methodology_url: string;
    status: string;
    description: string;
  }[];
  models: BenchmarkModel[];
}

export interface CostRow {
  day: string; model: string; price_version: string; requests: number;
  input_tokens: number; output_tokens: number; missing_usage: number;
  input_rate: number | null; output_rate: number | null;
  amount_tenth_micro_usd: number | null;
}
export interface CostSnapshot {
  currency: string; price_version: string; collected_at: string;
  today: string; timezone: string; coverage_start: string; notice: string;
  rates: { model: string; input_tenth_micro_usd: number | null; output_tenth_micro_usd: number | null; source: string; url: string; note: string }[];
  rows: CostRow[];
}

export interface UsageActivity {
  from: string;
  through: string;
  today: string;
  timezone: string;
  generated_at: string;
  source: 'request_logs';
  days: {
    date: string;
    coverage: 'matched' | 'partial' | 'unavailable';
    raw_requests: number;
    ledger_requests: number;
    hours: { hour: number; requests: number; input_tokens: number; output_tokens: number }[];
  }[];
}

export interface Settings {
  [key: string]: string;
}

export interface AccountStats {
  user_id: string;
  nickname: string;
  remark: string;
  total_requests: number;
  total_input_tokens: number;
  total_output_tokens: number;
  success_count: number;
  stream_count: number;
  by_model: { model: string; count: number }[];
  by_endpoint: { endpoint: string; count: number }[];
  avg_latency_ms: number;
  error_count: number;
  all_time?: {
    total_requests: number;
    total_input_tokens: number;
    total_output_tokens: number;
    error_count: number;
  };
  hourly: {
    hour: string;
    count: number;
    input_tokens: number;
    output_tokens: number;
    errors: number;
  }[];
}

export interface RequestLog {
  id: number;
  user_id: string;
  model: string;
  endpoint: string;
  stream: boolean;
  status_code: number;
  latency_ms: number;
  error_message: string;
  input_tokens: number;
  output_tokens: number;
  created_at: string;
}

const TOKEN_KEY = 'joycode_jwt';

function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

export function isAuthenticated(): boolean {
  return !!getToken();
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const token = getToken();
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  const resp = await fetch(path, {
    headers,
    ...options,
  });
  if (resp.status === 401) {
    clearToken();
    window.location.href = '/login';
    throw new Error('Unauthorized');
  }
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ detail: resp.statusText }));
    throw new Error(err.detail || `HTTP ${resp.status}`);
  }
  return resp.json();
}

async function authRequest<T>(path: string, options?: RequestInit): Promise<T> {
  const resp = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ detail: resp.statusText }));
    throw new Error(err.detail || `HTTP ${resp.status}`);
  }
  return resp.json();
}

export const authApi = {
  status: () => authRequest<{ initialized: boolean; exe_path?: string }>('/api/auth/status'),
  setup: (password: string) =>
    authRequest<{ ok: boolean; token: string }>('/api/auth/setup', {
      method: 'POST',
      body: JSON.stringify({ password }),
    }),
  login: (password: string) =>
    authRequest<{ ok: boolean; token: string }>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ password }),
    }),
  changePassword: (oldPassword: string, newPassword: string) =>
    request<{ ok: boolean }>('/api/auth/change-password', {
      method: 'POST',
      body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
    }),
};

export const api = {
  listAccounts: (signal?: AbortSignal) => request<{ accounts: Account[] }>('/api/accounts', { signal }).then(r => r.accounts),
  addAccount: (data: { user_id: string; pt_key: string; nickname?: string; is_default?: boolean; default_model?: string }) =>
    request<{ ok: boolean }>('/api/accounts', { method: 'POST', body: JSON.stringify(data) }),
  removeAccount: (userId: string) =>
    request<{ ok: boolean }>(`/api/accounts/${encodeURIComponent(userId)}`, { method: 'DELETE' }),
  setDefault: (userId: string) =>
    request<{ ok: boolean }>(`/api/accounts/${encodeURIComponent(userId)}/default`, { method: 'PUT' }),
  validateAccount: (userId: string) =>
    request<{ valid: boolean }>(`/api/accounts/${encodeURIComponent(userId)}/validate`, { method: 'POST' }),
  listModels: () => request<{ models: ModelInfo[] }>('/api/models').then(r => r.models),
  listAccountModels: (userId: string) =>
    request<{ models: ModelInfo[] }>(`/api/accounts/${encodeURIComponent(userId)}/models`).then(r => r.models),
  getUsageActivity: (from: string, through: string, signal?: AbortSignal) =>
    request<UsageActivity>(`/api/usage-activity?from=${encodeURIComponent(from)}&through=${encodeURIComponent(through)}`, { signal }),
  getCosts: (signal?: AbortSignal) => request<CostSnapshot>('/api/costs', { signal }),
  getModelBenchmarks: (signal?: AbortSignal) => request<BenchmarkSnapshot>('/api/model-benchmarks', { signal }),
  getStats: (signal?: AbortSignal) => request<Stats>('/api/stats', { signal }),
  getSettings: () => request<{ settings: Settings }>('/api/settings').then(r => r.settings),
  updateSettings: (data: Settings) =>
    request<{ ok: boolean }>('/api/settings', { method: 'PUT', body: JSON.stringify(data) }),
  getHealth: (signal?: AbortSignal) => request<{ status: string; accounts: number }>('/api/health', { signal }),
  updateAccountModel: (userId: string, defaultModel: string) =>
    request<{ ok: boolean }>(`/api/accounts/${encodeURIComponent(userId)}/model`, {
      method: 'PUT',
      body: JSON.stringify({ default_model: defaultModel }),
    }),
  getAccountStats: (userId: string) =>
    request<AccountStats>(`/api/accounts/${encodeURIComponent(userId)}/stats`),
  getAccountLogs: (userId: string, limit = 200) =>
    request<{ logs: RequestLog[]; total: number }>(`/api/accounts/${encodeURIComponent(userId)}/logs?limit=${limit}`),
  renewToken: (userId: string) =>
    request<{ ok: boolean; api_token: string }>(`/api/accounts/${encodeURIComponent(userId)}/renew-token`, { method: 'POST' }),
  autoLogin: () =>
    request<{ ok: boolean; user_id: string; nickname: string; real_name: string; is_default: boolean }>('/api/accounts-auto-login', { method: 'POST' }),
  qrLoginInit: () =>
    request<{ ok: boolean; session_id: string; qr_image: string }>('/api/qr-login/init', { method: 'POST' }),
  qrLoginStatus: (sessionId: string) =>
    request<{ status: string; ok?: boolean; user_id?: string; nickname?: string; real_name?: string; message?: string; verify_url?: string; risk_code?: number }>(`/api/qr-login/status?session=${encodeURIComponent(sessionId)}`),
  browserLogin: () =>
    request<{ ok: boolean; url: string; token: string }>('/api/browser-login', { method: 'POST' }),
  oauthSubmit: (ptKey: string) =>
    request<{ ok: boolean; user_id: string; nickname: string }>('/api/oauth-submit', { method: 'POST', body: JSON.stringify({ pt_key: ptKey }) }),
  getRecentErrors: (limit = 50) =>
    request<{ errors: RequestLog[]; total: number }>(`/api/errors?limit=${limit}`),
  getModelCapabilities: (signal?: AbortSignal) =>
    request<{ models: ModelCapability[]; upstream_cap_ctx: number; request_body_cap: number; probed_at: string }>('/api/model-capabilities', { signal }),
  getRecentLogs: (limit = 100, signal?: AbortSignal) =>
    request<{ logs: RequestLog[]; total: number }>(`/api/recent-logs?limit=${limit}`, { signal }),
  getRepoStars: (signal?: AbortSignal) =>
    request<{ github: number; gitee: number; repos: { github: string; gitee: string } }>('/api/github-stars', { signal }),
  clearAllAccounts: () =>
    request<{ ok: boolean; count: number }>('/api/accounts-clear-all', { method: 'POST' }),
  clearJoyCodeSession: () =>
    request<{ ok: boolean; message: string }>('/api/clear-joycode-session', { method: 'POST' }),
  updateRemark: (userId: string, remark: string) =>
    request<{ ok: boolean }>(`/api/accounts/${encodeURIComponent(userId)}/remark`, { method: 'PUT', body: JSON.stringify({ remark }) }),
  reorderAccounts: (userIds: string[]) =>
    request<{ ok: boolean }>('/api/accounts/reorder', { method: 'PUT', body: JSON.stringify({ user_ids: userIds }) }),
  exportAccounts: () =>
    request<{ ok: boolean; accounts: Array<{ user_id: string; nickname: string; remark: string; pt_key: string; is_default: boolean; default_model: string; display_order: number }>; count: number }>('/api/accounts-export'),
  importAccounts: (accounts: Array<{ user_id: string; nickname: string; remark: string; pt_key: string; is_default: boolean; default_model: string; display_order: number }>) =>
    request<{ ok: boolean; added: number; updated: number; total: number }>('/api/accounts-import', { method: 'POST', body: JSON.stringify({ accounts }) }),
};
