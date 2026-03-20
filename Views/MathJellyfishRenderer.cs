using System;
using System.Collections.Generic;
using System.Numerics;
using Microsoft.Graphics.Canvas;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Media;
using Windows.Foundation;
using Windows.UI;

namespace OpenClawAdapter.Views;

// Math-driven jellyfish inspired by a Heywhale community project (Shelter, 2025-08-04).
internal sealed class MathJellyfishRenderer
{
    private const float BaseRange = 10000f;
    private const float BaseCenter = 200f;
    private const float BaseScale = 520f;
    private const float MinEpsilon = 0.0001f;

    private readonly List<MathJellyfish> jellyfish = new();
    private readonly Random random = new(42);

    private Color accentBlue;
    private Color accentGreen;
    private Color mutedGlow;

    public MathJellyfishRenderer()
    {
        UpdatePalette();
        BuildJellyfish();
    }

    public void UpdatePalette()
    {
        accentBlue = ResolveColor("AccentBlueBrush", Color.FromArgb(130, 34, 211, 238));
        accentGreen = ResolveColor("AccentBrush", Color.FromArgb(120, 45, 212, 191));
        mutedGlow = ResolveColor("TextMutedBrush", Color.FromArgb(38, 148, 163, 184));

        foreach (var jelly in jellyfish)
        {
            jelly.ApplyPalette(accentBlue, accentGreen, mutedGlow);
        }
    }

    public void Draw(CanvasDrawingSession ds, Size bounds, double timeSeconds)
    {
        ds.Clear(Color.FromArgb(0, 0, 0, 0));

        if (bounds.Width <= 0 || bounds.Height <= 0)
        {
            return;
        }

        var baseScale = (float)(Math.Min(bounds.Width, bounds.Height) / BaseScale);
        var time = (float)timeSeconds;

        foreach (var jelly in jellyfish)
        {
            jelly.Draw(ds, bounds, time, baseScale);
        }
    }

    private void BuildJellyfish()
    {
        jellyfish.Clear();

        jellyfish.Add(MathJellyfish.Create(
            random,
            JellyfishKind.LongJellyfish,
            GenerateLinearPoints(2400, 100f),
            new Vector2(0.7f, 0.32f),
            0.95f,
            0.42f,
            0.55f));

        jellyfish.Add(MathJellyfish.Create(
            random,
            JellyfishKind.Fish,
            GenerateLinearPoints(2400, 350f),
            new Vector2(0.32f, 0.66f),
            0.85f,
            0.36f,
            0.5f));

        jellyfish.Add(MathJellyfish.Create(
            random,
            JellyfishKind.Worm,
            GenerateLinearPoints(2200, 235f),
            new Vector2(0.22f, 0.36f),
            0.72f,
            0.44f,
            0.45f));

        jellyfish.Add(MathJellyfish.Create(
            random,
            JellyfishKind.LittleJellyfish,
            GenerateLittleJellyfishPoints(2000),
            new Vector2(0.78f, 0.62f),
            0.62f,
            0.55f,
            0.4f));

        foreach (var jelly in jellyfish)
        {
            jelly.ApplyPalette(accentBlue, accentGreen, mutedGlow);
        }
    }

    private static Vector2[] GenerateLinearPoints(int count, float yDivisor)
    {
        var points = new Vector2[count];
        var step = BaseRange / Math.Max(1, count - 1);
        for (var i = 0; i < count; i++)
        {
            var x = i * step;
            points[i] = new Vector2(x, x / yDivisor);
        }

        return points;
    }

    private static Vector2[] GenerateLittleJellyfishPoints(int count)
    {
        var points = new Vector2[count];
        var step = BaseRange / Math.Max(1, count - 1);
        for (var i = 0; i < count; i++)
        {
            var x = i * step;
            points[i] = new Vector2(x % 200f, x / 55f);
        }

        return points;
    }

    private static Color ResolveColor(string key, Color fallback)
    {
        if (Application.Current?.Resources.TryGetValue(key, out var value) == true
            && value is SolidColorBrush brush)
        {
            var color = brush.Color;
            return Color.FromArgb(fallback.A, color.R, color.G, color.B);
        }

        return fallback;
    }

    private sealed class MathJellyfish
    {
        private readonly JellyfishKind kind;
        private readonly Vector2[] basePoints;
        private readonly Vector2[] positions;
        private readonly Vector2 baseCenter;
        private readonly Vector2 anchorRatio;
        private readonly float size;
        private readonly float timeScale;
        private readonly float opacity;
        private readonly float driftRadiusX;
        private readonly float driftRadiusY;
        private readonly float driftSpeedA;
        private readonly float driftSpeedB;
        private readonly float rotationSpeed;
        private readonly float rotationAmount;
        private readonly float phase;
        private readonly int glowStride;

        private Color pointColor;
        private Color glowColor;
        private float pointRadius;

        private MathJellyfish(
            JellyfishKind kind,
            Vector2[] basePoints,
            Vector2 baseCenter,
            Vector2 anchorRatio,
            float size,
            float timeScale,
            float opacity,
            float driftRadiusX,
            float driftRadiusY,
            float driftSpeedA,
            float driftSpeedB,
            float rotationSpeed,
            float rotationAmount,
            float phase,
            int glowStride)
        {
            this.kind = kind;
            this.basePoints = basePoints;
            this.baseCenter = baseCenter;
            this.anchorRatio = anchorRatio;
            this.size = size;
            this.timeScale = timeScale;
            this.opacity = opacity;
            this.driftRadiusX = driftRadiusX;
            this.driftRadiusY = driftRadiusY;
            this.driftSpeedA = driftSpeedA;
            this.driftSpeedB = driftSpeedB;
            this.rotationSpeed = rotationSpeed;
            this.rotationAmount = rotationAmount;
            this.phase = phase;
            this.glowStride = glowStride;
            positions = new Vector2[basePoints.Length];
        }

        public static MathJellyfish Create(
            Random random,
            JellyfishKind kind,
            Vector2[] basePoints,
            Vector2 anchorRatio,
            float size,
            float timeScale,
            float opacity)
        {
            var driftRadiusX = 30f + (float)random.NextDouble() * 40f;
            var driftRadiusY = 24f + (float)random.NextDouble() * 32f;
            var driftSpeedA = 0.08f + (float)random.NextDouble() * 0.1f;
            var driftSpeedB = 0.06f + (float)random.NextDouble() * 0.1f;
            var rotationSpeed = 0.08f + (float)random.NextDouble() * 0.1f;
            var rotationAmount = 0.05f + (float)random.NextDouble() * 0.08f;
            var phase = (float)random.NextDouble() * MathF.PI * 2f;
            var glowStride = 3 + random.Next(0, 2);
            var baseCenter = ComputeBaseCenter(kind, basePoints);

            return new MathJellyfish(
                kind,
                basePoints,
                baseCenter,
                anchorRatio,
                size,
                timeScale,
                opacity,
                driftRadiusX,
                driftRadiusY,
                driftSpeedA,
                driftSpeedB,
                rotationSpeed,
                rotationAmount,
                phase,
                glowStride);
        }

        public void ApplyPalette(Color accentBlue, Color accentGreen, Color glowBase)
        {
            var baseColor = kind switch
            {
                JellyfishKind.LongJellyfish => accentGreen,
                JellyfishKind.Worm => accentGreen,
                JellyfishKind.Fish => accentBlue,
                JellyfishKind.LittleJellyfish => accentBlue,
                _ => accentBlue,
            };

            var pointAlpha = (byte)Math.Clamp(opacity * 230f, 70f, 200f);
            var glowAlpha = (byte)Math.Clamp(opacity * 110f, 30f, 80f);
            pointColor = WithAlpha(baseColor, pointAlpha);
            glowColor = WithAlpha(glowBase, glowAlpha);
            pointRadius = 0.6f + opacity * 0.8f;
        }

        public void Draw(CanvasDrawingSession ds, Size bounds, float timeSeconds, float baseScale)
        {
            var t = timeSeconds * timeScale + phase;
            var center = new Vector2((float)bounds.Width * anchorRatio.X, (float)bounds.Height * anchorRatio.Y);
            var drift = new Vector2(
                MathF.Sin(t * driftSpeedA) * driftRadiusX * baseScale,
                MathF.Cos(t * driftSpeedB) * driftRadiusY * baseScale);
            var rotation = Matrix3x2.CreateRotation(rotationAmount * MathF.Sin(t * rotationSpeed));
            var scale = baseScale * size;
            var radius = Math.Clamp(pointRadius * scale, 0.7f, 2.2f);
            var glowRadius = radius * 2.6f;

            for (var i = 0; i < basePoints.Length; i++)
            {
                var p = Evaluate(kind, basePoints[i], t);
                var local = new Vector2(p.X - baseCenter.X, p.Y - baseCenter.Y);
                var transformed = Vector2.Transform(local, rotation) * scale + center + drift;
                positions[i] = transformed;
            }

            for (var i = 0; i < positions.Length; i += glowStride)
            {
                var pos = positions[i];
                if (IsFinite(pos))
                {
                    ds.FillCircle(pos, glowRadius, glowColor);
                }
            }

            for (var i = 0; i < positions.Length; i++)
            {
                var pos = positions[i];
                if (IsFinite(pos))
                {
                    ds.FillCircle(pos, radius, pointColor);
                }
            }
        }

        private static Vector2 ComputeBaseCenter(JellyfishKind kind, Vector2[] points)
        {
            var minX = float.MaxValue;
            var maxX = float.MinValue;
            var minY = float.MaxValue;
            var maxY = float.MinValue;

            foreach (var point in points)
            {
                var p = Evaluate(kind, point, 0f);
                if (!IsFinite(p))
                {
                    continue;
                }

                minX = MathF.Min(minX, p.X);
                maxX = MathF.Max(maxX, p.X);
                minY = MathF.Min(minY, p.Y);
                maxY = MathF.Max(maxY, p.Y);
            }

            if (!float.IsFinite(minX) || !float.IsFinite(minY))
            {
                return new Vector2(BaseCenter, BaseCenter);
            }

            return new Vector2((minX + maxX) / 2f, (minY + maxY) / 2f);
        }

        private static Vector2 Evaluate(JellyfishKind kind, Vector2 p, float t)
        {
            var x = p.X;
            var y = p.Y;

            return kind switch
            {
                JellyfishKind.Fish => LineToFish(x, y, t),
                JellyfishKind.LongJellyfish => LineToLongJellyfish(x, y, t),
                JellyfishKind.Worm => LineToWorm(x, y, t),
                JellyfishKind.LittleJellyfish => LineToLittleJellyfish(x, y, t),
                _ => LineToFish(x, y, t),
            };
        }

        private static Vector2 LineToFish(float x, float y, float t)
        {
            var k = (4f + MathF.Cos(y)) * MathF.Cos(x / 4f);
            var e = y / 8f - 20f;
            var d = MathF.Sqrt(k * k + e * e);
            var q = MathF.Sin(k * 3f) + MathF.Sin(y / 19f + 9f) * k * (6f + MathF.Sin(e * 14f - d));
            var c = d - t;

            return new Vector2(
                q * MathF.Cos(d / 8f + t / 4f) + 50f * MathF.Cos(c) + BaseCenter,
                q * MathF.Sin(c) + d * 7f * MathF.Sin(c / 4f) + BaseCenter);
        }

        private static Vector2 LineToLongJellyfish(float x, float y, float t)
        {
            var k = 5f * MathF.Cos(x / 19f) * MathF.Cos(y / 30f);
            var e = y / 8f - 12f;
            var d = (k * k + e * e) / 59f + 2f;
            var a = MathF.Atan2(k, e);
            var safeD = MathF.Max(d, MinEpsilon);
            var q = 4f * MathF.Sin(9f * a)
                + 9f * MathF.Sin(d - t)
                - (k / safeD) * (9f + 3f * MathF.Sin(d * 9f - t * 16f));
            var c = d * d / 7f - t;

            return new Vector2(
                q + 50f * MathF.Cos(c) + BaseCenter,
                q * MathF.Sin(c) + d * 45f - 9f);
        }

        private static Vector2 LineToWorm(float x, float y, float t)
        {
            var k = (4f + 3f * MathF.Sin(y * 2f - t)) * MathF.Cos(x / 29f);
            var e = y / 8f - 13f;
            var d = MathF.Sqrt(k * k + e * e);
            var safeK = MathF.Abs(k) < MinEpsilon ? (k < 0 ? -MinEpsilon : MinEpsilon) : k;
            var q = 3f * MathF.Sin(k * 2f)
                + 0.3f / safeK
                + MathF.Sin(y / 25f) * k * (9f + 4f * MathF.Sin(e * 9f - d * 3f + t * 2f));
            var c = d - t;

            return new Vector2(
                q + 30f * MathF.Cos(c) + BaseCenter,
                q * MathF.Sin(c) + d * 39f - 220f);
        }

        private static Vector2 LineToLittleJellyfish(float x, float y, float t)
        {
            var k = 9f * MathF.Cos(x / 8f);
            var e = y / 8f - 12.5f;
            var d = MathF.Sqrt(k * k + e * e);
            d = (d * d) / 99f + MathF.Sin(t) / 6f + 0.5f;
            var a = MathF.Atan2(k, e);
            var safeD = MathF.Max(d, MinEpsilon);
            var q = 99f - e * MathF.Sin(a * 7f) / safeD + k * (3f + 2f * MathF.Cos(d * d - t));
            var c = d / 2f + e / 69f - t / 8f;

            return new Vector2(
                q * MathF.Sin(c) + BaseCenter,
                (q + 19f * d) * MathF.Cos(c) + BaseCenter);
        }

        private static bool IsFinite(Vector2 value)
        {
            return float.IsFinite(value.X) && float.IsFinite(value.Y);
        }

        private static Color WithAlpha(Color color, byte alpha)
        {
            return Color.FromArgb(alpha, color.R, color.G, color.B);
        }
    }

    private enum JellyfishKind
    {
        Fish,
        LongJellyfish,
        Worm,
        LittleJellyfish,
    }
}


