CREATE TABLE IF NOT EXISTS org_forwarding_numbers (id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, org_id BIGINT NOT NULL, name VARCHAR(100) NOT NULL, phone VARCHAR(20) NOT NULL, UNIQUE KEY org_phone (org_id,phone));

CREATE TABLE IF NOT EXISTS org_incoming_routes (account_id BIGINT NOT NULL PRIMARY KEY, org_id BIGINT NOT NULL, mode VARCHAR(16) NOT NULL DEFAULT 'ai', destination_id BIGINT NOT NULL DEFAULT 0, KEY org_destination (org_id,destination_id));

CREATE TABLE IF NOT EXISTS incoming_route_snapshots (org_id BIGINT NOT NULL, call_sid VARCHAR(160) NOT NULL, mode VARCHAR(16) NOT NULL, phone VARCHAR(20) NOT NULL DEFAULT '', PRIMARY KEY(org_id,call_sid));
