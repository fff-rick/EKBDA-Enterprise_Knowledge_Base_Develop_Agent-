param(
    [Parameter(Mandatory = $true)]
    [string]$SessionID,
    [Parameter(Mandatory = $true)]
    [int]$Revision,
    [Parameter(Mandatory = $true)]
    [string]$PatchPath,
    [Parameter(Mandatory = $true)]
    [string]$Summary,
    [string[]]$CommandIDs = @('git-diff-check', 'go-test-all'),
    [string]$BaseUri = 'http://localhost:8080',
    [string]$UserID = 'developer-1'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$resolvedPatch = (Resolve-Path -LiteralPath $PatchPath).Path
$patch = [System.IO.File]::ReadAllText($resolvedPatch, [System.Text.Encoding]::UTF8)
if (-not $patch.EndsWith("`n")) {
    throw 'Patch must end with a newline.'
}
$bodyJson = @{
    revision = $Revision
    summary = $Summary
    patch = $patch
    command_ids = $CommandIDs
} | ConvertTo-Json -Depth 10
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
$headers = New-EKBDAHeaders -UserID $UserID -Roles 'developer' -Json

Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/development/sessions/$SessionID/proposals" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 20
