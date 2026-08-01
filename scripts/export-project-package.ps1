param(
    [Parameter(Mandatory = $true)]
    [string]$PackageID,
    [ValidateSet('markdown', 'docx')]
    [string]$Format = 'markdown',
    [string]$OutputPath = '',
    [string]$BaseUri = 'http://localhost:8080',
    [string]$UserID = 'developer-1'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$headers = New-EKBDAHeaders -UserID $UserID -Roles 'developer'
if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $extension = if ($Format -eq 'docx') { 'docx' } else { 'md' }
    $OutputPath = Join-Path (Get-Location) "$PackageID.$extension"
}

Invoke-WebRequest -UseBasicParsing -Uri "$BaseUri/api/v1/project-packages/$PackageID/export?format=$Format" -Headers $headers -OutFile $OutputPath
(Resolve-Path $OutputPath).Path
