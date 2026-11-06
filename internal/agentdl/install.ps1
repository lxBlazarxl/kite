# Kite agent installer for Windows. The hub serves this script at /install.ps1.
# Run it from an Administrator PowerShell:
#
#   $env:KITE_HUB='<hub>'; $env:KITE_TOKEN='<token>'; irm <hub>/install.ps1 | iex
#
# Optional: $env:KITE_NAME='<name>'. Running it again upgrades the agent.
# Uninstall: $env:KITE_UNINSTALL='1'; irm <hub>/install.ps1 | iex
#
# It runs inside your PowerShell window (iex), so it never calls exit;
# problems are reported with throw.
& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue' # the progress bar makes downloads very slow in PowerShell 5

    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $isAdmin = (New-Object Security.Principal.WindowsPrincipal($identity)).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

    if ($env:KITE_UNINSTALL -eq '1') {
        if (-not $isAdmin) { throw 'kite install: run this from an Administrator PowerShell' }
        $installed = Join-Path $env:ProgramFiles 'Kite\kite-agent.exe'
        if (Test-Path $installed) { & $installed uninstall }
        Remove-Item -Recurse -Force (Join-Path $env:ProgramFiles 'Kite') -ErrorAction SilentlyContinue
        Write-Host 'Kite agent removed.'
        return
    }

    $hub = $env:KITE_HUB
    $token = $env:KITE_TOKEN
    if (-not $hub) { throw 'kite install: set $env:KITE_HUB to the hub address first' }
    if (-not $token) { throw 'kite install: set $env:KITE_TOKEN to the agent token first' }
    if (-not $isAdmin) { throw 'kite install: run this from an Administrator PowerShell' }
    $hub = $hub.TrimEnd('/')

    $tmp = Join-Path $env:TEMP ('kite-agent-' + [guid]::NewGuid().ToString('N') + '.exe')
    try {
        Invoke-WebRequest -UseBasicParsing -Uri "$hub/download/kite-agent-windows-amd64.exe" -OutFile $tmp
        $agentArgs = @('install', '-hub', $hub, '-token', $token)
        if ($env:KITE_NAME) { $agentArgs += @('-name', $env:KITE_NAME) }
        & $tmp @agentArgs
        if ($LASTEXITCODE -ne 0) { throw "kite install: kite-agent.exe install failed (exit code $LASTEXITCODE)" }
    }
    finally {
        Remove-Item -Force $tmp -ErrorAction SilentlyContinue
    }
}
