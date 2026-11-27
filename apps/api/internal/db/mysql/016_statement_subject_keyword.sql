SET @statement_subject_keyword := (
  SELECT IF(COUNT(*) = 0,
    'ALTER TABLE statement_mailbox_card_links ADD COLUMN subject_keyword VARCHAR(80) NOT NULL DEFAULT ''''',
    'DO 0')
  FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'statement_mailbox_card_links'
    AND column_name = 'subject_keyword'
);
PREPARE statement_subject_keyword_stmt FROM @statement_subject_keyword;
EXECUTE statement_subject_keyword_stmt;
DEALLOCATE PREPARE statement_subject_keyword_stmt;
