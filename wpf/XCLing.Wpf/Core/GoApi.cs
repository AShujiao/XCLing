using System.Collections.Generic;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;
using XCLing.Wpf.Models;

namespace XCLing.Wpf.Core
{
    /// <summary>
    /// 类型化的服务层适配器：
    /// 视图模型只经由这里调用 sidecar，方法名与 cmd/core 注册的 Go 服务白名单一一对应。
    /// 草案（WhitelistDraft）作为不透明 JObject 透传，C# 不重复建模策略结构。
    /// 文件对话框不在此列——由 WPF 原生实现（见 PathPicker）。
    /// </summary>
    public sealed class GoApi
    {
        private readonly RpcClient _rpc;

        public GoApi(RpcClient rpc)
        {
            _rpc = rpc ?? throw new System.ArgumentNullException(nameof(rpc));
        }

        public string CorePath => _rpc.CorePath;

        // ApplyService
        public Task<ApplyStatus> GetApplyStatus() =>
            _rpc.CallAsync<ApplyStatus>("ApplyService.GetApplyStatus");

        public Task<ApplyResult> EnableProtection(string draftJson) =>
            _rpc.CallAsync<ApplyResult>("ApplyService.EnableProtection", draftJson);

        public Task<ApplyResult> EnableBlockOnlyProtection() =>
            _rpc.CallAsync<ApplyResult>("ApplyService.EnableBlockOnlyProtection");

        public Task<ProtectionResult> UnlockProtection() =>
            _rpc.CallAsync<ProtectionResult>("ApplyService.UnlockProtection");

        public Task<ProtectionResult> LockProtection() =>
            _rpc.CallAsync<ProtectionResult>("ApplyService.LockProtection");

        public Task<RuleChangeResult> AddTrustedPath(string path, string kind, string label) =>
            _rpc.CallAsync<RuleChangeResult>("ApplyService.AddTrustedPath", path, kind, label);

        public Task<RuleChangeResult> RemoveTrustedRule(string id) =>
            _rpc.CallAsync<RuleChangeResult>("ApplyService.RemoveTrustedRule", id);

        public Task<RestoreResult> RestoreOriginalPolicy(bool force) =>
            _rpc.CallAsync<RestoreResult>("ApplyService.RestoreOriginalPolicy", force);

        public Task<List<ProtectionEvent>> ListProtectionEvents() =>
            _rpc.CallAsync<List<ProtectionEvent>>("ApplyService.ListProtectionEvents");

        // WhitelistService
        public Task<List<DiscoveredApp>> ListDiscoveredApps() =>
            _rpc.CallAsync<List<DiscoveredApp>>("WhitelistService.ListDiscoveredApps");

        public Task<JObject> BuildWhitelistDraft(string selectionJson) =>
            _rpc.CallAsync<JObject>("WhitelistService.BuildWhitelistDraft", selectionJson);

        public Task<PreflightReport> PreflightWhitelistDraft(string draftJson) =>
            _rpc.CallAsync<PreflightReport>("WhitelistService.PreflightWhitelistDraft", draftJson);

        // AuditService（仅记录页使用的只读事件查询）
        public Task<AuditCapability> GetAuditCapability() =>
            _rpc.CallAsync<AuditCapability>("AuditService.GetAuditCapability");

        public Task<ListEventsResult> ListBlockedEvents(string filterJson) =>
            _rpc.CallAsync<ListEventsResult>("AuditService.ListBlockedEvents", filterJson);

        public Task<JObject> EnableAuditPolicy() =>
            _rpc.CallAsync<JObject>("AuditService.EnableAuditPolicy");

        // ShutdownService
        public Task<ShutdownConfig> GetShutdownConfig() =>
            _rpc.CallAsync<ShutdownConfig>("ShutdownService.GetConfig");

        public Task<JObject> SetShutdownConfig(bool enabled, int hour) =>
            _rpc.CallAsync<JObject>("ShutdownService.SetConfig", enabled, hour);

        public Task<JObject> CancelShutdown() =>
            _rpc.CallAsync<JObject>("ShutdownService.CancelShutdown");

        // BlocklistService
        public Task<BlocklistStatus> GetBlocklistStatus() =>
            _rpc.CallAsync<BlocklistStatus>("BlocklistService.GetBlocklistStatus");

        public Task<List<VendorPreset>> GetVendorPresets() =>
            _rpc.CallAsync<List<VendorPreset>>("BlocklistService.GetVendorPresets");

        public Task<BlocklistResult> ApplyVendorPreset(string vendorId) =>
            _rpc.CallAsync<BlocklistResult>("BlocklistService.ApplyVendorPreset", vendorId);

        public Task<BlocklistResult> RemoveVendorPreset(string vendorId) =>
            _rpc.CallAsync<BlocklistResult>("BlocklistService.RemoveVendorPreset", vendorId);

        public Task<BlocklistResult> AddBlockRule(string pattern, string kind, string label) =>
            _rpc.CallAsync<BlocklistResult>("BlocklistService.AddBlockRule", pattern, kind, label);

        public Task<BlocklistResult> RemoveBlockRule(string id) =>
            _rpc.CallAsync<BlocklistResult>("BlocklistService.RemoveBlockRule", id);

        public Task<List<BlockedVendorScan>> ScanVendorTargets() =>
            _rpc.CallAsync<List<BlockedVendorScan>>("BlocklistService.ScanVendorTargets");

        public Task<BlocklistResult> ApplyScanResult(List<string> installPaths) =>
            _rpc.CallAsync<BlocklistResult>("BlocklistService.ApplyScanResult", installPaths);

        // UpdateService（检查更新：仅用户点击时联网，无遥测）
        public Task<UpdateInfo> CheckUpdate() =>
            _rpc.CallAsync<UpdateInfo>("UpdateService.CheckUpdate");
    }
}
