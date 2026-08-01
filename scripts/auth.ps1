function New-EKBDAHeaders {
    param(
        [Parameter(Mandatory = $true)]
        [string]$UserID,
        [string]$Roles,
        [switch]$Json
    )

    $headers = @{}
    $accessToken = [Environment]::GetEnvironmentVariable('EKBDA_ACCESS_TOKEN')
    if ([string]::IsNullOrWhiteSpace($accessToken)) {
        $headers['X-User-ID'] = $UserID
        if (-not [string]::IsNullOrWhiteSpace($Roles)) {
            $headers['X-User-Roles'] = $Roles
        }
    } else {
        $headers['Authorization'] = "Bearer $accessToken"
    }
    if ($Json) {
        $headers['Content-Type'] = 'application/json; charset=utf-8'
    }
    return $headers
}
