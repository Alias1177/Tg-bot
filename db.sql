CREATE TABLE IF NOT EXISTS users (
                                     id SERIAL PRIMARY KEY,
                                     email TEXT NOT NULL,
                                     country VARCHAR NOT NULL,
                                     chat_id BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS payments (
                                        id VARCHAR PRIMARY KEY,
                                        user_id BIGINT REFERENCES users(id),
                                        status VARCHAR NOT NULL,
                                        session_id VARCHAR NOT NULL,
                                        amount BIGINT NOT NULL,
                                        created_at BIGINT NOT NULL,
                                        completed_at BIGINT
)