using System;
using System.Collections.ObjectModel;
using System.Globalization;
using System.Linq;
using System.Threading.Tasks;
using System.Windows.Input;
using Newtonsoft.Json.Linq;
using XCLing.Wpf.Core;
using XCLing.Wpf.Models;

namespace XCLing.Wpf.ViewModels
{
    public sealed class ConfirmRequest
    {
        public string Title { get; set; }
        public string Message { get; set; }
        public string ConfirmText { get; set; }
        public bool Danger { get; set; }
    }

    /// <summary>
    /// 主控制台：状态读取 + 白名单/黑名单模式启用 + 临时解锁 / 重新锁定 / 恢复原状。
    /// </summary>
    public sealed class ConsoleViewModel : ViewModelBase, IPageViewModel
    {
        private readonly AppServices _svc;
        private ApplyStatus _status;
        private bool _busy;
        private string _errorText = "";
        /// <summary>当前提示是否由操作失败产生：为真时，操作后的状态刷新不清除它。</summary>
        private bool _actionError;
        private string _recentError = "";
        private bool _loading;
        private bool _whitelistSelected = true;

        public ConsoleViewModel(AppServices services)
        {
            _svc = services ?? throw new ArgumentNullException(nameof(services));
            PrimaryCommand = new AsyncRelayCommand(RunPrimaryAsync, () => PrimaryEnabled, HandleCommandError);
            RefreshCommand = new AsyncRelayCommand(RefreshCoreAsync, () => !Busy, HandleCommandError);
            RestoreCommand = new AsyncRelayCommand(RunRestoreAsync, () => CanRestore, HandleCommandError);
            ShowOperationsCommand = new RelayCommand(() => _svc.Navigate("activity-operations"));
            RefreshShellCommand = new RelayCommand(RunShellRefresh);
            EnableSelectedCommand = new AsyncRelayCommand(
                () => WhitelistSelected ? RunEnableWhitelistAsync() : RunEnableBlacklistAsync(),
                () => CanEnableMode, HandleCommandError);
        }

        public string Key => "console";

        public ICommand PrimaryCommand { get; }
        public ICommand RefreshCommand { get; }
        public ICommand RestoreCommand { get; }
        public ICommand ShowOperationsCommand { get; }
        public ICommand EnableSelectedCommand { get; }
        public ICommand RefreshShellCommand { get; }
        /// <summary>资源管理器尚未加载最新策略：双击启动的程序可能不会被拦截。</summary>
        public bool ShellRefreshPending => ShellRefresh.IsShellStale(_svc.Settings);
        public ObservableCollection<ProtectionEvent> RecentOperations { get; } = new ObservableCollection<ProtectionEvent>();
        public bool HasRecentOperations => RecentOperations.Count > 0;
        /// <summary>最近操作列表的空状态：加载中或读取失败时不显示「暂无」。</summary>
        public bool ShowEmptyRecent => !Loading && !Busy && string.IsNullOrEmpty(RecentError) && !HasRecentOperations;
        public string RecentError
        {
            get => _recentError;
            private set { if (Set(ref _recentError, value)) Raise(nameof(ShowEmptyRecent)); }
        }
        public bool Loading { get => _loading; private set { if (Set(ref _loading, value)) RaiseAll(); } }
        public bool WhitelistSelected
        {
            get => _whitelistSelected;
            set { if (Set(ref _whitelistSelected, value)) { Raise(nameof(BlacklistSelected)); Raise(nameof(EnableSelectedText)); Raise(nameof(SelectedModeDescription)); } }
        }
        public bool BlacklistSelected { get => !WhitelistSelected; set { if (value) WhitelistSelected = false; } }
        public string EnableSelectedText => Busy ? "处理中..." : (WhitelistSelected ? "启用白名单模式" : "启用黑名单模式");
        public string SelectedModeDescription => WhitelistSelected ? "仅系统目录和白名单中的程序允许运行。" : "默认允许程序运行，仅拦截黑名单中的程序。";
        public bool ShowModeSelection => Status != null && !IsManaged;

        public ApplyStatus Status
        {
            get { return _status; }
            private set { _status = value; RaiseAll(); }
        }

        public bool Busy
        {
            get { return _busy; }
            private set { if (Set(ref _busy, value)) { RaiseAll(); } }
        }

        public string ErrorText
        {
            get { return _errorText; }
            private set { if (Set(ref _errorText, value)) RaiseAll(); }
        }

        public string State => Status != null && !string.IsNullOrEmpty(Status.ProtectionState)
            ? Status.ProtectionState
            : (string.IsNullOrEmpty(ErrorText) ? "loading" : "unavailable");

        public string StateLabel
        {
            get
            {
                var blockOnly = Status != null && Status.PolicyMode == "blacklist";
                switch (State)
                {
                    case "loading": return "正在读取策略";
                    case "unavailable": return "状态读取失败";
                    case "locked": return blockOnly ? "拦截中" : "保护中";
                    case "unlocked": return "临时解锁";
                    case "attention": return "需要处理";
                    default: return "尚未启用";
                }
            }
        }

        public string StateDescription => Status != null ? Status.Reason : "正在读取策略状态...";

        public string PrimaryText
        {
            get
            {
                if (Busy)
                {
                    return "处理中...";
                }
                switch (State)
                {
                    case "locked": return "临时解锁";
                    case "unlocked": return "重新锁定";
                    case "attention": return "从备份恢复";
                    default: return "启用保护";
                }
            }
        }

        /// <summary>已接管时操作当前策略，未启用时使用模式选择器。</summary>
        public bool ShowPrimary => IsManaged;
        public bool PrimaryEnabled => !Busy && !Loading && Status != null &&
            ((State == "locked" && Status.CanUnlock) || (State == "unlocked" && Status.CanLock) || (State == "attention" && Status.CanRestore));
        public bool CanRestore => !Busy && !Loading && Status != null && Status.CanRestore;

        public bool IsManaged => Status != null && State != "unmanaged";
        public bool IsBlacklistActive => IsManaged && Status != null && Status.PolicyMode == "blacklist";
        /// <summary>旧恢复记录无 policyMode 字段，后端按白名单处理，这里保持一致。</summary>
        public bool IsWhitelistActive => IsManaged && !IsBlacklistActive;
        public string ModeLabel => IsBlacklistActive ? "黑名单模式" : (IsWhitelistActive ? "白名单模式" : "未启用");
        public bool CanEnableMode => !Busy && !Loading && !IsManaged && Status != null && Status.CanApply;

        public string RuleCountText => Status == null ? "—" : Status.ExistingRuleCount + " 条";
        public string AdminText => Status == null ? "—" : (Status.IsAdmin ? "管理员" : "需要管理员权限");
        public string BackupText => Status == null ? "—" : FormatTime(Status.BackupCreatedAt);
        public string SourceText => Status == null ? "—" : (Status.DomainJoined ? "域策略" : "本机策略");

        public Task OnActivatedAsync() => RefreshCoreAsync(true);

        /// <summary>供托盘等外部路径修改策略后刷新界面。</summary>
        public Task RefreshAsync() => RefreshCoreAsync(true);

        /// <summary>状态读取失败：下一次成功的状态读取会覆盖它。</summary>
        private void SetStatusError(string message)
        {
            _actionError = false;
            ErrorText = message;
        }

        /// <summary>操作失败：保留到用户下一次显式刷新或重新发起操作。</summary>
        private void SetActionError(string message)
        {
            _actionError = true;
            ErrorText = message;
        }

        private void ClearError()
        {
            _actionError = false;
            ErrorText = "";
        }

        /// <summary>手动重启资源管理器，让当前策略对双击启动的程序立即生效。</summary>
        private void RunShellRefresh()
        {
            string error;
            if (ShellRefresh.TryRefresh(_svc.Settings, out error))
            {
                _svc.Toast("已重启资源管理器，新策略对双击启动的程序即时生效", false);
            }
            else
            {
                _svc.Toast("重启资源管理器失败：" + (string.IsNullOrEmpty(error) ? "未知原因" : error), true);
            }
            Raise(nameof(ShellRefreshPending));
        }

        /// <summary>操作完成后的刷新：默认保留操作失败提示，避免刚报的错被状态读取成功清掉。</summary>
        private Task RefreshCoreAsync() => RefreshCoreAsync(false);

        private async Task RefreshCoreAsync(bool clearActionError)
        {
            if (Loading) return;
            Loading = true;
            try
            {
                Status = await _svc.Api.GetApplyStatus();
                if (clearActionError || !_actionError)
                {
                    ClearError();
                }
            }
            catch (Exception ex)
            {
                Status = null;
                SetStatusError(ErrorMessages.Humanize(ex, _svc.AppName));
            }
            finally { Loading = false; }

            try
            {
                var events = await _svc.Api.ListProtectionEvents();
                RecentOperations.Clear();
                foreach (var item in (events ?? new System.Collections.Generic.List<ProtectionEvent>()).Take(4)) RecentOperations.Add(item);
                RecentError = "";
            }
            catch (Exception ex)
            {
                RecentOperations.Clear();
                RecentError = "操作记录暂不可用：" + ErrorMessages.Humanize(ex, _svc.AppName);
            }
            Raise(nameof(HasRecentOperations));
            Raise(nameof(ShowEmptyRecent));
        }

        private async Task RunPrimaryAsync()
        {
            switch (State)
            {
                case "locked":
                    await TransitionAsync(false);
                    break;
                case "unlocked":
                    await TransitionAsync(true);
                    break;
                case "attention":
                    if (_svc.Confirm(new ConfirmRequest
                    {
                        Title = "恢复接管前状态",
                        Message = "当前策略发生过外部变化。继续将以接管前备份覆盖当前 SRP。",
                        ConfirmText = "恢复原状",
                        Danger = true,
                    }))
                    {
                        await RestoreOriginalAsync(true);
                    }
                    break;
            }
        }

        private async Task RunEnableWhitelistAsync()
        {
            var takeOver = Status != null && Status.CanTakeOver;
            var ruleCount = Status != null ? Status.ExistingRuleCount : 0;
            if (_svc.Confirm(new ConfirmRequest
            {
                Title = takeOver ? "备份并启用白名单模式" : "启用白名单模式",
                Message = takeOver
                    ? "将先完整备份现有 SRP（检测到 " + ruleCount + " 条规则），再启用白名单保护：仅系统目录与白名单中的程序允许运行。"
                    : "将启用可信目录策略，阻止从下载、临时目录和其他用户可写位置启动程序。",
                ConfirmText = takeOver ? "备份并启用" : "启用白名单模式",
                Danger = false,
            }))
            {
                await EnableProtectionAsync();
            }
        }

        private async Task RunEnableBlacklistAsync()
        {
            if (_svc.Confirm(new ConfirmRequest
            {
                Title = "启用黑名单模式",
                Message = "将完整备份现有 SRP 并启用黑名单模式：所有程序默认放行，仅拦截「黑名单」中的程序。可随时「恢复原状」。",
                ConfirmText = "启用黑名单模式",
                Danger = false,
            }))
            {
                await EnableBlockOnlyAsync();
            }
        }

        private async Task RunRestoreAsync()
        {
            if (_svc.Confirm(new ConfirmRequest
            {
                Title = "恢复接管前状态",
                Message = _svc.AppName + " 的活动策略将被移除，并完整恢复接管前状态。",
                ConfirmText = "恢复原状",
                Danger = true,
            }))
            {
                await RestoreOriginalAsync(false);
            }
        }

        private async Task EnableProtectionAsync()
        {
            Busy = true;
            ClearError();
            try
            {
                var selection = SelectionBuilder.DefaultSelection(_svc.AppName, _svc.Api.CorePath, _svc.Settings, _svc.PendingPaths);
                JObject draft = await _svc.Api.BuildWhitelistDraft(Json.Serialize(selection));
                var draftJson = draft.ToString(Newtonsoft.Json.Formatting.None);

                PreflightReport report = await _svc.Api.PreflightWhitelistDraft(draftJson);
                if (report != null && report.Blocked)
                {
                    SetActionError("草案预检未通过：" + FirstBlockMessage(report));
                    _svc.Toast(ErrorText, true);
                    return;
                }

                ApplyResult result = await _svc.Api.EnableProtection(draftJson);
                _svc.Toast(result != null ? result.Message : "保护已启用", false);
                // 白名单模式把 DefaultLevel 改成 Disallowed：资源管理器必须重新加载才会拦住双击启动的程序。
                await _svc.NotifyPolicyShapeChangedAsync();
                Raise(nameof(ShellRefreshPending));
            }
            catch (Exception ex)
            {
                SetActionError(ErrorMessages.Humanize(ex, _svc.AppName));
                _svc.Toast(ErrorText, true);
            }
            finally
            {
                Busy = false;
                await RefreshCoreAsync();
            }
        }

        private async Task EnableBlockOnlyAsync()
        {
            Busy = true;
            ClearError();
            try
            {
                ApplyResult result = await _svc.Api.EnableBlockOnlyProtection();
                _svc.Toast(result != null && !string.IsNullOrEmpty(result.Message) ? result.Message : "黑名单模式已启用", false);
            }
            catch (Exception ex)
            {
                SetActionError(ErrorMessages.Humanize(ex, _svc.AppName));
                _svc.Toast(ErrorText, true);
            }
            finally
            {
                Busy = false;
                await RefreshCoreAsync();
            }
        }

        private async Task TransitionAsync(bool toLock)
        {
            // 白名单模式的锁定/解锁切换的是 DefaultLevel，属于策略生效形态变化；
            // 黑名单模式只是增删拦截规则，已在运行的进程会读到规则变化，无需重启资源管理器。
            var shapeChanged = !IsBlacklistActive;
            Busy = true;
            ClearError();
            try
            {
                ProtectionResult result = toLock
                    ? await _svc.Api.LockProtection()
                    : await _svc.Api.UnlockProtection();
                _svc.Toast(result != null ? result.Message : "", false);
                if (shapeChanged)
                {
                    await _svc.NotifyPolicyShapeChangedAsync();
                    Raise(nameof(ShellRefreshPending));
                }
            }
            catch (Exception ex)
            {
                SetActionError(ErrorMessages.Humanize(ex, _svc.AppName));
                _svc.Toast(ErrorText, true);
            }
            finally
            {
                Busy = false;
                await RefreshCoreAsync();
            }
        }

        private async Task RestoreOriginalAsync(bool force)
        {
            Busy = true;
            ClearError();
            try
            {
                RestoreResult result = await _svc.Api.RestoreOriginalPolicy(force);
                _svc.Toast(result != null ? result.Message : "", false);
                // 策略被移除后，已加载旧策略的资源管理器仍会按旧规则拦截，需要重新加载。
                await _svc.NotifyPolicyShapeChangedAsync();
                Raise(nameof(ShellRefreshPending));
            }
            catch (Exception ex)
            {
                SetActionError(ErrorMessages.Humanize(ex, _svc.AppName));
                _svc.Toast(ErrorText, true);
            }
            finally
            {
                Busy = false;
                await RefreshCoreAsync();
            }
        }

        private void HandleCommandError(Exception ex)
        {
            SetActionError(ErrorMessages.Humanize(ex, _svc.AppName));
            _svc.Toast(ErrorText, true);
        }

        private static string FirstBlockMessage(PreflightReport report)
        {
            foreach (var check in report.Checks)
            {
                if (check.Status == "block")
                {
                    return check.Message;
                }
            }
            return report.Summary ?? "";
        }

        private static string FormatTime(string rfc3339)
        {
            if (string.IsNullOrWhiteSpace(rfc3339))
            {
                return "无";
            }
            DateTimeOffset parsed;
            if (DateTimeOffset.TryParse(rfc3339, CultureInfo.InvariantCulture, DateTimeStyles.RoundtripKind, out parsed))
            {
                return parsed.ToLocalTime().ToString("yyyy/M/d HH:mm:ss", CultureInfo.InvariantCulture);
            }
            return rfc3339;
        }

        private void RaiseAll()
        {
            Raise(nameof(Status));
            Raise(nameof(State));
            Raise(nameof(StateLabel));
            Raise(nameof(StateDescription));
            Raise(nameof(PrimaryText));
            Raise(nameof(ShowPrimary));
            Raise(nameof(PrimaryEnabled));
            Raise(nameof(CanRestore));
            Raise(nameof(IsManaged));
            Raise(nameof(IsBlacklistActive));
            Raise(nameof(IsWhitelistActive));
            Raise(nameof(ModeLabel));
            Raise(nameof(CanEnableMode));
            Raise(nameof(RuleCountText));
            Raise(nameof(AdminText));
            Raise(nameof(BackupText));
            Raise(nameof(SourceText));
            Raise(nameof(ShowModeSelection));
            Raise(nameof(EnableSelectedText));
            Raise(nameof(ShowEmptyRecent));
            Raise(nameof(ShellRefreshPending));
            CommandManager.InvalidateRequerySuggested();
        }
    }
}
