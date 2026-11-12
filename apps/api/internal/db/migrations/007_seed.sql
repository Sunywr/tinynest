-- 007: default categories, idempotent per household.
-- A stable id means re-running migrations never duplicates or overwrites user edits.
INSERT OR IGNORE INTO categories (id, household_id, kind, name, level, sort)
SELECT 'cat_' || h.id || '_' || t.code, h.id, 'expense', t.name, 1, t.sort
FROM households h CROSS JOIN (
  SELECT 'food'      AS code, '饮食' AS name,  1 AS sort UNION ALL
  SELECT 'snack',     '零食',  2 UNION ALL
  SELECT 'sport',     '运动',  3 UNION ALL
  SELECT 'clothing',  '服饰',  4 UNION ALL
  SELECT 'transport', '交通',  5 UNION ALL
  SELECT 'living',    '生活',  6 UNION ALL
  SELECT 'furniture', '家具',  7 UNION ALL
  SELECT 'toys',      '玩具',  8 UNION ALL
  SELECT 'games',     '游戏',  9 UNION ALL
  SELECT 'fun',       '娱乐', 10 UNION ALL
  SELECT 'education', '教育', 11 UNION ALL
  SELECT 'parenting', '育儿', 12 UNION ALL
  SELECT 'work',      '工作', 13 UNION ALL
  SELECT 'travel',    '旅行', 14 UNION ALL
  SELECT 'housing',   '住房', 15 UNION ALL
  SELECT 'car',       '汽车', 16
) t;

INSERT OR IGNORE INTO categories (id, household_id, kind, name, level, sort)
SELECT 'cat_' || h.id || '_' || t.code, h.id, 'income', t.name, 1, t.sort
FROM households h CROSS JOIN (
  SELECT 'salary'     AS code, '工资' AS name, 1 AS sort UNION ALL
  SELECT 'reimburse', '报销', 2 UNION ALL
  SELECT 'redpacket', '红包', 3 UNION ALL
  SELECT 'interest',  '利息', 4
) t;

INSERT OR IGNORE INTO categories (id, household_id, kind, name, level, sort)
SELECT 'cat_' || h.id || '_dividend', h.id, 'income', '投资分红', 1, 5
FROM households h;
