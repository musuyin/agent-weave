CREATE TABLE agent_skills (
    agent_id VARCHAR(36) NOT NULL,
    skill_id VARCHAR(36) NOT NULL,
    PRIMARY KEY (agent_id, skill_id),
    CONSTRAINT fk_agent_skills_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_skills_skill FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
