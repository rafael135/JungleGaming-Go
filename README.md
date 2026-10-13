# Jungle Gaming — Backend Distributed Wagering Service

Serviço em Go para processamento distribuído de apostas e liquidações financeiras de provedores de jogos, com garantias de precisão monetária, concorrência, idempotência persistente, ledger append-only e integração via AWS SQS FIFO e Keycloak OIDC.

---

## 1. Pré-requisitos

Para executar o ambiente e os testes localmente, certifique-se de possuir:

* **Docker & Docker Compose** (Docker v24+ / Compose v2.20+)
* **Go** (versão 1.22 ou superior, conforme `go.mod`)
* **Make** (opcional, para atalhos no Linux/macOS/WSL) ou **PowerShell** (no Windows)
* **cURL** (para testes manuais de chamadas HTTP)

---

## 2. Variáveis de Ambiente

O projeto utiliza um arquivo `.env` para carregar as configurações locais da aplicação e do Docker Compose. Um modelo com valores padrão de desenvolvimento está disponível em [.env.example](./.env.example):

```bash
# Copie o arquivo de exemplo para criar o seu .env local
cp .env.example .env
```

### Principais Variáveis

| Variável | Valor Padrão Local | Descrição |
| :--- | :--- | :--- |
| `PORT` | `8081` | Porta HTTP da aplicação Go |
| `DATABASE_URL` | `postgres://jungle:jungle_secret@localhost:5432/jungle_db?sslmode=disable` | String de conexão com o PostgreSQL 16 |
| `AWS_ENDPOINT` | `http://localhost:4566` | Endpoint do LocalStack (SQS e CloudFormation) |
| `AWS_REGION` | `us-east-1` | Região AWS emulada |
| `SQS_QUEUE_URL` | `http://localhost:4566/000000000000/wager-transactions.fifo` | Fila SQS FIFO de entrada de operações |
| `SQS_DLQ_URL` | `http://localhost:4566/000000000000/wager-transactions-dlq.fifo` | Fila DLQ da entrada de operações |
| `SQS_EVENTS_QUEUE_URL`| `http://localhost:4566/000000000000/wager-events.fifo` | Fila de destino de eventos da Outbox |
| `KEYCLOAK_URL` | `http://localhost:8080` | URL do IdP Keycloak |
| `KEYCLOAK_REALM` | `jungle` | Realm configurado no Keycloak |

---

## 3. Inicialização do Ambiente Local

O ambiente sobe de forma 100% autônoma (zero setup manual) via Docker Compose:

```bash
# Iniciar PostgreSQL, LocalStack e Keycloak em segundo plano
docker compose up -d

# Ou usando o Makefile
make up
```

### Serviços Provisionados:
1. **PostgreSQL 16+** (`localhost:5432`): Banco relacional com `healthcheck` ativo via `pg_isready`.
2. **LocalStack 3.8** (`localhost:4566`): Emulador AWS com SQS e CloudFormation.
3. **Keycloak 26** (`localhost:8080`): Provedor de Identidade OIDC com o realm `jungle` importado no boot.

---

## 4. Provisionamento Automático das Filas SQS FIFO

As filas SQS FIFO e suas respectivas DLQs são provisionadas de forma declarativa via **AWS CloudFormation** através do template [infra/sqs-template.yaml](./infra/sqs-template.yaml):

* **Fila de Operações (Inbound):** `wager-transactions.fifo`
  * Redrive Policy apontando para `wager-transactions-dlq.fifo` com `maxReceiveCount: 3`.
  * `ContentBasedDeduplication: false` (exige `MessageDeduplicationId` explícito).
* **Fila de Eventos (Outbox Outbound):** `wager-events.fifo`
  * Redrive Policy apontando para `wager-events-dlq.fifo` com `maxReceiveCount: 3`.

O deploy do CloudFormation é executado automaticamente pelo hook de inicialização do LocalStack ao subir o container.

---

## 5. Autenticação e Provedores de Teste (Keycloak OIDC)

A autenticação é estritamente baseada em **OAuth 2.0 / OIDC** via fluxo `client_credentials` (comunicação M2M).

### Clientes Pré-configurados:

| Client ID | Client Secret | Role | Identidade / Claims | Propósito |
| :--- | :--- | :--- | :--- | :--- |
| `provider-a` | `provider-a-secret` | `provider` | `providerId: provider-a` | Provedor de jogos A |
| `provider-b` | `provider-b-secret` | `provider` | `providerId: provider-b` | Provedor de jogos B (teste de isolamento) |
| `internal-service` | `internal-service-secret`| `internal` | - | Operações internas (abertura de carteira e reconciliação) |

### Como Obter um Token JWT de Teste:

#### No PowerShell (Windows):
```powershell
powershell -ExecutionPolicy Bypass -File scripts\get-token.ps1 -Client provider-a
```

#### No Linux / macOS / Bash:
```bash
bash scripts/get-token.sh provider-a
```

O script obtém o access token e exibe o payload do JWT decodificado no terminal.

---

## 6. Verificação de Saúde da Infraestrutura

Para certificar-se de que o banco de dados, a stack CloudFormation do SQS e o Keycloak estão prontos e operacionais:

```powershell
# No Windows (PowerShell)
powershell -ExecutionPolicy Bypass -File scripts\verify-infra.ps1

# No Linux / macOS (Bash) ou via Makefile
make verify
```

---

## 7. Exemplos de Chamadas Autenticadas (cURL)

### 7.1. Obter Token do Provedor A
```bash
TOKEN=$(curl -s -X POST "http://localhost:8080/realms/jungle/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" \
  -d "client_id=provider-a" \
  -d "client_secret=provider-a-secret" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)
```

### 7.2. Obter Token do Serviço Interno
```bash
INTERNAL_TOKEN=$(curl -s -X POST "http://localhost:8080/realms/jungle/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" \
  -d "client_id=internal-service" \
  -d "client_secret=internal-service-secret" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)
```

### 7.3. Abertura de Carteira (Restrito ao Serviço Interno)
```bash
curl -X POST "http://localhost:8081/wallets" \
  -H "Authorization: Bearer $INTERNAL_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "playerId": "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
    "initialBalance": { "amount": "1000.00", "currency": "BRL" }
  }'
```

### 7.4. Envio de Operação de Aposta (Provider A)
```bash
curl -X POST "http://localhost:8081/wagering/transactions" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: provider-a:transaction-123" \
  -H "Content-Type: application/json" \
  -d '{
    "providerId": "provider-a",
    "externalTransactionId": "transaction-123",
    "playerId": "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
    "walletId": "0192f291-27dd-7d3f-8071-5f8685deef37",
    "roundId": "round-987",
    "gameId": "fortune-chimp",
    "kind": "BET",
    "money": { "amount": "25.00", "currency": "BRL" }
  }'
```

---

## 8. Comandos do Desafio

Os comandos padrões estipulados na avaliação:

```bash
# Execução dos testes automatizados da aplicação
go test ./...

# Execução com detecção de concorrência / data races
go test -race ./...

# Análise estática do código Go
go vet ./...

# Formatação do código
gofmt -s -w .

# Derrubar a infraestrutura e limpar volumes
make down
```

---

## 9. Documentação Adicional

* [ARCHITECTURE.md](./ARCHITECTURE.md): Decisões de arquitetura, escolhas de infraestrutura, modelagem de segurança e garantias do sistema.
* [InstrucoesTeste.md](./InstrucoesTeste.md): Especificação e requisitos completos do desafio técnico.

