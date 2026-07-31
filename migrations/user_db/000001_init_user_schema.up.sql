CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(64) PRIMARY KEY,
    email VARCHAR(255) UNIQUE,
    given_name VARCHAR(64),
    family_name VARCHAR(64),
    username VARCHAR(32) UNIQUE,
    bio TEXT,
    birthdate TIMESTAMP WITH TIME ZONE,
    profile_picture_id VARCHAR(255),
    email_notifications BOOLEAN DEFAULT TRUE,
    push_notifications BOOLEAN DEFAULT TRUE,
    digest_frequency VARCHAR(20) DEFAULT 'DAILY',
    next_change_name_date TIMESTAMP WITH TIME ZONE,
    next_change_birthdate_date TIMESTAMP WITH TIME ZONE,
    next_change_username_date TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS friends (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    friend_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT idx_user_friend UNIQUE (user_id, friend_id)
);

CREATE TABLE IF NOT EXISTS friend_requests (
    id SERIAL PRIMARY KEY,
    sender_id VARCHAR(64) NOT NULL,
    receiver_id VARCHAR(64) NOT NULL,
    status VARCHAR(20) DEFAULT 'PENDING',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT idx_sender_receiver UNIQUE (sender_id, receiver_id)
);

CREATE TABLE IF NOT EXISTS user_blocks (
    id SERIAL PRIMARY KEY,
    blocker_id VARCHAR(64) NOT NULL,
    blocked_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT idx_blocker_blocked UNIQUE (blocker_id, blocked_id)
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_friends_user_id ON friends(user_id);
CREATE INDEX IF NOT EXISTS idx_friend_requests_receiver ON friend_requests(receiver_id);
