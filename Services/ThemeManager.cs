using Microsoft.UI.Xaml;

namespace OpenClawAdapter.Services;

public static class ThemeManager
{
    public static void ApplyTheme(Window? window, string? themeMode)
    {
        if (window?.Content is not FrameworkElement root)
        {
            return;
        }

        root.RequestedTheme = themeMode switch
        {
            "Light" => ElementTheme.Light,
            "Dark" => ElementTheme.Dark,
            _ => ElementTheme.Default,
        };
    }
}
