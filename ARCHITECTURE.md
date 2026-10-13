# Arquitetura e Decisões de Projeto — Jungle Gaming Backend

Este documento registra as decisões de arquitetura e infraestrutura implementadas **até o momento** no projeto, além das premissas estabelecidas para o desenvolvimento do core em Go.

---

## 1. O que foi Implementado até o Momento

Nesta etapa inicial de fundação da solução, foi construído todo o ambiente local automatizado, declarativo e reprodutível, eliminando qualquer dependência de configuração manual:

```
[Desenvolvedor / Avaliador]
       │
       ▼  docker compose up -d
+───────────────────────────────────────────────────────────────────────────+
│                            Docker Compose                                 │
│                                                                           │
│  ┌──────────────────┐  ┌───────────────────────────┐  ┌────────────────┐  │
│  │  PostgreSQL 16+  │  │      LocalStack 3.8       │  │  Keycloak 26   │  │
│  │   (postgres:16)  │  │   (CloudFormation SQS)    │  │ (Quarkus Dev)  │  │
│  │                  │  │                           │  │                │  │
│  │  - jungle_db     │  │  - wager-transactions.fifo│  │  - Realm jungle│  │
│  │  - Healthcheck   │  │  - wager-events.fifo      │  │  - provider-a  │  │
│  │    (pg_isready)  │  │  - DLQs + Redrive         │  │  - provider-b  │  │
│  │                  │  │                           │  │  - internal    │  │
│  └──────────────────┘  └───────────────────────────┘  └────────────────┘  │
+───────────────────────────────────────────────────────────────────────────+
```

---

## 2. Decisões de Infraestrutura e Mensageria

### 2.1. IaC Declarativo com AWS CloudFormation em YAML
* **Decisão:** O provisionamento das filas SQS FIFO no LocalStack foi estruturado em um template declarativo AWS CloudFormation ([infra/sqs-template.yaml](file:///d:/Testes%20t%C3%A9cnicos/JungleGaming%20Go/infra/sqs-template.yaml)).
* **Justificativa:** Substitui scripts procedurais em Bash por um manifesto de infraestrutura formal e versionado (padrão oficial AWS / SAM), com resolução automática de referências (`!GetAtt DLQ.Arn`).
* **Automação no Boot:** O LocalStack sobe com os serviços `sqs,cloudformation` e aplica a stack automaticamente via hook de inicialização, garantindo execução com zero comandos manuais.
* **Topologia Provisionada:**
  1. **Entrada (Consumidor de Operações - Seção 10):**
     * Fila principal: `wager-transactions.fifo` (`FifoQueue: true`, `ContentBasedDeduplication: false`).
     * Fila morta: `wager-transactions-dlq.fifo` associada via `RedrivePolicy` com `maxReceiveCount: 3`.
  2. **Saída (Transactional Outbox - Seção 11):**
     * Fila principal: `wager-events.fifo` para publicação de eventos de integração.
     * Fila morta: `wager-events-dlq.fifo`.

### 2.2. Banco de Dados PostgreSQL 16+
* **Decisão:** Uso da imagem oficial `postgres:16-alpine`.
* **Justificativa:** Atende ao requisito do teste de PostgreSQL 16+, com pegada de memória mínima e `healthcheck` ativo via `pg_isready` para sincronização de dependências.

---

## 3. Decisões de Autenticação e Autorização (OIDC / Keycloak)

### 3.1. Justificativa do IdP e Fluxo M2M
* **Decisão:** Adoção do Keycloak 26 Quarkus em modo de desenvolvimento (`start-dev --import-realm`), utilizando o fluxo **Client Credentials Grant** (RFC 6749).
* **Justificativa:** A integração com provedores de jogos é uma comunicação máquina-a-máquina (M2M). Não há envolvimento de formulários de login de usuários finais nem telas interativas.

### 3.2. Modelo de Permissões e Identidades de Teste
O arquivo [keycloak/realm-export.json](file:///d:/Testes%20t%C3%A9cnicos/JungleGaming%20Go/keycloak/realm-export.json) pré-configura o realm `jungle` com as seguintes identidades:
* `provider-a` (Secret: `provider-a-secret`): Representa o Provedor A. O token emitido contém a role `provider` e a claim `providerId: provider-a`.
* `provider-b` (Secret: `provider-b-secret`): Representa o Provedor B para validação de **isolamento multi-tenant** (rejeição de acessos cruzados com `403 Forbidden`).
* `internal-service` (Secret: `internal-service-secret`): Serviço interno com a role `internal`, exclusivo para abertura de carteira e reconciliação.

---

## 4. Decisões de Reprodutibilidade e Tooling

* **Configuração Centralizada:** Todas as variáveis e credenciais de teste locais foram isoladas no [.env.example](file:///d:/Testes%20t%C3%A9cnicos/JungleGaming%20Go/.env.example) e [.env](file:///d:/Testes%20t%C3%A9cnicos/JungleGaming%20Go/.env), seguindo as diretrizes de Twelve-Factor App.
* **Automação Cross-Platform:**
  * [Makefile](file:///d:/Testes%20t%C3%A9cnicos/JungleGaming%20Go/Makefile): Padroniza os comandos exigidos pelo desafio (`make up`, `make verify`, `make test`, `make vet`, `make test-race`).
  * Scripts PowerShell (`.ps1`) e Bash (`.sh`): Garantem que testes de fumaça e geração de tokens funcionem perfeitamente tanto no Windows nativo quanto no Linux/macOS/CI.

---

---

## 5. Status do Desenvolvimento & Próximos Passos

### 5.1. Fase 1: Camada de Domínio Puro (CONCLUÍDA ✅)
A camada de domínio puro foi totalmente implementada em Go com zero dependências externas e 100% de cobertura nos requisitos da Seção 6 do teste técnico, validada por **118 testes unitários automatizados**:
* **Value Object `Money` ([internal/domain/money.go](file:///d:/Testes%20t%C3%A9cnicos/JungleGaming%20Go/internal/domain/money.go)):** Imutável, escala fixa de 2 casas em `int64` centavos, **zero floats** em parsing/operações/formatação, checagem matemática de overflow de `math.MaxInt64`/`math.MinInt64`.
* **Agregado `Wallet` ([internal/domain/wallet.go](file:///d:/Testes%20t%C3%A9cnicos/JungleGaming%20Go/internal/domain/wallet.go)):** Raiz de agregado com separação estrita de criação (`NewWallet`) e reidratação (`RehydrateWallet`), controle de versão incremental (`version++`), proteção contra saldo negativo no `Debit` e isolamento por moeda.
* **Entidade `WalletLedgerEntry` ([internal/domain/ledger.go](file:///d:/Testes%20t%C3%A9cnicos/JungleGaming%20Go/internal/domain/ledger.go)):** Lançamento contábil imutável (append-only) com verificação da equação invariante `balanceAfter = balanceBefore ± amount`.
* **Entidade `WagerTransaction` ([internal/domain/transaction.go](file:///d:/Testes%20t%C3%A9cnicos/JungleGaming%20Go/internal/domain/transaction.go)):** Máquina de estados com proteção contra mutações a partir de estados terminais (`PROCESSED`, `REJECTED`, `FAILED`), validações de `BET`, `WIN`, `LOSS` (saldo zero), `REFUND`/`ROLLBACK` (com referência externa obrigatória) e abertura interna `OPENING`.

### 5.2. Fase 2: Banco de Dados, Schema & Migrations (PLANEJADA 📋)
1. **Schema SQL com Constraints no PostgreSQL 16+:**
   * `wallets`: `CONSTRAINT chk_wallet_balance_non_negative CHECK (balance_cents >= 0)` e `UNIQUE(player_id, currency)`.
   * `wallet_ledger_entries`: `UNIQUE(wallet_id, transaction_id)` e trigger `BEFORE UPDATE OR DELETE` garantindo imutabilidade append-only.
   * `wager_transactions`: `UNIQUE(provider_id, idempotency_key)` e hash canônico SHA-256.
   * `inbox_messages` e `outbox_messages` para resiliência distribuída e Transactional Outbox.
2. **Repositórios com `pgx/v5`:**
   * Pool de conexões com `pgxpool.Pool`.
   * Locking pessimista por carteira: `SELECT ... FOR UPDATE` por ID de carteira (garante paralelismo entre carteiras distintas e serialização segura na mesma carteira).
   * Demarcação de transação SQL explícita (`pgx.Tx`) envolvendo saldo, ledger, transação e outbox no mesmo commit atômico.
3. **Migrations:** Versionamento com arquivos `.up.sql` e `.down.sql` e automação via `Makefile`.
