param(
    [Parameter(Mandatory = $true)]
    [string]$SessionID,
    [Parameter(Mandatory = $true)]
    [int]$Revision,
    [string]$BaseUri = 'http://localhost:8080'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$bodyJson = @{ revision = $Revision } | ConvertTo-Json -Compress
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
$headers = New-EKBDAHeaders -UserID 'developer-1' -Roles 'developer' -Json

Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/planning/sessions/$SessionID/role-reviews" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 30
