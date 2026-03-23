using System.Text.Json;
using System.Text.Json.Serialization;
using System.Text.Json.Serialization.Metadata;
using ClashForClaw.Models;

namespace ClashForClaw.Services;

public static class AppJson
{
    public static string Serialize<TValue>(TValue value, JsonTypeInfo<TValue> typeInfo)
        => JsonSerializer.Serialize(value, typeInfo);

    public static TValue? Deserialize<TValue>(string json, JsonTypeInfo<TValue> typeInfo)
        => JsonSerializer.Deserialize(json, typeInfo);
}

public sealed class EmptyRequest
{
    public static EmptyRequest Instance { get; } = new();

    private EmptyRequest()
    {
    }
}

public sealed class ProxyConfigUpdateRequest
{
    [JsonPropertyName("proxy")]
    public ProxyConfigPatch Proxy { get; init; } = new();
}

public sealed class ProxyConfigPatch
{
    [JsonPropertyName("mode")]
    public string? Mode { get; init; }

    [JsonPropertyName("local_port")]
    public int? LocalPort { get; init; }

    [JsonPropertyName("subscription_refresh_hours")]
    public int? SubscriptionRefreshHours { get; init; }

    [JsonPropertyName("subscription_probe_minutes")]
    public int? SubscriptionProbeMinutes { get; init; }
}

public sealed class SubscriptionCreateRequest
{
    [JsonPropertyName("url")]
    public string Url { get; init; } = string.Empty;

    [JsonPropertyName("name")]
    public string Name { get; init; } = string.Empty;
}

public sealed class SubscriptionImportRequest
{
    [JsonPropertyName("path")]
    public string Path { get; init; } = string.Empty;
}

public sealed class SubscriptionRenameRequest
{
    [JsonPropertyName("name")]
    public string Name { get; init; } = string.Empty;
}

internal sealed class ServiceCommandResponse
{
    [JsonPropertyName("ok")]
    public bool Ok { get; init; }

    [JsonPropertyName("action")]
    public string Action { get; init; } = string.Empty;

    [JsonPropertyName("mode")]
    public string Mode { get; init; } = string.Empty;

    [JsonPropertyName("status")]
    public string Status { get; init; } = string.Empty;

    [JsonPropertyName("error")]
    public string Error { get; init; } = string.Empty;

    [JsonPropertyName("reason")]
    public string Reason { get; init; } = string.Empty;

    [JsonPropertyName("hint")]
    public string Hint { get; init; } = string.Empty;

    [JsonPropertyName("requiresElevation")]
    public bool RequiresElevation { get; init; }

    [JsonPropertyName("requiresTaskSchedulerAccess")]
    public bool RequiresTaskSchedulerAccess { get; init; }
}

[JsonSourceGenerationOptions(WriteIndented = false)]
[JsonSerializable(typeof(AdapterBaseResponse))]
[JsonSerializable(typeof(AdapterBilling))]
[JsonSerializable(typeof(AdapterConfig))]
[JsonSerializable(typeof(AdapterConfigResponse))]
[JsonSerializable(typeof(AdapterCopyResponse))]
[JsonSerializable(typeof(AdapterGateway))]
[JsonSerializable(typeof(AdapterNonceResponse))]
[JsonSerializable(typeof(AdapterProbe))]
[JsonSerializable(typeof(AdapterProxy))]
[JsonSerializable(typeof(AdapterProxyStatus))]
[JsonSerializable(typeof(AdapterReloadResponse))]
[JsonSerializable(typeof(AdapterStatusResponse))]
[JsonSerializable(typeof(AdapterSubscription))]
[JsonSerializable(typeof(AdapterSubscriptionsResponse))]
[JsonSerializable(typeof(AdapterSystemProxyStatus))]
[JsonSerializable(typeof(AppSettings))]
[JsonSerializable(typeof(EmptyRequest))]
[JsonSerializable(typeof(ProxyConfigPatch))]
[JsonSerializable(typeof(ProxyConfigUpdateRequest))]
[JsonSerializable(typeof(ServiceCommandResponse))]
[JsonSerializable(typeof(SubscriptionCreateRequest))]
[JsonSerializable(typeof(SubscriptionImportRequest))]
[JsonSerializable(typeof(SubscriptionRenameRequest))]
internal partial class AppJsonContext : JsonSerializerContext
{
}

[JsonSourceGenerationOptions(WriteIndented = true)]
[JsonSerializable(typeof(AppSettings))]
internal partial class AppJsonIndentedContext : JsonSerializerContext
{
}
