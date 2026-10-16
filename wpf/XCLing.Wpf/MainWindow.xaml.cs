using System;
using System.Windows;
using System.Windows.Input;
using System.Windows.Media.Animation;

namespace XCLing.Wpf
{
    public partial class MainWindow : Window
    {
        public MainWindow()
        {
            InitializeComponent();
            var area = SystemParameters.WorkArea;
            MinWidth = Math.Min(880, area.Width);
            MinHeight = Math.Min(580, area.Height);
            Width = Math.Min(1180, area.Width);
            Height = Math.Min(760, area.Height);
            Loaded += OnLoaded;
        }

        private void OnLoaded(object sender, RoutedEventArgs e)
        {
            Loaded -= OnLoaded;
            UpdateWindowStateVisuals();
            PlayPageAnimation();
        }

        private void TitleBar_MouseLeftButtonDown(object sender, MouseButtonEventArgs e)
        {
            if (e.ClickCount == 2)
            {
                ToggleMaximize();
                return;
            }

            if (e.LeftButton == MouseButtonState.Pressed && WindowState != WindowState.Maximized)
            {
                DragMove();
            }
        }

        private void MinimizeButton_Click(object sender, RoutedEventArgs e)
        {
            WindowState = WindowState.Minimized;
        }

        private void MaximizeButton_Click(object sender, RoutedEventArgs e)
        {
            ToggleMaximize();
        }

        private void CloseButton_Click(object sender, RoutedEventArgs e)
        {
            Close();
        }

        private void ToggleMaximize()
        {
            WindowState = WindowState == WindowState.Maximized
                ? WindowState.Normal
                : WindowState.Maximized;
        }

        private void OnWindowStateChanged(object sender, EventArgs e)
        {
            UpdateWindowStateVisuals();
        }

        private void UpdateWindowStateVisuals()
        {
            if (MaximizeButton == null)
            {
                return;
            }

            var maximized = WindowState == WindowState.Maximized;
            MaximizeButton.Content = maximized
                ? TryFindResource("RestoreIcon")
                : TryFindResource("MaximizeIcon");
            MaximizeButton.ToolTip = maximized ? "还原" : "最大化";
            MaximizeButton.SetValue(System.Windows.Automation.AutomationProperties.NameProperty, maximized ? "还原" : "最大化");
        }

        public void PlayPageAnimation()
        {
            if (PageHost == null)
            {
                return;
            }
            // Animate opacity and margin directly. A TranslateTransform can be sealed by WPF
            // after it is assigned to a visual, so it is not safe as a reusable page target.
            PageHost.Opacity = 0;
            PageHost.Margin = new Thickness(14, 0, 0, 0);
            var opacity = new DoubleAnimation(0, 1, TimeSpan.FromMilliseconds(220));
            var slide = new ThicknessAnimation(
                new Thickness(14, 0, 0, 0),
                new Thickness(0),
                TimeSpan.FromMilliseconds(280))
            {
                EasingFunction = new System.Windows.Media.Animation.QuadraticEase { EasingMode = EasingMode.EaseOut },
            };
            PageHost.BeginAnimation(OpacityProperty, opacity);
            PageHost.BeginAnimation(FrameworkElement.MarginProperty, slide);
        }
    }
}
