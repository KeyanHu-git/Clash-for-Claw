using Microsoft.UI.Xaml.Controls;
using OpenClawAdapter;
using OpenClawAdapter.ViewModels;

namespace OpenClawAdapter.Views;

public partial class LogsPage : Page
{
    public MainViewModel ViewModel { get; } = AppState.ViewModel;

    public LogsPage()
    {
        InitializeComponent();
        DataContext = ViewModel;
    }
}
