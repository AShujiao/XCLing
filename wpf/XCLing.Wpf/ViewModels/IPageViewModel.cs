using System.Threading.Tasks;

namespace XCLing.Wpf.ViewModels
{
    /// <summary>可导航的页面视图模型。</summary>
    public interface IPageViewModel
    {
        /// <summary>导航键（与侧边栏 NavItem.Key 对应）。</summary>
        string Key { get; }

        /// <summary>页面被切换到前台时调用，用于按需加载数据。</summary>
        Task OnActivatedAsync();
    }
}
