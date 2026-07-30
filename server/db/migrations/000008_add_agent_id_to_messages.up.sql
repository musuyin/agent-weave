ALTER TABLE messages
    ADD COLUMN agent_id VARCHAR(36) NULL AFTER role,
    ADD CONSTRAINT fk_messages_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL;
