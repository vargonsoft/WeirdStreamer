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
#   setup.ps1 help              this text
#
# Without the service, run the server with .\bin\server.exe from this folder.

param([string]$Command = '')

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
    Write-Host '  help              this text'
}

switch ($Command) {
    '' {
        Step-FFmpeg
        Step-Config
        Step-Admin
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
