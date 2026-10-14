# Arquitetura e Decisões de Projeto — Jungle Gaming Backend

Este documento detalha todas as decisões arquiteturais, padrões de concorrência, integridade contábil, segurança, mensageria e ciclo de vida adotados no projeto Jungle Gaming, atendendo rigorosamente aos requisitos das **Instruções do Teste Técnico** (Seções 1 a 16).

---

## 1. Visão Geral da Arquitetura

O sistema adota os princípios da **Clean Architecture** (Arquitetura Limpa / Hexagonal), dividindo a base de código em camadas desacopladas com fluxo unidirecional de dependência:

```
┌────────────────────────────────────────────────────────────────────────┐
│                   Camada de Infraestrutura / Adapters                   │
│  ┌───────────────────────┐  ┌───────────────────┐  ┌────────────────┐  │
│  │   HTTP Handlers &     │  │   PostgreSQL 16+  │  │  AWS SQS FIFO  │  │
│  │   Keycloak OIDC Auth  │  │     (pgx/v5)      │  │  (LocalStack)  │  │
│  └───────────┬───────────┘  └─────────┬─────────┘  └────────┬───────┘  │
│              │                        │                     │          │
│              ▼                        ▼                     ▼          │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │              Camada de Aplicação (Casos de Uso & DTOs)           │  │
│  │   - CreateWalletUseCase      - ProcessWagerUseCase               │  │
│  │   - ReconcileWalletUseCase   - ResolvePendingReferencesUseCase   │  │
│  │   - GetWalletUseCase         - GetWalletLedgerUseCase            │  │
│  │   - Canonical Hasher (SHA-256)  - Ports & Interfaces             │  │
│  └──────────────────────────────────┬───────────────────────────────┘  │
│                                     │                                  │
│                                     ▼                                  │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                  Camada de Domínio Puro (Go Puro)                │  │
│  │   - Value Object: Money (int64 centavos, zero floats)            │  │
│  │   - Agregado: Wallet (versionamento, debit/credit)               │  │
│  │   - Entidade: WalletLedgerEntry (livro-razão imutável)           │  │
│  │   - Entidade: WagerTransaction (máquina de estados)              │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────┘
```

* **Domínio Puro (`internal/domain`):** Zero dependências externas (nem banco, nem HTTP, nem frameworks). Lógica de negócio expressa em Go puro e imutável.
* **Aplicação (`internal/application`):** Orquestra fluxos através de interfaces (`ports.go`), gerencia fronteiras transacionais com `TransactionManager` e computa hashes de idempotência canônicos.
* **Infraestrutura (`internal/infrastructure`):** Implementa os adaptadores técnicos: PostgreSQL 16+ com connection pool, AWS SDK v2 para filas SQS FIFO, validação OIDC Keycloak com RSA JWKS, HTTP Handlers nativos (Go 1.22+ mux) e background workers.
* **Injeção de Dependência & Composição (`cmd/api`):** Toda a montagem de dependências e ciclo de vida é governada pelo **Uber Fx (`go.uber.org/fx`)**.

---

## 2. Decisões Financeiras: Precisão e Zero Floats

### 2.1. Representação e Escala Fixa
* **Decisão:** Todos os valores monetários transitam no domínio como o Value Object `Money` (`internal/domain/money.go`), encapsulando `amountCents int64` e `currency string` (ISO 4217, 3 caracteres).
* **Zero Floats:** Não há qualquer uso de `float32` ou `float64` em operações financeiras, parsing de strings ou formatação. A escala é fixa em 2 casas decimais (1 unidade monetária = 100 centavos).
* **Prevenção de Overflow:** O construtor `ParseMoney` e as operações aritméticas (`Add`, `Subtract`) realizam checagens explícitas contra overflow de inteiros de 64 bits (`math.MaxInt64` e `math.MinInt64`), retornando `ErrAmountOverflow`.
* **Persistência no Banco:** Mapeado estritamente como colunas **`BIGINT`** (`balance_cents`, `amount_cents`, `balance_before_cents`, `balance_after_cents`), garantindo precisão matemática exata a nível de hardware e banco.

---

## 3. Concorrência e Estratégia de Locks

### 3.1. Lock Pessimista por Carteira (`SELECT ... FOR UPDATE`)
* **Decisão:** A coordenação de operações financeiras sobre a mesma carteira utiliza **Lock Pessimista** no PostgreSQL:
  ```sql
  SELECT id, player_id, currency, balance_cents, version, created_at, updated_at
  FROM wallets
  WHERE id = $1
  FOR UPDATE;
  ```
* **Justificativa:** Em sistemas de alta concorrência financeira, o controle pessimista na linha da carteira serializa com segurança os débitos e créditos, prevenindo race conditions, double-spending e saldos negativos sem depender de retries excessivos no cliente.
* **Isolamento de Carteiras Distintas:** Operações em carteiras diferentes nunca bloqueiam umas às outras, permitindo throughput escalável horizontalmente.

### 3.2. Double-Checked Locking para Idempotência Concorrente
* Quando múltiplas requisições com a mesma chave de idempotência disparam no mesmo milissegundo:
  1. A primeira transação adquire o lock `FOR UPDATE` da carteira, processa e commita.
  2. As demais transações aguardavam o lock. Ao adquirirem a posse da carteira, realizam um **re-check imediato de idempotência** antes de qualquer mutação.
  3. Identificando que a transação foi commitada pelo vencedor, devolvem o **replay idempotente seguro (`idempotentReplay: true`)** com o saldo observado original, sem gerar conflitos de constraint única nem erros 500.

---

## 4. Idempotência e Hash Canônico Determinístico

### 4.1. Algoritmo e Normalização de Hash
* **Algoritmo:** SHA-256 sobre JSON canônico com chaves ordenadas alfabeticamente ([internal/application/hasher.go](./internal/application/hasher.go)).
* **Campos Incluídos no Hash de Negócio:**
  * `amount`: formatado estritamente em string decimal com 2 casas (`"25.00"`).
  * `currency`: normalizado em maiúsculo (`"BRL"`).
  * `externalTransactionId`: identificador do provedor.
  * `gameId` e `roundId`: identificadores do jogo e rodada.
  * `kind`: tipo de operação (`"BET"`, `"WIN"`, `"REFUND"`, etc.).
  * `playerId` e `walletId`: identidades do jogador e da carteira.
  * `providerId`: identificador do provedor.
  * `referenceExternalTransactionId`: referência externa (quando aplicável).
* **Campos Excluídos:** Metadados de transporte (`Idempotency-Key`, `ConsumerName`, `MessageID`, timestamps de chegada, cabeçalhos HTTP).
* **Garantia de Equivalência HTTP e SQS:** Tanto uma mensagem vinda por HTTP quanto vinda por SQS geram o mesmo hash determinístico exato.

### 4.2. Políticas de Replay e Conflito
* **Mesma Chave + Mesmo Payload:** Devolve o resultado original persistido (`idempotentReplay: true`), retornando o **saldo observado original** na época do processamento, mesmo que a carteira já tenha sofrido movimentações posteriores.
* **Mesma Chave + Payload Diferente:** Retorna `409 Conflict` (`ErrIdempotencyConflict`).
* **Mesmo `(providerId, externalTransactionId)` com Chave Diferente:** Retorna `409 Conflict` (`ErrExternalTransactionConflict`).

---

## 5. Livro-Razão (Ledger) e Reconciliação Contábil

### 5.1. Imutabilidade do Ledger
* A tabela `wallet_ledger_entries` é estritamente **append-only**.
* Protegida no PostgreSQL por trigger PL/pgSQL `prevent_ledger_modification()` que bloqueia qualquer comando `UPDATE` ou `DELETE`.
* Todo lançamento valida em código e em banco a equação invariante:
  $$\text{balanceAfter} = \text{balanceBefore} \pm \text{amount}$$

### 5.2. Reconciliação Matemática (`POST /wallets/{walletId}/reconciliation`)
* O caso de uso `ReconcileWalletUseCase` reconstrói o saldo da carteira somando os créditos e subtraindo os débitos de todos os lançamentos contábeis (incluindo a abertura `OPENING`).
* Compara o saldo recalculado contra o saldo armazenado na tabela `wallets`:
  $$\text{difference} = \text{storedBalance} - \text{calculatedBalance}$$
* Retorna `consistent: true` e `difference: "0.00"` caso a integridade contábil esteja 100% perfeita. Divergências são emitidas em logs e métricas sem alterar o saldo.

---

## 6. Mensageria e Confiabilidade (Inbox e Transactional Outbox)

### 6.1. Consumidor SQS FIFO com Inbox Pattern
* **Fila de Entrada:** `wager-transactions.fifo` com redrive policy para `wager-transactions-dlq.fifo` (`maxReceiveCount: 3`).
* **Deduplicação Atômica via Inbox:** Na entrada SQS, `TryRecord` insere `(consumer_name, message_id)` dentro da **mesma transação SQL** que processa o débito e grava o ledger. Se a mensagem for reentregue pelo SQS, a chave primária da inbox descarta o processamento duplicado com segurança.
* **Remoção Segura da Fila:** O `DeleteMessage` no SQS só é executado **após o commit bem-sucedido** da transação no PostgreSQL.

### 6.2. Publicação com Transactional Outbox
* **Fila de Saída:** `wager-events.fifo` com fila morta `wager-events-dlq.fifo`.
* **Atomicidade:** Os eventos (`WagerTransactionProcessed`, `WalletBalanceChanged`, etc.) são inseridos na tabela `outbox_messages` dentro da mesma transação do negócio.
* **Worker de Publicação Concorrente:**
  * Utiliza `SELECT ... FOR UPDATE SKIP LOCKED` em lotes de 20 mensagens.
  * Permite múltiplos publishers concorrentes rodando em paralelo sem deadlock e sem disputa pelas mesmas linhas.
  * Implementa retry com backoff exponencial (`next_attempt_at = now + 2^retries * 500ms`). Mensagens com mais de 5 tentativas esgotadas são marcadas como `FAILED` para investigação sem travar a outbox.
* **Eventos Emitidos:**
  * `WagerTransactionProcessed`
  * `WagerTransactionRejected`
  * `WalletBalanceChanged`
  * `WagerTransactionPendingReference`

### 6.3. Resolução de Referências Pendentes
* Quando um `REFUND` ou `ROLLBACK` chega antes da `BET` original correspondente, a operação transiciona para `PENDING_REFERENCE`.
* Um background worker periódico (`PendingReferenceWorker`) busca referências pendentes e tenta resolvê-las. Operações com pendência superior a 24 horas (`TTL`) são transicionadas para `REJECTED` por expiração.

---

## 7. Autenticação, Autorização e Isolamento Multi-Tenant

### 7.1. Keycloak 26 OIDC (Client Credentials)
* Autenticação via tokens JWT assinados com RSA (algoritmo `RS256`).
* O validador OIDC ([internal/infrastructure/auth/oidc_validator.go](./internal/infrastructure/auth/oidc_validator.go)) realiza cache em memória das chaves públicas obtidas do endpoint JWKS (`/protocol/openid-connect/certs`), evitando roundtrips de rede a cada requisição.

### 7.2. Matriz de Autorização e Isolamento de Tenants
| Identidade | Roles | Permissões |
| :--- | :--- | :--- |
| `internal-service` | `internal` | Criar carteiras (`POST /wallets`), consultar ledgers, executar reconciliação. |
| `provider-a` | `provider` | Enviar apostas (`POST /wagering/transactions`), consultar suas próprias transações. |
| `provider-b` | `provider` | Enviar apostas, restrito exclusivamente ao tenant `provider-b`. |

* **Isolamento de Provedor (Multi-Tenant Guard):**
  * Se um token emitido para `provider-a` tentar enviar ou consultar uma transação com `providerId: provider-b`, o middleware bloqueia a chamada retornando **`403 Forbidden`**.
  * Se um provedor tentar acessar rotas internas de abertura de carteira ou reconciliação, recebe **`403 Forbidden`**.

---

## 8. Composição e Ciclo de Vida com Uber Fx

### 8.1. Grafo de Dependências (`cmd/api/app.go`)
O Uber Fx gerencia o grafo de injeção de dependências desacoplando completamente a inicialização dos módulos:
* `fx.Provide`: Registra Config, Logger estruturado, Connection Pool do Postgres, Repositórios, SQS Publisher/Consumer, Use Cases, Token Validator e HTTP Router.
* `fx.Invoke`: Registra os ganchos de ciclo de vida (`fx.Lifecycle`) para o servidor HTTP e para os workers em background.

### 8.2. Graceful Shutdown (Encerramento Seguro)
Ao receber `SIGINT` ou `SIGTERM`, os ganchos `OnStop` são disparados na ordem inversa de criação:
1. **HTTP Server:** Cancela novas conexões e drena requisições em voo com timeout seguro (`server.Shutdown(ctx)`).
2. **SQS Consumer:** Cancela o contexto de long polling, interrompendo requisições ativas instantaneamente (`consumer.Stop()`).
3. **Outbox Publisher & Pending Worker:** Aguardam a conclusão do lote atual e finalizam seus tickers.
4. **PostgreSQL Pool:** Drena e encerra todas as conexões do pool (`pool.Close()`).
* Todo o ciclo de parada foi verificado via teste automatizado (`cmd/api/app_test.go`), concluindo em menos de 1.5s sem processos zumbis ou conexões órfãs.

---

## 9. Comprovação dos Testes Obrigatórios de Concorrência

Os testes automatizados em [cmd/api/concurrency_test.go](./cmd/api/concurrency_test.go) comprovam os cenários exigidos na Seção 13 do desafio:

1. **Mesma aposta 50 vezes em paralelo:**
   * 50 goroutines simultâneas disparadas por barreira de sincronização.
   * Resultado: **Exatamente 1 débito efetuado**, 49 replays com o saldo original observado, e reconciliação confirmando consistência contábil de 2 lançamentos no ledger.
2. **Disputa de duas apostas de 80.00 BRL sobre saldo de 100.00 BRL:**
   * Disparo concorrente de duas apostas distintas.
   * Resultado: **Exatamente 1 aposta processada (`PROCESSED`)** e **1 aposta rejeitada (`REJECTED` com `INSUFFICIENT_FUNDS`)**. Saldo final permaneceu estritamente em **20.00 BRL** sem permitir saldo negativo.
3. **Processamento paralelo de 10 carteiras distintas:**
   * Executadas simultaneamente sem concorrência espúria nem deadlocks.

---

## 10. Guia de Execução Rápida

### Subir o ambiente completo via Docker Compose:
```sh
docker compose up --build
```

### Rodar todos os testes automatizados (unitários, integração e concorrência):
```sh
make test
```

### Rodar testes com detector de race conditions do Go:
```sh
make test-race
```

### Rodar análise estática e formatação:
```sh
make vet
make fmt
```
