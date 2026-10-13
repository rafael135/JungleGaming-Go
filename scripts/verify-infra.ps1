Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "  Verifying Jungle Gaming Infrastructure Services  " -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

$allPassed = $true

# 1. PostgreSQL Check
Write-Host "`n[1/3] Checking PostgreSQL 16..." -ForegroundColor Yellow
try {
    $pgCheck = docker exec jungle-postgres pg_isready -U jungle -d jungle_db 2>&1
    if ($LASTEXITCODE -eq 0) {
        Write-Host "  [SUCCESS] PostgreSQL is healthy and accepting connections: $pgCheck" -ForegroundColor Green
    } else {
        Write-Host "  [FAILURE] PostgreSQL is not ready: $pgCheck" -ForegroundColor Red
        $allPassed = $false
    }
} catch {
    Write-Host "  [FAILURE] Could not communicate with PostgreSQL container: $_" -ForegroundColor Red
    $allPassed = $false
}

# 2. LocalStack CloudFormation Stack & SQS FIFO Check
Write-Host "`n[2/3] Checking LocalStack CloudFormation Stack & SQS FIFO Queues..." -ForegroundColor Yellow
try {
    # Check CloudFormation Stack
    $stack = docker exec jungle-localstack awslocal cloudformation describe-stacks --stack-name jungle-messaging-stack 2>&1
    if ($stack -match "CREATE_COMPLETE|UPDATE_COMPLETE") {
        Write-Host "  [SUCCESS] CloudFormation stack 'jungle-messaging-stack' is deployed and ACTIVE!" -ForegroundColor Green
    } else {
        Write-Host "  [WARNING] CloudFormation stack status: $stack" -ForegroundColor Yellow
    }

    $queues = docker exec jungle-localstack awslocal sqs list-queues 2>&1
    if ($queues -match "wager-transactions\.fifo" -and $queues -match "wager-transactions-dlq\.fifo" -and $queues -match "wager-events\.fifo") {
        Write-Host "  [SUCCESS] SQS FIFO Queues provisioned (inbound wager-transactions + outbound wager-events)!" -ForegroundColor Green
        
        # Check queue attributes & redrive policy
        $attrs = docker exec jungle-localstack awslocal sqs get-queue-attributes --queue-url "http://localhost:4566/000000000000/wager-transactions.fifo" --attribute-names RedrivePolicy FifoQueue 2>&1
        Write-Host "  Inbound Queue Attributes: $attrs" -ForegroundColor Gray
    } else {
        Write-Host "  [FAILURE] SQS FIFO queues missing from LocalStack! Output: $queues" -ForegroundColor Red
        $allPassed = $false
    }
} catch {
    Write-Host "  [FAILURE] Could not verify LocalStack CloudFormation / SQS queues: $_" -ForegroundColor Red
    $allPassed = $false
}

# 3. Keycloak Realm & Client Credentials Check
Write-Host "`n[3/3] Checking Keycloak Realm & Token Authentication..." -ForegroundColor Yellow
try {
    $tokenUrl = "http://127.0.0.1:8080/realms/jungle/protocol/openid-connect/token"
    $body = @{
        grant_type    = "client_credentials"
        client_id     = "provider-a"
        client_secret = "provider-a-secret"
    }
    $res = Invoke-RestMethod -Uri $tokenUrl -Method Post -Body $body -ContentType "application/x-www-form-urlencoded" -TimeoutSec 10
    if ($res.access_token) {
        Write-Host "  [SUCCESS] Successfully authenticated client 'provider-a' against Keycloak realm 'jungle'!" -ForegroundColor Green
        Write-Host "  Access Token received (length: $($res.access_token.Length))" -ForegroundColor Gray
    } else {
        Write-Host "  [FAILURE] No access_token returned by Keycloak: $res" -ForegroundColor Red
        $allPassed = $false
    }
} catch {
    Write-Host "  [FAILURE] Keycloak authentication check failed: $_" -ForegroundColor Red
    $allPassed = $false
}

Write-Host "`n--------------------------------------------------" -ForegroundColor Cyan
if ($allPassed) {
    Write-Host "ALL INFRASTRUCTURE SERVICES ARE HEALTHY & CONFIGURED!" -ForegroundColor Green
} else {
    Write-Host "SOME CHECKS FAILED. Please inspect docker compose logs." -ForegroundColor Red
}
Write-Host "--------------------------------------------------" -ForegroundColor Cyan

