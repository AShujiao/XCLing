using System;
using System.IO;
using Newtonsoft.Json;

namespace XCLing.Wpf.Core
{
    /// <summary>
    /// WPF 壳的本地设置，持久化到 %AppData%\XCLing\wpf-settings.json。
    /// 保存界面侧的兼容开关与主题偏好，仅落用户数据目录，绝不触碰 SRP。
    /// </summary>
    public sealed class Settings
    {
        public bool AllowPackagedApps { get; set; } = true;
        public bool AllowDefenderUpdates { get; set; } = true;
        /// <summary>主题：light（默认）或 dark，保留用户已保存的选择。</summary>
        public string Theme { get; set; } = "light";
        /// <summary>
        /// 策略生效形态发生变化的时间点（UTC，ISO 8601）；非空表示资源管理器需要重新加载策略。
        /// 用于判断资源管理器是否仍在按旧策略放行程序。
        /// </summary>
        public string ShellRefreshPendingSinceUtc { get; set; } = "";

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
