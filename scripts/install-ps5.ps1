# radii5 installer for Windows (PS5 compatible)
# Usage: irm https://raw.githubusercontent.com/ohcass/radii5/main/scripts/install-ps5.ps1 | iex

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
Add-Type -AssemblyName System.Net.Http
if (-not ([System.Management.Automation.PSTypeName]'ChunkDownloader').Type) {
Add-Type -Language CSharp -ReferencedAssemblies @(
    'mscorlib',
    'System',
    'System.Net',
    'System.Net.Http',
    'System.Threading.Tasks',
    'System.Collections.Concurrent'
) @"
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

    const int    TrackLen    = 8;
    const int    TrailSteps  = 6;
    const int    HoldStart   = 30;
    const int    HoldEnd     = 9;
    const int    CycleLen    = TrackLen + HoldEnd + (TrackLen - 1) + HoldStart;
    const int    TickMs      = 45;
    const double MinAlpha    = 0.3;
    const double InactiveFac = 0.6;

    static int       _frame;
    static int       _displayPct;
    static DateTime  _displayTime;
    static bool      _cursorHidden;
    static readonly byte[]   _base  = new byte[] { 104, 163, 235 };
    static readonly byte[][] _trail;

    static ChunkDownloader() {
        _trail = new byte[TrailSteps][];
        for (int i = 0; i < TrailSteps; i++) {
            double alpha  = i == 0 ? 1.0 : (i == 1 ? 0.9 : Math.Pow(0.65, i - 1));
            double factor = i == 1 ? 1.15 : 1.0;
            _trail[i] = new byte[] {
                ClampByte(104 * factor * alpha),
                ClampByte(163 * factor * alpha),
                ClampByte(235 * factor * alpha)
            };
        }
    }

    static byte ClampByte(double v) {
        if (v > 255) v = 255;
        if (v < 0) v = 0;
        return (byte)v;
    }

    struct FrameState {
        public int  ActivePos, HoldProg, HoldTotal, MoveProg, MoveTotal;
        public bool IsHolding, Forward;

        public int ColorIdx(int ch) {
            int dirDist = Forward ? ActivePos - ch : ch - ActivePos;
            if (IsHolding) return dirDist + HoldProg;
            if (dirDist < 0 || dirDist >= TrailSteps) return -1;
            return dirDist;
        }

        public double DotAlpha() {
            if (IsHolding && HoldTotal > 0) {
                double prog = Math.Min(1.0, (double)HoldProg / HoldTotal);
                return InactiveFac * Math.Max(MinAlpha, 1.0 - prog * (1.0 - MinAlpha));
            }
            if (!IsHolding && MoveTotal > 0) {
                int    den  = Math.Max(1, MoveTotal - 1);
                double prog = Math.Min(1.0, (double)MoveProg / den);
                return InactiveFac * (MinAlpha + prog * (1.0 - MinAlpha));
            }
            return InactiveFac;
        }
    }

    static FrameState GetFrameState(int f) {
        if (f < TrackLen)
            return new FrameState { ActivePos = f, MoveProg = f, MoveTotal = TrackLen, Forward = true };
        if (f < TrackLen + HoldEnd)
            return new FrameState { ActivePos = TrackLen - 1, IsHolding = true, HoldProg = f - TrackLen, HoldTotal = HoldEnd, Forward = true };
        if (f < TrackLen + HoldEnd + (TrackLen - 1)) {
            int back = f - TrackLen - HoldEnd;
            return new FrameState { ActivePos = TrackLen - 2 - back, MoveProg = back, MoveTotal = TrackLen - 1, Forward = false };
        }
        return new FrameState { ActivePos = 0, IsHolding = true, HoldProg = f - TrackLen - HoldEnd - (TrackLen - 1), HoldTotal = HoldStart, Forward = false };
    }

    static void DrawBar(long cur, long tot) {
        if (!_cursorHidden) {
            _cursorHidden = true;
            _frame        = 0;
            _displayPct   = 0;
            _displayTime  = DateTime.Now;
            Console.Write("\u001b[?25l");
        }

        int ipct = tot > 0 ? (int)((double)cur / tot * 100) : 0;
        if (ipct > 100) ipct = 100;
        if (ipct > _displayPct) {
            double elapsed = (DateTime.Now - _displayTime).TotalSeconds;
            double rate    = elapsed > 0 ? (ipct - _displayPct) / elapsed : 0;
            int    step    = rate > 50 ? 10 : (rate > 10 ? 5 : 1);
            int    disp    = (ipct / step) * step;
            if (disp > _displayPct || ipct >= 100) {
                _displayPct  = disp;
                _displayTime = DateTime.Now;
            }
        }

        FrameState state = GetFrameState(_frame);
        var sb = new System.Text.StringBuilder(TrackLen * 24);
        for (int ch = 0; ch < TrackLen; ch++) {
            int idx = state.ColorIdx(ch);
            if (idx >= 0 && idx < TrailSteps) {
                byte[] c = _trail[idx];
                sb.AppendFormat("\u001b[38;2;{0};{1};{2}m\u25A0", c[0], c[1], c[2]);
            } else {
                double a = state.DotAlpha();
                sb.AppendFormat("\u001b[38;2;{0};{1};{2}m\u2B1D",
                    ClampByte(_base[0] * a), ClampByte(_base[1] * a), ClampByte(_base[2] * a));
            }
        }
        sb.Append("\u001b[0m \u001b[1m").Append(_displayPct).Append("%\u001b[0m");
        Console.Write("\u001b[2K\r  " + sb.ToString());
        _frame = (_frame + 1) % CycleLen;
    }

    static void DrawBarDone() {
        _cursorHidden = false;
        Console.Write("\u001b[2K\r\u001b[?25h\n");
    }

    public static void Download(string url, string dest, int numThreads) {
        System.Net.ServicePointManager.SecurityProtocol = (System.Net.SecurityProtocolType)3072;
        System.Net.ServicePointManager.DefaultConnectionLimit = 256;
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
                DrawBarDone();
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
                                Thread.Sleep(500 * (attempt + 1));
                            }
                        }
                    }
                });
            }

            while (!Task.WhenAll(tasks).Wait(TickMs)) {
                DrawBar(_downloaded, total);
            }
            DrawBar(total, total);
            DrawBarDone();

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
    Write-Host "  ${esc}[32m✓${esc}[0m${esc}[2m done  ${mbps} MB/s  (${elapsed}s,  $threads threads)${esc}[0m"
}

# ── 1. yt-dlp ─────────────────────────────────────────────────────────────────
if (Get-Command "yt-dlp.exe" -ErrorAction SilentlyContinue) {
    Write-Host "  ${esc}[2mOK yt-dlp already installed${esc}[0m"
    Write-Host ""
} else {
    Write-Host "  ${esc}[36m>${esc}[0m  yt-dlp"
    $ytRel   = Get-GHRelease "yt-dlp/yt-dlp"
    $ytAsset = $ytRel.assets | Where-Object { $_.name -eq "yt-dlp.exe" } | Select-Object -First 1
    if (-not $ytAsset) { Write-Host "  ${esc}[31mFAIL${esc}[0m yt-dlp.exe not found" -ForegroundColor Red; exit 1 }

    $ytDest = Join-Path $installDir "yt-dlp.exe"
    Write-Host "  ${esc}[2mversion${esc}[0m  $($ytRel.tag_name)"
    Write-Host "  ${esc}[2mdest   ${esc}[0m  $ytDest"
    Write-Host ""
    Install-Binary -Url $ytAsset.browser_download_url -Dest $ytDest
    Write-Host "  ${esc}[32mOK${esc}[0m yt-dlp $($ytRel.tag_name)"
    Write-Host ""
}

# ── 2. ffmpeg ─────────────────────────────────────────────────────────────────
$ffDest = Join-Path $installDir "ffmpeg.exe"
if (Test-Path $ffDest) {
    Write-Host "  ${esc}[2mOK ffmpeg already installed${esc}[0m"
    Write-Host ""
} else {
    Write-Host "  ${esc}[36m>${esc}[0m  ffmpeg"
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

        Write-Host "  ${esc}[2msize   ${esc}[0m  $([math]::Round($ffAsset.size / 1MB, 1)) MB (zip)"
        Write-Host "  ${esc}[2mdest   ${esc}[0m  $installDir"
        Write-Host ""

        Install-Binary -Url $ffAsset.browser_download_url -Dest $ffZip

        Write-Host "  ${esc}[2mextracting...${esc}[0m"
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

        Write-Host "  ${esc}[32mOK${esc}[0m ffmpeg installed"
        Write-Host ""
    } catch {
        Write-Host "  ${esc}[33mWARN${esc}[0m ffmpeg install failed: $_" -ForegroundColor Yellow
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
        Write-Host "  ${esc}[2mOK radii5 up to date${esc}[0m"
    } else {
        Write-Host "  ${esc}[36m>${esc}[0m  updating radii5..."
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

                Write-Host "  ${esc}[2mpatch: $patchName${esc}[0m"
                Install-Binary -Url $patchAsset.browser_download_url -Dest $patchPath

                $proc = Start-Process -FilePath $r5Dest -ArgumentList "bspatch `"$r5Dest`" `"$newBin`" `"$patchPath`"" -NoNewWindow -Wait -PassThru
                if ($proc.ExitCode -eq 0 -and (Test-Path $newBin)) {
                    $newHash = (Get-FileHash $newBin -Algorithm SHA256).Hash.ToLower()
                    if ($newHash -eq $expectedHash) {
                        Move-Item -Path $newBin -Destination $r5Dest -Force
                        $patched = $true
                        Write-Host "  ${esc}[32mOK${esc}[0m radii5 updated (patch)"
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
            Write-Host "  ${esc}[32mOK${esc}[0m radii5 $latest installed (full)"
        }
    }
} elseif (Get-Command "go" -ErrorAction SilentlyContinue) {
    Write-Host "  ${esc}[36m>${esc}[0m  building radii5 from source..."
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
    Write-Host "  ${esc}[32mOK${esc}[0m radii5 built"
} else {
    Write-Host "  ${esc}[33mWARN${esc}[0m Go not found — install Go or download radii5 binary manually"
}
Write-Host ""

# ── 4. PATH ───────────────────────────────────────────────────────────────────
$curPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")
if ($curPath -notlike "*$installDir*") {
    [System.Environment]::SetEnvironmentVariable("PATH", "$curPath;$installDir", "User")
    $env:PATH = "$env:PATH;$installDir"
    Write-Host "  ${esc}[32mOK${esc}[0m Added $installDir to PATH"
} else {
    Write-Host "  ${esc}[2mOK $installDir already in PATH${esc}[0m"
}

Write-Host ""
Write-Host "  ${esc}[1m${esc}[32mAll done!${esc}[0m  Try: radii5 --version"
Write-Host ""
