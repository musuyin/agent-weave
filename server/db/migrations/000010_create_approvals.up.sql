CREATE TABLE approvals (
    block_id        VARCHAR(64)  PRIMARY KEY,
    conversation_id VARCHAR(36)  NOT NULL,
    tool_name       VARCHAR(255) NOT NULL,
    status          VARCHAR(20)  NOT NULL DEFAULT 'pending',
    created_at      DATETIME     NOT NULL,
    decided_at      DATETIME,
    INDEX idx_approvals_conv (conversation_id)
);
