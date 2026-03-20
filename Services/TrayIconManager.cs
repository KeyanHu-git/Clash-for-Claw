using System.Diagnostics;
using System.Drawing;
using System.Windows.Forms;

namespace OpenClawAdapter.Services;

public sealed class TrayIconManager : IDisposable
{
    private NotifyIcon? notifyIcon;
    private ToolStripMenuItem? statusItem;
    private bool isInitialized;

    public void Initialize(Action showCallback, Action exitCallback, Func<string> statusProvider)
    {
        if (isInitialized)
        {
            return;
        }

        isInitialized = true;
        notifyIcon = new NotifyIcon
        {
            Text = "OpenClaw \u9002\u914D\u5668",
            Icon = LoadIcon(),
            Visible = true,
        };

        statusItem = new ToolStripMenuItem(statusProvider())
        {
            Enabled = false,
        };

        var menu = new ContextMenuStrip();
        menu.Items.Add(statusItem);
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add(new ToolStripMenuItem("\u663E\u793A\u4E3B\u754C\u9762", null, (_, _) => showCallback()));
        menu.Items.Add(new ToolStripMenuItem("\u9000\u51FA", null, (_, _) => exitCallback()));

        notifyIcon.ContextMenuStrip = menu;
        notifyIcon.DoubleClick += (_, _) => showCallback();
    }

    public void UpdateStatus(string status)
    {
        if (statusItem is not null)
        {
            statusItem.Text = status;
        }
    }

    public void Show()
    {
        if (notifyIcon is not null)
        {
            notifyIcon.Visible = true;
        }
    }

    public void Hide()
    {
        if (notifyIcon is not null)
        {
            notifyIcon.Visible = false;
        }
    }

    public void Dispose()
    {
        if (notifyIcon is not null)
        {
            notifyIcon.Visible = false;
            notifyIcon.Dispose();
            notifyIcon = null;
            statusItem = null;
        }
    }

    private static Icon LoadIcon()
    {
        try
        {
            var path = Environment.ProcessPath ?? Process.GetCurrentProcess().MainModule?.FileName;
            if (!string.IsNullOrWhiteSpace(path))
            {
                var icon = Icon.ExtractAssociatedIcon(path);
                if (icon is not null)
                {
                    return icon;
                }
            }
        }
        catch
        {
            // Ignore icon loading errors.
        }

        return SystemIcons.Application;
    }
}

