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

## 5. Próximos Passos (Planejamento do Core da Aplicação)

Com a infraestrutura de apoio 100% pronta e testada, os próximos passos do projeto englobam:

1. **Camada de Domínio Puro:**
   * Value Object `Money` (imutável, escala de 2 casas, `int64` centavos, proteção contra overflow, sem floats).
   * Agregado `Wallet` (invariantes de saldo, controle de concorrência por linha).
   * Entidade `WagerTransaction` e máquina de estados (`PENDING`, `PENDING_REFERENCE`, `PROCESSED`, `REJECTED`, `FAILED`).
   * `WalletLedgerEntry` (append-only ledger).
2. **Camada de Persistência e Migrations:**
   * Schema SQL com constraints (`CHECK (balance >= 0)`, unicidade de idempotência e inbox).
   * Repositórios com `pgx` e demarcação explícita de transações.
3. **Casos de Uso e Workers:**
   * Caso de uso unificado para HTTP e SQS com deduplicação e hash canônico.
   * Consumidor SQS com Inbox Pattern e transactional outbox worker.
4. **Composição com Uber Fx e Lifecycle:**
   * Injeção de dependência e graceful shutdown observável.
