using System;
using System.IO;
using Newtonsoft.Json;

namespace XCLing.Wpf.Core
{
    /// <summary>
    /// WPF 壳的本地设置，持久化到 %AppData%\XCLing\wpf-settings.json。
    /// 对应 Vue 版 whitelist store 里的兼容开关（Vue 版为会话内临时状态，此处持久化）。
    /// 仅落用户数据目录，绝不触碰 SRP。
    /// </summary>
    public sealed class Settings
    {
        public bool AllowPackagedApps { get; set; } = true;
        public bool AllowDefenderUpdates { get; set; } = true;
        /// <summary>主题：dark（默认）或 light。</summary>
        public string Theme { get; set; } = "dark";

        [JsonIgnore]
        public static string FilePath
        {
            get
            {
                var dir = Path.Combine(
                    Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),
                    "XCLing");
                return Path.Combine(dir, "wpf-settings.json");
            }
        }

        public static Settings Load()
        {
            try
            {
                var path = FilePath;
                if (File.Exists(path))
                {
                    var text = File.ReadAllText(path);
                    var loaded = JsonConvert.DeserializeObject<Settings>(text);
                    if (loaded != null)
                    {
                        return loaded;
                    }
                }
            }
            catch
            {
                // 损坏或不可读时回退默认值。
            }
            return new Settings();
        }

        public void Save()
        {
            try
            {
                var path = FilePath;
                Directory.CreateDirectory(Path.GetDirectoryName(path));
                File.WriteAllText(path, JsonConvert.SerializeObject(this, Formatting.Indented));
            }
            catch
            {
                // 设置写入失败不应中断主流程。
            }
        }
    }
}
