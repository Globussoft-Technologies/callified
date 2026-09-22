-- Persist the AI-classified final outcome displayed in the call conclusion.
-- Empty values are retained for historical non-booked reviews; regenerating
-- their conclusion will classify them as pending or not_interested.

DELIMITER $$

CREATE PROCEDURE IF NOT EXISTS add_col_if_missing(
    IN p_table VARCHAR(128),
    IN p_column VARCHAR(128),
    IN p_definition VARCHAR(512)
)
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = p_table
          AND COLUMN_NAME = p_column
    ) THEN
        SET @ddl = CONCAT('ALTER TABLE ', p_table,
                          ' ADD COLUMN ', p_column, ' ', p_definition);
        PREPARE stmt FROM @ddl;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DELIMITER ;

CALL add_col_if_missing(
    'call_reviews',
    'call_outcome',
    'VARCHAR(32) NOT NULL DEFAULT '''' AFTER appointment_booked'
);

UPDATE call_reviews
SET call_outcome = 'appointment_booked'
WHERE appointment_booked = 1 AND call_outcome = '';
