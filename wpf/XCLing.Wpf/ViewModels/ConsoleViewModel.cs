using System;
using System.Globalization;
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

        public ConsoleViewModel(AppServices services)
        {
            _svc = services ?? throw new ArgumentNullException(nameof(services));
            PrimaryCommand = new AsyncRelayCommand(RunPrimaryAsync, () => PrimaryEnabled, HandleCommandError);
            RefreshCommand = new AsyncRelayCommand(RefreshCoreAsync, () => !Busy, HandleCommandError);
            RestoreCommand = new AsyncRelayCommand(RunRestoreAsync, () => CanRestore, HandleCommandError);
            EnableWhitelistCommand = new AsyncRelayCommand(RunEnableWhitelistAsync, () => CanEnableMode, HandleCommandError);
            EnableBlacklistCommand = new AsyncRelayCommand(RunEnableBlacklistAsync, () => CanEnableMode, HandleCommandError);
            DonateCommand = new RelayCommand(ShowDonate);
        }

        public string Key => "console";

        public ICommand PrimaryCommand { get; }
        public ICommand RefreshCommand { get; }
        public ICommand RestoreCommand { get; }
        public ICommand EnableWhitelistCommand { get; }
        public ICommand EnableBlacklistCommand { get; }
        public ICommand DonateCommand { get; }

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
            private set { Set(ref _errorText, value); }
        }

        public string State => Status != null && !string.IsNullOrEmpty(Status.ProtectionState)
            ? Status.ProtectionState
            : "unmanaged";

        public string StateLabel
        {
            get
            {
                var blockOnly = Status != null && Status.PolicyMode == "blacklist";
                switch (State)
                {
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

        /// <summary>主操作按钮只服务已接管状态（解锁/锁定/从备份恢复）；未启用时通过模式卡片启用。</summary>
        public bool ShowPrimary => IsManaged;
        public bool PrimaryEnabled => !Busy && IsManaged;
        public bool CanRestore => !Busy && Status != null && Status.CanRestore;

        public bool IsManaged => State != "unmanaged";
        public bool IsBlacklistActive => IsManaged && Status != null && Status.PolicyMode == "blacklist";
        /// <summary>旧恢复记录无 policyMode 字段，后端按白名单处理，这里保持一致。</summary>
        public bool IsWhitelistActive => IsManaged && !IsBlacklistActive;
        public string ModeLabel => IsBlacklistActive ? "黑名单模式" : (IsWhitelistActive ? "白名单模式" : "未启用");
        public bool CanEnableMode => !Busy && !IsManaged && Status != null && Status.CanApply;

        public string RuleCountText => (Status != null ? Status.ExistingRuleCount : 0) + " 条";
        public string AdminText => Status != null && Status.IsAdmin ? "管理员" : "需要管理员权限";
        public string BackupText => FormatTime(Status != null ? Status.BackupCreatedAt : "");
        public string SourceText => Status != null && Status.DomainJoined ? "域策略" : "本机策略";

        public Task OnActivatedAsync() => RefreshCoreAsync();

        /// <summary>供托盘等外部路径修改策略后刷新界面。</summary>
        public Task RefreshAsync() => RefreshCoreAsync();

        private void ShowDonate()
        {
            _svc.ShowDonate?.Invoke();
        }

        private async Task RefreshCoreAsync()
        {
            try
            {
                Status = await _svc.Api.GetApplyStatus();
                ErrorText = "";
            }
            catch (Exception ex)
            {
                ErrorText = ErrorMessages.Humanize(ex, _svc.AppName);
            }
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
            ErrorText = "";
            try
            {
                var selection = SelectionBuilder.DefaultSelection(_svc.AppName, _svc.Api.CorePath, _svc.Settings, _svc.PendingPaths);
                JObject draft = await _svc.Api.BuildWhitelistDraft(Json.Serialize(selection));
                var draftJson = draft.ToString(Newtonsoft.Json.Formatting.None);

                PreflightReport report = await _svc.Api.PreflightWhitelistDraft(draftJson);
                if (report != null && report.Blocked)
                {
                    ErrorText = "草案预检未通过：" + FirstBlockMessage(report);
                    _svc.Toast(ErrorText, true);
                    return;
                }

                ApplyResult result = await _svc.Api.EnableProtection(draftJson);
                _svc.Toast(result != null ? result.Message : "保护已启用", false);
            }
            catch (Exception ex)
            {
                ErrorText = ErrorMessages.Humanize(ex, _svc.AppName);
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
            ErrorText = "";
            try
            {
                ApplyResult result = await _svc.Api.EnableBlockOnlyProtection();
                _svc.Toast(result != null && !string.IsNullOrEmpty(result.Message) ? result.Message : "黑名单模式已启用", false);
            }
            catch (Exception ex)
            {
                ErrorText = ErrorMessages.Humanize(ex, _svc.AppName);
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
            Busy = true;
            ErrorText = "";
            try
            {
                ProtectionResult result = toLock
                    ? await _svc.Api.LockProtection()
                    : await _svc.Api.UnlockProtection();
                _svc.Toast(result != null ? result.Message : "", false);
            }
            catch (Exception ex)
            {
                ErrorText = ErrorMessages.Humanize(ex, _svc.AppName);
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
            ErrorText = "";
            try
            {
                RestoreResult result = await _svc.Api.RestoreOriginalPolicy(force);
                _svc.Toast(result != null ? result.Message : "", false);
            }
            catch (Exception ex)
            {
                ErrorText = ErrorMessages.Humanize(ex, _svc.AppName);
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
            ErrorText = ErrorMessages.Humanize(ex, _svc.AppName);
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
        }
    }
}
