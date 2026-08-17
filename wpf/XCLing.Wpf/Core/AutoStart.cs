using System;
using System.Diagnostics;
using System.IO;
using Microsoft.Win32;

namespace XCLing.Wpf.Core
{
    /// <summary>
    /// 开机自启动管理，逻辑对齐 Go 版 autostart_windows.go：
    ///   1) 首选“登录触发 + 最高权限”的计划任务（schtasks，需管理员）；
    ///   2) 无权限注册任务时回退到 HKCU Run 键（登录后以普通权限进入托盘）；
    ///   3) 偏好持久化到 HKCU\Software\XCLing\AutoStartEnabled。
    ///
    /// 注意：这里启动的是 WPF GUI 主程序（本进程），不是核心服务；故自启动天然是 GUI 侧职责，
    /// 不经 sidecar。任务名与 Run 值沿用 XCLing。
    /// </summary>
    public static class AutoStart
    {
        private const string RunKeyPath = @"Software\Microsoft\Windows\CurrentVersion\Run";
        private const string RunValueName = "XCLing";
        private const string TaskName = "XCLing";
        private const string PreferencePath = @"Software\XCLing";
        private const string PreferenceName = "AutoStartEnabled";

        /// <summary>启动时应用已保存的偏好（默认启用）。返回失败信息，成功返回 null。</summary>
        public static string ApplyPreference()
        {
            bool enabled = true;
            try
            {
                using (var key = Registry.CurrentUser.OpenSubKey(PreferencePath))
                {
                    if (key != null)
                    {
                        var value = key.GetValue(PreferenceName);
                        if (value is int intValue)
                        {
                            enabled = intValue != 0;
                        }
                    }
                }
            }
            catch (Exception ex)
            {
                return "无法读取开机自启动设置：" + ex.Message;
            }
            return SetEnabled(enabled);
        }

        /// <summary>两种机制中任一已注册即视为启用。</summary>
        public static bool IsEnabled()
        {
            return LogonTaskExists() || RunValueExists();
        }

        /// <summary>
        /// 读取已持久化的开机自启动偏好（默认启用）。只读 HKCU，不触发子进程，
        /// 用作设置页勾选框的可靠状态源——IsEnabled 依赖 schtasks /Query，
        /// 在 Win7 上对“最高权限”任务可能误报，不能作为界面状态。
        /// </summary>
        public static bool GetPreference()
        {
            try
            {
                using (var key = Registry.CurrentUser.OpenSubKey(PreferencePath))
                {
                    var value = key?.GetValue(PreferenceName);
                    if (value is int intValue)
                    {
                        return intValue != 0;
                    }
                }
            }
            catch
            {
                // 读取失败按默认启用处理。
            }
            return true;
        }

        /// <summary>持久化偏好并切换两种机制。返回失败信息，成功返回 null。</summary>
        public static string SetEnabled(bool enabled)
        {
            if (!enabled)
            {
                var disableErr = Disable();
                if (disableErr != null)
                {
                    return disableErr;
                }
            }
            else
            {
                var executable = SelectionBuilder.GuiPath();
                if (RegisterLogonTask(executable))
                {
                    DeleteRunValue(); // 任务已生效，移除 Run 键避免双重启动
                }
                else if (!LogonTaskExists())
                {
                    var runErr = SetRunValue("\"" + executable + "\" --minimized");
                    if (runErr != null)
                    {
                        return runErr;
                    }
                }
                // 已有任务仍会自启动（本次无权限更新任务），无需额外处理。
            }
            // 偏好只在注册/移除成功后才持久化，避免“偏好已关但任务仍在”的漂移，
            // 保证勾选框状态与实际自启动机制一致。
            return SavePreference(enabled);
        }

        private static string Disable()
        {
            DeleteLogonTask();
            DeleteRunValue();
            if (LogonTaskExists())
            {
                return "无法删除开机自启动计划任务（可能需要管理员权限）。";
            }
            return null;
        }

        private static string SavePreference(bool enabled)
        {
            try
            {
                using (var key = Registry.CurrentUser.CreateSubKey(PreferencePath))
                {
                    key.SetValue(PreferenceName, enabled ? 1 : 0, RegistryValueKind.DWord);
                }
                return null;
            }
            catch (Exception ex)
            {
                return "无法保存开机自启动设置：" + ex.Message;
            }
        }

        private static bool RegisterLogonTask(string executable)
        {
            return RunSchtasks(
                "/Create", "/F",
                "/TN", TaskName,
                "/SC", "ONLOGON",
                "/RL", "HIGHEST",
                "/TR", "\"" + executable + "\" --minimized") == 0;
        }

        private static void DeleteLogonTask()
        {
            RunSchtasks("/Delete", "/F", "/TN", TaskName);
        }

        private static bool LogonTaskExists()
        {
            if (RunSchtasks("/Query", "/TN", TaskName) == 0)
            {
                return true;
            }
            // Win7 兼容：schtasks /Query 对“最高权限”任务在未提权进程里可能返回
            // 访问拒绝而非“找不到”，导致误判任务不存在。用任务定义文件是否存在兜底
            // （schtasks /Create /TN 无子文件夹时写入 %SystemRoot%\System32\Tasks\<任务名>）。
            try
            {
                var tasksDir = Path.Combine(
                    Environment.GetFolderPath(Environment.SpecialFolder.System), "Tasks");
                return File.Exists(Path.Combine(tasksDir, TaskName));
            }
            catch
            {
                return false;
            }
        }

        private static int RunSchtasks(params string[] args)
        {
            Process process = null;
            try
            {
                var psi = new ProcessStartInfo
                {
                    FileName = "schtasks.exe",
                    UseShellExecute = false,
                    CreateNoWindow = true,
                    RedirectStandardOutput = true,
                    RedirectStandardError = true,
                };
                foreach (var arg in args)
                {
                    psi.Arguments += QuoteArg(arg) + " ";
                }
                process = Process.Start(psi);
                if (process == null)
                {
                    return -1;
                }
                process.StandardOutput.ReadToEnd();
                process.StandardError.ReadToEnd();
                if (!process.WaitForExit(15000))
                {
                    try { process.Kill(); } catch { }
                    return -1;
                }
                return process.ExitCode;
            }
            catch
            {
                return -1;
            }
            finally
            {
                process?.Dispose();
            }
        }

        // schtasks /TR 的值本身含引号，整体作为一个参数时用引号包裹并转义内部引号。
        private static string QuoteArg(string arg)
        {
            if (arg.IndexOf(' ') < 0 && arg.IndexOf('"') < 0)
            {
                return arg;
            }
            return "\"" + arg.Replace("\"", "\\\"") + "\"";
        }

        private static bool RunValueExists()
        {
            try
            {
                using (var key = Registry.CurrentUser.OpenSubKey(RunKeyPath))
                {
                    return key != null && key.GetValue(RunValueName) != null;
                }
            }
            catch
            {
                return false;
            }
        }

        private static string SetRunValue(string command)
        {
            try
            {
                using (var key = Registry.CurrentUser.CreateSubKey(RunKeyPath))
                {
                    key.SetValue(RunValueName, command, RegistryValueKind.String);
                }
                return null;
            }
            catch (Exception ex)
            {
                return "无法启用开机自启动：" + ex.Message;
            }
        }

        private static void DeleteRunValue()
        {
            try
            {
                using (var key = Registry.CurrentUser.OpenSubKey(RunKeyPath, writable: true))
                {
                    key?.DeleteValue(RunValueName, throwOnMissingValue: false);
                }
            }
            catch
            {
                // 删除失败忽略。
            }
        }
    }
}
