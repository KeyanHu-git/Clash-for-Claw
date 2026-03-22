using System.Diagnostics;
using System.Text;

namespace ClashForClaw.Services;

public sealed class ProcessResult
{
    public int ExitCode { get; init; }
    public string StandardOutput { get; init; } = string.Empty;
    public string StandardError { get; init; } = string.Empty;
}

public static class ProcessRunner
{
    public static ProcessResult Run(string fileName, string arguments)
    {
        var startInfo = new ProcessStartInfo
        {
            FileName = fileName,
            Arguments = arguments,
            CreateNoWindow = true,
            UseShellExecute = false,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
        };

        using var process = new Process { StartInfo = startInfo };
        var output = new StringBuilder();
        var error = new StringBuilder();

        process.OutputDataReceived += (_, args) =>
        {
            if (args.Data is not null)
            {
                output.AppendLine(args.Data);
            }
        };
        process.ErrorDataReceived += (_, args) =>
        {
            if (args.Data is not null)
            {
                error.AppendLine(args.Data);
            }
        };

        process.Start();
        process.BeginOutputReadLine();
        process.BeginErrorReadLine();
        process.WaitForExit();

        return new ProcessResult
        {
            ExitCode = process.ExitCode,
            StandardOutput = output.ToString().Trim(),
            StandardError = error.ToString().Trim(),
        };
    }
}
