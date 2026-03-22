using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using ClashForClaw;
using ClashForClaw.ViewModels;

namespace ClashForClaw.Views;

public partial class LogsPage : Page
{
    public MainViewModel ViewModel { get; } = AppState.ViewModel;

    public LogsPage()
    {
        InitializeComponent();
        DataContext = ViewModel;
        ApplyAdaptiveLayout();
    }

    private void OnPageSizeChanged(object sender, SizeChangedEventArgs e)
    {
        ApplyAdaptiveLayout(e.NewSize.Width);
    }

    private void ApplyAdaptiveLayout(double? widthOverride = null)
    {
        var width = widthOverride ?? ActualWidth;
        if (width >= 780)
        {
            LogsColumnB.Width = new GridLength(1, GridUnitType.Star);
            Grid.SetColumn(AdvancedCard, 1);
            Grid.SetRow(AdvancedCard, 0);
            return;
        }

        LogsColumnB.Width = new GridLength(0);
        Grid.SetColumn(AdvancedCard, 0);
        Grid.SetRow(AdvancedCard, 1);
    }
}
