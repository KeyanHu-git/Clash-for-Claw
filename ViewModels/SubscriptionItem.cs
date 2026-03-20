using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Runtime.CompilerServices;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Media;

namespace OpenClawAdapter.ViewModels;

public sealed class SubscriptionItem : INotifyPropertyChanged
{
    private bool isActive;
    private string name;
    private string source;
    private string url;
    private string filePath;
    private string state;
    private long lastSuccessAt;
    private double usageUsed;
    private double usageLimit;
    private string usageUnit;
    private long expireAt;
    private long updatedAt;

    public event PropertyChangedEventHandler? PropertyChanged;

    public string Id { get; }

    public SubscriptionItem(
        string id,
        string name,
        string source,
        string url,
        string filePath,
        string state,
        long lastSuccessAt,
        double usageUsed,
        double usageLimit,
        string usageUnit,
        long expireAt,
        long updatedAt)
    {
        Id = id;
        this.name = name;
        this.source = source;
        this.url = url;
        this.filePath = filePath;
        this.state = state;
        this.lastSuccessAt = lastSuccessAt;
        this.usageUsed = usageUsed;
        this.usageLimit = usageLimit;
        this.usageUnit = string.IsNullOrWhiteSpace(usageUnit) ? "GB" : usageUnit;
        this.expireAt = expireAt;
        this.updatedAt = updatedAt;
    }

    public string FilePath => filePath;

    public bool IsActive
    {
        get => isActive;
        set
        {
            if (SetField(ref isActive, value))
            {
                OnPropertyChanged(nameof(StateLineBrush));
                OnPropertyChanged(nameof(StateLineOpacity));
            }
        }
    }

    public bool HasUrl => !string.IsNullOrWhiteSpace(url);
    public bool CanRefresh => HasUrl;
    public bool IsFile => string.Equals(source, "file", StringComparison.OrdinalIgnoreCase)
        && !string.IsNullOrWhiteSpace(filePath);

    public string DisplayName => string.IsNullOrWhiteSpace(name) ? "未命名订阅" : name;
    public string DisplayUrl => string.IsNullOrWhiteSpace(url) ? "本地文件" : ShortenUrl(url);

    public string UpdatedText
    {
        get
        {
            if (updatedAt <= 0)
            {
                return "更新于 未更新";
            }
            var now = DateTimeOffset.Now;
            var time = DateTimeOffset.FromUnixTimeSeconds(updatedAt);
            var diff = now - time;
            if (diff.TotalMinutes < 1)
            {
                return "更新于 刚刚";
            }
            if (diff.TotalHours < 1)
            {
                return $"更新于 {(int)diff.TotalMinutes} 分钟前";
            }
            if (diff.TotalDays < 1)
            {
                return $"更新于 {(int)diff.TotalHours} 小时前";
            }
            return $"更新于 {(int)diff.TotalDays} 天前";
        }
    }

    public string UsageText
    {
        get
        {
            if (usageLimit <= 0)
            {
                return "用量未知";
            }
            return $"已用 {usageUsed:0.0}{usageUnit} / {usageLimit:0.0}{usageUnit}";
        }
    }

    public string ExpireText
    {
        get
        {
            if (expireAt <= 0)
            {
                return "到期未知";
            }
            var date = DateTimeOffset.FromUnixTimeSeconds(expireAt).ToLocalTime();
            return $"到期 {date:yyyy-MM-dd}";
        }
    }

    public double ProgressValue => usageLimit > 0 ? usageUsed : 0.15;
    public double ProgressMaximum => usageLimit > 0 ? usageLimit : 1;
    public double ProgressOpacity => usageLimit > 0 ? 1 : 0.3;

    public Brush StateLineBrush
    {
        get
        {
            if (string.Equals(state, "error", StringComparison.OrdinalIgnoreCase))
            {
                return GetBrush("StatusBadBrush");
            }
            if (IsActive)
            {
                return GetBrush("AccentBlueBrush");
            }
            return GetBrush("SubscriptionInactiveBrush");
        }
    }

    public double StateLineOpacity
    {
        get
        {
            if (string.Equals(state, "error", StringComparison.OrdinalIgnoreCase))
            {
                return 0.55;
            }
            if (string.Equals(state, "unknown", StringComparison.OrdinalIgnoreCase))
            {
                return 0.2;
            }
            return IsActive ? 1 : 0;
        }
    }

    public void Update(
        string nextName,
        string nextSource,
        string nextUrl,
        string nextFilePath,
        string nextState,
        long nextLastSuccessAt,
        double nextUsageUsed,
        double nextUsageLimit,
        string nextUsageUnit,
        long nextExpireAt,
        long nextUpdatedAt)
    {
        name = nextName;
        source = nextSource;
        url = nextUrl;
        filePath = nextFilePath;
        state = nextState;
        lastSuccessAt = nextLastSuccessAt;
        usageUsed = nextUsageUsed;
        usageLimit = nextUsageLimit;
        usageUnit = string.IsNullOrWhiteSpace(nextUsageUnit) ? "GB" : nextUsageUnit;
        expireAt = nextExpireAt;
        updatedAt = nextUpdatedAt;

        OnPropertyChanged(nameof(DisplayName));
        OnPropertyChanged(nameof(DisplayUrl));
        OnPropertyChanged(nameof(UpdatedText));
        OnPropertyChanged(nameof(UsageText));
        OnPropertyChanged(nameof(ExpireText));
        OnPropertyChanged(nameof(ProgressValue));
        OnPropertyChanged(nameof(ProgressMaximum));
        OnPropertyChanged(nameof(ProgressOpacity));
        OnPropertyChanged(nameof(StateLineBrush));
        OnPropertyChanged(nameof(StateLineOpacity));
        OnPropertyChanged(nameof(HasUrl));
        OnPropertyChanged(nameof(CanRefresh));
        OnPropertyChanged(nameof(IsFile));
    }

    private static Brush GetBrush(string key)
    {
        if (Application.Current?.Resources.TryGetValue(key, out var value) is true && value is Brush brush)
        {
            return brush;
        }
        return new SolidColorBrush();
    }

    private static string ShortenUrl(string raw)
    {
        if (string.IsNullOrWhiteSpace(raw))
        {
            return raw;
        }
        var text = raw.Trim();
        if (Uri.TryCreate(text, UriKind.Absolute, out var uri) && !string.IsNullOrWhiteSpace(uri.Host))
        {
            var host = uri.Host;
            var path = uri.AbsolutePath.Trim('/');
            if (string.IsNullOrWhiteSpace(path))
            {
                return host;
            }
            var segments = path.Split('/', StringSplitOptions.RemoveEmptyEntries);
            var first = segments.Length > 0 ? segments[0] : string.Empty;
            var display = string.IsNullOrWhiteSpace(first) ? host : $"{host}/{first}";
            if (segments.Length > 1 || !string.IsNullOrWhiteSpace(uri.Query))
            {
                display += "…";
            }
            return display;
        }
        return text;
    }

    private bool SetField<T>(ref T field, T value, [CallerMemberName] string? name = null)
    {
        if (EqualityComparer<T>.Default.Equals(field, value))
        {
            return false;
        }

        field = value;
        OnPropertyChanged(name);
        return true;
    }

    private void OnPropertyChanged([CallerMemberName] string? name = null)
        => PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
}
