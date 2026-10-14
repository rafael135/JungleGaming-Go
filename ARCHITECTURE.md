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
* **Decisão:** O provisionamento das filas SQS FIFO no LocalStack foi estruturado em um template declarativo AWS CloudFormation ([infra/sqs-template.yaml](./infra/sqs-template.yaml)).
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
O arquivo [keycloak/realm-export.json](./keycloak/realm-export.json) pré-configura o realm `jungle` com as seguintes identidades:
* `provider-a` (Secret: `provider-a-secret`): Representa o Provedor A. O token emitido contém a role `provider` e a claim `providerId: provider-a`.
* `provider-b` (Secret: `provider-b-secret`): Representa o Provedor B para validação de **isolamento multi-tenant** (rejeição de acessos cruzados com `403 Forbidden`).
* `internal-service` (Secret: `internal-service-secret`): Serviço interno com a role `internal`, exclusivo para abertura de carteira e reconciliação.

---

## 4. Decisões de Reprodutibilidade e Tooling

* **Configuração Centralizada:** Todas as variáveis e credenciais de teste locais foram isoladas no [.env.example](./.env.example) e [.env](./.env), seguindo as diretrizes de Twelve-Factor App.
* **Automação Cross-Platform:**
  * [Makefile](./Makefile): Padroniza os comandos exigidos pelo desafio (`make up`, `make verify`, `make test`, `make vet`, `make test-race`).
  * Scripts PowerShell (`.ps1`) e Bash (`.sh`): Garantem que testes de fumaça e geração de tokens funcionem perfeitamente tanto no Windows nativo quanto no Linux/macOS/CI.

---

---

## 5. Status do Desenvolvimento & Próximos Passos

### 5.1. Fase 1: Camada de Domínio Puro (CONCLUÍDA ✅)
A camada de domínio puro foi totalmente implementada em Go com zero dependências externas e 100% de cobertura nos requisitos da Seção 6 do teste técnico, validada por **118 testes unitários automatizados**:
* **Value Object `Money` ([internal/domain/money.go](./internal/domain/money.go)):** Imutável, escala fixa de 2 casas em `int64` centavos, **zero floats** em parsing/operações/formatação, checagem matemática de overflow de `math.MaxInt64`/`math.MinInt64`.
* **Agregado `Wallet` ([internal/domain/wallet.go](./internal/domain/wallet.go)):** Raiz de agregado com separação estrita de criação (`NewWallet`) e reidratação (`RehydrateWallet`), controle de versão incremental (`version++`), proteção contra saldo negativo no `Debit` e isolamento por moeda.
* **Entidade `WalletLedgerEntry` ([internal/domain/ledger.go](./internal/domain/ledger.go)):** Lançamento contábil imutável (append-only) com verificação da equação invariante `balanceAfter = balanceBefore ± amount`.
* **Entidade `WagerTransaction` ([internal/domain/transaction.go](./internal/domain/transaction.go)):** Máquina de estados com proteção contra mutações a partir de estados terminais (`PROCESSED`, `REJECTED`, `FAILED`), validações de `BET`, `WIN`, `LOSS` (saldo zero), `REFUND`/`ROLLBACK` (com referência externa obrigatória) e abertura interna `OPENING`.

### 5.2. Fase 2: Banco de Dados, Persistência & Migrations (CONCLUÍDA ✅)

A persistência do sistema foi construída para garantir que as invariantes financeiras e a imutabilidade contábil sejam impostas pelo PostgreSQL 16+, independentemente da memória da aplicação:

#### 1. Biblioteca Escolhida (`pgx/v5` e `golang-migrate`)
* **Driver do Banco:** `github.com/jackc/pgx/v5` com connection pool `pgxpool.Pool` ([internal/infrastructure/database/postgres.go](./internal/infrastructure/database/postgres.go)). O driver foi configurado com limites de conexões (`MaxConns = 25`, `MinConns = 5`), timeouts de ociosidade e verificação ativa de liveness via `Ping` no boot.
* **Mecanismo de Migrations:** Adotado o padrão da biblioteca oficial `github.com/golang-migrate/migrate/v4` com suporte ao driver `pgx5` e arquivos SQL embutidos no binário via `source/iofs` (`//go:embed *.sql`).
  * **Self-Migrating no Boot:** O binário aplica as migrations automaticamente na inicialização.
  * **Controle Manual:** CLI dedicado em [cmd/migrate/main.go](./cmd/migrate/main.go) com comandos `up`, `down` e `version` integrados ao Makefile.
  * **Tabela de Auditoria:** `schema_migrations` gerencia a versão ativa e a flag `dirty`.

#### 2. Mapeamento de `Money` no PostgreSQL
* **Princípio Zero Floats:** Nenhum cálculo, casting ou serialização transita por ponto flutuante (`float32`/`float64`).
* **Representação no Banco:**
  * Valores monetários são persistidos estritamente como **`BIGINT`** em centavos mínimos (`balance_cents`, `amount_cents`, `balance_before_cents`, `balance_after_cents`).
  * A moeda é persistida como **`VARCHAR(3)`** (código ISO 4217).
* **Mapeamento Bidirecional:**
  * Leitura: `domain.NewMoney(balanceCents, currency)` reconstitui o Value Object imutável.
  * Escrita: `money.Cents()` e `money.Currency()` extraem os dados sem conversão decimal.

#### 3. Delimitação da Transação SQL (Unit of Work / Transaction Manager)
* **Fronteira Transacional:** A atomicidade entre alterações de saldo, inserção no ledger, atualização da máquina de estados e enfileiramento na outbox é delimitada pelo padrão **`TransactionManager`** ([internal/application/ports.go](./internal/application/ports.go)), implementado por `PostgresTransactionManager` ([internal/infrastructure/database/transaction_manager.go](./internal/infrastructure/database/transaction_manager.go)).
* **Padrão Callback / Closure:**
  ```go
  err := tm.Execute(ctx, func(tx pgx.Tx) error {
      // 1. SELECT ... FOR UPDATE na carteira
      // 2. Operações de débito/crédito no domínio
      // 3. walletRepo.Update(ctx, tx, wallet)
      // 4. ledgerRepo.Create(ctx, tx, ledgerEntry)
      // 5. wagerRepo.Create/Update(ctx, tx, wagerTx)
      // 6. outboxRepo.Create(ctx, tx, event)
      // Retornar nil confirma (COMMIT); retornar erro desfaz tudo (ROLLBACK)
  })
  ```
* Se qualquer operação falhar ou a aplicação sofrer interrupção abrupta (`SIGKILL`), o PostgreSQL efetua `ROLLBACK` atômico completo, impedindo estados inconsistentes ou publicações prematuras.

#### 4. Constraints e Proteções no Banco
* **`wallets`:** `CONSTRAINT chk_wallet_balance_non_negative CHECK (balance_cents >= 0)` (garantia física de saldo não-negativo) e `UNIQUE(player_id, currency)`.
* **`wallet_ledger_entries` (Livro-Razão):**
  * `UNIQUE(wallet_id, transaction_id)`: Impede lançamentos contábeis duplicados para a mesma transação.
  * Trigger `prevent_ledger_modification()`: Função PL/pgSQL ativada `BEFORE UPDATE OR DELETE` que dispara exceção de segurança, garantindo que o ledger seja **estritamente append-only**.
* **`wager_transactions`:** 4 índices parciais únicos no PostgreSQL:
  1. `uq_wager_provider_idempotency`: `(provider_id, idempotency_key)` onde ambos são não-nulos.
  2. `uq_wager_opening`: `UNIQUE(wallet_id)` onde `transaction_type = 'OPENING'` (impede abertura duplicada).
  3. `uq_wager_reversal_type`: `(provider_id, reference_external_transaction_id, transaction_type)` com `status = 'PROCESSED'` (impede duas reversões bem-sucedidas do mesmo tipo sobre a mesma aposta).
  4. `idx_wager_pending_reference`: Índice parcial para busca rápida de transações aguardando referências externas.
* **`inbox_messages`:** `PRIMARY KEY (consumer_name, message_id)` para deduplicação atômica com `ON CONFLICT DO NOTHING`.
* **`outbox_messages`:** Transactional Outbox com polling concorrente via `SELECT ... FOR UPDATE SKIP LOCKED`, permitindo múltiplos publishers em paralelo sem deadlock.

---

### 5.3. Fase 3: Camada de Aplicação & Casos de Uso (EM ANDAMENTO 🚀)
1. **`CreateWalletUseCase` (CONCLUÍDO ✅):**
   * Criação de carteiras com validação monetária e transação atômica.
   * Emissão de transação `OPENING`, lançamento `CREDIT` no ledger e eventos `WagerTransactionProcessed` e `WalletBalanceChanged` na outbox quando saldo inicial for positivo.
   * Regra de saldo zero: não gera `OPENING`, ledger nem outbox.
   * Validado por 7 testes unitários cobrindo todos os casos de borda ([internal/application/usecase_wallet_test.go](./internal/application/usecase_wallet_test.go)).
2. **`ProcessWagerUseCase` (PRÓXIMO PASSO 📋):**
   * Orquestração unificada para HTTP (`POST /wagering/transactions`) e SQS FIFO (`WagerTransactionRequested`).
   * Deduplicação via Inbox, idempotência persistente via hash canônico SHA-256 e detecção de conflitos (`409 Conflict`).
   * Lock pessimista por carteira (`SELECT ... FOR UPDATE`).
   * Máquina de estados para `BET`, `WIN`, `LOSS`, `REFUND` e `ROLLBACK`.
   * Suporte a chegadas fora de ordem com status `PENDING_REFERENCE`.
3. **Casos de Uso de Leitura e Reconciliação:**
   * `GetWalletUseCase`, `GetWalletLedgerUseCase` (paginação opaca por cursor).
   * `ReconcileWalletUseCase` (`POST /wallets/:walletId/reconciliation`).

