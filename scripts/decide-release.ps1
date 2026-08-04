param(
    [Parameter(Mandatory = $true)][string]$ReleaseID,
    [Parameter(Mandatory = $true)][int]$Revision,
    [ValidateSet('approve','reject')][string]$Decision = 'approve',
    [string]$Comment = 'Release evidence and risk accepted',
    [string]$BaseUri = 'http://localhost:8080',
    [string]$UserID = 'release-approver-1'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$bodyBytes = [Text.Encoding]::UTF8.GetBytes((@{revision=$Revision;decision=$Decision;comment=$Comment}|ConvertTo-Json))
$headers = New-EKBDAHeaders -UserID $UserID -Roles 'release_approver' -Json
Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/releases/$ReleaseID/decision" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 30
