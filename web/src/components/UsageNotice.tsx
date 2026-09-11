import { Typography } from 'antd';

export const usageStatement = '本项目仅供大语言模型（LLM）的学习、技术研究及经授权的测试使用，非 JoyCode 官方产品。使用者应遵守适用法律法规、服务提供方条款及所在组织的信息安全、保密和数据处理规定。未经合法授权及必要审批，不得上传、传输或披露公司机密、个人信息、访问凭据及其他敏感数据；严禁用于违法违规活动或侵犯第三方权益。软件按“现状”提供，不对可用性、准确性或特定用途适用性作出保证。在适用法律允许的范围内，使用者应对其使用行为及后果承担责任，作者及贡献者不对因使用或无法使用本项目而产生的损失承担责任；本声明不排除法律规定不得排除或限制的责任。';

// Variya 2026-09-12: the full statement renders permanently — no collapsed state.
export default function UsageNotice() {
  return <aside aria-label="使用与信息安全声明" style={{padding:16,overflowWrap:'anywhere'}}>
    <Typography.Paragraph type="secondary" style={{fontSize:12,marginBottom:8}}>
      仅限 LLM 学习研究及授权测试 · 注意公司信息安全与保密 · 严禁违法用途
    </Typography.Paragraph>
    <Typography.Paragraph type="secondary" style={{fontSize:12,margin:0}}>{usageStatement}</Typography.Paragraph>
  </aside>;
}
