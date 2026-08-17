using System.Collections.Generic;
using System.Diagnostics;
using XCLing.Wpf.Models;

namespace XCLing.Wpf.Core
{
    /// <summary>
    /// 构建 WhitelistSelection 的 JSON，供“启用保护”和白名单向导的应用流程共用。
    /// 约定：sidecar 架构下 GUI 与核心服务是两个 exe，二者都必须显式加入白名单，
    /// 否则启用保护后 GUI 无法再启动核心服务（Go 侧仅自保护核心服务的 os.Executable）。
    /// </summary>
    public static class SelectionBuilder
    {
        /// <summary>返回 GUI 主程序与核心服务两条精确文件项，二者都必须在白名单内。</summary>
        public static List<object> SelfPaths(string appName, string corePath)
        {
            return new List<object>
            {
                new { id = "self-gui", path = GuiPath(), kind = "file", label = appName + " 主程序（手动加入）" },
                new { id = "self-core", path = corePath, kind = "file", label = appName + " 核心服务（手动加入）" },
            };
        }

        public static List<object> CompatPaths(Settings settings)
        {
            var paths = new List<object>();
            if (settings.AllowPackagedApps)
            {
                paths.Add(new { id = "compat-packaged-apps", path = @"C:\Program Files\WindowsApps", kind = "directory", label = "Windows 商店应用" });
            }
            if (settings.AllowDefenderUpdates)
            {
                paths.Add(new { id = "compat-defender-platform", path = @"C:\ProgramData\Microsoft\Windows Defender\Platform", kind = "directory", label = "Defender 平台更新" });
                paths.Add(new { id = "compat-defender-definitions", path = @"C:\ProgramData\Microsoft\Windows Defender\Definition Updates", kind = "directory", label = "Defender 定义更新" });
            }
            return paths;
        }

        /// <summary>
        /// 组装默认保护的选择（无勾选应用，self + compat + 用户待启用路径）。用于主控制台“启用保护”。
        /// </summary>
        public static object DefaultSelection(string appName, string corePath, Settings settings, IEnumerable<CustomPathEntry> pending = null)
        {
            var customPaths = SelfPaths(appName, corePath);
            customPaths.AddRange(CompatPaths(settings));
            if (pending != null)
            {
                foreach (var entry in pending)
                {
                    customPaths.Add(new { id = entry.Id, path = entry.Path, kind = entry.Kind, label = entry.Label });
                }
            }
            return new
            {
                apps = new object[0],
                customPaths,
                policyName = appName + " 默认保护",
                adminBypass = false,
            };
        }

        public static string GuiPath()
        {
            try
            {
                return Process.GetCurrentProcess().MainModule.FileName;
            }
            catch
            {
                return typeof(SelectionBuilder).Assembly.Location;
            }
        }
    }
}
