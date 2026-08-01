param(
    [string]$Path = (Join-Path $PSScriptRoot '..\evaluations\answer_cases.example.json'),
    [string]$BaseUri = 'http://localhost:8080'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'auth.ps1')
$resolvedPath = (Resolve-Path $Path).Path
$bodyJson = [System.IO.File]::ReadAllText($resolvedPath, [System.Text.Encoding]::UTF8)
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
$headers = New-EKBDAHeaders -UserID 'evaluation-admin' -Roles 'knowledge_admin' -Json

Invoke-RestMethod -Method Post -Uri "$BaseUri/api/v1/evaluations/answers" -Headers $headers -Body $bodyBytes |
    ConvertTo-Json -Depth 8
