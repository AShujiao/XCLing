using WinForms = System.Windows.Forms;

namespace XCLing.Wpf.Core
{
    /// <summary>
    /// 原生文件/目录选择对话框。文件对话框是 GUI 壳的原生职责，不经 sidecar 暴露。
    /// </summary>
    public static class PathPicker
    {
        /// <summary>选择一个目录，取消返回 null。</summary>
        public static string SelectDirectory(string description = "选择要放行的目录")
        {
            using (var dialog = new WinForms.FolderBrowserDialog())
            {
                dialog.Description = description;
                dialog.ShowNewFolderButton = false;
                return dialog.ShowDialog() == WinForms.DialogResult.OK ? dialog.SelectedPath : null;
            }
        }

        /// <summary>选择一个可执行文件，取消返回 null。</summary>
        public static string SelectExecutable(string title = "选择要放行的程序")
        {
            using (var dialog = new WinForms.OpenFileDialog())
            {
                dialog.Title = title;
                dialog.Filter = "可执行文件 (*.exe)|*.exe|所有文件 (*.*)|*.*";
                dialog.CheckFileExists = true;
                dialog.Multiselect = false;
                return dialog.ShowDialog() == WinForms.DialogResult.OK ? dialog.FileName : null;
            }
        }
    }
}
