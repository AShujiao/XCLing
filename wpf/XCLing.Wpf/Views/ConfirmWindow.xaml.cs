using System.Windows;
using XCLing.Wpf.ViewModels;

namespace XCLing.Wpf.Views
{
    /// <summary>深色主题的模态确认框，替代 Vue 版 ConfirmDialog。</summary>
    public partial class ConfirmWindow : Window
    {
        public ConfirmWindow()
        {
            InitializeComponent();
        }

        /// <summary>owner 可为 null（主窗口未显示时居屏居中）。返回用户是否确认。</summary>
        public static bool Ask(Window owner, ConfirmRequest request)
        {
            var window = new ConfirmWindow
            {
                Title = request.Title ?? "",
            };
            window.TitleText.Text = request.Title ?? "";
            window.MessageText.Text = request.Message ?? "";
            window.ConfirmButton.Content = string.IsNullOrEmpty(request.ConfirmText) ? "确认" : request.ConfirmText;
            if (request.Danger)
            {
                window.ConfirmButton.Style = (Style)window.FindResource("PgDangerButton");
            }
            if (owner != null && owner.IsVisible)
            {
                window.Owner = owner;
            }
            else
            {
                window.WindowStartupLocation = WindowStartupLocation.CenterScreen;
            }
            return window.ShowDialog() == true;
        }

        private void OnConfirm(object sender, RoutedEventArgs e)
        {
            DialogResult = true;
        }

        private void OnCancel(object sender, RoutedEventArgs e)
        {
            DialogResult = false;
        }
    }
}
