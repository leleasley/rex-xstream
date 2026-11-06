CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS playlists (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    username_enc TEXT NOT NULL,
    password_enc TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'new',
    provider_json TEXT NOT NULL DEFAULT '{}',
    subscription_end INTEGER,
    max_connections INTEGER NOT NULL DEFAULT 0,
    last_test_at INTEGER,
    last_sync_at INTEGER,
    last_sync_error TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    name TEXT NOT NULL,
    UNIQUE(playlist_id, kind, provider_id)
);
CREATE INDEX IF NOT EXISTS idx_categories_lookup ON categories(playlist_id, kind);

CREATE TABLE IF NOT EXISTS channels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    name TEXT NOT NULL,
    number TEXT NOT NULL DEFAULT '',
    logo TEXT NOT NULL DEFAULT '',
    epg_id TEXT NOT NULL DEFAULT '',
    category_ids TEXT NOT NULL DEFAULT '[]',
    tv_archive INTEGER NOT NULL DEFAULT 0,
    stream_url TEXT NOT NULL DEFAULT '',
    probe_status TEXT NOT NULL DEFAULT 'pending',
    probe_error TEXT NOT NULL DEFAULT '',
    media_info TEXT NOT NULL DEFAULT '',
    rex_id TEXT NOT NULL DEFAULT '',
    pushed_at INTEGER,
    dirty INTEGER NOT NULL DEFAULT 1,
    active INTEGER NOT NULL DEFAULT 1,
    updated_at INTEGER NOT NULL,
    UNIQUE(playlist_id, provider_id)
);
CREATE INDEX IF NOT EXISTS idx_channels_lookup ON channels(playlist_id, active);
CREATE INDEX IF NOT EXISTS idx_channels_epg ON channels(playlist_id, epg_id);

CREATE TABLE IF NOT EXISTS movies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    title TEXT NOT NULL,
    year INTEGER NOT NULL DEFAULT 0,
    tmdb_id TEXT NOT NULL DEFAULT '',
    imdb_id TEXT NOT NULL DEFAULT '',
    poster TEXT NOT NULL DEFAULT '',
    backdrop TEXT NOT NULL DEFAULT '',
    plot TEXT NOT NULL DEFAULT '',
    genre TEXT NOT NULL DEFAULT '',
    rating TEXT NOT NULL DEFAULT '',
    duration TEXT NOT NULL DEFAULT '',
    container TEXT NOT NULL DEFAULT '',
    category_ids TEXT NOT NULL DEFAULT '[]',
    stream_url TEXT NOT NULL DEFAULT '',
    probe_status TEXT NOT NULL DEFAULT 'pending',
    probe_error TEXT NOT NULL DEFAULT '',
    media_info TEXT NOT NULL DEFAULT '',
    rex_id TEXT NOT NULL DEFAULT '',
    pushed_at INTEGER,
    dirty INTEGER NOT NULL DEFAULT 1,
    active INTEGER NOT NULL DEFAULT 1,
    updated_at INTEGER NOT NULL,
    UNIQUE(playlist_id, provider_id)
);
CREATE INDEX IF NOT EXISTS idx_movies_lookup ON movies(playlist_id, active);
CREATE INDEX IF NOT EXISTS idx_movies_tmdb ON movies(tmdb_id);

CREATE TABLE IF NOT EXISTS series (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    title TEXT NOT NULL,
    year INTEGER NOT NULL DEFAULT 0,
    tmdb_id TEXT NOT NULL DEFAULT '',
    poster TEXT NOT NULL DEFAULT '',
    backdrop TEXT NOT NULL DEFAULT '',
    plot TEXT NOT NULL DEFAULT '',
    genre TEXT NOT NULL DEFAULT '',
    rating TEXT NOT NULL DEFAULT '',
    category_ids TEXT NOT NULL DEFAULT '[]',
    provider_modified TEXT NOT NULL DEFAULT '',
    last_detail_fetch INTEGER,
    active INTEGER NOT NULL DEFAULT 1,
    updated_at INTEGER NOT NULL,
    UNIQUE(playlist_id, provider_id)
);
CREATE INDEX IF NOT EXISTS idx_series_lookup ON series(playlist_id, active);

CREATE TABLE IF NOT EXISTS seasons (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    number INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    poster TEXT NOT NULL DEFAULT '',
    UNIQUE(series_id, number)
);

CREATE TABLE IF NOT EXISTS episodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    season_number INTEGER NOT NULL,
    episode_number INTEGER NOT NULL,
    provider_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    plot TEXT NOT NULL DEFAULT '',
    image TEXT NOT NULL DEFAULT '',
    container TEXT NOT NULL DEFAULT '',
    tmdb_episode INTEGER NOT NULL DEFAULT 0,
    stream_url TEXT NOT NULL DEFAULT '',
    probe_status TEXT NOT NULL DEFAULT 'pending',
    probe_error TEXT NOT NULL DEFAULT '',
    media_info TEXT NOT NULL DEFAULT '',
    rex_id TEXT NOT NULL DEFAULT '',
    pushed_at INTEGER,
    dirty INTEGER NOT NULL DEFAULT 1,
    active INTEGER NOT NULL DEFAULT 1,
    updated_at INTEGER NOT NULL,
    UNIQUE(series_id, season_number, episode_number)
);
CREATE INDEX IF NOT EXISTS idx_episodes_lookup ON episodes(series_id, active);
CREATE INDEX IF NOT EXISTS idx_episodes_probe ON episodes(probe_status);

CREATE TABLE IF NOT EXISTS programmes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    channel_epg_id TEXT NOT NULL,
    start_utc INTEGER NOT NULL,
    stop_utc INTEGER NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    icon TEXT NOT NULL DEFAULT '',
    season INTEGER NOT NULL DEFAULT 0,
    episode INTEGER NOT NULL DEFAULT 0,
    UNIQUE(playlist_id, channel_epg_id, start_utc)
);
CREATE INDEX IF NOT EXISTS idx_programmes_lookup ON programmes(playlist_id, channel_epg_id, start_utc);

CREATE TABLE IF NOT EXISTS libraries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    playlist_id INTEGER REFERENCES playlists(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    directory TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(playlist_id, kind)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_libraries_live ON libraries(kind) WHERE kind = 'live';

CREATE TABLE IF NOT EXISTS sync_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    started_at INTEGER NOT NULL,
    finished_at INTEGER,
    status TEXT NOT NULL DEFAULT 'running',
    phase TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    counts_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_sync_runs_playlist ON sync_runs(playlist_id, started_at DESC);
