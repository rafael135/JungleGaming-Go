param (
    [ValidateSet("provider-a", "provider-b", "internal-service")]
    [string]$Client = "provider-a",

    [string]$KeycloakUrl = "http://127.0.0.1:8080",
    [string]$Realm = "jungle"
)

$secrets = @{
    "provider-a"       = "provider-a-secret"
    "provider-b"       = "provider-b-secret"
    "internal-service" = "internal-service-secret"
}

$secret = $secrets[$Client]
$tokenUrl = "$KeycloakUrl/realms/$Realm/protocol/openid-connect/token"

Write-Host "==> Requesting token for client '$Client' from $tokenUrl..." -ForegroundColor Cyan

$body = @{
    grant_type    = "client_credentials"
    client_id     = $Client
    client_secret = $secret
}

try {
    $response = Invoke-RestMethod -Uri $tokenUrl -Method Post -Body $body -ContentType "application/x-www-form-urlencoded"
    $token = $response.access_token

    Write-Host "`n[OK] Access Token obtained successfully!" -ForegroundColor Green
    Write-Host "`nToken:" -ForegroundColor Yellow
    Write-Output $token

    # Decode and show payload for debugging
    $parts = $token.Split('.')
    if ($parts.Length -ge 2) {
        $payload = $parts[1]
        $mod4 = $payload.Length % 4
        if ($mod4 -gt 0) { $payload += "=" * (4 - $mod4) }
        $payload = $payload.Replace('-', '+').Replace('_', '/')
        $bytes = [Convert]::FromBase64String($payload)
        $json = [System.Text.Encoding]::UTF8.GetString($bytes)
        Write-Host "`nDecoded JWT Payload:" -ForegroundColor Magenta
        Write-Output ($json | ConvertFrom-Json | ConvertTo-Json -Depth 5)
    }
}
catch {
    Write-Error "Failed to obtain token: $_"
}

