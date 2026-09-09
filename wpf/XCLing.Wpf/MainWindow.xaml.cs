using System;
using System.Windows;

namespace XCLing.Wpf
{
    public partial class MainWindow : Window
    {
        public MainWindow()
        {
            InitializeComponent();
            var area = SystemParameters.WorkArea;
            MinWidth = Math.Min(760, area.Width);
            MinHeight = Math.Min(480, area.Height);
            Width = Math.Min(1100, area.Width);
            Height = Math.Min(700, area.Height);
        }
    }
}
