using System;
using System.Threading;
using System.Threading.Tasks;
using Microsoft.UI.Xaml;

namespace ClashForClaw.Services;

public static class SingleInstanceManager
{
    private const string MutexName = @"Local\ClashForClaw.SingleInstance";
    private const string ActivateEventName = @"Local\ClashForClaw.Activate";
    private static readonly TimeSpan SignalRetryWindow = TimeSpan.FromSeconds(3);
    private static readonly TimeSpan SignalRetryInterval = TimeSpan.FromMilliseconds(120);

    private static Mutex? instanceMutex;
    private static EventWaitHandle? activateEvent;
    private static Task? activationTask;

    public static bool TryAcquirePrimaryInstance()
    {
        if (instanceMutex is not null)
        {
            return true;
        }

        instanceMutex = new Mutex(initiallyOwned: true, MutexName, out var createdNew);
        if (!createdNew)
        {
            instanceMutex.Dispose();
            instanceMutex = null;
            return false;
        }

        activateEvent = new EventWaitHandle(false, EventResetMode.AutoReset, ActivateEventName);
        return true;
    }

    public static bool SignalPrimaryInstance()
    {
        var deadline = DateTime.UtcNow + SignalRetryWindow;
        while (DateTime.UtcNow < deadline)
        {
            try
            {
                using var existingEvent = EventWaitHandle.OpenExisting(ActivateEventName);
                existingEvent.Set();
                return true;
            }
            catch (WaitHandleCannotBeOpenedException)
            {
                // The primary instance owns the mutex but may still be wiring the wake signal.
            }
            catch
            {
                return false;
            }

            Thread.Sleep(SignalRetryInterval);
        }

        return false;
    }

    public static void StartActivationListener(Window window)
    {
        if (activateEvent is null || activationTask is not null)
        {
            return;
        }

        activationTask = Task.Run(() =>
        {
            while (true)
            {
                activateEvent.WaitOne();
                window.DispatcherQueue.TryEnqueue(() =>
                {
                    try
                    {
                        WindowManager.Show(window);
                    }
                    catch
                    {
                        // A wake signal must never destabilize the primary instance.
                    }
                });
            }
        });
    }
}
