using System;
using System.Globalization;
using System.Windows;
using System.Windows.Data;

namespace XCLing.Wpf.Core
{
    /// <summary>非空字符串 → Visible，空/null → Collapsed。</summary>
    public sealed class StringToVisibilityConverter : IValueConverter
    {
        public object Convert(object value, Type targetType, object parameter, CultureInfo culture)
        {
            return string.IsNullOrWhiteSpace(value as string) ? Visibility.Collapsed : Visibility.Visible;
        }

        public object ConvertBack(object value, Type targetType, object parameter, CultureInfo culture) =>
            throw new NotSupportedException();
    }

    /// <summary>bool → Visibility，参数 "invert" 时反转。</summary>
    public sealed class BoolToVisibilityConverter : IValueConverter
    {
        public object Convert(object value, Type targetType, object parameter, CultureInfo culture)
        {
            var flag = value is bool b && b;
            if ("invert".Equals(parameter as string, StringComparison.OrdinalIgnoreCase))
            {
                flag = !flag;
            }
            return flag ? Visibility.Visible : Visibility.Collapsed;
        }

        public object ConvertBack(object value, Type targetType, object parameter, CultureInfo culture) =>
            throw new NotSupportedException();
    }

    /// <summary>bool → 反转的 bool。</summary>
    public sealed class InverseBoolConverter : IValueConverter
    {
        public object Convert(object value, Type targetType, object parameter, CultureInfo culture)
        {
            return value is bool b ? !b : true;
        }

        public object ConvertBack(object value, Type targetType, object parameter, CultureInfo culture)
        {
            return value is bool b ? !b : false;
        }
    }

    /// <summary>ISO 时间字符串 → 本地可读时间；空/非法原样或返回破折号。</summary>
    public sealed class TimeConverter : IValueConverter
    {
        public object Convert(object value, Type targetType, object parameter, CultureInfo culture)
        {
            var raw = value as string;
            if (string.IsNullOrWhiteSpace(raw))
            {
                return "—";
            }
            DateTimeOffset parsed;
            if (DateTimeOffset.TryParse(raw, CultureInfo.InvariantCulture, DateTimeStyles.RoundtripKind, out parsed))
            {
                return parsed.ToLocalTime().ToString("yyyy/M/d HH:mm:ss", CultureInfo.InvariantCulture);
            }
            return raw;
        }

        public object ConvertBack(object value, Type targetType, object parameter, CultureInfo culture) =>
            throw new NotSupportedException();
    }
}
