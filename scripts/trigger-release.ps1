param(
    [Parameter(Mandatory = $true)][string]$ReleaseID,
    [Parameter(Mandatory = $true)][int]$Revision,
    [Parameter(Mandatory = $true)][string]$Confirmation,
    [string]$BaseUri = 'http://localhost:8080',
    [string]$UserID = 'release-engineer-1'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$bodyBytes = [Text.Encoding]::UTF8.GetBytes((@{revision=$Revision;confirmation=$Confirmation}|ConvertTo-Json))
$headers = New-EKBDAHeaders -UserID $UserID -Roles 'release_engineer' -Json
Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/releases/$ReleaseID/trigger" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 30
