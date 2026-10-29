-- Optional explicit identity for the product's voice agent.
ALTER TABLE products ADD COLUMN agent_name VARCHAR(100) NULL;
