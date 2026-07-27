CREATE TABLE threads (
    id              VARCHAR(36)  NOT NULL PRIMARY KEY,
    conversation_id VARCHAR(36)  NOT NULL,
    agent_id        VARCHAR(255) NOT NULL COMMENT 'orchestrator | sub-agent name',
    status          VARCHAR(50)  NOT NULL DEFAULT 'pending'
                    COMMENT 'pending | running | done | cancelled | error',
    blocked_by      JSON         NOT NULL DEFAULT (JSON_ARRAY()),
    created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    INDEX idx_threads_conversation (conversation_id),
    CONSTRAINT fk_threads_conversation
        FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
