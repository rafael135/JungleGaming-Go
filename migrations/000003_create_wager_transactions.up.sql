CREATE TABLE wager_transactions (
    id UUID PRIMARY KEY,
    external_transaction_id VARCHAR(255),
    provider_id VARCHAR(255),
    idempotency_key VARCHAR(255),
    payload_hash VARCHAR(64),
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    player_id UUID NOT NULL,
    game_id VARCHAR(100),
    round_id VARCHAR(100),
    transaction_type VARCHAR(20) NOT NULL,
    amount_cents BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL,
    reference_external_transaction_id VARCHAR(255),
    status VARCHAR(25) NOT NULL DEFAULT 'PENDING',
    failure_code VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wager_transaction_type CHECK (
        transaction_type IN ('OPENING', 'BET', 'WIN', 'LOSS', 'REFUND', 'ROLLBACK')
    ),
    CONSTRAINT chk_wager_status CHECK (
        status IN ('PENDING', 'PENDING_REFERENCE', 'PROCESSED', 'REJECTED', 'FAILED')
    )
);

CREATE UNIQUE INDEX uq_wager_provider_idempotency
    ON wager_transactions (provider_id, idempotency_key)
    WHERE provider_id IS NOT NULL AND idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX uq_wager_opening
    ON wager_transactions (wallet_id)
    WHERE transaction_type = 'OPENING';

CREATE UNIQUE INDEX uq_wager_reversal_type
    ON wager_transactions (provider_id, reference_external_transaction_id, transaction_type)
    WHERE status = 'PROCESSED' AND reference_external_transaction_id IS NOT NULL;

CREATE INDEX idx_wager_pending_reference
    ON wager_transactions (status, created_at)
    WHERE status = 'PENDING_REFERENCE';