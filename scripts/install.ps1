# radii5 installer for Windows
# Usage: irm https://raw.githubusercontent.com/ohcass/radii5/main/scripts/install.ps1 | iex

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$esc = [char]27

$repo       = "ohcass/radii5"
$installDir = "$env:USERPROFILE\.radii5\bin"
$threads    = 8

# ── arch ──────────────────────────────────────────────────────────────────────
$arch   = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
$suffix = if ($arch -eq "Arm64") { "windows-arm64" } else { "windows-amd64" }

Write-Host ""
Write-Host "  radii5 installer" -ForegroundColor Cyan
Write-Host "  platform: $suffix" -ForegroundColor DarkGray
Write-Host ""

New-Item -ItemType Directory -Force -Path $installDir | Out-Null

# ── compile C# parallel chunk downloader ──────────────────────────────────────
if (-not ([System.Management.Automation.PSTypeName]'ChunkDownloader').Type) {
Add-Type -Language CSharp @"
using System;
using System.IO;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Threading;
using System.Threading.Tasks;
using System.Collections.Concurrent;

public static class ChunkDownloader
{
    static long _downloaded;
    static long _total;
    const  int  BarWidth = 30;

    static string FmtBytes(long n) {
        if (n >= 1 << 20) return string.Format("{0:F1} MB", (double)n / (1 << 20));
        if (n >= 1 << 10) return string.Format("{0:F1} KB", (double)n / (1 << 10));
        return n + " B";
    }

    static void DrawBar(long cur, long tot) {
        int filled = (tot > 0) ? (int)Math.Min((double)cur / tot * BarWidth, BarWidth) : BarWidth / 2;
        int pct    = (tot > 0) ? (int)((double)cur / tot * 100) : 0;
        string bar = new string('\u2588', filled) + new string('\u2591', BarWidth - filled);
        string line = string.Format("  \u001b[36m[{0}]\u001b[0m  {1} / {2}  ({3}%)",
            bar, FmtBytes(cur), FmtBytes(tot), pct);
        Console.Write("\r" + line + "\u001b[K");
    }

    static void DrawBarDone(long tot) {
        string bar  = new string('\u2588', BarWidth);
        string line = string.Format("  \u001b[32m[{0}]\u001b[0m  {1} \u2713", bar, FmtBytes(tot));
        Console.Write("\r" + line + "\u001b[K\n");
    }

    public static void Download(string url, string dest, int numThreads) {
        using (var client = new HttpClient()) {
            client.Timeout = TimeSpan.FromMinutes(30);
            client.DefaultRequestHeaders.UserAgent.ParseAdd("radii5-installer");

            long total = 0;
            try {
                var headReq = new HttpRequestMessage(HttpMethod.Head, url);
                var headRes = client.SendAsync(headReq).GetAwaiter().GetResult();
                total = headRes.Content.Headers.ContentLength ?? 0;
            } catch {}

            _downloaded = 0;
            _total      = total;

            if (total <= 0 || numThreads <= 1) {
                using (var rs = client.GetStreamAsync(url).GetAwaiter().GetResult())
                using (var fs = File.OpenWrite(dest)) {
                    var buf = new byte[65536];
                    int n;
                    while ((n = rs.Read(buf, 0, buf.Length)) > 0) {
                        fs.Write(buf, 0, n);
                        Interlocked.Add(ref _downloaded, n);
                        DrawBar(_downloaded, total);
                    }
                }
                DrawBarDone(_downloaded);
                return;
            }

            long chunkSize = total / numThreads;
            var  tmpFiles  = new string[numThreads];
            var  tasks     = new Task[numThreads];
            var  errors    = new ConcurrentBag<string>();

            for (int i = 0; i < numThreads; i++) {
                tmpFiles[i] = Path.GetTempFileName();
                long start  = i * chunkSize;
                long end    = (i == numThreads - 1) ? total - 1 : start + chunkSize - 1;
                string tmp  = tmpFiles[i];

                tasks[i] = Task.Run(async () => {
                    const int maxRetries = 3;
                    for (int attempt = 0; attempt < maxRetries; attempt++) {
                        try {
                            var req = new HttpRequestMessage(HttpMethod.Get, url);
                            req.Headers.Range = new RangeHeaderValue(start, end);
                            var res    = await client.SendAsync(req, HttpCompletionOption.ResponseHeadersRead);
                            using (var rs = await res.Content.ReadAsStreamAsync())
                            using (var fs = File.OpenWrite(tmp)) {
                                var buf = new byte[65536];
                                int n;
                                while ((n = await rs.ReadAsync(buf, 0, buf.Length)) > 0) {
                                    fs.Write(buf, 0, n);
                                    Interlocked.Add(ref _downloaded, (long)n);
                                }
                            }
                            return;
                        } catch (Exception ex) {
                            if (attempt == maxRetries - 1)
                                errors.Add(string.Format("chunk failed after {0} attempts: {1}", maxRetries, ex.Message));
                            else {
                                var fi = new FileInfo(tmp);
                                if (fi.Exists) {
                                    Interlocked.Add(ref _downloaded, -fi.Length);
                                    fi.Delete();
                                }
                                await Task.Delay(500 * (attempt + 1));
                            }
                        }
                    }
                });
            }

            while (!Task.WhenAll(tasks).Wait(80)) {
                DrawBar(_downloaded, total);
            }
            DrawBar(total, total);
            DrawBarDone(total);

            if (!errors.IsEmpty) {
                string msg;
                errors.TryTake(out msg);
                throw new Exception("Chunk failed: " + msg);
            }

            using (var fs = File.OpenWrite(dest)) {
                foreach (var tmp in tmpFiles) {
                    byte[] bytes = File.ReadAllBytes(tmp);
                    fs.Write(bytes, 0, bytes.Length);
                    File.Delete(tmp);
                }
            }
        }
    }
}
"@
} # end if ChunkDownloader not loaded

# ── helpers ───────────────────────────────────────────────────────────────────
function Get-GHRelease([string]$Repo) {
    Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest" `
        -Headers @{ "User-Agent" = "radii5-installer"; "Accept" = "application/vnd.github+json" }
}

function Install-Binary([string]$Url, [string]$Dest) {
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    [ChunkDownloader]::Download($Url, $Dest, $threads)
    $sw.Stop()
    $secs    = $sw.Elapsed.TotalSeconds
    $size    = (Get-Item $Dest).Length
    $mbps    = [math]::Round(($size / 1MB) / $secs, 1)
    $elapsed = [math]::Round($secs, 1)
    Write-Host "  $esc[2m${mbps} MB/s  (${elapsed}s,  $threads threads)$esc[0m"
}

# ── 1. yt-dlp ─────────────────────────────────────────────────────────────────
if (Get-Command "yt-dlp.exe" -ErrorAction SilentlyContinue) {
    Write-Host "  `e[2m✓ yt-dlp already installed`e[0m"
    Write-Host ""
} else {
    Write-Host "  `e[36m→`e[0m  yt-dlp"
    $ytRel   = Get-GHRelease "yt-dlp/yt-dlp"
    $ytAsset = $ytRel.assets | Where-Object { $_.name -eq "yt-dlp.exe" } | Select-Object -First 1
    if (-not $ytAsset) { Write-Host "  `e[31m✗`e[0m yt-dlp.exe not found" -ForegroundColor Red; exit 1 }

    $ytDest = Join-Path $installDir "yt-dlp.exe"
    Write-Host "  `e[2mversion`e[0m  $($ytRel.tag_name)"
    Write-Host "  `e[2mdest   `e[0m  $ytDest"
    Write-Host ""
    Install-Binary -Url $ytAsset.browser_download_url -Dest $ytDest
    Write-Host "  `e[32m✓`e[0m yt-dlp $($ytRel.tag_name)"
    Write-Host ""
}

# ── 2. ffmpeg ─────────────────────────────────────────────────────────────────
$ffDest = Join-Path $installDir "ffmpeg.exe"
if (Test-Path $ffDest) {
    Write-Host "  `e[2m✓ ffmpeg already installed`e[0m"
    Write-Host ""
} else {
    Write-Host "  `e[36m→`e[0m  ffmpeg"
    try {
        $ffRel   = Get-GHRelease "BtbN/FFmpeg-Builds"
        $ffAsset = $ffRel.assets |
            Where-Object { $_.name -eq "ffmpeg-master-latest-win64-gpl.zip" } |
            Select-Object -First 1
        if (-not $ffAsset) {
            $ffAsset = $ffRel.assets |
                Where-Object { $_.name -like "*win64*gpl*.zip" -and $_.name -notlike "*shared*" } |
                Select-Object -First 1
        }
        if (-not $ffAsset) { throw "No matching asset found" }

        $ffZip = Join-Path $env:TEMP "ffmpeg-radii5.zip"
        $ffTmp = Join-Path $env:TEMP "ffmpeg-radii5-extract"

        Write-Host "  `e[2msize   `e[0m  $([math]::Round($ffAsset.size / 1MB, 1)) MB (zip)"
        Write-Host "  `e[2mdest   `e[0m  $installDir"
        Write-Host ""

        Install-Binary -Url $ffAsset.browser_download_url -Dest $ffZip

        Write-Host "  `e[2mextracting...`e[0m"
        if (Test-Path $ffTmp) { Remove-Item $ffTmp -Recurse -Force }
        Expand-Archive -Path $ffZip -DestinationPath $ffTmp -Force
        Remove-Item $ffZip -Force

        $ffExe = Get-ChildItem $ffTmp -Recurse -Filter "ffmpeg.exe" | Select-Object -First 1
        if (-not $ffExe) { throw "ffmpeg.exe not found in archive" }

        foreach ($exe in @("ffmpeg.exe", "ffprobe.exe", "ffplay.exe")) {
            $src = Join-Path $ffExe.DirectoryName $exe
            if (Test-Path $src) { Copy-Item $src -Destination $installDir -Force }
        }
        Remove-Item $ffTmp -Recurse -Force

        Write-Host "  `e[32m✓`e[0m ffmpeg installed"
        Write-Host ""
    } catch {
        Write-Host "  `e[33m⚠`e[0m ffmpeg install failed: $_" -ForegroundColor Yellow
        Write-Host "  Install manually: https://ffmpeg.org/download.html"
        Write-Host ""
    }
}

# ── 3. radii5 ─────────────────────────────────────────────────────────────────
$r5Dest = Join-Path $installDir "radii5.exe"
if (Test-Path $r5Dest) {
    $rel = Get-GHRelease $repo
    $latest = $rel.tag_name
    $assetName = "radii5-$suffix.exe"
    $installedHash = (Get-FileHash $r5Dest -Algorithm SHA256).Hash.ToLower()
    $checksumsUrl = "https://github.com/$repo/releases/download/$latest/checksums.txt"
    $checksums = (Invoke-RestMethod $checksumsUrl) -split "`n"
    $expectedHash = ($checksums | Where-Object { $_ -like "*$assetName*" } | ForEach-Object { $_.Split(' ')[0] }).ToLower()
    if ($installedHash -eq $expectedHash) {
        Write-Host "  `e[2m✓ radii5 up to date`e[0m"
    } else {
        Write-Host "  `e[36m→`e[0m  updating radii5..."
        $patched = $false

        # Try bsdiff patch for incremental update
        $installedVersion = & "$r5Dest" --version 2>$null
        if ($installedVersion -match 'v\d+\.\d+\.\d+[^\s]*') {
            $installedVersion = $matches[0]
            $patchName = "patch_${suffix}_${installedVersion}_${latest}.bspatch"
            $patchAsset = $rel.assets | Where-Object { $_.name -eq $patchName } | Select-Object -First 1

            if ($patchAsset) {
                $tmpDir = Join-Path $env:TEMP "radii5-update-$([System.Guid]::NewGuid().ToString().Substring(0,8))"
                New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null
                $patchPath = Join-Path $tmpDir "update.bspatch"
                $newBin = Join-Path $tmpDir "radii5-new.exe"

                Write-Host "  `e[2mpatch: $patchName`e[0m"
                Install-Binary -Url $patchAsset.browser_download_url -Dest $patchPath

                $proc = Start-Process -FilePath $r5Dest -ArgumentList "bspatch `"$r5Dest`" `"$newBin`" `"$patchPath`"" -NoNewWindow -Wait -PassThru
                if ($proc.ExitCode -eq 0 -and (Test-Path $newBin)) {
                    $newHash = (Get-FileHash $newBin -Algorithm SHA256).Hash.ToLower()
                    if ($newHash -eq $expectedHash) {
                        Move-Item -Path $newBin -Destination $r5Dest -Force
                        $patched = $true
                        Write-Host "  `e[32m✓`e[0m radii5 updated (patch)"
                    }
                }
                Remove-Item $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
            }
        }

        if (-not $patched) {
            $asset = $rel.assets | Where-Object { $_.name -eq $assetName } | Select-Object -First 1
            if (-not $asset) { throw "no asset: $assetName" }
            Write-Host ""
            Install-Binary -Url $asset.browser_download_url -Dest $r5Dest
            Write-Host "  `e[32m✓`e[0m radii5 $latest installed (full)"
        }
    }
} elseif (Get-Command "go" -ErrorAction SilentlyContinue) {
    Write-Host "  `e[36m→`e[0m  building radii5 from source..."
    $tmpDir = Join-Path $env:TEMP "radii5-build"
    if (Test-Path $tmpDir) { Remove-Item $tmpDir -Recurse -Force }
    New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null
    $zipUrl = "https://github.com/$repo/archive/main.zip"
    $zipPath = Join-Path $env:TEMP "radii5-source.zip"
    [ChunkDownloader]::Download($zipUrl, $zipPath, $threads)
    Expand-Archive -Path $zipPath -DestinationPath $tmpDir -Force
    Remove-Item $zipPath -Force
    $srcDir = Join-Path $tmpDir "radii5-main"
    Push-Location $srcDir
    go build -o $r5Dest ./cmd/radii5/ 2>&1 | Out-Null
    Pop-Location
    Remove-Item $tmpDir -Recurse -Force
    Write-Host "  `e[32m✓`e[0m radii5 built"
} else {
    Write-Host "  `e[33m⚠`e[0m Go not found — install Go or download radii5 binary manually"
}
Write-Host ""

# ── 4. PATH ───────────────────────────────────────────────────────────────────
$curPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")
if ($curPath -notlike "*$installDir*") {
    [System.Environment]::SetEnvironmentVariable("PATH", "$curPath;$installDir", "User")
    $env:PATH = "$env:PATH;$installDir"
    Write-Host "  `e[32m✓`e[0m Added $installDir to PATH"
} else {
    Write-Host "  `e[2m✓ $installDir already in PATH`e[0m"
}

Write-Host ""
Write-Host "  `e[1m`e[32mAll done!`e[0m  Try: radii5 --version"
Write-Host ""
