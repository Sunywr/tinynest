-- Run against the TinyNest target database with utf8mb4 enabled.
-- Only the shared household of the two confirmed pfm users is eligible.
START TRANSACTION;

SELECT id, household_id FROM users
WHERE username IN ('sunywr', 'liuj')
ORDER BY id FOR UPDATE;

INSERT INTO categories (id, household_id, kind, name, emoji, color, sort)
SELECT UUID(), target.household_id, planned.kind, planned.name, planned.emoji, planned.color, planned.sort
FROM (
  SELECT MIN(household_id) AS household_id
  FROM users
  WHERE username IN ('sunywr', 'liuj')
  HAVING COUNT(*) = 2 AND COUNT(DISTINCT username) = 2 AND COUNT(DISTINCT household_id) = 1
) AS target
CROSS JOIN (
  SELECT 'expense' AS kind, '理财' AS name, 'investment' AS emoji, '#6B8E77' AS color, 1000 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '股票' AS name, 'investment' AS emoji, '#6386AB' AS color, 1001 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '谷子' AS name, 'toys' AS emoji, '#B880AA' AS color, 1002 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '保险' AS name, 'insurance' AS emoji, '#6386AB' AS color, 1003 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '医疗' AS name, 'hospital' AS emoji, '#C56872' AS color, 1004 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '电子产品' AS name, 'device' AS emoji, '#707FA7' AS color, 1005 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '通讯' AS name, 'phone' AS emoji, '#628DA3' AS color, 1006 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '家人支持' AS name, 'family' AS emoji, '#C77C88' AS color, 1007 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '结婚' AS name, '🎀' AS emoji, '#C77C88' AS color, 1008 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '礼金' AS name, '🎁' AS emoji, '#C47D5C' AS color, 1009 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '乐器' AS name, 'music' AS emoji, '#8E76AA' AS color, 1010 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '人情往来' AS name, 'family' AS emoji, '#BD8566' AS color, 1011 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '礼物' AS name, '🎁' AS emoji, '#C77C88' AS color, 1012 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '美容美发' AS name, '🎀' AS emoji, '#B880AA' AS color, 1013 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '公益' AS name, '🍀' AS emoji, '#6B8E77' AS color, 1014 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '其他' AS name, '⭐' AS emoji, '#827B8B' AS color, 1015 AS sort
  UNION ALL
  SELECT 'expense' AS kind, '税金' AS name, 'receipt' AS emoji, '#8C745E' AS color, 1016 AS sort
  UNION ALL
  SELECT 'income' AS kind, '出售' AS name, 'shop' AS emoji, '#708C72' AS color, 1017 AS sort
  UNION ALL
  SELECT 'income' AS kind, '礼金' AS name, '🎁' AS emoji, '#C47D5C' AS color, 1018 AS sort
  UNION ALL
  SELECT 'income' AS kind, '家人支持' AS name, 'family' AS emoji, '#C77C88' AS color, 1019 AS sort
  UNION ALL
  SELECT 'income' AS kind, '人情往来' AS name, 'family' AS emoji, '#BD8566' AS color, 1020 AS sort
  UNION ALL
  SELECT 'income' AS kind, '其他' AS name, '⭐' AS emoji, '#827B8B' AS color, 1021 AS sort
) AS planned
WHERE NOT EXISTS (
  SELECT 1 FROM categories AS existing
  WHERE existing.household_id = target.household_id
    AND existing.kind = planned.kind
    AND existing.name = planned.name
    AND existing.active = 1
);

SELECT ROW_COUNT() AS inserted_categories;
COMMIT;
