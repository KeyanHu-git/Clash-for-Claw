using Microsoft.UI.Xaml;

namespace ClashForClaw.Controls;

public sealed class BooleanStateTrigger : StateTriggerBase
{
    public static readonly DependencyProperty IsActiveProperty =
        DependencyProperty.Register(
            nameof(IsActive),
            typeof(bool),
            typeof(BooleanStateTrigger),
            new PropertyMetadata(false, OnStateChanged));

    public static readonly DependencyProperty ValueProperty =
        DependencyProperty.Register(
            nameof(Value),
            typeof(bool),
            typeof(BooleanStateTrigger),
            new PropertyMetadata(true, OnStateChanged));

    public bool IsActive
    {
        get => (bool)GetValue(IsActiveProperty);
        set => SetValue(IsActiveProperty, value);
    }

    public bool Value
    {
        get => (bool)GetValue(ValueProperty);
        set => SetValue(ValueProperty, value);
    }

    private static void OnStateChanged(DependencyObject d, DependencyPropertyChangedEventArgs e)
    {
        if (d is BooleanStateTrigger trigger)
        {
            trigger.SetActive(trigger.IsActive == trigger.Value);
        }
    }
}

