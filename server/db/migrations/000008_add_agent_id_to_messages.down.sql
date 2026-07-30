ALTER TABLE messages
    DROP FOREIGN KEY fk_messages_agent,
    DROP COLUMN agent_id;
