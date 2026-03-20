using System;
using System.IO;
using System.Linq;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;
using OpenClawAdapter.Services;
using OpenClawAdapter.Views;

namespace OpenClawAdapter
{
    /// <summary>
    /// Provides application-specific behavior to supplement the default Application class.
    /// </summary>
    public partial class App : Application
    {
        private Window? window;

        public static Window? MainWindow { get; private set; }

        /// <summary>
        /// Initializes the singleton application object.  This is the first line of authored code
        /// executed, and as such is the logical equivalent of main() or WinMain().
        /// </summary>
        public App()
        {
            InitializeComponent();
            UnhandledException += OnUnhandledException;
        }

        /// <summary>
        /// Invoked when the application is launched normally by the end user.  Other entry points
        /// will be used such as when the application is launched to open a specific file.
        /// </summary>
        /// <param name="e">Details about the launch request and process.</param>
        protected override void OnLaunched(LaunchActivatedEventArgs e)
        {
            AppState.Initialize();

            window ??= new Window();
            MainWindow = window;
            window.Title = "Clash for Claw";
            window.ExtendsContentIntoTitleBar = true;
            TrySetWindowIcon(window);

            if (window.Content is not Frame rootFrame)
            {
                rootFrame = new Frame();
                rootFrame.NavigationFailed += OnNavigationFailed;
                window.Content = rootFrame;
            }

            try
            {
                _ = rootFrame.Navigate(typeof(RootPage), e.Arguments);
            }
            catch (Exception ex)
            {
                LogCrash(ex);
                throw;
            }
            window.Activate();

            AppState.ApplySettings();
            HookWindowEvents(window);
            AppState.InitializeTray(window);

            if (ShouldStartSilent(e.Arguments))
            {
                AppState.HideToTray(window);
            }
        }

        private void OnUnhandledException(object sender, Microsoft.UI.Xaml.UnhandledExceptionEventArgs e)
        {
            LogCrash(e.Exception);
        }

        private static void LogCrash(Exception exception)
        {
            try
            {
                var dir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "OpenClawAdapter");
                Directory.CreateDirectory(dir);
                var logPath = Path.Combine(dir, "crash.log");
                File.AppendAllText(logPath, $"{DateTimeOffset.Now:u}\n{exception}\n\n");
            }
            catch
            {
                // Ignore logging failures.
            }
        }

        /// <summary>
        /// Invoked when Navigation to a certain page fails
        /// </summary>
        /// <param name="sender">The Frame which failed navigation</param>
        /// <param name="e">Details about the navigation failure</param>
        void OnNavigationFailed(object sender, NavigationFailedEventArgs e)
        {
            throw new Exception("Failed to load Page " + e.SourcePageType.FullName);
        }

        
        private static void TrySetWindowIcon(Window targetWindow)
        {
            var appWindow = WindowManager.GetAppWindow(targetWindow);
            if (appWindow is null)
            {
                return;
            }

            try
            {
                var iconPath = Path.Combine(AppContext.BaseDirectory, "Assets", "ClashForClaw.ico");
                if (File.Exists(iconPath))
                {
                    appWindow.SetIcon(iconPath);
                }
            }
            catch
            {
                // Ignore icon failures.
            }
        }

        private void HookWindowEvents(Window targetWindow)
        {
            var appWindow = WindowManager.GetAppWindow(targetWindow);
            if (appWindow is not null)
            {
                appWindow.Closing += OnAppWindowClosing;
            }
        }

        private void OnAppWindowClosing(AppWindow sender, AppWindowClosingEventArgs args)
        {
            if (AppState.AllowClose)
            {
                return;
            }

            if (SettingsStore.Current.CloseToTrayEnabled && !SettingsStore.Current.ServiceModeEnabled)
            {
                args.Cancel = true;
                if (window is not null)
                {
                    AppState.HideToTray(window);
                }
            }
        }

        private static bool ShouldStartSilent(string? args)
        {
            if (string.IsNullOrWhiteSpace(args))
            {
                return false;
            }

            return args.Split(' ', StringSplitOptions.RemoveEmptyEntries)
                .Any(part => part.Equals("--silent", StringComparison.OrdinalIgnoreCase));
        }
    }
}


