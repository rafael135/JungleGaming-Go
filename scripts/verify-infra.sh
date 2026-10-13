#!/usr/bin/env bash
set -euo pipefail

echo "=================================================="
echo "  Verifying Jungle Gaming Infrastructure Services  "
echo "=================================================="

# 1. PostgreSQL Check
echo -e "\n[1/3] Checking PostgreSQL 16..."
docker exec jungle-postgres pg_isready -U jungle -d jungle_db
echo "  [SUCCESS] PostgreSQL is healthy!"

# 2. LocalStack CloudFormation & SQS FIFO Check
echo -e "\n[2/3] Checking LocalStack CloudFormation Stack & SQS FIFO Queues..."
STACK_STATUS=$(docker exec jungle-localstack awslocal cloudformation describe-stacks --stack-name jungle-messaging-stack | grep -o '"StackStatus": "[^"]*' || true)
echo "  Stack Status: ${STACK_STATUS}"

QUEUES=$(docker exec jungle-localstack awslocal sqs list-queues)
echo "$QUEUES"
if echo "$QUEUES" | grep -q "wager-transactions.fifo" && echo "$QUEUES" | grep -q "wager-events.fifo"; then
  echo "  [SUCCESS] SQS FIFO queues (inbound & outbound outbox) successfully deployed via CloudFormation!"
else
  echo "  [FAILURE] Missing SQS FIFO queues!"
  exit 1
fi

# 3. Keycloak Check
echo -e "\n[3/3] Checking Keycloak Realm & Client Credentials..."
TOKEN_RES=$(curl -s -X POST "http://localhost:8080/realms/jungle/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" \
  -d "client_id=provider-a" \
  -d "client_secret=provider-a-secret")

if echo "$TOKEN_RES" | grep -q "access_token"; then
  echo "  [SUCCESS] Successfully authenticated client 'provider-a' with Keycloak!"
else
  echo "  [FAILURE] Could not authenticate with Keycloak:"
  echo "$TOKEN_RES"
  exit 1
fi

echo -e "\n=================================================="
echo "ALL INFRASTRUCTURE SERVICES ARE HEALTHY & CONFIGURED!"
echo "=================================================="

