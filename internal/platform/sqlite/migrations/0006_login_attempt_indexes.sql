CREATE INDEX login_attempts_username_time_idx
    ON login_attempts(username, attempted_at DESC);

CREATE INDEX login_attempts_time_idx
    ON login_attempts(attempted_at);
