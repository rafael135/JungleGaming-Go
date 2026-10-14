DROP TRIGGER IF EXISTS trg_prevent_ledger_modification ON wallet_ledger_entries;
DROP FUNCTION IF EXISTS prevent_ledger_modification();
DROP TABLE IF EXISTS wallet_ledger_entries;