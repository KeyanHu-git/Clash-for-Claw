using System;
using System.Collections.Generic;
using System.Net;
using System.Net.Http;
using System.Text;
using System.Text.Json.Serialization;
using System.Text.Json.Serialization.Metadata;
using System.Threading.Tasks;

namespace ClashForClaw.Services;

public sealed class AdapterApiClient
{
    private static readonly EmptyRequest EmptyBody = EmptyRequest.Instance;

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
        => SendAsync(HttpMethod.Get, "/config", requireNonce: false, AppJsonContext.Default.AdapterConfigResponse);

    public Task<AdapterStatusResponse> GetStatusAsync()
        => SendAsync(HttpMethod.Get, "/status", requireNonce: false, AppJsonContext.Default.AdapterStatusResponse, requireOkResponse: false);

    public Task<AdapterReloadResponse> ReloadAsync()
        => SendAsync(HttpMethod.Post, "/config/reload", EmptyBody, requireNonce: true, AppJsonContext.Default.EmptyRequest, AppJsonContext.Default.AdapterReloadResponse);

    public Task SetConfigAsync(ProxyConfigUpdateRequest payload)
        => SendAsync(HttpMethod.Post, "/config", payload, requireNonce: true, AppJsonContext.Default.ProxyConfigUpdateRequest, AppJsonContext.Default.AdapterBaseResponse);

    public Task<AdapterSubscriptionsResponse> GetSubscriptionsAsync()
        => SendAsync(HttpMethod.Get, "/subscriptions", requireNonce: false, AppJsonContext.Default.AdapterSubscriptionsResponse);

    public Task EnableSystemProxyAsync()
        => SendAsync(HttpMethod.Post, "/system-proxy/enable", EmptyBody, requireNonce: true, AppJsonContext.Default.EmptyRequest, AppJsonContext.Default.AdapterBaseResponse);

    public Task DisableSystemProxyAsync()
        => SendAsync(HttpMethod.Post, "/system-proxy/disable", EmptyBody, requireNonce: true, AppJsonContext.Default.EmptyRequest, AppJsonContext.Default.AdapterBaseResponse);

    public Task ShutdownDesktopBackendAsync()
        => SendAsync(HttpMethod.Post, "/daemon/shutdown", EmptyBody, requireNonce: true, AppJsonContext.Default.EmptyRequest, AppJsonContext.Default.AdapterBaseResponse);

    public Task CreateSubscriptionAsync(string url, string? name)
    {
        var payload = new SubscriptionCreateRequest
        {
            Url = url,
            Name = string.IsNullOrWhiteSpace(name) ? string.Empty : name,
        };
        return SendAsync(HttpMethod.Post, "/subscriptions", payload, requireNonce: true, AppJsonContext.Default.SubscriptionCreateRequest, AppJsonContext.Default.AdapterBaseResponse);
    }

    public Task ImportSubscriptionAsync(string path)
    {
        var payload = new SubscriptionImportRequest
        {
            Path = path,
        };
        return SendAsync(HttpMethod.Post, "/subscriptions/import", payload, requireNonce: true, AppJsonContext.Default.SubscriptionImportRequest, AppJsonContext.Default.AdapterBaseResponse);
    }

    public Task ActivateSubscriptionAsync(string id)
        => SendAsync(HttpMethod.Post, $"/subscriptions/{id}/activate", EmptyBody, requireNonce: true, AppJsonContext.Default.EmptyRequest, AppJsonContext.Default.AdapterBaseResponse);

    public Task RefreshSubscriptionAsync(string id)
        => SendAsync(HttpMethod.Post, $"/subscriptions/{id}/refresh", EmptyBody, requireNonce: true, AppJsonContext.Default.EmptyRequest, AppJsonContext.Default.AdapterBaseResponse);

    public Task RefreshAllSubscriptionsAsync()
        => SendAsync(HttpMethod.Post, "/subscriptions/refresh", EmptyBody, requireNonce: true, AppJsonContext.Default.EmptyRequest, AppJsonContext.Default.AdapterBaseResponse);

    public Task RenameSubscriptionAsync(string id, string name)
    {
        var payload = new SubscriptionRenameRequest
        {
            Name = name,
        };
        return SendAsync(HttpMethod.Post, $"/subscriptions/{id}/rename", payload, requireNonce: true, AppJsonContext.Default.SubscriptionRenameRequest, AppJsonContext.Default.AdapterBaseResponse);
    }

    public Task DeleteSubscriptionAsync(string id)
        => SendAsync(HttpMethod.Delete, $"/subscriptions/{id}", requireNonce: true, AppJsonContext.Default.AdapterBaseResponse);

    public async Task<string> CopySubscriptionUrlAsync(string id)
    {
        var resp = await SendAsync(HttpMethod.Post, $"/subscriptions/{id}/copy", EmptyBody, requireNonce: true, AppJsonContext.Default.EmptyRequest, AppJsonContext.Default.AdapterCopyResponse);
        return resp.Url ?? string.Empty;
    }

    private Task<TResponse> SendAsync<TResponse>(HttpMethod method, string path, bool requireNonce, JsonTypeInfo<TResponse> responseTypeInfo, bool requireOkResponse = true)
        where TResponse : AdapterBaseResponse
        => SendAsync(method, path, payloadJson: null, requireNonce, responseTypeInfo, requireOkResponse);

    private Task<TResponse> SendAsync<TRequest, TResponse>(HttpMethod method, string path, TRequest payload, bool requireNonce, JsonTypeInfo<TRequest> requestTypeInfo, JsonTypeInfo<TResponse> responseTypeInfo, bool requireOkResponse = true)
        where TResponse : AdapterBaseResponse
        => SendAsync(method, path, AppJson.Serialize(payload, requestTypeInfo), requireNonce, responseTypeInfo, requireOkResponse);

    private async Task<TResponse> SendAsync<TResponse>(HttpMethod method, string path, string? payloadJson, bool requireNonce, JsonTypeInfo<TResponse> responseTypeInfo, bool requireOkResponse)
        where TResponse : AdapterBaseResponse
    {
        var attempt = 0;
        while (true)
        {
            attempt++;
            try
            {
                using var request = new HttpRequestMessage(method, BaseUrl + path);
                if (!string.IsNullOrWhiteSpace(payloadJson))
                {
                    request.Content = new StringContent(payloadJson, Encoding.UTF8, "application/json");
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
                var parsed = AppJson.Deserialize(body, responseTypeInfo);
                if (parsed is null)
                {
                    throw new InvalidOperationException("Invalid response.");
                }

                if (requireOkResponse && !parsed.Ok)
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

        var response = await SendAsync(HttpMethod.Get, "/nonce", requireNonce: false, AppJsonContext.Default.AdapterNonceResponse);
        if (string.IsNullOrWhiteSpace(response.Nonce))
        {
            throw new InvalidOperationException("本地服务未返回授权令牌。");
        }

        nonce = response.Nonce;
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

public sealed class AdapterNonceResponse : AdapterBaseResponse
{
    [JsonPropertyName("nonce")]
    public string? Nonce { get; set; }
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

    [JsonPropertyName("proxy_url")]
    public string? ProxyUrl { get; set; }

    [JsonPropertyName("mihomo_active")]
    public bool MihomoActive { get; set; }

    [JsonPropertyName("mihomo_error")]
    public string? MihomoError { get; set; }

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
