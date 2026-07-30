CREATE TABLE conversation_agents (
    conversation_id VARCHAR(36) NOT NULL,
    agent_id        VARCHAR(36) NOT NULL,
    created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (conversation_id, agent_id),
    CONSTRAINT fk_conv_agents_conv  FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
    CONSTRAINT fk_conv_agents_agent FOREIGN KEY (agent_id)        REFERENCES agents(id)        ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
