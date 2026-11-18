-- Existing products retain the previously enabled behavior. New products are
-- explicitly created with FALSE by the backend; no existing product is reset.
ALTER TABLE products ADD COLUMN allow_customer_follow_ups BOOLEAN NOT NULL DEFAULT TRUE;
