using System;

namespace XCLing.Wpf.Core
{
    /// <summary>
    /// 携带 sidecar 返回的错误。Message 是 Go 侧 err.Error() 原文（"CODE: 说明" 形状），
    /// Code 是冒号前的稳定错误码，供 ErrorMessages 映射为用户可读文案。
    /// </summary>
    public sealed class RpcException : Exception
    {
        public RpcException(string message) : base(message ?? "")
        {
            var text = message ?? "";
            var colon = text.IndexOf(':');
            Code = colon > 0 ? text.Substring(0, colon).Trim() : text.Trim();
        }

        public string Code { get; }
    }
}
