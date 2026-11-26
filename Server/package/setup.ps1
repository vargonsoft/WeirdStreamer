# Sets up a Weird Streamer release package for Windows (the folder this script is in): checks for
# ffmpeg (offers winget), writes config.yml (HTTP or HTTPS, with a self-signed certificate if
# asked), can create the admin user, can install the server as a Windows service that starts at
# boot, and prints the address where the web UI is, where the admin adds the libraries in Settings.
# The counterpart of setup.sh.
#
# Run it with setup.cmd (double-click it, or run it from a PowerShell window), or:
#   powershell -ExecutionPolicy Bypass -File setup.ps1
#
#   setup.ps1                   all of it, asking as it goes
#   setup.ps1 service           install the Windows service only (needs an administrator PowerShell)
#   setup.ps1 remove-service    stop and remove the service
#   setup.ps1 guide [-Check]    set up the Guide's voice-overs (Piper and two voices) and music
#                               (-Check: only report)
#   setup.ps1 help              this text
#
# Without the service, run the server with .\bin\server.exe from this folder.

param([string]$Command = '', [switch]$Check)

Set-Location $PSScriptRoot

function Ask($question, $default) {
    $answer = Read-Host "$question [$default]"
    if ([string]::IsNullOrWhiteSpace($answer)) { return $default }
    return $answer.Trim()
}

function AskYes($question, $defaultYes) {
    if ($defaultYes) { $hint = '[Y/n]' } else { $hint = '[y/N]' }
    $answer = Read-Host "$question $hint"
    if ([string]::IsNullOrWhiteSpace($answer)) { return [bool]$defaultYes }
    return ($answer.Trim() -match '^[Yy]')
}

function Fail($message) {
    Write-Host $message -ForegroundColor Red
    exit 1
}

# Invoke-Cli runs bin\cli.exe with these arguments in this window and returns its exit status. Not with
# & (which would pipe its output inside a function): the cli asks for passwords on the console,
# and its "Password: " prompt (no newline) wouldn't show through a pipe.
function Invoke-Cli {
    $quoted = ($args | ForEach-Object { '"' + ($_ -replace '"', '\"') + '"' }) -join ' '
    $p = Start-Process -FilePath (Join-Path $PSScriptRoot 'bin\cli.exe') -ArgumentList $quoted `
        -WorkingDirectory $PSScriptRoot -NoNewWindow -Wait -PassThru
    return $p.ExitCode
}

function Test-Elevated {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Step-FFmpeg {
    $onPath = [bool](Get-Command ffmpeg -ErrorAction SilentlyContinue)
    $here = Test-Path 'ffmpeg\bin\ffmpeg.exe'
    if ($onPath -or $here) { return }
    Write-Host '== ffmpeg'
    Write-Host 'The server needs ffmpeg (and ffprobe) to play most videos and make thumbnails. None was found.'
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        if (AskYes 'Install it now with winget (Gyan.FFmpeg)?' $true) {
            winget install --id Gyan.FFmpeg -e --accept-source-agreements | Out-Host
            Write-Host 'A new PowerShell window (and the service, started after this) will find it on the PATH.'
        } else {
            Write-Host 'Install ffmpeg (https://www.gyan.dev/ffmpeg/builds/, the "full" build) and put its bin folder on the PATH, or into ffmpeg\bin here.'
        }
    } else {
        Write-Host 'Install ffmpeg (https://www.gyan.dev/ffmpeg/builds/, the "full" build) and put its bin folder on the PATH, or into ffmpeg\bin here.'
    }
}

function Step-Config {
    Write-Host '== Settings (written to config.yml; config.example.yml explains them all)'
    if (Test-Path 'config.yml') {
        Write-Host 'config.yml is there already; keeping it.'
        return
    }
    $name = Ask 'Server name, which the apps show' $env:COMPUTERNAME
    $port = Ask 'Port to listen on' '9000'
    $options = @('-name', $name, '-port', $port)
    if (AskYes 'Use HTTPS (with HTTP/2)?' $false) {
        $dir = Ask 'Folder for the certificate and key' 'var\config'
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
        if ((Test-Path "$dir\cert.pem") -and (Test-Path "$dir\key.pem")) {
            Write-Host "Using $dir\cert.pem and $dir\key.pem."
        } elseif (AskYes 'Generate a self-signed certificate and key there?' $true) {
            if ((Invoke-Cli config self-signed $dir $env:COMPUTERNAME) -ne 0) { Fail 'Making the certificate failed (see above).' }
        } else {
            Write-Host "Put your certificate and key in $dir as cert.pem and key.pem before starting."
        }
        $options += @('-cert', "$dir\cert.pem", '-key', "$dir\key.pem")
    }
    if ((Invoke-Cli config init config.example.yml @options) -ne 0) { Fail 'Writing config.yml failed (see above).' }
    Write-Host 'config.yml holds the sign-in secret: only this account (and administrators) can read it.'
}

function Step-Admin {
    Write-Host '== Admin user'
    if (-not (AskYes 'Create the admin user now?' $true)) { return }
    $admin = Ask 'Admin username' 'admin'
    if ((Invoke-Cli user create $admin --admin) -ne 0) {
        Write-Host "Couldn't create $admin (see above). Add it later with: .\bin\cli.exe user create $admin --admin" -ForegroundColor Yellow
    }
}

# Returns $true when the user wants the service (whether or not it could be installed here).
function Step-Service {
    Write-Host '== Service'
    if (-not (AskYes 'Run the server as a Windows service (it starts at boot)?' $true)) { return $false }
    if (Test-Elevated) {
        if ((Invoke-Cli service install) -ne 0) { Write-Host 'Installing the service failed (see above).' -ForegroundColor Yellow }
    } else {
        Write-Host 'Installing a service needs an administrator PowerShell: open one (Run as administrator), then run:'
        Write-Host "  cd `"$PSScriptRoot`""
        Write-Host '  .\bin\cli.exe service install'
    }
    return $true
}

function Step-Address {
    $port = '9000'
    $https = $false
    if (Test-Path 'config.yml') {
        foreach ($line in (Get-Content 'config.yml')) {
            if ($line -match '^  port: *(\d+)') { $port = $matches[1]; break }
        }
        foreach ($line in (Get-Content 'config.yml')) {
            if ($line -match '^  certFile: *"?[^"\s]') { $https = $true; break }
        }
    }
    $scheme = 'http'
    if ($https) { $scheme = 'https' }
    Write-Host ''
    Write-Host "Open $scheme`://$($env:COMPUTERNAME):$port/ in a browser, and sign in as the admin. Add your libraries in Settings."
    if ($https) { Write-Host 'With a self-signed certificate, the browser asks you to accept it first.' }
}

# The Guide's extras (docs/GUIDE.md), listed with their checksums in guide-extras.tsv, as setup.sh's
# guide step: voice-overs on the credits (Piper, a pinned piper-tts in var\piper, and two announcers'
# voices in var\piper-voices: about 180 MB with Piper's own) and the music under the Weather channel and
# the Guide channel's cards (20 tracks, about 140 MB, in var\guide-music). Each is asked about;
# config.yml is set through bin\cli.exe. $Mode: ask (a running service restarts after), flow (from
# setup: the server isn't running yet) or check (only report).
$PiperVersion = '1.8.0'

function Get-GuideExtras($kind) {
    Get-Content (Join-Path $PSScriptRoot 'guide-extras.tsv') | Where-Object { $_ -notmatch '^#' -and $_.Trim() } |
        ForEach-Object { $f = $_ -split "`t"; if ($f[0] -eq $kind) { [pscustomobject]@{ Name = $f[1]; Url = $f[2]; Sha = $f[3] } } }
}

function Test-GuideExtras($kind, $folder) {
    foreach ($e in Get-GuideExtras $kind) { if (-not (Test-Path (Join-Path $folder $e.Name))) { return $false } }
    return $true
}

# Downloads $kind's files into $folder, each checked against its SHA-256 (one there already is kept).
function Get-Checked($kind, $folder) {
    New-Item -ItemType Directory -Force -Path $folder | Out-Null
    $ok = $true
    $ProgressPreference = 'SilentlyContinue' # Invoke-WebRequest is many times slower drawing its bar
    foreach ($e in Get-GuideExtras $kind) {
        $file = Join-Path $folder $e.Name
        if ((Test-Path $file) -and (Get-FileHash $file -Algorithm SHA256).Hash -eq $e.Sha.ToUpper()) { continue }
        Write-Host "  $($e.Name)"
        $part = "$file.part"
        try {
            Invoke-WebRequest -Uri $e.Url -OutFile $part -UseBasicParsing
            if ((Get-FileHash $part -Algorithm SHA256).Hash -ne $e.Sha.ToUpper()) { throw 'not the file expected' }
            Move-Item -Force $part $file
        } catch {
            Remove-Item -Force -ErrorAction SilentlyContinue $part
            Write-Host "  $($e.Name) couldn't be downloaded, or wasn't the file expected." -ForegroundColor Yellow
            $ok = $false
        }
    }
    return $ok
}

# The python.exe of a Python 3.9 or later with venv (through the py launcher, or python; not the
# Microsoft Store's placeholder, which only opens the Store), or $null.
function Find-Python {
    # what an install just now (winget) added to the PATH, which this window's PATH predates
    $env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' + [Environment]::GetEnvironmentVariable('Path', 'User')
    $check = 'import sys, venv, ensurepip; print(sys.executable) if sys.version_info >= (3, 9) else sys.exit(1)'
    $candidates = @('py', 'python')
    # and where python.org's installers put it (for this user, or for everyone), newest first
    foreach ($root in @("$env:LOCALAPPDATA\Programs\Python", "$env:ProgramFiles")) {
        Get-ChildItem -Path $root -Directory -Filter 'Python3*' -ErrorAction SilentlyContinue | Sort-Object Name -Descending |
            ForEach-Object { $candidates += (Join-Path $_.FullName 'python.exe') }
    }
    foreach ($exe in $candidates) {
        if (-not (Get-Command $exe -ErrorAction SilentlyContinue)) { continue }
        if ($exe -eq 'py') { $out = & py -3 -c $check 2>$null } else { $out = & $exe -c $check 2>$null }
        $path = ([string]$out).Trim()
        if ($LASTEXITCODE -eq 0 -and $path -and (Test-Path $path)) { return $path }
    }
    return $null
}

# Whether Piper says a line with Ryan's voice (writes nothing that stays); shows why when it can't.
function Test-Piper($piperExe, $voices, [switch]$Quiet) {
    if (-not (Test-Path $piperExe)) { return $false }
    $test = Join-Path $PSScriptRoot 'var\.piper-test.wav'
    $said = 'Stay tuned.' | & $piperExe -m (Join-Path $voices 'en_US-ryan-high.onnx') -f $test 2>&1
    $ok = Test-Path $test
    Remove-Item -Force -ErrorAction SilentlyContinue $test
    if (-not $ok -and -not $Quiet) { $said | Select-Object -Last 3 | ForEach-Object { Write-Host "  $_" } } # why Piper couldn't
    return $ok
}

function Step-Guide($Mode) {
    $piper = Join-Path $PSScriptRoot 'var\piper'
    $voices = Join-Path $PSScriptRoot 'var\piper-voices'
    $music = Join-Path $PSScriptRoot 'var\guide-music'
    $piperExe = Join-Path $piper 'Scripts\piper.exe'
    $haveVoices = Test-GuideExtras voice $voices
    $havePiper = $haveVoices -and (Test-Piper $piperExe $voices -Quiet) # installed, and it runs
    $haveMusic = Test-GuideExtras music $music
    $config = ''
    if (Test-Path 'config.yml') { $config = Get-Content 'config.yml' -Raw }
    if ($Mode -eq 'check') {
        if ($havePiper -and $haveVoices) { Write-Host "Voice-overs: Piper and its voices are installed ($piper)." }
        else { Write-Host 'Voice-overs: not set up (setup.cmd guide installs Piper and two voices, about 180 MB).' }
        if ($haveMusic) { Write-Host "Music: the 20 tracks are in $music." }
        else { Write-Host 'Music: not downloaded (setup.cmd guide downloads 20 tracks, about 140 MB).' }
        if ($config -notmatch '(?m)^  piperCommand:') { Write-Host 'config.yml has no guide.piperCommand: the server makes no voice-overs.' }
        return
    }
    if (-not $config) { Write-Host "There's no config.yml yet: run setup.cmd first." -ForegroundColor Yellow; return }
    $changed = $false

    # Voice-overs: Piper in its own Python environment, and the announcers' voices
    if ($havePiper -and $haveVoices) {
        Write-Host 'Voice-overs: Piper and its voices are installed.'
        if ($config -notmatch '(?m)^  piperCommand:') { if ((Invoke-Cli config set-piper $piperExe $voices) -eq 0) { $changed = $true } }
    } elseif (AskYes 'Set up the Guide''s voice-overs ("Stay tuned for..." over the credits): Piper and two voices, about 180 MB?' $true) {
        $python = Find-Python
        if (-not $python -and (Get-Command winget -ErrorAction SilentlyContinue) -and
            (AskYes 'Piper needs Python 3: install Python 3.12 with winget?' $true)) {
            winget install --id Python.Python.3.12 -e --accept-source-agreements --accept-package-agreements | Out-Host
            $python = Find-Python
        }
        if (-not $python) {
            Write-Host 'Piper needs Python 3.9 or later (python.org, or: winget install Python.Python.3.12), then setup.cmd guide.' -ForegroundColor Yellow
        } else {
            # onnxruntime, which Piper runs on, needs Microsoft's Visual C++ runtime
            if (-not (Test-Path "$env:SystemRoot\System32\vcruntime140_1.dll")) {
                if ((Get-Command winget -ErrorAction SilentlyContinue) -and
                    (AskYes 'Piper needs the Microsoft Visual C++ runtime: install it with winget?' $true)) {
                    winget install --id Microsoft.VCRedist.2015+.x64 -e --accept-source-agreements --accept-package-agreements | Out-Host
                } else {
                    Write-Host 'Install the Microsoft Visual C++ Redistributable (x64), from learn.microsoft.com or winget.' -ForegroundColor Yellow
                }
            }
            Write-Host "Installing Piper into $piper"
            $pip = Join-Path $piper 'Scripts\pip.exe'
            # each step checked by what it made (a command that isn't there leaves $LASTEXITCODE as it was)
            if (-not (Test-Path $pip)) { & $python -m venv $piper }
            # (all of it again: a Piper that's there but doesn't run is mended)
            if (Test-Path $pip) { & $pip install -q --disable-pip-version-check --force-reinstall "piper-tts==$PiperVersion" }
            $ok = Test-Path $piperExe
            if ($ok) { Write-Host 'Downloading the voices'; $ok = Get-Checked voice $voices }
            if ($ok) { $ok = Test-Piper $piperExe $voices }
            if ($ok -and (Invoke-Cli config set-piper $piperExe $voices) -eq 0) {
                $changed = $true
                Write-Host 'Voice-overs are set up: the announcers and their fixes are in guide-voices.yml beside config.yml.'
            } else {
                Write-Host 'Setting up voice-overs failed (see above); setup.cmd guide tries again.' -ForegroundColor Yellow
            }
        }
    }

    # Music
    if ($haveMusic) {
        Write-Host 'Music: the tracks are downloaded.'
        if ($config -notmatch '(?m)^  musicPath:') { if ((Invoke-Cli config set-guide-music $music) -eq 0) { $changed = $true } }
    } elseif (AskYes 'Download the Guide''s music (Kevin MacLeod''s smooth jazz, CC BY: 20 tracks, about 140 MB)?' $true) {
        Write-Host 'Downloading the music'
        if (-not (Get-Checked music $music)) {
            Write-Host "Some tracks couldn't be downloaded; setup.cmd guide tries again (the rest play meanwhile)." -ForegroundColor Yellow
        }
        $utf8 = New-Object Text.UTF8Encoding $false # no byte-order mark: the credit shows on screen as written
        foreach ($e in Get-GuideExtras music) {
            $title = [IO.Path]::GetFileNameWithoutExtension($e.Name)
            [IO.File]::WriteAllText((Join-Path $music "$title.txt"),
                "`"$title`" by Kevin MacLeod (incompetech.com), licensed under Creative Commons: By Attribution 4.0`n", $utf8)
        }
        if ((Invoke-Cli config set-guide-music $music) -eq 0) { $changed = $true }
    }

    # The server reads these settings at start
    if ($changed -and $Mode -eq 'ask') {
        # only a service of this folder's server
        $svc = Get-CimInstance Win32_Service -Filter "Name='WeirdStreamer'" -ErrorAction SilentlyContinue
        $ours = $svc -and $svc.PathName -and $svc.PathName.ToLower().Contains((Join-Path $PSScriptRoot 'bin\server.exe').ToLower())
        if ($ours -and $svc.State -eq 'Running') {
            $status = Invoke-Cli server restart
            if ($status -eq 2 -and (AskYes 'Restarting the server ends what''s being watched. Restart it now?' $false)) {
                $status = Invoke-Cli server restart --force
            }
            if ($status -ne 0) { Write-Host 'Restart the server when no one is watching (.\bin\cli.exe server restart) for these to take effect.' }
        } else {
            Write-Host 'Restart the server for these to take effect.'
        }
    }
}

function Show-Help {
    Write-Host 'Weird Streamer setup (Windows). Run it from this folder: setup.cmd, or'
    Write-Host '  powershell -ExecutionPolicy Bypass -File setup.ps1'
    Write-Host ''
    Write-Host 'With no argument it checks for ffmpeg, writes config.yml (asks for the name, port and'
    Write-Host 'HTTPS, making a self-signed certificate if asked), can create the admin user, can install'
    Write-Host 'the server as a Windows service that starts at boot, and prints the address to open.'
    Write-Host ''
    Write-Host 'Subcommands:'
    Write-Host '  service           install the Windows service only (an administrator PowerShell)'
    Write-Host '  remove-service    stop and remove the service'
    Write-Host '  guide [-Check]    set up the Guide''s voice-overs (Piper and two voices) and music'
    Write-Host '  help              this text'
}

switch ($Command) {
    '' {
        Step-FFmpeg
        Step-Config
        Step-Admin
        Write-Host '== The Guide''s voice-overs and music (optional)'
        Step-Guide flow
        $service = Step-Service
        $startNow = $false
        if (-not $service) {
            $startNow = AskYes 'Start the server now, in this window?' $true
        }
        Step-Address
        if ($startNow) {
            Write-Host ''
            & .\bin\server.exe
        } elseif (-not $service) {
            Write-Host 'Start it with .\bin\server.exe (from this folder). setup.cmd service sets it up to start at boot.'
        }
    }
    'service' {
        [void](Step-Service)
    }
    'guide' {
        if ($Check) { Step-Guide check } else { Step-Guide ask }
    }
    'remove-service' {
        [void](Invoke-Cli service remove)
    }
    'help' {
        Show-Help
    }
    default {
        Write-Host "Unknown subcommand: $Command"
        Show-Help
        exit 2
    }
}
