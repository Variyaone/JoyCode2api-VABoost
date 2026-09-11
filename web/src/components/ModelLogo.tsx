// Modified by Variya, 2026-09-11: shared light-surface model identity.
const logos: Record<string, string> = {
  GLM: '/logo-glm.svg', Kimi: '/logo-kimi.svg', Claude: '/logo-claude.svg',
  GPT: '/logo-openai.svg', DeepSeek: '/logo-deepseek.svg', MiniMax: '/logo-minimax.svg',
  Doubao: '/logo-bytedance.svg', JoyAI: '/logo-jd.ico', JoyCode: '/logo-jd.ico',
};
export default function ModelLogo({ model, size = 22 }: { model: string; size?: number }) {
  const prefix = Object.keys(logos).find(key => model.toLowerCase().startsWith(key.toLowerCase()));
  if (!prefix) return null;
  return <img className="jc-model-logo" src={logos[prefix]} alt="" width={size} height={size}
    style={{ flexShrink: 0, objectFit: 'contain' }} />;
}
