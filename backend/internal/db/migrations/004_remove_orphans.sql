DELETE FROM albums WHERE NOT EXISTS (SELECT 1 FROM songs WHERE songs.album_id = albums.id);
DELETE FROM artists WHERE NOT EXISTS (SELECT 1 FROM songs WHERE songs.artist_id = artists.id);
