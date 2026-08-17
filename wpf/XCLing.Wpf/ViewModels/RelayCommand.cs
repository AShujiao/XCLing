using System;
using System.Threading.Tasks;
using System.Windows.Input;

namespace XCLing.Wpf.ViewModels
{
    /// <summary>同步命令。</summary>
    public sealed class RelayCommand : ICommand
    {
        private readonly Action _execute;
        private readonly Func<bool> _canExecute;

        public RelayCommand(Action execute, Func<bool> canExecute = null)
        {
            _execute = execute ?? throw new ArgumentNullException(nameof(execute));
            _canExecute = canExecute;
        }

        public event EventHandler CanExecuteChanged
        {
            add { CommandManager.RequerySuggested += value; }
            remove { CommandManager.RequerySuggested -= value; }
        }

        public bool CanExecute(object parameter) => _canExecute == null || _canExecute();

        public void Execute(object parameter) => _execute();
    }

    /// <summary>
    /// 异步命令：执行期间自动禁用，异常交给调用方传入的错误处理器（不允许静默吞掉）。
    /// </summary>
    public sealed class AsyncRelayCommand : ICommand
    {
        private readonly Func<Task> _execute;
        private readonly Func<bool> _canExecute;
        private readonly Action<Exception> _onError;
        private bool _running;

        public AsyncRelayCommand(Func<Task> execute, Func<bool> canExecute, Action<Exception> onError)
        {
            _execute = execute ?? throw new ArgumentNullException(nameof(execute));
            _canExecute = canExecute;
            _onError = onError ?? throw new ArgumentNullException(nameof(onError));
        }

        public event EventHandler CanExecuteChanged
        {
            add { CommandManager.RequerySuggested += value; }
            remove { CommandManager.RequerySuggested -= value; }
        }

        public bool CanExecute(object parameter) => !_running && (_canExecute == null || _canExecute());

        public async void Execute(object parameter)
        {
            _running = true;
            CommandManager.InvalidateRequerySuggested();
            try
            {
                await _execute();
            }
            catch (Exception ex)
            {
                _onError(ex);
            }
            finally
            {
                _running = false;
                CommandManager.InvalidateRequerySuggested();
            }
        }
    }

    /// <summary>带参数的同步命令。</summary>
    public sealed class RelayCommand<T> : ICommand
    {
        private readonly Action<T> _execute;
        private readonly Func<T, bool> _canExecute;

        public RelayCommand(Action<T> execute, Func<T, bool> canExecute = null)
        {
            _execute = execute ?? throw new ArgumentNullException(nameof(execute));
            _canExecute = canExecute;
        }

        public event EventHandler CanExecuteChanged
        {
            add { CommandManager.RequerySuggested += value; }
            remove { CommandManager.RequerySuggested -= value; }
        }

        public bool CanExecute(object parameter) => _canExecute == null || _canExecute(Cast(parameter));

        public void Execute(object parameter) => _execute(Cast(parameter));

        private static T Cast(object parameter) => parameter is T t ? t : default;
    }

    /// <summary>带参数的异步命令。执行期间不重入同一参数调用。</summary>
    public sealed class AsyncRelayCommand<T> : ICommand
    {
        private readonly Func<T, Task> _execute;
        private readonly Func<T, bool> _canExecute;
        private readonly Action<Exception> _onError;
        private bool _running;

        public AsyncRelayCommand(Func<T, Task> execute, Action<Exception> onError, Func<T, bool> canExecute = null)
        {
            _execute = execute ?? throw new ArgumentNullException(nameof(execute));
            _onError = onError ?? throw new ArgumentNullException(nameof(onError));
            _canExecute = canExecute;
        }

        public event EventHandler CanExecuteChanged
        {
            add { CommandManager.RequerySuggested += value; }
            remove { CommandManager.RequerySuggested -= value; }
        }

        public bool CanExecute(object parameter) => !_running && (_canExecute == null || _canExecute(Cast(parameter)));

        public async void Execute(object parameter)
        {
            _running = true;
            CommandManager.InvalidateRequerySuggested();
            try
            {
                await _execute(Cast(parameter));
            }
            catch (Exception ex)
            {
                _onError(ex);
            }
            finally
            {
                _running = false;
                CommandManager.InvalidateRequerySuggested();
            }
        }

        private static T Cast(object parameter) => parameter is T t ? t : default;
    }
}
