using System;
using System.Collections.Generic;
using System.Net;
using System.Net.Http;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading.Tasks;

namespace OpenClawAdapter.Services;

public sealed class AdapterApiClient
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    private readonly HttpClient httpClient = new();
    private string? nonce;

    public AdapterApiClient(string? baseUrl = null)
    {
        BaseUrl = string.IsNullOrWhiteSpace(baseUrl)
            ? "http://127.0.0.1:13000"
            : baseUrl.TrimEnd('/');
        httpClient.Timeout = TimeSpan.FromSeconds(8);
    }

    public string BaseUrl { get; }

    public Task<AdapterConfigResponse> GetConfigAsync()
        => SendAsync<AdapterConfigResponse>(HttpMethod.Get, "/config", null, false);

    public Task<AdapterStatusResponse> GetStatusAsync()
        => SendAsync<AdapterStatusResponse>(HttpMethod.Get, "/status", null, false);

    public Task<AdapterReloadResponse> ReloadAsync()
        => SendAsync<AdapterReloadResponse>(HttpMethod.Post, "/config/reload", new { }, true);

    public Task SetConfigAsync(Dictionary<string, object> payload)
        => SendAsync<AdapterBaseResponse>(HttpMethod.Post, "/config", payload, true);

    public Task<AdapterSubscriptionsResponse> GetSubscriptionsAsync()
        => SendAsync<AdapterSubscriptionsResponse>(HttpMethod.Get, "/subscriptions", null, false);

    public Task EnableSystemProxyAsync()
        => SendAsync<AdapterBaseResponse>(HttpMethod.Post, "/system-proxy/enable", new { }, true);

    public Task DisableSystemProxyAsync()
        => SendAsync<AdapterBaseResponse>(HttpMethod.Post, "/system-proxy/disable", new { }, true);

    public Task CreateSubscriptionAsync(string url, string? name)
    {
        var payload = new Dictionary<string, object>
        {
            ["url"] = url,
            ["name"] = string.IsNullOrWhiteSpace(name) ? string.Empty : name,
        };
        return SendAsync<AdapterBaseResponse>(HttpMethod.Post, "/subscriptions", payload, true);
    }

    public Task ImportSubscriptionAsync(string path)
    {
        var payload = new Dictionary<string, object>
        {
            ["path"] = path,
        };
        return SendAsync<AdapterBaseResponse>(HttpMethod.Post, "/subscriptions/import", payload, true);
    }

    public Task ActivateSubscriptionAsync(string id)
        => SendAsync<AdapterBaseResponse>(HttpMethod.Post, $"/subscriptions/{id}/activate", new { }, true);

    public Task RefreshSubscriptionAsync(string id)
        => SendAsync<AdapterBaseResponse>(HttpMethod.Post, $"/subscriptions/{id}/refresh", new { }, true);

    public Task RefreshAllSubscriptionsAsync()
        => SendAsync<AdapterBaseResponse>(HttpMethod.Post, "/subscriptions/refresh", new { }, true);

    public Task RenameSubscriptionAsync(string id, string name)
    {
        var payload = new Dictionary<string, object>
        {
            ["name"] = name,
        };
        return SendAsync<AdapterBaseResponse>(HttpMethod.Post, $"/subscriptions/{id}/rename", payload, true);
    }

    public Task DeleteSubscriptionAsync(string id)
        => SendAsync<AdapterBaseResponse>(HttpMethod.Delete, $"/subscriptions/{id}", null, true);

    public async Task<string> CopySubscriptionUrlAsync(string id)
    {
        var resp = await SendAsync<AdapterCopyResponse>(HttpMethod.Post, $"/subscriptions/{id}/copy", new { }, true);
        return resp.Url ?? string.Empty;
    }

    private async Task<T> SendAsync<T>(HttpMethod method, string path, object? payload, bool requireNonce)
        where T : AdapterBaseResponse
    {
        var attempt = 0;
        while (true)
        {
            attempt++;
            try
            {
                using var request = new HttpRequestMessage(method, BaseUrl + path);
                if (payload is not null)
                {
                    var json = JsonSerializer.Serialize(payload);
                    request.Content = new StringContent(json, Encoding.UTF8, "application/json");
                }
                if (requireNonce)
                {
                    await EnsureNonceAsync();
                    if (!string.IsNullOrWhiteSpace(nonce))
                    {
                        request.Headers.Add("X-Adapter-Nonce", nonce);
                    }
                }

                using var response = await httpClient.SendAsync(request);
                if (requireNonce && response.StatusCode == HttpStatusCode.Unauthorized && attempt == 1)
                {
                    nonce = null;
                    continue;
                }

                var body = await response.Content.ReadAsStringAsync();
                var parsed = JsonSerializer.Deserialize<T>(body, JsonOptions);
                if (parsed is null)
                {
                    throw new InvalidOperationException("Invalid response.");
                }
                if (!parsed.Ok)
                {
                    throw new InvalidOperationException(string.IsNullOrWhiteSpace(parsed.Error) ? "Request failed." : parsed.Error);
                }
                return parsed;
            }
            catch (HttpRequestException ex)
            {
                throw new InvalidOperationException("无法连接本地服务。", ex);
            }
            catch (TaskCanceledException ex)
            {
                throw new InvalidOperationException("本地服务响应超时。", ex);
            }
        }
    }

    private async Task EnsureNonceAsync()
    {
        if (!string.IsNullOrWhiteSpace(nonce))
        {
            return;
        }

        var html = await httpClient.GetStringAsync(BaseUrl + "/");
        nonce = ExtractNonce(html);
    }

    private static string? ExtractNonce(string html)
    {
        const string marker = "adapter-nonce\" content=\"";
        var idx = html.IndexOf(marker, StringComparison.OrdinalIgnoreCase);
        if (idx < 0)
        {
            return null;
        }
        idx += marker.Length;
        var end = html.IndexOf('"', idx);
        if (end <= idx)
        {
            return null;
        }
        return html.Substring(idx, end - idx);
    }
}

public class AdapterBaseResponse
{
    [JsonPropertyName("ok")]
    public bool Ok { get; set; }

    [JsonPropertyName("error")]
    public string? Error { get; set; }
}

public sealed class AdapterCopyResponse : AdapterBaseResponse
{
    [JsonPropertyName("url")]
    public string? Url { get; set; }
}

public sealed class AdapterConfigResponse : AdapterBaseResponse
{
    [JsonPropertyName("config")]
    public AdapterConfig? Config { get; set; }

    [JsonPropertyName("token_present")]
    public bool TokenPresent { get; set; }

    [JsonPropertyName("token_hint")]
    public string? TokenHint { get; set; }
}

public sealed class AdapterConfig
{
    [JsonPropertyName("gateway")]
    public AdapterGateway? Gateway { get; set; }

    [JsonPropertyName("proxy")]
    public AdapterProxy? Proxy { get; set; }
}

public sealed class AdapterGateway
{
    [JsonPropertyName("url")]
    public string? Url { get; set; }
}

public sealed class AdapterProxy
{
    [JsonPropertyName("mode")]
    public string? Mode { get; set; }

    [JsonPropertyName("local_port")]
    public int LocalPort { get; set; }

    [JsonPropertyName("subscription_url")]
    public string? SubscriptionUrl { get; set; }

    [JsonPropertyName("subscription_refresh_hours")]
    public int SubscriptionRefreshHours { get; set; }

    [JsonPropertyName("subscription_probe_minutes")]
    public int SubscriptionProbeMinutes { get; set; }
}

public sealed class AdapterStatusResponse : AdapterBaseResponse
{
    [JsonPropertyName("proxy")]
    public AdapterProxyStatus? Proxy { get; set; }

    [JsonPropertyName("billing")]
    public AdapterBilling? Billing { get; set; }

    [JsonPropertyName("system_proxy")]
    public AdapterSystemProxyStatus? SystemProxy { get; set; }

    [JsonPropertyName("probe")]
    public AdapterProbe? Probe { get; set; }
}

public sealed class AdapterSystemProxyStatus
{
    [JsonPropertyName("enabled")]
    public bool Enabled { get; set; }

    [JsonPropertyName("server")]
    public string? Server { get; set; }
}

public sealed class AdapterProxyStatus
{
    [JsonPropertyName("mode")]
    public string? Mode { get; set; }

    [JsonPropertyName("effective_mode")]
    public string? EffectiveMode { get; set; }

    [JsonPropertyName("fallback")]
    public bool Fallback { get; set; }

    [JsonPropertyName("fallback_reason")]
    public string? FallbackReason { get; set; }
}

public sealed class AdapterBilling
{
    [JsonPropertyName("upload")]
    public long Upload { get; set; }

    [JsonPropertyName("download")]
    public long Download { get; set; }

    [JsonPropertyName("used")]
    public double Used { get; set; }

    [JsonPropertyName("limit")]
    public double Limit { get; set; }

    [JsonPropertyName("unit")]
    public string? Unit { get; set; }

    [JsonPropertyName("state")]
    public string? State { get; set; }

    [JsonPropertyName("updated_at")]
    public string? UpdatedAt { get; set; }
}

public sealed class AdapterReloadResponse : AdapterBaseResponse
{
    [JsonPropertyName("proxy")]
    public AdapterProxyStatus? Proxy { get; set; }

    [JsonPropertyName("probe")]
    public AdapterProbe? Probe { get; set; }
}

public sealed class AdapterProbe
{
    [JsonPropertyName("gateway_ok")]
    public bool GatewayOk { get; set; }

    [JsonPropertyName("internet_ok")]
    public bool InternetOk { get; set; }
}

public sealed class AdapterSubscriptionsResponse : AdapterBaseResponse
{
    [JsonPropertyName("active_id")]
    public string? ActiveId { get; set; }

    [JsonPropertyName("subscriptions")]
    public List<AdapterSubscription> Subscriptions { get; set; } = new();
}

public sealed class AdapterSubscription
{
    [JsonPropertyName("id")]
    public string? Id { get; set; }

    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("source")]
    public string? Source { get; set; }

    [JsonPropertyName("url")]
    public string? Url { get; set; }

    [JsonPropertyName("file_path")]
    public string? FilePath { get; set; }

    [JsonPropertyName("state")]
    public string? State { get; set; }

    [JsonPropertyName("last_success_at")]
    public long LastSuccessAt { get; set; }

    [JsonPropertyName("usage_used")]
    public double UsageUsed { get; set; }

    [JsonPropertyName("usage_limit")]
    public double UsageLimit { get; set; }

    [JsonPropertyName("usage_unit")]
    public string? UsageUnit { get; set; }

    [JsonPropertyName("expire_at")]
    public long ExpireAt { get; set; }

    [JsonPropertyName("updated_at")]
    public long UpdatedAt { get; set; }
}
