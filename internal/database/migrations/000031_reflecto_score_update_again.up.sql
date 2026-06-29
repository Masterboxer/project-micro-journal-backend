ALTER TABLE reflecto_score_events
  ADD CONSTRAINT uq_score_events_user_post_action
  UNIQUE (user_id, post_id, action_type);