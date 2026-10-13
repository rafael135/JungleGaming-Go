#!/usr/bin/env bash
set -euo pipefail

CLIENT="${1:-provider-a}"
KEYCLOAK_URL="${KEYCLOAK_URL:-http://localhost:8080}"
REALM="${KEYCLOAK_REALM:-jungle}"

declare -A SECRETS
SECRETS["provider-a"]="provider-a-secret"
SECRETS["provider-b"]="provider-b-secret"
SECRETS["internal-service"]="internal-service-secret"

SECRET="${SECRETS[$CLIENT]:-}"
if [ -z "$SECRET" ]; then
  echo "Unknown client: $CLIENT. Choose: provider-a, provider-b, or internal-service"
  exit 1
fi

TOKEN_URL="${KEYCLOAK_URL}/realms/${REALM}/protocol/openid-connect/token"
echo "==> Requesting token for client '$CLIENT' from $TOKEN_URL..."

RESPONSE=$(curl -s -X POST "$TOKEN_URL" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" \
  -d "client_id=${CLIENT}" \
  -d "client_secret=${SECRET}")

if echo "$RESPONSE" | grep -q "access_token"; then
  TOKEN=$(echo "$RESPONSE" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)
  echo "[OK] Access token retrieved:"
  echo "$TOKEN"
else
  echo "[ERROR] Failed to obtain token:"
  echo "$RESPONSE"
  exit 1
fi

