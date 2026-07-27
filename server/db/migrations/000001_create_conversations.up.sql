CREATE TABLE conversations (
    id         VARCHAR(36)  NOT NULL PRIMARY KEY,
    user_id    VARCHAR(255) NOT NULL,
    title      VARCHAR(500) NOT NULL DEFAULT '',
    created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    INDEX idx_conversations_user_created (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
