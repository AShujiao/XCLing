using System.Windows;

namespace XCLing.Wpf.Views
{
    /// <summary>捐助弹窗：展示微信 / 支付宝收款二维码。</summary>
    public partial class DonateWindow : Window
    {
        public DonateWindow()
        {
            InitializeComponent();
        }

        /// <summary>owner 可为 null（主窗口未显示时居屏居中）。</summary>
        public static void Show(Window owner)
        {
            var window = new DonateWindow();
            if (owner != null && owner.IsVisible)
            {
                window.Owner = owner;
            }
            else
            {
                window.WindowStartupLocation = WindowStartupLocation.CenterScreen;
            }
            window.ShowDialog();
        }

        private void OnClose(object sender, RoutedEventArgs e)
        {
            Close();
        }
    }
}
