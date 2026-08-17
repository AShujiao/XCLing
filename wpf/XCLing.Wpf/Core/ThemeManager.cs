using System;
using System.Windows;
using System.Windows.Media;

namespace XCLing.Wpf.Core
{
    /// <summary>
    /// 主题管理：在深色与明亮主题之间切换。
    /// 核心修复：不移除/重载任何资源字典（避免控件样式丢失后无法自动重新应用），
    /// 而是直接在已加载的主题字典中替换画笔资源。直接修改字典条目会触发
    /// WPF 资源变更通知，所有使用 DynamicResource 的控件自动刷新颜色，
    /// 这是最稳定可靠的主题切换方案。
    /// </summary>
    public static class ThemeManager
    {
        public const string Dark = "dark";
        public const string Light = "light";

        private static string _current = Dark;

        /// <summary>当前已应用的主题名称（dark 或 light）。</summary>
        public static string Current => _current;

        /// <summary>应用指定主题。theme 为 dark 或 light，其他值回退 dark。</summary>
        public static void Apply(string theme)
        {
            if (!string.Equals(theme, Light, StringComparison.OrdinalIgnoreCase))
            {
                theme = Dark;
            }

            if (string.Equals(_current, theme, StringComparison.OrdinalIgnoreCase))
            {
                return;
            }

            try
            {
                // 1. 找到当前已加载的主题字典（不改变它在MergedDictionaries中的位置）
                ResourceDictionary themeDict = FindThemeDictionary();
                if (themeDict == null)
                {
                    // 极端情况：没找到，退回到添加新字典的方案
                    var fallbackDict = new ResourceDictionary
                    {
                        Source = new Uri("pack://application:,,,/Themes/" +
                            (string.Equals(theme, Light, StringComparison.OrdinalIgnoreCase) ? "Light" : "Dark") + ".xaml",
                            UriKind.Absolute)
                    };
                    Application.Current.Resources.MergedDictionaries.Add(fallbackDict);
                    _current = theme;
                    return;
                }

                // 2. 加载新主题字典获取新画笔
                var themeFileName = string.Equals(theme, Light, StringComparison.OrdinalIgnoreCase) ? "Light" : "Dark";
                var newThemeSource = new Uri("pack://application:,,,/Themes/" + themeFileName + ".xaml", UriKind.Absolute);
                var newThemeDict = new ResourceDictionary { Source = newThemeSource };

                // 3. 关键：直接在原有主题字典对象上替换所有画笔值！
                // 直接修改字典条目会触发WPF的资源变更通知，DynamicResource自动更新UI
                foreach (var key in newThemeDict.Keys)
                {
                    if (newThemeDict[key] is Brush brush)
                    {
                        // 直接设置键值，字典对象不变，内容更新
                        themeDict[key] = brush;
                    }
                }

                _current = theme;

                // 触发UI重绘处理边缘情况
                TryRefreshVisualTree();
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("Theme switch failed: " + ex.Message);
            }
        }

        /// <summary>初始化主题管理器。</summary>
        public static void Init(string theme)
        {
            if (!string.Equals(theme, Light, StringComparison.OrdinalIgnoreCase))
            {
                theme = Dark;
            }

            _current = theme;

            // 如果保存的是浅色主题，应用它
            if (string.Equals(theme, Light, StringComparison.OrdinalIgnoreCase) && Application.Current != null)
            {
                Apply(theme);
            }
        }

        private static ResourceDictionary FindThemeDictionary()
        {
            if (Application.Current == null) return null;

            var dicts = Application.Current.Resources.MergedDictionaries;
            foreach (var d in dicts)
            {
                if (IsThemeDictionary(d.Source))
                {
                    return d;
                }
            }
            return null;
        }

        private static bool IsThemeDictionary(Uri source)
        {
            if (source == null) return false;
            var s = source.OriginalString;
            return s.EndsWith("/Themes/Dark.xaml", StringComparison.OrdinalIgnoreCase) ||
                   s.EndsWith("/Themes/Light.xaml", StringComparison.OrdinalIgnoreCase);
        }

        private static void TryRefreshVisualTree()
        {
            if (Application.Current == null || Application.Current.MainWindow == null)
            {
                return;
            }

            try
            {
                var window = Application.Current.MainWindow;
                window.InvalidateVisual();
                window.UpdateLayout();
            }
            catch
            {
                // 忽略
            }
        }
    }
}
