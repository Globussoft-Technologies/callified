-- Migration: add company column to leads table
-- Required by Go backend Lead.Company / leadCols / leadColsL queries.
ALTER TABLE leads ADD COLUMN company VARCHAR(255) NULL AFTER interest;
