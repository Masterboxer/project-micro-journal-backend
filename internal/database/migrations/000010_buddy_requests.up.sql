CREATE TABLE buddy_requests (
    id SERIAL PRIMARY KEY,
    requester_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected')),
    created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW() NOT NULL,
    CONSTRAINT buddy_requests_check CHECK (requester_id <> recipient_id),
    CONSTRAINT buddy_requests_unique UNIQUE (requester_id, recipient_id)
);

CREATE INDEX idx_buddy_requests_recipient ON buddy_requests(recipient_id) WHERE status = 'pending';
CREATE INDEX idx_buddy_requests_requester ON buddy_requests(requester_id);
CREATE INDEX idx_buddy_requests_status ON buddy_requests(status);
