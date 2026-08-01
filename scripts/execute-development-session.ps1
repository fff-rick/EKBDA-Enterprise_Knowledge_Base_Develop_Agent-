param(
    [Parameter(Mandatory = $true)]
    [string]$SessionID,
    [Parameter(Mandatory = $true)]
    [int]$Revision,
    [Parameter(Mandatory = $true)]
    [string]$PatchHash,
    [string]$BaseUri = 'http://localhost:8080',
    [string]$UserID = 'developer-1'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$bodyJson = @{
    revision = $Revision
    patch_hash = $PatchHash
    confirmation = 'execute_approved_patch'
} | ConvertTo-Json
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
$headers = New-EKBDAHeaders -UserID $UserID -Roles 'developer' -Json

Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/development/sessions/$SessionID/execute" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 20
