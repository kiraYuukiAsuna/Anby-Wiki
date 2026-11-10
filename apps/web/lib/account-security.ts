import { z } from "zod";
import { ResponseError } from "../../../contracts/generated/typescript";

export const SECURITY_ACTION_LABELS: Record<string, string> = {
  verify_email: "发送验证邮件",
  change_email: "变更邮箱",
  setup_mfa: "绑定验证器",
  enable_mfa: "确认启用二次验证",
  disable_mfa: "停用二次验证",
  regenerate_codes: "重新生成恢复码",
  change_password: "修改密码",
  revoke_session: "撤销设备会话",
  revoke_sessions: "退出所有设备并撤销 CLI 授权",
  verify_email_confirmed: "邮箱验证完成",
  change_email_confirmed: "邮箱变更完成",
  reset_password_requested: "申请密码恢复",
  reset_password: "密码已恢复",
  mfa_login_verified: "二次验证登录",
  mfa_recovery_code_used: "使用一次性恢复码",
  mail_updated: "邮件配置已更新",
  mail_tested: "发送邮件配置测试",
  system_admin_granted: "授予系统管理员",
  system_admin_revoked: "撤销系统管理员",
};

export const newPasswordSchema = z.string().refine(
  (value) => [...value].length >= 12 && [...value].length <= 128,
  "密码须为 12–128 个字符",
);

export async function securityErrorMessage(error: unknown) {
  if (error instanceof ResponseError) {
    const parsed = z.object({ message: z.string() }).safeParse(
      await error.response.clone().json().catch(() => null),
    );
    if (parsed.success) return parsed.data.message;
  }
  return "操作失败，请稍后重试";
}
