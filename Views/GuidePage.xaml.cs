using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace ClashForClaw.Views;

public partial class GuidePage : Page
{
    public GuidePage()
    {
        InitializeComponent();
        ApplyAdaptiveLayout();
    }

    private void OnPageSizeChanged(object sender, SizeChangedEventArgs e)
    {
        ApplyAdaptiveLayout(e.NewSize.Width);
    }

    private void ApplyAdaptiveLayout(double? widthOverride = null)
    {
        var width = widthOverride ?? ActualWidth;
        if (width >= 820)
        {
            GuideColumnB.Width = new GridLength(1, GridUnitType.Star);

            Grid.SetColumn(GuidePrinciplesCard, 0);
            Grid.SetRow(GuidePrinciplesCard, 0);
            Grid.SetColumnSpan(GuidePrinciplesCard, 1);
            Grid.SetColumn(GuideModesCard, 1);
            Grid.SetRow(GuideModesCard, 0);
            Grid.SetColumnSpan(GuideModesCard, 1);
            Grid.SetColumn(GuideInterfacesCard, 0);
            Grid.SetRow(GuideInterfacesCard, 1);
            Grid.SetColumnSpan(GuideInterfacesCard, 1);
            Grid.SetColumn(GuideServiceCard, 1);
            Grid.SetRow(GuideServiceCard, 1);
            Grid.SetColumnSpan(GuideServiceCard, 1);
            Grid.SetColumn(GuideCreditsCard, 0);
            Grid.SetRow(GuideCreditsCard, 2);
            Grid.SetColumnSpan(GuideCreditsCard, 2);
            return;
        }

        GuideColumnB.Width = new GridLength(0);

        Grid.SetColumn(GuidePrinciplesCard, 0);
        Grid.SetRow(GuidePrinciplesCard, 0);
        Grid.SetColumnSpan(GuidePrinciplesCard, 1);
        Grid.SetColumn(GuideModesCard, 0);
        Grid.SetRow(GuideModesCard, 1);
        Grid.SetColumnSpan(GuideModesCard, 1);
        Grid.SetColumn(GuideInterfacesCard, 0);
        Grid.SetRow(GuideInterfacesCard, 2);
        Grid.SetColumnSpan(GuideInterfacesCard, 1);
        Grid.SetColumn(GuideServiceCard, 0);
        Grid.SetRow(GuideServiceCard, 3);
        Grid.SetColumnSpan(GuideServiceCard, 1);
        Grid.SetColumn(GuideCreditsCard, 0);
        Grid.SetRow(GuideCreditsCard, 4);
        Grid.SetColumnSpan(GuideCreditsCard, 1);
    }
}
