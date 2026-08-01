param(
    [Parameter(Mandatory = $true)]
    [string]$PackageID,
    [Parameter(Mandatory = $true)]
    [string]$PackageHash,
    [Parameter(Mandatory = $true)]
    [ValidateSet('prd', 'architecture', 'api', 'test', 'deployment', 'monitoring', 'risk')]
    [string]$ArtifactType,
    [Parameter(Mandatory = $true)]
    [ValidateSet('approve', 'request_changes')]
    [string]$Decision,
    [Parameter(Mandatory = $true)]
    [string]$Comment,
    [string]$BaseUri = 'http://localhost:8080',
    [string]$UserID = 'approver-1'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$bodyJson = @{
    artifact_type = $ArtifactType
    package_hash = $PackageHash
    decision = $Decision
    comment = $Comment
} | ConvertTo-Json
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
$headers = New-EKBDAHeaders -UserID $UserID -Roles 'project_approver' -Json

Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/project-packages/$PackageID/reviews" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 20
