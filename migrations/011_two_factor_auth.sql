-- Two-factor authentication (TOTP) fields on users table.
-- totp_secret is AES-GCM encrypted (hex-encoded ciphertext); recovery_codes
-- are argon2id hashes so plaintext codes are never stored at rest.
ALTER TABLE users
    ADD COLUMN totp_secret    TEXT,
    ADD COLUMN totp_enabled   BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN recovery_codes TEXT[];
