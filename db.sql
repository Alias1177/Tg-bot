CREATE TABLE IF NOT EXISTS users (
                                     id SERIAL PRIMARY KEY,
                                     email TEXT NOT NULL,
                                     country VARCHAR NOT NULL,
                                     chat_id BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS payments (
                                        id VARCHAR PRIMARY KEY DEFAULT gen_random_uuid(),
                                        user_id BIGINT REFERENCES users(id),
    status VARCHAR NOT NULL CHECK (status IN ('pending', 'completed', 'failed')), 
    session_id VARCHAR NOT NULL UNIQUE,
    amount BIGINT NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL DEFAULT extract(epoch from now()),
    completed_at BIGINT
    );