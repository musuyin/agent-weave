CREATE TABLE audit_logs (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    conversation_id VARCHAR(36),
    tool_name       VARCHAR(255) NOT NULL,
    param_keys      JSON,
    success         TINYINT(1)   NOT NULL DEFAULT 1,
    error_message   TEXT,
    created_at      DATETIME     NOT NULL,
    INDEX idx_audit_logs_conv (conversation_id),
    INDEX idx_audit_logs_tool (tool_name)
);
