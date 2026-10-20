-- Pocket Documents replaced Paperless-ngx as the documents source: the
-- source is now called "documents". The days frozen while Day read
-- Paperless keep their facts under the new name, so they show as before (the
-- same payload, rendered by the same card) and are not pulled again; the
-- documents kept on a day (pins) follow. Nothing is deleted: a day that
-- somehow has both keeps its Paperless rows as they were.
UPDATE facts SET source = 'documents'
  WHERE source = 'paperless'
    AND NOT EXISTS (SELECT 1 FROM facts d WHERE d.user_id = facts.user_id AND d.date = facts.date AND d.source = 'documents');
UPDATE OR IGNORE pins SET source = 'documents',
    ref = CASE WHEN ref LIKE 'paperless:%' THEN 'documents:' || substr(ref, 11) ELSE ref END
  WHERE source = 'paperless';
