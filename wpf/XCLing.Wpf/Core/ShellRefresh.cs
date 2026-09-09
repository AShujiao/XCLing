using System;
using System.Diagnostics;
using System.Globalization;
using System.Threading;

namespace XCLing.Wpf.Core
{
    /// <summary>
    /// 让资源管理器重新加载 SRP 策略。
    ///
    /// 背景（实测结论）：Windows 的软件限制策略对“进程创建”的判定依赖发起进程
    /// 启动时看到的策略形态。拦截规则内容的变化会被已在运行的进程读到，但
    /// 「从没有拦截规则到出现拦截规则」以及白名单模式下 DefaultLevel 的切换这类
    /// 结构性变化，启动更早的进程不会采用——典型表现就是刚加完规则、双击程序仍然
    /// 能打开，重启资源管理器后立刻生效。
    ///
    /// 因此：策略生效形态变化后记录时间点，若资源管理器比该时间点更早启动，就重启它。
    /// </summary>
    public static class ShellRefresh
    {
        private static readonly object Gate = new object();

        /// <summary>资源管理器是否仍在使用策略变更之前的形态（true 表示双击启动的程序可能不会被拦）。</summary>
        public static bool IsShellStale(Settings settings)
        {
            var pending = ParseUtc(settings != null ? settings.ShellRefreshPendingSinceUtc : null);
            if (pending == null)
            {
                return false;
            }
            var shells = ShellProcesses();
            if (shells.Length == 0)
            {
                return false;
            }
            foreach (var shell in shells)
            {
                try
                {
                    if (shell.StartTime.ToUniversalTime() < pending.Value)
                    {
                        // 只要有一个资源管理器是在策略变更之前启动的，它就可能仍按旧形态放行。
                        return true;
                    }
                }
                catch
                {
                    // 读不到启动时间时按“无需刷新”处理，避免反复打扰用户。
                    return false;
                }
            }
            return false;
        }

        /// <summary>
        /// 记录策略生效形态变化，并在资源管理器尚未加载新形态时重启它。
        /// 返回是否执行了重启。
        /// </summary>
        public static bool MarkAndRefresh(Settings settings)
        {
            if (settings == null)
            {
                return false;
            }
            MarkPending(settings);
            return RefreshIfNeeded(settings);
        }

        /// <summary>仅在需要时重启资源管理器；返回是否执行了重启。</summary>
        public static bool RefreshIfNeeded(Settings settings)
        {
            if (settings == null)
            {
                return false;
            }
            if (!IsShellStale(settings))
            {
                // 资源管理器已经是策略变更之后启动的，标记可以清掉，不必再打扰用户。
                ClearPending(settings);
                return false;
            }
            return TryRefresh(settings, out _);
        }

        /// <summary>重启资源管理器；返回是否成功。</summary>
        public static bool TryRefresh(Settings settings, out string error)
        {
            error = null;
            if (settings == null)
            {
                error = "设置不可用";
                return false;
            }
            return RestartShell(settings, out error);
        }

        private static void MarkPending(Settings settings)
        {
            lock (Gate)
            {
                settings.ShellRefreshPendingSinceUtc = DateTime.UtcNow.ToString("o", CultureInfo.InvariantCulture);
                settings.Save();
            }
        }

        /// <summary>重启资源管理器；成功后清除待刷新标记。</summary>
        private static bool RestartShell(Settings settings, out string error)
        {
            error = null;
            foreach (var shell in ShellProcesses())
            {
                try
                {
                    shell.Kill();
                }
                catch (Exception ex)
                {
                    error = ex.Message;
                }
            }

            // 等旧进程真正退出：没退出就不能声称已刷新，否则标记会被错误清除。
            var deadline = DateTime.UtcNow.AddSeconds(5);
            while (ShellProcesses().Length > 0 && DateTime.UtcNow < deadline)
            {
                Thread.Sleep(100);
            }
            if (ShellProcesses().Length > 0)
            {
                return false;
            }

            // 系统通常会自己拉起资源管理器；没有的话补一个。
            try
            {
                Process.Start(new ProcessStartInfo
                {
                    FileName = "explorer.exe",
                    UseShellExecute = true,
                });
            }
            catch (Exception ex)
            {
                error = ex.Message;
                return false;
            }

            lock (Gate)
            {
                settings.ShellRefreshPendingSinceUtc = "";
                settings.Save();
            }
            return true;
        }

        private static void ClearPending(Settings settings)
        {
            lock (Gate)
            {
                if (string.IsNullOrEmpty(settings.ShellRefreshPendingSinceUtc))
                {
                    return;
                }
                settings.ShellRefreshPendingSinceUtc = "";
                settings.Save();
            }
        }

        private static Process[] ShellProcesses()
        {
            try
            {
                return Process.GetProcessesByName("explorer");
            }
            catch
            {
                return new Process[0];
            }
        }

        private static DateTime? ParseUtc(string value)
        {
            if (string.IsNullOrWhiteSpace(value))
            {
                return null;
            }
            DateTime parsed;
            if (!DateTime.TryParse(value, CultureInfo.InvariantCulture, DateTimeStyles.RoundtripKind, out parsed))
            {
                return null;
            }
            return parsed.ToUniversalTime();
        }
    }
}
