using System;
using Newtonsoft.Json;
using Newtonsoft.Json.Serialization;

namespace XCLing.Wpf.Core
{
    /// <summary>
    /// 统一的 JSON 序列化约定：camelCase 输出（与 Go 侧 json tag 一致）。
    /// 反序列化时 Newtonsoft 默认大小写不敏感，Go 的 camelCase 自动映射到 C# PascalCase 属性。
    /// </summary>
    public static class Json
    {
        public static readonly JsonSerializerSettings Settings = new JsonSerializerSettings
        {
            ContractResolver = new CamelCasePropertyNamesContractResolver(),
            NullValueHandling = NullValueHandling.Include,
        };

        public static readonly JsonSerializer Serializer = JsonSerializer.Create(Settings);

        public static string Serialize(object value)
        {
            return JsonConvert.SerializeObject(value, Formatting.None, Settings);
        }
    }
}
