-- Keyspace definition for Post View History
CREATE KEYSPACE IF NOT EXISTS post_history
WITH replication = {
    'class': 'SimpleStrategy',
    'replication_factor': 1
};

-- Table storing raw qualified view events partitioned by (user_id, month) bucket
-- Clustered by observed_at DESC to fetch latest views efficiently
CREATE TABLE IF NOT EXISTS post_history.views_by_user_month (
    user_id text,
    month text, -- Format: YYYY-MM
    observed_at timestamp,
    event_id text,
    post_id text,
    visible_duration_ms int,
    visible_ratio double,
    surface text,
    schema_version int,
    PRIMARY KEY ((user_id, month), observed_at, event_id)
) WITH CLUSTERING ORDER BY (observed_at DESC, event_id ASC)
AND default_time_to_live = 7776000 -- 90 days in seconds (90 * 86400)
AND comment = 'Raw qualified view events partitioned by user and month bucket with 90-day retention';
