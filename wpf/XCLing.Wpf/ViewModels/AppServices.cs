using System;
using System.Collections.ObjectModel;
using XCLing.Wpf.Core;
using XCLing.Wpf.Models;

namespace XCLing.Wpf.ViewModels
{
    /// <summary>
    /// 传给各页面视图模型的共享依赖：sidecar 适配器、品牌名、确认框、Toast、设置与导航。
    /// 集中在一处，避免每个页面视图模型各自重复注入。
    /// </summary>
    public sealed class AppServices
    {
        public AppServices(
            GoApi api,
            string appName,
            string coreVersion,
            Settings settings,
            Func<ConfirmRequest, bool> confirm,
            Action<string, bool> toast,
            Action<string> navigate,
            Action showDonate)
        {
            Api = api;
            AppName = appName;
            CoreVersion = coreVersion;
            Settings = settings;
            Confirm = confirm;
            Toast = toast;
            Navigate = navigate;
            ShowDonate = showDonate;
        }

        public GoApi Api { get; }
        public string AppName { get; }
        public string CoreVersion { get; }
        public Settings Settings { get; }
        public Func<ConfirmRequest, bool> Confirm { get; }
        public Action<string, bool> Toast { get; }
        public Action<string> Navigate { get; }
        public Action ShowDonate { get; }

        /// <summary>
        /// 保护未启用时用户在“规则维护”页添加的待启用路径，启用保护时一并纳入草案。
        /// 与 Vue 版共享 whitelist store 的 customPaths 行为对齐（跨页面共享）。
        /// </summary>
        public ObservableCollection<CustomPathEntry> PendingPaths { get; } = new ObservableCollection<CustomPathEntry>();
    }
}

