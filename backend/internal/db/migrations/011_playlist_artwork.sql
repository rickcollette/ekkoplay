UPDATE playlists
SET artwork='playlist-gradient:' || CASE (id % 8)
  WHEN 0 THEN 'aurora' WHEN 1 THEN 'cobalt' WHEN 2 THEN 'sunset'
  WHEN 3 THEN 'orchid' WHEN 4 THEN 'forest' WHEN 5 THEN 'ember'
  WHEN 6 THEN 'lagoon' ELSE 'berry' END
WHERE artwork='';
