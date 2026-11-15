// Model options for the default-model pickers.
//
// The upstream publishes its model list at runtime and it changes without a
// release, so the live catalog is the source of truth. The built-in list
// below is only the fallback shown before the catalog arrives (or when it
// cannot be read) and carries the descriptive labels.
import { useEffect, useMemo, useState } from 'react';
import { api } from './api';
import type { ModelInfo } from './api';

export const BUILTIN_MODELS: ModelInfo[] = [
  { id: 'JoyAI-Code-1.6', name: 'JoyAI-Code-1.6' },
  { id: 'JoyAI-Code-1.5', name: 'JoyAI-Code-1.5' },
  { id: 'Claude-Opus-5', name: 'Claude-Opus-5' },
  { id: 'Claude-Opus-4.8', name: 'Claude-Opus-4.8' },
  { id: 'GLM-5.3', name: 'GLM-5.3' },
  { id: 'JD-GLM-5.3', name: 'JD-GLM-5.3' },
  { id: 'GLM-5.3-jcloud', name: 'GLM-5.3-jcloud' },
  { id: 'JD-GLM-5.2', name: 'JD-GLM-5.2' },
  { id: 'GLM-5.2-jcloud', name: 'GLM-5.2-jcloud' },
  { id: 'Kimi-K3', name: 'Kimi-K3' },
  { id: 'Kimi-K3-jcloud', name: 'Kimi-K3-jcloud' },
  { id: 'JD-Kimi-K3', name: 'JD-Kimi-K3' },
  { id: 'Kimi-K3.1-Pro-Highspeed', name: 'Kimi-K3.1-Pro-Highspeed' },
  { id: 'DeepSeek-V4-Pro', name: 'DeepSeek-V4-Pro' },
  { id: 'JD-DeepSeek-V4-Pro', name: 'JD-DeepSeek-V4-Pro' },
  { id: 'DeepSeek-V4.1-Flash-1017', name: 'DeepSeek-V4.1-Flash-1017' },
  { id: 'MiniMax-M3', name: 'MiniMax-M3' },
  { id: 'Qwen 3.8-Max', name: 'Qwen 3.8-Max' },
  { id: 'GPT-6.1 Sol', name: 'GPT-6.1 Sol' },
  { id: 'GPT-6 Astra', name: 'GPT-6 Astra' },
  { id: 'GPT-5.6 Sol', name: 'GPT-5.6 Sol' },
  { id: 'Doubao-Seed-2.0-pro', name: 'Doubao-Seed-2.0-pro' },
];

// Descriptive labels for the built-in entries, shown until the live catalog
// replaces them.
const BUILTIN_LABELS: Record<string, string> = {
  'JoyAI-Code-1.6': 'JoyAI-Code-1.6 — 主力代码模型（推荐）',
  'JoyAI-Code-1.5': 'JoyAI-Code-1.5 — 主力代码模型（推荐）',
  'Claude-Opus-5': 'Claude-Opus-5 — Anthropic Opus 5',
  'Claude-Opus-4.8': 'Claude-Opus-4.8 — Anthropic Opus 4.8',
  'GLM-5.3': 'GLM-5.3 — 智谱 GLM 5.3',
  'Kimi-K3': 'Kimi-K3 — Moonshot Kimi K3',
  'MiniMax-M3': 'MiniMax-M3',
  'GPT-6 Astra': 'GPT-6 Astra — OpenAI GPT 6',
  'GPT-5.6 Sol': 'GPT-5.6 Sol',
  'Doubao-Seed-2.0-pro': 'Doubao-Seed-2.0-pro — 豆包 Seed 2.0 Pro',
};

// useModelOptions returns the models a client may pick as its default.
// The live upstream catalog is merged over the built-in list: new models
// appear as soon as upstream publishes them.
export function useModelOptions() {
  const [catalog, setCatalog] = useState<ModelInfo[] | null>(null);

  useEffect(() => {
    let alive = true;
    api.getModelCatalog()
      .then(c => {
        if (alive && c.models.length > 0) setCatalog(c.models);
      })
      .catch(() => {});
    return () => { alive = false; };
  }, []);

  return useMemo(() => {
    const options: { label: string; value: string }[] = BUILTIN_MODELS.map(m => ({
      label: BUILTIN_LABELS[m.id] ?? m.id,
      value: m.id,
    }));
    const seen = new Set(options.map(o => o.value));
    for (const m of catalog ?? []) {
      if (seen.has(m.id)) continue;
      seen.add(m.id);
      options.push({
        label: m.chat_api_model && m.chat_api_model !== m.id
          ? `${m.id} · 上游 id ${m.chat_api_model}`
          : m.id,
        value: m.id,
      });
    }
    return options;
  }, [catalog]);
}
