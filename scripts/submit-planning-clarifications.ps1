param(
    [Parameter(Mandatory = $true)]
    [string]$SessionID,
    [string]$Path = (Join-Path $PSScriptRoot '..\planning\order-export.clarifications.example.json'),
    [string]$BaseUri = 'http://localhost:8080'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$resolvedPath = (Resolve-Path $Path).Path
$bodyJson = [System.IO.File]::ReadAllText($resolvedPath, [System.Text.Encoding]::UTF8)
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
$headers = New-EKBDAHeaders -UserID 'developer-1' -Roles 'developer' -Json

Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/planning/sessions/$SessionID/clarifications" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 20
