param(
    [Parameter(Mandatory = $true)]
    [string]$InputPath,
    [string]$BaseUri = 'http://localhost:8080',
    [string]$UserID = 'release-engineer-1'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$resolved = (Resolve-Path -LiteralPath $InputPath).Path
$bodyBytes = [System.IO.File]::ReadAllBytes($resolved)
$headers = New-EKBDAHeaders -UserID $UserID -Roles 'release_engineer' -Json
Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/releases" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 30
