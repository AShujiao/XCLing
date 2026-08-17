using System;
using System.Collections.Generic;

namespace XCLing.Wpf.Core
{
    /// <summary>
    /// 错误码到用户可读文案的映射（与 Go 服务层错误码一一对应）。
    /// </summary>
    public static class ErrorMessages
    {
        private static readonly Dictionary<string, string> Map = new Dictionary<string, string>
        {
            ["ADMIN_REQUIRED"] = "请关闭程序后，右键选择“以管理员身份运行”。",
            ["DOMAIN_MANAGED"] = "电脑已加入域，{app} 不会覆盖可能由 GPO 管理的策略。",
            ["EXISTING_SRP"] = "检测到既有 SRP，{app} 拒绝合并或覆盖。",
            ["LEGACY_SRP_DRIFTED"] = "空旧 SRP 配置在备份后发生变化，已中止应用或恢复。",
            ["ALREADY_APPLIED"] = "已有活动策略，请先在系统页恢复。",
            ["OPERATION_IN_PROGRESS"] = "另一个应用或恢复操作尚未完成。",
            ["PREFLIGHT_BLOCKED"] = "草案预检未通过，不能应用。",
            ["SELF_NOT_ALLOWED"] = "{app} 主程序未加入白名单。",
            ["WRITE_FAILED_ROLLED_BACK"] = "写入失败，已恢复到原状态。",
            ["VERIFY_FAILED_ROLLED_BACK"] = "写后校验失败，已恢复到原状态。",
            ["POLICY_DRIFTED"] = "策略已被外部修改，已拒绝自动恢复。",
            ["INVALID_STATE"] = "当前状态不支持此操作，请刷新状态。",
            ["RULE_NOT_REMOVABLE"] = "这条基础规则用于维持系统和恢复能力，不能删除。",
            ["NO_RECOVERY_RECORD"] = "没有可用的恢复记录。",
            ["RECOVERY_FAILED"] = "自动恢复未完成，请按诊断信息人工处理。",
            ["UPDATE_CHECK_FAILED"] = "检查更新失败，请检查网络后重试。",
            ["RPC_CORE_EXITED"] = "核心服务已退出，请重新启动 {app}。",
        };

        // 写入/恢复类失败必须透出底层原因，否则“写入失败”无法定位（原因来自
        // Go 侧 err.Error() 冒号后的诊断文本，同时也会记录在防护事件里）。
        private static readonly HashSet<string> DetailCodes = new HashSet<string>
        {
            "WRITE_FAILED_ROLLED_BACK",
            "VERIFY_FAILED_ROLLED_BACK",
            "RECOVERY_FAILED",
            "LEGACY_SRP_DRIFTED",
            "UPDATE_CHECK_FAILED",
        };

        public static string Humanize(Exception cause, string appName)
        {
            var raw = cause?.Message ?? "";
            var code = cause is RpcException rpc ? rpc.Code : raw.Split(':')[0];
            if (!Map.TryGetValue(code, out var text))
            {
                return raw;
            }
            text = text.Replace("{app}", appName);
            if (DetailCodes.Contains(code))
            {
                var colon = raw.IndexOf(':');
                var detail = colon >= 0 ? raw.Substring(colon + 1).Trim() : "";
                if (detail.Length > 0)
                {
                    text += "（诊断：" + detail + "）";
                }
            }
            return text;
        }
    }
}
